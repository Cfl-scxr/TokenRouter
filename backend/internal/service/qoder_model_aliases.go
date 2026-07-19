package service

import "strings"

// lookupQoderModelAlias 仅解析当前公开 alias，未命中的模型由调用方原样透传。
func lookupQoderModelAlias(model string) (qoderModelInfo, bool) {
	model = normalizeQoderAliasModel(model)
	info, ok := defaultQoderModelAliases[model]
	return info, ok
}

func isQoderAliasBillingModel(model string) bool {
	model = normalizeQoderAliasModel(model)
	if model == "" {
		return false
	}
	if _, ok := defaultQoderModelAliases[model]; ok {
		return true
	}
	return isQoderAliasRouteKey(model, defaultQoderModelAliases)
}

func isQoderAliasRouteKey(model string, aliases map[string]qoderModelInfo) bool {
	for _, info := range aliases {
		if normalizeQoderAliasModel(info.Key) == model {
			return true
		}
	}
	return false
}

func normalizeQoderAliasModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func QoderAliasRequiresManualPricing(model string) bool {
	return isQoderAliasBillingModel(model)
}

func qoderAliasRequiresManualPricingAny(models ...string) bool {
	for _, model := range models {
		if isQoderAliasBillingModel(model) {
			return true
		}
	}
	return false
}
