package qoder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQoderDeviceAuthRequestAuthorizationURL(t *testing.T) {
	req := &DeviceAuthRequest{
		Nonce:         "nonce-1",
		CodeChallenge: "challenge-1",
		MachineID:     "machine-1",
		ClientID:      OAuthClientID,
	}

	rawURL := req.AuthorizationURL()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	require.Equal(t, DeviceAuthorizationURL, parsed.Scheme+"://"+parsed.Host+parsed.Path)

	values := parsed.Query()
	require.Equal(t, "nonce-1", values.Get("nonce"))
	require.Equal(t, "challenge-1", values.Get("challenge"))
	require.Equal(t, "S256", values.Get("challenge_method"))
	require.Equal(t, OAuthClientID, values.Get("client_id"))
	require.Equal(t, "machine-1", values.Get("machine_id"))
}

func TestQoderCNDeviceAuthRequestUsesCNProfileAndNonce(t *testing.T) {
	req, err := NewDeviceAuthRequestForSite(SiteCN)
	require.NoError(t, err)
	require.Len(t, req.Nonce, 32)
	require.NotContains(t, req.Nonce, "-")

	parsed, err := url.Parse(req.AuthorizationURL())
	require.NoError(t, err)
	require.Equal(t, CNDeviceAuthorizationURL, parsed.Scheme+"://"+parsed.Host+parsed.Path)
	require.Equal(t, CNOAuthClientID, parsed.Query().Get("client_id"))
	require.Empty(t, parsed.Query().Get("redirect_uri"))
	require.Equal(t, req.MachineID, parsed.Query().Get("machine_id"))
}

func TestExchangeQoderCN20PATCompletesUserInfoAndStatus(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case JobTokenExchangePath:
			require.Equal(t, "qoder/"+CNClientVersion, r.Header.Get("User-Agent"))
			require.Equal(t, CNClientVersion, r.Header.Get("Cosy-Version"))
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "pat-cn", body["personal_token"])
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "openapi-access",
				"refresh_token": "openapi-refresh",
				"user_id":       "user-token",
				"expires_in":    "3600",
			})
		case UserInfoPath:
			require.Equal(t, "Bearer openapi-access", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id":   "user-info",
				"user_name": "CN User",
			})
		case "/algo" + AuthStatusPath:
			require.Equal(t, CNClientVersion, r.Header.Get("Cosy-Version"))
			require.Equal(t, "machine-token", r.Header.Get("Cosy-Machinetoken"))
			decoded, err := DecodeString(readAllString(t, r))
			require.NoError(t, err)
			var params AuthStatusParams
			require.NoError(t, json.Unmarshal([]byte(decoded), &params))
			require.Equal(t, "user-info", params.UserID)
			require.Equal(t, "openapi-access", params.SecurityOauthToken)
			require.Equal(t, "openapi-refresh", params.RefreshToken)
			_ = json.NewEncoder(w).Encode(AuthStatusResult{
				Name:               "Status User",
				ID:                 "cosy-uid",
				AccountID:          "cosy-aid",
				OrganizationID:     "org-cn",
				OrganizationName:   "CN Org",
				UserType:           "enterprise_standard",
				SecurityOauthToken: "cosy-token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	profile := MustProfileForSite(SiteCN)
	profile.OpenAPIBaseURL = server.URL
	profile.GatewayBaseURL = server.URL
	identity, expiresAt, err := ExchangeQoderCN20PATContext(context.Background(), "pat-cn", &MachineIdentity{
		MachineID:    "machine-id",
		MachineToken: "machine-token",
		MachineType:  "machine-type",
	}, profile, server.Client().Do)

	require.NoError(t, err)
	require.Equal(t, []string{JobTokenExchangePath, UserInfoPath, "/algo" + AuthStatusPath}, paths)
	require.Equal(t, "cosy-token", identity.SecurityOauthToken)
	require.Equal(t, "openapi-refresh", identity.RefreshToken)
	require.Equal(t, "cosy-uid", identity.UID)
	require.Equal(t, "cosy-aid", identity.AID)
	require.Equal(t, "org-cn", identity.OrganizationID)
	require.Equal(t, "enterprise_standard", identity.UserType)
	require.True(t, expiresAt.After(time.Now().Add(59*time.Minute)))
}

func TestQoderCN20PATErrorRedactsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"personal_token":"pat-secret","refresh_token":"refresh-secret","message":"invalid"}`)
	}))
	defer server.Close()
	profile := MustProfileForSite(SiteCN)
	profile.OpenAPIBaseURL = server.URL
	client := NewOAuthClientForProfile(profile, server.Client())

	_, err := client.ExchangeQoderCN20PAT(context.Background(), "pat-secret")
	require.Error(t, err)
	var apiErr *OpenAPIError
	require.ErrorAs(t, err, &apiErr)
	require.True(t, apiErr.InvalidCredentials())
	require.False(t, apiErr.UpstreamFailure())
	upstreamErr := &OpenAPIError{Operation: "PAT exchange", StatusCode: http.StatusServiceUnavailable}
	require.False(t, upstreamErr.InvalidCredentials())
	require.True(t, upstreamErr.UpstreamFailure())
	require.NotContains(t, err.Error(), "pat-secret")
	require.NotContains(t, err.Error(), "refresh-secret")
	require.True(t, strings.Contains(err.Error(), "status 401"))
}

func TestQoderCN20RefreshPostsRefreshTokenAndValidatesExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, DeviceTokenRefreshPath, r.URL.Path)
		require.Equal(t, "qoder/"+CNClientVersion, r.Header.Get("User-Agent"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "old-refresh", body["refresh_token"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
		})
	}))
	defer server.Close()
	profile := MustProfileForSite(SiteCN)
	profile.OpenAPIBaseURL = server.URL
	client := NewOAuthClientForProfile(profile, server.Client())

	token, err := client.RefreshQoderCN20Token(context.Background(), "old-refresh")
	require.NoError(t, err)
	require.Equal(t, "new-access", token.AccessTokenValue())
	require.Equal(t, "new-refresh", token.RefreshToken)
}

func TestQoderGenerateCodeChallenge(t *testing.T) {
	require.Equal(t, "iMnq5o6zALKXGivsnlom_0F5_WYda32GHkxlV7mq7hQ", GenerateCodeChallenge("verifier"))
}

func TestQoderOAuthClientPollDeviceTokenPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, DevicePollPath, r.URL.Path)
		require.Equal(t, "nonce-1", r.URL.Query().Get("nonce"))
		require.Equal(t, "verifier-1", r.URL.Query().Get("verifier"))
		require.Equal(t, "S256", r.URL.Query().Get("challenge_method"))
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewOAuthClient(server.URL, server.Client())
	token, ready, err := client.PollDeviceToken(context.Background(), "nonce-1", "verifier-1")
	require.NoError(t, err)
	require.False(t, ready)
	require.Nil(t, token)
}

func TestQoderOAuthClientPollDeviceTokenCompletedAndUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DevicePollPath:
			require.Equal(t, "nonce-2", r.URL.Query().Get("nonce"))
			require.Equal(t, "verifier-2", r.URL.Query().Get("verifier"))
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
				Token:        "access-token",
				RefreshToken: "refresh-token",
				UserID:       "user-from-token",
			})
		case UserInfoPath:
			require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(UserInfo{
				ID:       "user-from-info",
				Name:     "Qoder User",
				UserType: "personal_pro",
			})
		case OrganizationTagsPathPrefix + "user-from-info/tags":
			require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(OrganizationTags{
				OrganizationID:   "org-1",
				OrganizationName: "Org 1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOAuthClient(server.URL, server.Client())
	token, ready, err := client.PollDeviceToken(context.Background(), "nonce-2", "verifier-2")
	require.NoError(t, err)
	require.True(t, ready)
	require.Equal(t, "access-token", token.AccessTokenValue())
	require.Equal(t, "refresh-token", token.RefreshToken)

	user, err := client.GetUserInfo(context.Background(), token.AccessTokenValue())
	require.NoError(t, err)
	require.Equal(t, "user-from-info", user.ID)
	require.Equal(t, "Qoder User", user.Name)
	require.Equal(t, "personal_pro", user.UserType)

	tags, err := client.GetOrganizationTags(context.Background(), token.AccessTokenValue(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "org-1", tags.OrganizationID)
	require.Equal(t, "Org 1", tags.OrganizationName)
}

func TestQoderOAuthClientRedactsSensitiveErrorBodies(t *testing.T) {
	sensitiveBody := `{"message":"failed","securityOauthToken":"sec-secret","refresh_token":"rt-secret","uid":"uid-secret","cookie":"sid=secret"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, sensitiveBody, http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOAuthClient(server.URL, server.Client())

	_, _, err := client.PollDeviceToken(context.Background(), "nonce", "verifier")
	require.Error(t, err)
	assertQoderOAuthErrorRedacted(t, err.Error())

	_, err = client.GetUserInfo(context.Background(), "sec-token")
	require.Error(t, err)
	assertQoderOAuthErrorRedacted(t, err.Error())

	_, err = client.GetOrganizationTags(context.Background(), "sec-token", "uid-1")
	require.Error(t, err)
	assertQoderOAuthErrorRedacted(t, err.Error())
}

