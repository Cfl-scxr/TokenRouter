package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/pkg/qoder"
	"github.com/TokenFlux/TokenRouter/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestValidateQoderCosyCredentialsAcceptsDirectToken(t *testing.T) {
	account := &Account{
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
			"uid":                  "uid-1",
		},
	}

	require.NoError(t, ValidateQoderCosyCredentials(context.Background(), account))
}

func TestValidateQoderCosyCredentialsAcceptsDirectTokenWithAID(t *testing.T) {
	account := &Account{
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
			"aid":                  "aid-1",
		},
	}

	require.NoError(t, ValidateQoderCosyCredentials(context.Background(), account))
}

func TestValidateQoderCosyCredentialsRejectsDirectTokenWithoutIdentity(t *testing.T) {
	account := &Account{
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"security_oauth_token": "dt-token",
			"machine_id":           "machine-1",
		},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "uid or aid")
}

func TestValidateQoderCosyCredentialsRejectsUnknownSiteAndRefreshMode(t *testing.T) {
	baseCredentials := map[string]any{
		"security_oauth_token": "dt-token",
		"machine_id":           "machine-1",
		"uid":                  "uid-1",
	}
	account := &Account{Platform: PlatformQoder, Type: AccountTypeCosy, Credentials: baseCredentials}
	account.Credentials["site"] = "unknown"
	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "unsupported site")

	account.Credentials["site"] = "cn"
	account.Credentials["refresh_mode"] = "unknown"
	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "unsupported refresh_mode")
}

func TestValidateQoderCosyCredentialsRejectsMachineIDOnly(t *testing.T) {
	account := &Account{
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"machine_id": "machine-1"},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "pat or security_oauth_token")
}

func TestValidateQoderCosyCredentialsRejectsNonCosyQoderAccountType(t *testing.T) {
	account := &Account{
		Platform:    PlatformQoder,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key"},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "require cosy")
}

func TestValidateQoderCosyCredentialsRejectsCosyNonQoderPlatform(t *testing.T) {
	account := &Account{
		Platform:    PlatformAnthropic,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"pat": "pat"},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "requires qoder platform")
}

func TestValidateQoderCosyCredentialsRejectsMachineIDWithAuthDir(t *testing.T) {
	account := &Account{
		Platform: PlatformQoder,
		Type:     AccountTypeCosy,
		Credentials: map[string]any{
			"machine_id": "machine-1",
			"auth_dir":   "/tmp/qoder-auth",
		},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "pat or security_oauth_token")
}

func TestValidateQoderCosyCredentialsExchangesPAT(t *testing.T) {
	old := qoderValidatePAT
	defer func() { qoderValidatePAT = old }()
	calls := 0
	qoderValidatePAT = func(ctx context.Context, account *Account, pat string, machine *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		calls++
		require.Equal(t, "pat-123", pat)
		return &qoder.AuthIdentity{UID: "uid", SecurityOauthToken: "dt-token"}, nil
	}

	account := &Account{
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"pat": "pat-123"},
	}

	require.NoError(t, ValidateQoderCosyCredentials(context.Background(), account))
	require.Equal(t, 1, calls)
	require.NotEmpty(t, account.GetCredential("machine_id"))
	require.NotEmpty(t, account.GetCredential("machine_token"))
	require.NotEmpty(t, account.GetCredential("machine_type"))
}

func TestValidateQoderCosyCredentialsDefersUnchangedPATExchangeForSiteEdit(t *testing.T) {
	old := qoderValidatePAT
	defer func() { qoderValidatePAT = old }()
	qoderValidatePAT = func(context.Context, *Account, string, *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		t.Fatal("站点编辑不应在保存阶段重新交换未变化的 PAT")
		return nil, nil
	}
	account := &Account{
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"site": "cn", "pat": "pat-123"},
	}

	err := validateQoderCosyCredentialsWithOptions(context.Background(), account, nil, nil, true)

	require.NoError(t, err)
	require.NotEmpty(t, account.GetCredential("machine_id"))
	require.NotEmpty(t, account.GetCredential("machine_token"))
	require.NotEmpty(t, account.GetCredential("machine_type"))
}

func TestUpdateQoderPATAccountDefersCompatibilityCheckWhenSiteChanges(t *testing.T) {
	old := qoderValidatePAT
	defer func() { qoderValidatePAT = old }()
	qoderValidatePAT = func(context.Context, *Account, string, *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		t.Fatal("切换站点应保存原 PAT，并由后续连接测试判断兼容性")
		return nil, nil
	}
	const accountID int64 = 1201
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformQoder,
			Type:     AccountTypeCosy,
			Status:   StatusActive,
			Credentials: map[string]any{
				"site": "global",
				"pat":  "pat-123",
			},
		},
	}}
	svc := &adminServiceImpl{accountRepo: &upstreamBillingProbeAdminRepo{baseRepo}}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{"site": "cn"},
	})

	require.NoError(t, err)
	require.Equal(t, "cn", updated.GetCredential("site"))
	require.Equal(t, "pat-123", updated.GetCredential("pat"))
	require.NotEmpty(t, updated.GetCredential("machine_id"))
}

func TestValidateQoderCosyCredentialsRejectsBadPAT(t *testing.T) {
	old := qoderValidatePAT
	defer func() { qoderValidatePAT = old }()
	qoderValidatePAT = func(ctx context.Context, account *Account, pat string, machine *qoder.MachineIdentity) (*qoder.AuthIdentity, error) {
		return nil, errors.New("bad pat")
	}

	account := &Account{
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Credentials: map[string]any{"pat": "pat-123"},
	}

	require.ErrorContains(t, ValidateQoderCosyCredentials(context.Background(), account), "bad pat")
}

func TestValidateQoderCosyCredentialsPATUsesAccountDoer(t *testing.T) {
	upstream := &qoderValidationHTTPUpstreamStub{}
	proxyID := int64(9)
	account := &Account{
		ID:          109,
		Platform:    PlatformQoder,
		Type:        AccountTypeCosy,
		Concurrency: 5,
		ProxyID:     &proxyID,
		Proxy:       &Proxy{Protocol: "http", Host: "proxy.example.com", Port: 8080},
		Credentials: map[string]any{"pat": "pat-123"},
	}

	err := validateQoderCosyCredentials(context.Background(), account, upstream, nil)

	require.NoError(t, err)
	require.Equal(t, "http://proxy.example.com:8080", upstream.proxyURL)
	require.Equal(t, int64(109), upstream.accountID)
	require.Equal(t, 5, upstream.accountConcurrency)
}

type qoderValidationHTTPUpstreamStub struct {
	proxyURL           string
	accountID          int64
	accountConcurrency int
}

func (s *qoderValidationHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *qoderValidationHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	s.proxyURL = proxyURL
	s.accountID = accountID
	s.accountConcurrency = accountConcurrency
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{
			"id":"user-1",
			"name":"User",
			"userType":"personal_standard",
			"securityOauthToken":"dt-from-center",
			"refreshToken":"rt-from-center"
		}`)),
		Request: req,
	}, nil
}
