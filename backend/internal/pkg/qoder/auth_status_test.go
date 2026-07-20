package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshCosySessionForProfileUsesGatewayRefreshPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/algo"+CosyRefreshTokenPath, r.URL.Path)
		decoded, err := DecodeString(readAllString(t, r))
		require.NoError(t, err)
		var params AuthStatusParams
		require.NoError(t, json.Unmarshal([]byte(decoded), &params))
		require.Equal(t, "uid-1", params.UserID)
		require.Equal(t, "org-1", params.OrganizationID)
		require.Equal(t, "old-token", params.SecurityOauthToken)
		require.Equal(t, "old-refresh", params.RefreshToken)

		inner, err := json.Marshal(AuthStatusResult{
			ID:                 "uid-1",
			AccountID:          "aid-1",
			OrganizationID:     "org-1",
			SecurityOauthToken: "new-token",
			RefreshToken:       "new-refresh",
			UserType:           "enterprise_standard",
		})
		require.NoError(t, err)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCodeValue": http.StatusOK,
			"body":            string(inner),
		})
	}))
	defer server.Close()

	profile := MustProfileForSite(SiteCN)
	profile.GatewayBaseURL = server.URL
	identity, err := RefreshCosySessionForProfileContext(
		context.Background(),
		profile,
		"old-refresh",
		"old-token",
		"uid-1",
		"org-1",
		&MachineIdentity{MachineID: "machine-1", MachineToken: "machine-token", MachineType: "machine-type"},
		server.Client().Do,
	)
	require.NoError(t, err)
	require.Equal(t, "new-token", identity.SecurityOauthToken)
	require.Equal(t, "new-refresh", identity.RefreshToken)
	require.Equal(t, "enterprise_standard", identity.UserType)
}