func TestBuildIdentityFromDeviceToken(t *testing.T) {
	identity := BuildIdentityFromDeviceToken(&UserInfo{
		ID:       "user-1",
		Name:     "Qoder User",
		UserType: "personal_pro",
	}, &DeviceTokenResponse{
		Token:        "token-1",
		RefreshToken: "refresh-1",
		UserID:       "fallback-user",
	})

	require.Equal(t, "Qoder User", identity.Name)
	require.Equal(t, "user-1", identity.UID)
	require.Equal(t, "user-1", identity.AID)
	require.Equal(t, "personal_pro", identity.UserType)
	require.Equal(t, "token-1", identity.SecurityOauthToken)
	require.Equal(t, "refresh-1", identity.RefreshToken)
}

func assertQoderOAuthErrorRedacted(t *testing.T, errText string) {
	t.Helper()
	require.Contains(t, errText, "status 500")
	require.NotContains(t, errText, "sec-secret")
	require.NotContains(t, errText, "rt-secret")
	require.NotContains(t, errText, "uid-secret")
	require.NotContains(t, errText, "sid=secret")
	require.Contains(t, errText, "***")
}

func TestBuildIdentityFromDeviceTokenCopiesOrganizationFromUserInfo(t *testing.T) {
	identity := BuildIdentityFromDeviceToken(&UserInfo{
		ID:               "user-1",
		OrganizationID:   "org-1",
		OrganizationName: "Org 1",
	}, &DeviceTokenResponse{
		Token:  "token-1",
		UserID: "fallback-user",
	})

	require.Equal(t, "org-1", identity.OrganizationID)
	require.Equal(t, "Org 1", identity.OrganizationName)
}
