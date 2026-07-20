package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/qoder"
	"github.com/TokenFlux/TokenRouter/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestQoderTokenProviderBuildsAndCachesDirectSession(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:       101,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
			"uid":                  "uid-1",
			"organization_id":      "org-1",
			"organization_name":    "Org 1",
		},
	}

	session1, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, session1)
	require.Equal(t, "dt-token", session1.Identity.SecurityOauthToken)
	require.Equal(t, "uid-1", session1.Identity.UID)
	require.Equal(t, "uid-1", session1.Identity.AID)
	require.Equal(t, "org-1", session1.Identity.OrganizationID)
	require.Equal(t, "Org 1", session1.Identity.OrganizationName)
	require.Equal(t, "machine-1", session1.Machine.MachineID)

	session2, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Same(t, session1, session2, "session should be cached per account credentials")

	account.Credentials["organization_id"] = "org-2"
	session3, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.NotSame(t, session1, session3)
	require.Equal(t, "org-2", session3.Identity.OrganizationID)
}

func TestQoderTokenProviderRejectsUnsupportedCredentialShape(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:          102,
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"security_oauth_token": "dt-token"},
	}

	_, err := provider.GetSession(context.Background(), account)
	require.ErrorContains(t, err, "machine_id")
}

func TestQoderTokenProviderRejectsDirectTokenWithoutIdentity(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:       105,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
		},
	}

	_, err := provider.GetSession(context.Background(), account)
	require.ErrorContains(t, err, "uid or aid")
}

func TestQoderTokenProviderRejectsCN20RefreshModeOnGlobalSite(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:       32,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"site":                 "global",
			"refresh_mode":         qoder.RefreshModeQoderCN20,
			"security_oauth_token": "token",
			"machine_id":           "machine",
			"uid":                  "uid",
		},
	}

	_, err := provider.GetSession(context.Background(), account)
	require.ErrorContains(t, err, "require cn site")
}

func TestQoderTokenProviderRejectsMachineIDOnlyWithoutReadingLocalAuth(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:       106,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"machine_id": "machine-1",
		},
	}

	_, err := provider.GetSession(context.Background(), account)
	require.ErrorContains(t, err, "pat or security_oauth_token")
}

func TestQoderTokenProviderRejectsExplicitAuthDirWithoutReadingLocalAuth(t *testing.T) {
	provider := NewQoderTokenProvider()
	account := &Account{
		ID:       106,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"machine_id": "machine-1",
			"auth_dir":   "/tmp/qoder-auth",
		},
	}

	_, err := provider.GetSession(context.Background(), account)
	require.ErrorContains(t, err, "pat or security_oauth_token")
}

func TestQoderTokenProviderSupportsInjectedPATExchange(t *testing.T) {
	calls := 0
	orgCalls := 0
	var exchangedPATs []string
	provider := NewQoderTokenProvider()
	provider.exchangePAT = func(_ context.Context, pat string, machine *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		calls++
		exchangedPATs = append(exchangedPATs, pat)
		require.NotEmpty(t, machine.MachineID)
		return &qoder.AuthIdentity{
			Name:               "PAT User",
			UID:                "uid-1",
			AID:                "uid-1",
			UserType:           "personal_standard",
			SecurityOauthToken: "dt-from-pat",
			RefreshToken:       "rt-from-pat",
		}, nil
	}
	provider.getOrgTags = func(context.Context, string, string) (*qoder.OrganizationTags, error) {
		orgCalls++
		return nil, nil
	}

	account := &Account{
		ID:       103,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"pat":               "pat-123",
			"organization_id":   "org-from-account",
			"organization_name": "Org From Account",
		},
	}

	session1, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "dt-from-pat", session1.Identity.SecurityOauthToken)
	require.Equal(t, "org-from-account", session1.Identity.OrganizationID)
	require.Equal(t, "Org From Account", session1.Identity.OrganizationName)

	session2, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Same(t, session1, session2)
	require.Equal(t, 1, calls, "PAT exchange should not run after cache hit")
	require.Equal(t, 0, orgCalls, "account-provided organization metadata should skip org lookup")

	account.Credentials["pat"] = "pat-456"
	session3, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.NotSame(t, session1, session3)
	require.Equal(t, []string{"pat-123", "pat-456"}, exchangedPATs)
}

