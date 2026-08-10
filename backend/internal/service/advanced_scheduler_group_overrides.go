package service

import (
	"context"
	"fmt"
	"math"

	"github.com/TokenFlux/TokenRouter/internal/pkg/ctxkey"
)

// advancedSchedulerEffectiveSettings 是完成全局与分组覆盖合并后的请求级配置。
// 分组字段优先级最高；缺失字段继续继承设置仓库和静态配置的结果。
type advancedSchedulerEffectiveSettings struct {
	stickyWeightedEnabled       bool
	subscriptionPriorityEnabled bool
	topK                        int
	weights                     GatewayAdvancedSchedulerScoreWeightsView
}

// ValidateGroupAdvancedSchedulerOverrides 校验分组稀疏覆盖的单字段边界。
// 当基础评分权重全部由分组覆盖时，同时拒绝全部为零的无效组合。
func ValidateGroupAdvancedSchedulerOverrides(overrides GroupAdvancedSchedulerOverrides) error {
	if overrides.LBTopK != nil && *overrides.LBTopK <= 0 {
		return fmt.Errorf("lb_top_k must be a positive integer")
	}

	weights := []struct {
		name  string
		value *float64
	}{
		{"weight_priority", overrides.WeightPriority},
		{"weight_load", overrides.WeightLoad},
		{"weight_queue", overrides.WeightQueue},
		{"weight_error_rate", overrides.WeightErrorRate},
		{"weight_ttft", overrides.WeightTTFT},
		{"weight_reset", overrides.WeightReset},
		{"weight_quota_headroom", overrides.WeightQuotaHeadroom},
		{"weight_previous_response", overrides.WeightPreviousResponse},
		{"weight_session_sticky", overrides.WeightSessionSticky},
	}
	for _, item := range weights {
		if item.value == nil {
			continue
		}
		if *item.value < 0 || math.IsNaN(*item.value) || math.IsInf(*item.value, 0) {
			return fmt.Errorf("%s must be a non-negative finite number", item.name)
		}
	}

	// 只有全部基础权重都由分组给出时，才能在不依赖全局值的前提下完整校验。
	baseWeights := []*float64{
		overrides.WeightPriority,
		overrides.WeightLoad,
		overrides.WeightQueue,
		overrides.WeightErrorRate,
		overrides.WeightTTFT,
		overrides.WeightReset,
		overrides.WeightQuotaHeadroom,
	}
	allBaseWeightsOverridden := true
	for _, weight := range baseWeights {
		if weight == nil {
			allBaseWeightsOverridden = false
			break
		}
	}
	if allBaseWeightsOverridden {
		resolved := GatewayAdvancedSchedulerScoreWeightsView{
			Priority:      *overrides.WeightPriority,
			Load:          *overrides.WeightLoad,
			Queue:         *overrides.WeightQueue,
			ErrorRate:     *overrides.WeightErrorRate,
			TTFT:          *overrides.WeightTTFT,
			Reset:         *overrides.WeightReset,
			QuotaHeadroom: *overrides.WeightQuotaHeadroom,
		}
		if overrides.WeightPreviousResponse != nil {
			resolved.Previous = *overrides.WeightPreviousResponse
		}
		if overrides.WeightSessionSticky != nil {
			resolved.SessionSticky = *overrides.WeightSessionSticky
		}
		if !resolved.configWeights().IsValid() {
			return fmt.Errorf("base score weights must not all be zero")
		}
	}
	return nil
}

// CloneGroupAdvancedSchedulerOverrides 返回覆盖对象及其指针字段的独立副本。
func CloneGroupAdvancedSchedulerOverrides(overrides GroupAdvancedSchedulerOverrides) GroupAdvancedSchedulerOverrides {
	cloneBool := func(value *bool) *bool {
		if value == nil {
			return nil
		}
		cloned := *value
		return &cloned
	}
	cloneInt := func(value *int) *int {
		if value == nil {
			return nil
		}
		cloned := *value
		return &cloned
	}
	cloneFloat := func(value *float64) *float64 {
		if value == nil {
			return nil
		}
		cloned := *value
		return &cloned
	}
	return GroupAdvancedSchedulerOverrides{
		StickyWeightedEnabled:       cloneBool(overrides.StickyWeightedEnabled),
		SubscriptionPriorityEnabled: cloneBool(overrides.SubscriptionPriorityEnabled),
		LBTopK:                      cloneInt(overrides.LBTopK),
		WeightPriority:              cloneFloat(overrides.WeightPriority),
		WeightLoad:                  cloneFloat(overrides.WeightLoad),
		WeightQueue:                 cloneFloat(overrides.WeightQueue),
		WeightErrorRate:             cloneFloat(overrides.WeightErrorRate),
		WeightTTFT:                  cloneFloat(overrides.WeightTTFT),
		WeightReset:                 cloneFloat(overrides.WeightReset),
		WeightQuotaHeadroom:         cloneFloat(overrides.WeightQuotaHeadroom),
		WeightPreviousResponse:      cloneFloat(overrides.WeightPreviousResponse),
		WeightSessionSticky:         cloneFloat(overrides.WeightSessionSticky),
	}
}

