package qoder

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineIPFromAddressesUsesFirstNonLoopbackIPv4(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("172.18.0.1"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.5.36"), Mask: net.CIDRMask(24, 32)},
	}

	require.Equal(t, "172.18.0.1", machineIPFromAddresses(addresses))
}

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
	require.Equal(t, "1.21.2", GlobalClientVersion)
	require.Equal(t, "1.21.2", global.ClientVersion)
	require.Equal(t, GlobalOAuthClientID, global.OAuthClientID)
	require.Equal(t, "Qoder/1.21.2", global.OpenAPIUserAgent())

	cn, err := ProfileForSite(SiteCN)
	require.NoError(t, err)
	require.Equal(t, CNDeviceAuthorizationURL, cn.DeviceAuthorizationURL)
	require.Equal(t, CNOpenAPIBaseURL, cn.OpenAPIBaseURL)
	require.Empty(t, cn.CenterBaseURL)
	require.Equal(t, CNGatewayBaseURL, cn.GatewayBaseURL)
	require.Equal(t, "1.10.0", CNClientVersion)
	require.Equal(t, "1.10.0", cn.ClientVersion)
	require.Equal(t, CNOAuthClientID, cn.OAuthClientID)
	require.Equal(t, "Qoder CN/1.10.0", cn.OpenAPIUserAgent())
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

func TestMachineOSForNormalizesArchitectureAliases(t *testing.T) {
	require.Equal(t, "aarch64_darwin", MachineOSFor("arm64", "darwin"))
	require.Equal(t, "aarch64_linux", MachineOSFor("aarch64", "linux"))
	require.Equal(t, "x86_64_linux", MachineOSFor("amd64", "linux"))
	require.Equal(t, "x86_64_darwin", MachineOSFor("x86_64", "darwin"))
}

func TestModelsAndAliasesAreSiteAware(t *testing.T) {
	require.Equal(t, []string{
		"claude-opus-4-6",
		"auto",
		"performance",
		"efficient",
		"lite",
		"qwen3.8-max",
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
		"qwen3.8-max",
		"qwen3.7-max",
		"qwen3.7-plus",
		"qwen3.6-flash",
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"glm-5.2",
		"kimi-k2.7-code",
		"minimax-m2.7",
	}, DefaultRequestModelIDsForSite(SiteCN))

	route, ok := AliasForSite(SiteGlobal, "qwen3.8-max")
	require.True(t, ok)
	require.Equal(t, "qmodel_38max", route)
	route, ok = AliasForSite(SiteCN, "qwen3.8-max")
	require.True(t, ok)
	require.Equal(t, "qmodel_38max", route)
	route, ok = AliasForSite(SiteCN, "minimax-m2.7")
	require.True(t, ok)
	require.Equal(t, "mmodel", route)
	// Preview 既不是公开 alias，也不是已知 route；旧请求只能按 raw key 透传或由账号显式映射。
	_, ok = AliasForSite(SiteGlobal, "qwen3.8-max-preview")
	require.False(t, ok)
	_, ok = AliasForSite(SiteCN, "qwen3.8-max-preview")
	require.False(t, ok)
	require.False(t, isKnownPublicAlias("qwen3.8-max-preview"))
	require.False(t, isKnownRouteKey("qmodel_preview"))
	require.True(t, ModelCompatibleWithSite(SiteGlobal, "qwen3.8-max"))
	require.True(t, ModelCompatibleWithSite(SiteGlobal, "qmodel_38max"))
	require.True(t, ModelCompatibleWithSite(SiteCN, "qwen3.8-max"))
	require.True(t, ModelCompatibleWithSite(SiteCN, "qmodel_38max"))
	require.False(t, ModelCompatibleWithSite(SiteGlobal, "minimax-m2.7"))
	require.True(t, ModelCompatibleWithSite(SiteCN, "minimax-m2.7"))
	require.True(t, ModelCompatibleWithSite(SiteGlobal, "unknown-raw-key"))
}