func TestQoderRefreshCredentialsHashTracksAuthenticationContext(t *testing.T) {
	base := map[string]any{
		"pat":             "pat-123",
		"site":            "global",
		"refresh_mode":    "cosy",
		"organization_id": "org-1",
		"model_mapping":   map[string]any{"auto": "auto"},
	}
	baseHash := qoderRefreshCredentialsHash(base)
	for _, change := range []map[string]any{
		{"pat": "pat-456"},
		{"site": "cn"},
		{"refresh_mode": qoder.RefreshModeQoderCN20},
		{"organization_id": "org-2"},
	} {
		credentials := MergeCredentials(base, change)
		require.NotEqual(t, baseHash, qoderRefreshCredentialsHash(credentials))
	}
	unrelated := MergeCredentials(base, map[string]any{
		"model_mapping": map[string]any{"custom": "auto"},
	})
	require.Equal(t, baseHash, qoderRefreshCredentialsHash(unrelated))
}

func TestQoderTokenProviderPATExchangePopulatesOrganizationFromAPI(t *testing.T) {
	provider := NewQoderTokenProvider()
	provider.exchangePAT = func(_ context.Context, _ string, _ *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		return &qoder.AuthIdentity{
			Name:               "PAT User",
			UID:                "uid-1",
			AID:                "aid-1",
			UserType:           "personal_standard",
			SecurityOauthToken: "dt-from-pat",
		}, nil
	}
	provider.getOrgTags = func(_ context.Context, token, uid string) (*qoder.OrganizationTags, error) {
		require.Equal(t, "dt-from-pat", token)
		require.Equal(t, "uid-1", uid)
		return &qoder.OrganizationTags{
			OrganizationID:   "org-from-api",
			OrganizationName: "Org From API",
		}, nil
	}

	account := &Account{
		ID:       104,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"pat": "pat-123",
		},
	}

	session, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "org-from-api", session.Identity.OrganizationID)
	require.Equal(t, "Org From API", session.Identity.OrganizationName)
}

func TestQoderTokenProviderRoutesCNPATAndBuildsCNSession(t *testing.T) {
	provider := NewQoderTokenProvider()
	provider.exchangeCNPAT = func(_ context.Context, pat string, machine *qoder.MachineIdentity) (*qoder.AuthIdentity, time.Time, error) {
		require.Equal(t, "cn-pat", pat)
		require.Equal(t, "machine-cn", machine.MachineID)
		return &qoder.AuthIdentity{
			UID:                "uid-cn",
			AID:                "aid-cn",
			OrganizationID:     "org-cn",
			SecurityOauthToken: "cosy-cn",
			RefreshToken:       "refresh-cn",
		}, time.Now().Add(time.Hour), nil
	}
	account := &Account{
		ID:       31,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"site":          "cn",
			"pat":           "cn-pat",
			"machine_id":    "machine-cn",
			"machine_token": "machine-token-cn",
			"machine_type":  "machine-type-cn",
		},
	}

	session, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, qoder.SiteCN, session.Site)
	require.Equal(t, qoder.CNClientVersion, session.ClientVersion)
	require.Equal(t, "cosy-cn", session.Identity.SecurityOauthToken)
	require.Equal(t, "machine-token-cn", session.Machine.MachineToken)
}

func TestQoderTokenProviderRebuildsExpiredCNPATSession(t *testing.T) {
	provider := NewQoderTokenProvider()
	expiresAt := time.Now().Add(time.Hour)
	exchangeCalls := 0
	provider.exchangeCNPAT = func(_ context.Context, _ string, _ *qoder.MachineIdentity) (*qoder.AuthIdentity, time.Time, error) {
		exchangeCalls++
		return &qoder.AuthIdentity{
			UID:                "uid-cn",
			AID:                "uid-cn",
			SecurityOauthToken: fmt.Sprintf("cosy-cn-%d", exchangeCalls),
			RefreshToken:       "refresh-cn",
		}, expiresAt, nil
	}
	account := &Account{
		ID:       32,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"site":       "cn",
			"pat":        "cn-pat",
			"machine_id": "machine-cn",
		},
	}

	first, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, expiresAt, provider.sessions[account.ID].expiresAt)

	// 模拟已缓存的 OpenAPI token 到期，下一次读取必须重新执行 PAT exchange。
	entry := provider.sessions[account.ID]
	entry.expiresAt = time.Now().Add(-time.Second)
	provider.sessions[account.ID] = entry
	second, err := provider.GetSession(context.Background(), account)

	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Equal(t, "cosy-cn-2", second.Identity.SecurityOauthToken)
	require.Equal(t, 2, exchangeCalls)
}

func TestQoderTokenProviderDirectTokenPopulatesOrganizationFromAPI(t *testing.T) {
	provider := NewQoderTokenProvider()
	provider.getOrgTags = func(_ context.Context, token, uid string) (*qoder.OrganizationTags, error) {
		require.Equal(t, "dt-token", token)
		require.Equal(t, "uid-1", uid)
		return &qoder.OrganizationTags{
			OrganizationID:   "org-from-api",
			OrganizationName: "Org From API",
		}, nil
	}

	account := &Account{
		ID:       110,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
			"uid":                  "uid-1",
		},
	}

	session, err := provider.GetSession(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "org-from-api", session.Identity.OrganizationID)
	require.Equal(t, "Org From API", session.Identity.OrganizationName)
}