// resolveAdvancedSchedulerEffectiveSettings 以全局生效配置为基线合并分组覆盖。
// 若历史脏数据导致合并后的权重失效，保留其余分组覆盖并回退权重到全局有效值。
func resolveAdvancedSchedulerEffectiveSettings(
	baseTopK int,
	baseWeights GatewayAdvancedSchedulerScoreWeightsView,
	global advancedSchedulerRuntimeSettings,
	overrides GroupAdvancedSchedulerOverrides,
) advancedSchedulerEffectiveSettings {
	if baseTopK <= 0 {
		baseTopK = 7
	}
	globalTopK := baseTopK
	if global.lbTopKOverride > 0 {
		globalTopK = global.lbTopKOverride
	}
	globalWeights := applyAdvancedSchedulerWeightOverrides(baseWeights, global.weightOverrides)
	if !globalWeights.configWeights().IsValid() {
		globalWeights = baseWeights
	}

	effective := advancedSchedulerEffectiveSettings{
		stickyWeightedEnabled:       global.stickyWeightedEnabled,
		subscriptionPriorityEnabled: global.subscriptionPriorityEnabled,
		topK:                        globalTopK,
		weights:                     globalWeights,
	}
	if overrides.StickyWeightedEnabled != nil {
		effective.stickyWeightedEnabled = *overrides.StickyWeightedEnabled
	}
	if overrides.SubscriptionPriorityEnabled != nil {
		effective.subscriptionPriorityEnabled = *overrides.SubscriptionPriorityEnabled
	}
	if overrides.LBTopK != nil && *overrides.LBTopK > 0 {
		effective.topK = *overrides.LBTopK
	}
	if overrides.WeightPriority != nil {
		effective.weights.Priority = *overrides.WeightPriority
	}
	if overrides.WeightLoad != nil {
		effective.weights.Load = *overrides.WeightLoad
	}
	if overrides.WeightQueue != nil {
		effective.weights.Queue = *overrides.WeightQueue
	}
	if overrides.WeightErrorRate != nil {
		effective.weights.ErrorRate = *overrides.WeightErrorRate
	}
	if overrides.WeightTTFT != nil {
		effective.weights.TTFT = *overrides.WeightTTFT
	}
	if overrides.WeightReset != nil {
		effective.weights.Reset = *overrides.WeightReset
	}
	if overrides.WeightQuotaHeadroom != nil {
		effective.weights.QuotaHeadroom = *overrides.WeightQuotaHeadroom
	}
	if overrides.WeightPreviousResponse != nil {
		effective.weights.Previous = *overrides.WeightPreviousResponse
	}
	if overrides.WeightSessionSticky != nil {
		effective.weights.SessionSticky = *overrides.WeightSessionSticky
	}
	if !effective.weights.configWeights().IsValid() {
		effective.weights = globalWeights
	}
	return effective
}

// advancedSchedulerEffectiveSettingsForGroup 将分组覆盖置于运行时全局设置之上。
func (s *OpenAIGatewayService) advancedSchedulerEffectiveSettingsForGroup(
	ctx context.Context,
	group *Group,
) advancedSchedulerEffectiveSettings {
	if ctx == nil {
		ctx = context.Background()
	}
	var overrides GroupAdvancedSchedulerOverrides
	if group != nil && group.UsesAdvancedScheduler() {
		overrides = group.AdvancedSchedulerOverrides
	}
	return resolveAdvancedSchedulerEffectiveSettings(
		s.openAIWSLBTopK(),
		s.openAIWSSchedulerWeights(),
		s.advancedSchedulerRuntimeSettings(ctx),
		overrides,
	)
}

// advancedSchedulerEffectiveSettingsForRequest 读取最终目标分组并生成请求级有效配置。
// 分组不存在或未被加载时只使用全局配置，保持无分组路径的历史行为。
func (s *OpenAIGatewayService) advancedSchedulerEffectiveSettingsForRequest(
	ctx context.Context,
	groupID *int64,
) advancedSchedulerEffectiveSettings {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.advancedSchedulerEffectiveSettingsForGroup(ctx, s.advancedSchedulerGroupForRequest(ctx, groupID))
}

// advancedSchedulerGroupForRequest 优先复用请求上下文的最终分组，必要时再读取调度快照。
func (s *OpenAIGatewayService) advancedSchedulerGroupForRequest(ctx context.Context, groupID *int64) *Group {
	if groupID == nil || *groupID <= 0 {
		return nil
	}
	if ctx != nil {
		if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == *groupID {
			return group
		}
	}
	if s == nil || s.schedulerSnapshot == nil {
		return nil
	}
	group, err := s.schedulerSnapshot.GetGroupByID(ctx, *groupID)
	if err != nil {
		return nil
	}
	return group
}
