package qoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSiteAndRefreshModeCompatibility(t *testing.T) {
	site, err := ParseSite("")
	require.NoError(t, err)
	require.Equal(t, SiteGlobal, site)

	site, err = ParseSite("cn")
	require.NoError(t, err)
	require.Equal(t, SiteCN, site)
	_, err = ParseSite("unknown")
	require.ErrorContains(t, err, "unsupported site")

	mode, err := ParseRefreshMode("")
	require.NoError(t, err)
	require.Equal(t, RefreshModeCosy, mode)
	mode, err = ParseRefreshMode(RefreshModeQoderCN20)
	require.NoError(t, err)
	require.Equal(t, RefreshModeQoderCN20, mode)
	_, err = ParseRefreshMode("unknown")
	require.ErrorContains(t, err, "unsupported refresh_mode")
}

func TestProfileForSiteUsesFrozenProductionValues(t *testing.T) {
	global, err := ProfileForSite(SiteGlobal)
	require.NoError(t, err)
	require.Equal(t, GlobalDeviceAuthorizationURL, global.DeviceAuthorizationURL)
	require.Equal(t, GlobalOpenAPIBaseURL, global.OpenAPIBaseURL)
	require.Equal(t, GlobalCenterBaseURL, global.CenterBaseURL)
	require.Equal(t, GlobalGatewayBaseURL, global.GatewayBaseURL)
	require.Equal(t, GlobalClientVersion, global.ClientVersion)
	require.Equal(t, GlobalOAuthClientID, global.OAuthClientID)

	cn, err := ProfileForSite(SiteCN)
	require.NoError(t, err)
	require.Equal(t, CNDeviceAuthorizationURL, cn.DeviceAuthorizationURL)
	require.Equal(t, CNOpenAPIBaseURL, cn.OpenAPIBaseURL)
	require.Empty(t, cn.CenterBaseURL)
	require.Equal(t, CNGatewayBaseURL, cn.GatewayBaseURL)
	require.Equal(t, CNClientVersion, cn.ClientVersion)
	require.Equal(t, CNOAuthClientID, cn.OAuthClientID)
}

func TestNormalizeProfileAllowsTestEndpointInjection(t *testing.T) {
	profile, err := NormalizeProfile(Profile{
		Site:           SiteCN,
		OpenAPIBaseURL: "http://127.0.0.1:18080/",
		GatewayBaseURL: "http://127.0.0.1:18081/",
	})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:18080", profile.OpenAPIBaseURL)
	require.Equal(t, "http://127.0.0.1:18081", profile.GatewayBaseURL)
	require.Equal(t, CNClientVersion, profile.ClientVersion)
}

func TestMachineOSForNormalizesArm64(t *testing.T) {
	require.Equal(t, "aarch64_darwin", MachineOSFor("arm64", "darwin"))
	require.Equal(t, "amd64_linux", MachineOSFor("amd64", "linux"))
}

func TestModelsAndAliasesAreSiteAware(t *testing.T) {
	require.Equal(t, []string{
		"claude-opus-4-6",
		"auto",
		"performance",
		"efficient",
		"lite",
		"qwen3.8-max-preview",
		"qwen3.7-max",
		"qwen3.7-plus",
		"kimi-k3",
		"kimi-k2.7-code",
		"glm-5.2",
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"minimax-m3",
	}, DefaultRequestModelIDsForSite(SiteGlobal))
	require.Equal(t, []string{
		"auto",
		"qwen3.8-max-preview",
		"qwen3.7-max",
		"qwen3.7-plus",
		"qwen3.6-flash",
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"glm-5.2",
		"kimi-k2.7-code",
		"minimax-m2.7",
	}, DefaultRequestModelIDsForSite(SiteCN))

	route, ok := AliasForSite(SiteGlobal, "qwen3.8-max-preview")
	require.True(t, ok)
	require.Equal(t, "qmodel_preview", route)
	route, ok = AliasForSite(SiteCN, "minimax-m2.7")
	require.True(t, ok)
	require.Equal(t, "mmodel", route)
	require.False(t, ModelCompatibleWithSite(SiteGlobal, "minimax-m2.7"))
	require.True(t, ModelCompatibleWithSite(SiteCN, "minimax-m2.7"))
	require.True(t, ModelCompatibleWithSite(SiteGlobal, "unknown-raw-key"))
}
