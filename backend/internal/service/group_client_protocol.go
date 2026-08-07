package service

import "github.com/TokenFlux/TokenRouter/internal/domain"

const (
	GroupClientProtocolAnthropicMessages     = domain.GroupClientProtocolAnthropicMessages
	GroupClientProtocolOpenAIResponses       = domain.GroupClientProtocolOpenAIResponses
	GroupClientProtocolOpenAIChatCompletions = domain.GroupClientProtocolOpenAIChatCompletions
	GroupClientProtocolGeminiGenerateContent = domain.GroupClientProtocolGeminiGenerateContent
)

// EffectiveAllowedClientProtocols 返回可用于热路径判定的协议集合。
// nil 来自旧认证缓存；必选协议平台的空数组可能由滚动升级中的旧进程写入，
// 两种情况都按升级前的隐式行为恢复。Qoder 的显式空数组保持为空。
func (g *Group) EffectiveAllowedClientProtocols() []GroupClientProtocol {
	if g == nil {
		return []GroupClientProtocol{}
	}
	if g.AllowedClientProtocols == nil ||
		(len(g.AllowedClientProtocols) == 0 && len(domain.RequiredGroupClientProtocols(g.Platform)) > 0) {
		return domain.LegacyGroupClientProtocols(g.Platform, g.AllowMessagesDispatch)
	}
	return append([]GroupClientProtocol{}, g.AllowedClientProtocols...)
}

// AllowsClientProtocol 判断分组是否允许指定客户端协议。
func (g *Group) AllowsClientProtocol(protocol GroupClientProtocol) bool {
	if g == nil {
		return false
	}
	return domain.HasGroupClientProtocol(g.EffectiveAllowedClientProtocols(), protocol)
}

// normalizeExplicitGroupClientProtocols 校验显式 API 输入并保持固定顺序。
func normalizeExplicitGroupClientProtocols(platform string, protocols []GroupClientProtocol) ([]GroupClientProtocol, error) {
	return domain.ValidateGroupClientProtocols(platform, protocols)
}

// coerceGroupClientProtocolsForPlatform 在平台切换时保留仍受支持的协议并补齐基础协议。
// 仅用于省略新字段的平台切换兼容，不接受外部显式输入。
func coerceGroupClientProtocolsForPlatform(platform string, protocols []GroupClientProtocol) []GroupClientProtocol {
	supportedProtocols := domain.SupportedGroupClientProtocols(platform)
	supported := make(map[GroupClientProtocol]struct{}, len(supportedProtocols))
	for _, protocol := range supportedProtocols {
		supported[protocol] = struct{}{}
	}
	selectedSet := make(map[GroupClientProtocol]struct{}, len(protocols)+len(domain.RequiredGroupClientProtocols(platform)))
	for _, protocol := range protocols {
		if _, ok := supported[protocol]; ok {
			selectedSet[protocol] = struct{}{}
		}
	}
	for _, protocol := range domain.RequiredGroupClientProtocols(platform) {
		selectedSet[protocol] = struct{}{}
	}
	selected := make([]GroupClientProtocol, 0, len(selectedSet))
	for _, protocol := range supportedProtocols {
		if _, ok := selectedSet[protocol]; ok {
			selected = append(selected, protocol)
		}
	}
	normalized, err := domain.ValidateGroupClientProtocols(platform, selected)
	if err != nil {
		return domain.DefaultGroupClientProtocols(platform)
	}
	return normalized
}

// defaultGroupClientProtocols 返回新建分组的基础协议集合。
func defaultGroupClientProtocols(platform string) []GroupClientProtocol {
	return domain.DefaultGroupClientProtocols(platform)
}

// setGroupClientProtocol 更新兼容字段对应的单个协议。
func setGroupClientProtocol(protocols []GroupClientProtocol, protocol GroupClientProtocol, enabled bool) []GroupClientProtocol {
	return domain.SetGroupClientProtocol(protocols, protocol, enabled)
}