func TestThinkingCapabilityForSiteUsesSiteSnapshot(t *testing.T) {
	tests := []struct {
		site  Site
		model string
		want  ThinkingCapability
	}{
		{site: SiteGlobal, model: "qwen3.8-max", want: ThinkingToggleOnly},
		{site: SiteGlobal, model: " QMODEL_38MAX ", want: ThinkingToggleOnly},
		{site: SiteGlobal, model: "qwen3.7-max", want: ThinkingToggleOnly},
		{site: SiteGlobal, model: "qwen3.7-plus", want: ThinkingToggleOnly},
		{site: SiteGlobal, model: "deepseek-v4-pro", want: ThinkingHighMax},
		{site: SiteGlobal, model: "deepseek-v4-flash", want: ThinkingHighMax},
		{site: SiteGlobal, model: "glm-5.2", want: ThinkingHighMax},
		{site: SiteGlobal, model: "kimi-k3", want: ThinkingUnsupported},
		// 空站点与 ParseSite 的旧账号兼容语义一致，按国际站查询能力。
		{site: "", model: "deepseek-v4-pro", want: ThinkingHighMax},
		{site: SiteCN, model: "auto", want: ThinkingUnsupported},
		{site: SiteCN, model: "qwen3.8-max", want: ThinkingToggleOnly},
		{site: SiteCN, model: " QMODEL_38MAX ", want: ThinkingToggleOnly},
		{site: SiteCN, model: "qwen3.7-max", want: ThinkingToggleOnly},
		{site: SiteCN, model: "qwen3.7-plus", want: ThinkingToggleOnly},
		{site: SiteCN, model: "qwen3.6-flash", want: ThinkingUnsupported},
		{site: SiteCN, model: "deepseek-v4-pro", want: ThinkingHighMax},
		{site: SiteCN, model: "deepseek-v4-flash", want: ThinkingHighMax},
		{site: SiteCN, model: "glm-5.2", want: ThinkingHighMax},
		{site: SiteCN, model: "kimi-k2.7-code", want: ThinkingUnsupported},
		{site: SiteCN, model: "minimax-m2.7", want: ThinkingUnsupported},
	}

	for _, tt := range tests {
		t.Run(string(tt.site)+"/"+tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, ThinkingCapabilityForSite(tt.site, tt.model))
		})
	}

	// 国内站已有的 route key、空白和大小写必须与公开 alias 得到相同能力。
	require.Equal(t, ThinkingToggleOnly, ThinkingCapabilityForSite(SiteCN, " QMODEL_LATEST "))
	require.Equal(t, ThinkingHighMax, ThinkingCapabilityForSite(SiteCN, "dmodel"))
	// 两站共有 route key 必须一致，国际站独有模型和未知路由仍保持隔离。
	require.Equal(t, ThinkingToggleOnly, ThinkingCapabilityForSite(SiteGlobal, " QMODEL_LATEST "))
	require.Equal(t, ThinkingHighMax, ThinkingCapabilityForSite(SiteGlobal, "dmodel"))
	require.Equal(t, ThinkingUnsupported, ThinkingCapabilityForSite(SiteGlobal, "kmodel_latest"))
	require.Equal(t, ThinkingUnsupported, ThinkingCapabilityForSite(SiteCN, "custom-model"))
	require.Equal(t, ThinkingUnsupported, ThinkingCapabilityForSite(Site("invalid"), "qmodel_38max"))
}

func TestThinkingCapabilitySnapshotsCoverEveryModel(t *testing.T) {
	tests := []struct {
		name         string
		models       []Model
		aliases      map[string]string
		capabilities map[string]ThinkingCapability
	}{
		{name: "国际站", models: globalModels, aliases: globalAliases, capabilities: globalThinkingCapabilities},
		{name: "国内站", models: cnModels, aliases: cnAliases, capabilities: cnThinkingCapabilities},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 能力表必须显式覆盖站点的所有 route key，避免新增模型静默沿用零值能力。
			wantRoutes := make(map[string]struct{}, len(tt.aliases))
			for _, model := range tt.models {
				route, ok := tt.aliases[model.ID]
				require.True(t, ok, "%s模型 %q 缺少 route key", tt.name, model.ID)
				wantRoutes[route] = struct{}{}
			}
			require.Len(t, tt.aliases, len(tt.models), "%s模型和 alias 数量必须一致", tt.name)

			gotRoutes := make(map[string]struct{}, len(tt.capabilities))
			for route := range tt.capabilities {
				gotRoutes[route] = struct{}{}
			}
			require.Equal(t, wantRoutes, gotRoutes, "%s Thinking 能力表必须与模型 route key 完全一致", tt.name)
		})
	}
}

func TestSharedThinkingCapabilitiesStayInSyncAcrossSites(t *testing.T) {
	// 两站共有 route 必须使用同一能力，站点独有 route 则继续由各自快照维护。
	for route, globalCapability := range globalThinkingCapabilities {
		cnCapability, shared := cnThinkingCapabilities[route]
		if !shared {
			continue
		}
		require.Equal(t, cnCapability, globalCapability, "共有 route %q 的 Thinking 能力不一致", route)
	}
}