func TestQoderTokenProviderPATExchangeUsesAccountDoer(t *testing.T) {
	upstream := &qoderCenterHTTPUpstreamStub{}
	provider := NewQoderTokenProvider()
	provider.SetHTTPUpstream(upstream, nil)
	account := &Account{
		ID:          107,
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Concurrency: 3,
		ProxyID:     ptrInt64ForQoderTest(9),
		Proxy:       &Proxy{Protocol: "http", Host: "proxy.example.com", Port: 8080},
		Credentials: map[string]any{"pat": "pat-123"},
	}

	session, err := provider.GetSession(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "dt-from-center", session.Identity.SecurityOauthToken)
	require.Equal(t, "http://proxy.example.com:8080", upstream.proxyURL)
	require.Equal(t, int64(107), upstream.accountID)
	require.Equal(t, 3, upstream.accountConcurrency)
}

func TestQoderTokenProviderPATOrganizationTagsUsesAccountDoer(t *testing.T) {
	upstream := &qoderCenterHTTPUpstreamStub{
		body: `{"organization_id":"org-via-upstream","organization_name":"Org Via Upstream"}`,
	}
	provider := NewQoderTokenProvider()
	provider.SetHTTPUpstream(upstream, &TLSFingerprintProfileService{})
	provider.exchangePAT = func(_ context.Context, _ string, _ *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		return &qoder.AuthIdentity{
			Name:               "PAT User",
			UID:                "uid-1",
			AID:                "uid-1",
			UserType:           "personal_standard",
			SecurityOauthToken: "dt-from-pat",
		}, nil
	}
	account := &Account{
		ID:          108,
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Concurrency: 4,
		ProxyID:     ptrInt64ForQoderTest(10),
		Proxy:       &Proxy{Protocol: "http", Host: "proxy.example.com", Port: 8081},
		Credentials: map[string]any{"pat": "pat-123"},
		Extra:       map[string]any{"enable_tls_fingerprint": true},
	}

	session, err := provider.GetSession(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "org-via-upstream", session.Identity.OrganizationID)
	require.Equal(t, "Org Via Upstream", session.Identity.OrganizationName)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, upstream.requests[0].URL.Path, qoder.OrganizationTagsPathPrefix+"uid-1/tags")
	require.Equal(t, "http://proxy.example.com:8081", upstream.proxyURL)
	require.Equal(t, int64(108), upstream.accountID)
	require.Equal(t, 4, upstream.accountConcurrency)
	require.True(t, upstream.profileSet)
}

func TestQoderTokenProviderOrganizationTagsErrorRedactsSensitiveBody(t *testing.T) {
	upstream := &qoderCenterHTTPUpstreamStub{
		statusCode: http.StatusInternalServerError,
		body:       `{"message":"failed","securityOauthToken":"sec-secret","refresh_token":"rt-secret","uid":"uid-secret","cookie":"sid=secret"}`,
	}
	provider := NewQoderTokenProvider()
	provider.SetHTTPUpstream(upstream, nil)
	account := &Account{
		ID:       109,
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "sec-token",
			"machine_id":           "machine-1",
			"uid":                  "uid-1",
		},
	}

	_, err := provider.getOrganizationTagsForAccount(context.Background(), account, "sec-token", "uid-1")

	require.Error(t, err)
	errText := err.Error()
	require.Contains(t, errText, "status 500")
	require.NotContains(t, errText, "sec-secret")
	require.NotContains(t, errText, "rt-secret")
	require.NotContains(t, errText, "uid-secret")
	require.NotContains(t, errText, "sid=secret")
	require.Contains(t, errText, "***")
}

type qoderCenterHTTPUpstreamStub struct {
	proxyURL           string
	accountID          int64
	accountConcurrency int
	statusCode         int
	body               string
	profileSet         bool
	requests           []*http.Request
}

func (s *qoderCenterHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *qoderCenterHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	s.proxyURL = proxyURL
	s.accountID = accountID
	s.accountConcurrency = accountConcurrency
	s.profileSet = profile != nil
	s.requests = append(s.requests, req)
	body := s.body
	if body == "" {
		body = `{
			"id":"user-1",
			"name":"User",
			"userType":"personal_standard",
			"securityOauthToken":"dt-from-center",
			"refreshToken":"rt-from-center"
		}`
	}
	statusCode := s.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func ptrInt64ForQoderTest(v int64) *int64 {
	return &v
}
