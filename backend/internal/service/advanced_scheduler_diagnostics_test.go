package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

type advancedSchedulerDiagnosticSourceStub struct {
	account  *Account
	group    *Group
	accounts []Account
	pool     []Account
}

func (s *advancedSchedulerDiagnosticSourceStub) GetAccount(_ context.Context, _ int64) (*Account, error) {
	return s.account, nil
}

func (s *advancedSchedulerDiagnosticSourceStub) GetGroup(_ context.Context, _ int64) (*Group, error) {
	return s.group, nil
}

func (s *advancedSchedulerDiagnosticSourceStub) ListAccounts(_ context.Context, _, _ int, _, _, _, _ string, _ int64, _, _, _ string) ([]Account, int64, error) {
	return s.accounts, int64(len(s.accounts)), nil
}

func (s *advancedSchedulerDiagnosticSourceStub) ListSchedulableAccountsForAdvancedSchedulerScore(_ context.Context, _ *int64, _ string) ([]Account, error) {
	return s.pool, nil
}

func advancedSchedulerDiagnosticBool(value bool) *bool {
	return &value
}

func advancedSchedulerDiagnosticInt(value int) *int {
	return &value
}

func advancedSchedulerDiagnosticFloat(value float64) *float64 {
	return &value
}

func findAdvancedSchedulerDiagnosticMetric(metrics []AdvancedSchedulerScoreDiagnosticMetric, key string) *AdvancedSchedulerScoreDiagnosticMetric {
	for index := range metrics {
		if metrics[index].Key == key {
			return &metrics[index]
		}
	}
	return nil
}

func findAdvancedSchedulerDiagnosticSetting(settings []AdvancedSchedulerScoreDiagnosticSetting, key string) *AdvancedSchedulerScoreDiagnosticSetting {
	for index := range settings {
		if settings[index].Key == key {
			return &settings[index]
		}
	}
	return nil
}

func TestAdvancedSchedulerScoreDiagnosticService_UsesActualFormulaAndSafeDTO(t *testing.T) {
	group := &Group{
		ID:            301,
		Name:          "advanced",
		Platform:      PlatformGemini,
		SchedulerType: GroupSchedulerTypeAdvanced,
		AdvancedSchedulerOverrides: GroupAdvancedSchedulerOverrides{
			StickyWeightedEnabled:  advancedSchedulerDiagnosticBool(true),
			LBTopK:                 advancedSchedulerDiagnosticInt(2),
			WeightPriority:         advancedSchedulerDiagnosticFloat(2),
			WeightLoad:             advancedSchedulerDiagnosticFloat(1),
			WeightQueue:            advancedSchedulerDiagnosticFloat(0),
			WeightErrorRate:        advancedSchedulerDiagnosticFloat(0),
			WeightTTFT:             advancedSchedulerDiagnosticFloat(0),
			WeightReset:            advancedSchedulerDiagnosticFloat(0),
			WeightQuotaHeadroom:    advancedSchedulerDiagnosticFloat(0),
			WeightPreviousResponse: advancedSchedulerDiagnosticFloat(0),
			WeightSessionSticky:    advancedSchedulerDiagnosticFloat(3),
		},
	}
	target := &Account{
		ID:            101,
		Name:          "target",
		Platform:      PlatformGemini,
		Type:          AccountTypeOAuth,
		Status:        StatusActive,
		Schedulable:   true,
		Priority:      1,
		Credentials:   map[string]any{"access_token": "secret-token"},
		AccountGroups: []AccountGroup{{GroupID: group.ID, Group: group}},
	}
	other := Account{
		ID:          102,
		Name:        "other",
		Platform:    PlatformGemini,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Priority:    2,
	}
	source := &advancedSchedulerDiagnosticSourceStub{
		account:  target,
		group:    group,
		accounts: []Account{*target, other},
		pool:     []Account{*target, other},
	}
	rateLimitService := NewRateLimitService(nil, nil, nil, nil, nil)
	diagnostics := NewAdvancedSchedulerScoreDiagnosticService(source, nil, rateLimitService)

	result, err := diagnostics.GetDetail(context.Background(), target.ID, AdvancedSchedulerScoreDiagnosticRequest{
		GroupID:         group.ID,
		StickyAccountID: target.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Detail)
	require.True(t, result.Detail.Eligible)
	require.NotNil(t, result.Detail.Score)
	require.InDelta(t, 2.5, result.Detail.Score.BaseScore, 0.000001)
	require.InDelta(t, 3, result.Detail.Score.StickyBonus, 0.000001)
	require.InDelta(t, 5.5, result.Detail.Score.FinalScore, 0.000001)
	require.Equal(t, "sticky_forced_first", result.Detail.Score.SelectionMode)
	require.NotNil(t, result.Detail.Score.SelectionWeight)
	require.InDelta(t, 6, *result.Detail.Score.SelectionWeight, 0.000001)
	require.NotNil(t, result.Detail.Score.SelectionProbability)
	require.InDelta(t, 6.0/7.0, *result.Detail.Score.SelectionProbability, 0.000001)
	require.Contains(t, result.Detail.Score.Formula, "2.0000×1.0000")

	loadMetric := findAdvancedSchedulerDiagnosticMetric(result.Detail.Metrics, "load")
	require.NotNil(t, loadMetric)
	require.True(t, loadMetric.Neutral)
	require.False(t, loadMetric.Available)
	require.InDelta(t, 0.5, loadMetric.NormalizedValue, 0.000001)

	errorMetric := findAdvancedSchedulerDiagnosticMetric(result.Detail.Metrics, "error_rate")
	require.NotNil(t, errorMetric)
	require.True(t, errorMetric.Neutral)
	require.Contains(t, errorMetric.Normalization, "中性值")

	prioritySetting := findAdvancedSchedulerDiagnosticSetting(result.Detail.EffectiveSettings, "weight_priority")
	require.NotNil(t, prioritySetting)
	require.Equal(t, "group_override", prioritySetting.Source)
	require.Equal(t, "2.0000", prioritySetting.Value)

	payload, marshalErr := json.Marshal(result)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(payload), "credentials")
	require.NotContains(t, string(payload), "secret-token")
	require.NotContains(t, strings.ToLower(string(payload)), "access_token")
}

func TestAdvancedSchedulerRuntimeFeedbackSnapshot_RecordsSamplesAndObservedTime(t *testing.T) {
	stats := newAdvancedAccountRuntimeStats()
	ttft := 240
	stats.report(401, true, &ttft)
	stats.report(401, false, nil)

	snapshot := stats.feedbackSnapshot(401)
	require.True(t, snapshot.HasFeedback)
	require.EqualValues(t, 2, snapshot.ErrorSamples)
	require.NotNil(t, snapshot.LastObservedAt)
	require.True(t, snapshot.HasTTFT)
	require.EqualValues(t, 1, snapshot.TTFTSamples)
	require.NotNil(t, snapshot.LastTTFTAt)
	require.InDelta(t, 0.2, snapshot.ErrorRate, 0.000001)
	require.InDelta(t, 240, snapshot.TTFT, 0.000001)
}

func TestAdvancedSchedulerScoreDiagnosticService_UsesProcessConfigBeforeFallback(t *testing.T) {
	group := &Group{ID: 501, Name: "advanced", Platform: PlatformGemini, SchedulerType: GroupSchedulerTypeAdvanced}
	target := &Account{
		ID:            5011,
		Name:          "target",
		Platform:      PlatformGemini,
		Status:        StatusActive,
		Schedulable:   true,
		Priority:      1,
		AccountGroups: []AccountGroup{{GroupID: group.ID, Group: group}},
	}
	other := Account{ID: 5012, Name: "other", Platform: PlatformGemini, Status: StatusActive, Schedulable: true, Priority: 2}
	source := &advancedSchedulerDiagnosticSourceStub{
		account:  target,
		group:    group,
		accounts: []Account{*target, other},
		pool:     []Account{*target, other},
	}
	rateLimitService := NewRateLimitService(nil, nil, &config.Config{
		Gateway: config.GatewayConfig{AdvancedScheduler: config.GatewayAdvancedSchedulerConfig{
			LBTopK: 1,
			ScoreWeights: config.GatewayAdvancedSchedulerScoreWeights{
				Priority: 4,
			},
		}},
	}, nil, nil)
	diagnostics := NewAdvancedSchedulerScoreDiagnosticService(source, nil, rateLimitService)

	result, err := diagnostics.GetDetail(context.Background(), target.ID, AdvancedSchedulerScoreDiagnosticRequest{GroupID: group.ID})
	require.NoError(t, err)
	require.NotNil(t, result.Detail)
	require.NotNil(t, result.Detail.Score)
	require.InDelta(t, 4, result.Detail.Score.BaseScore, 0.000001)
	require.Equal(t, 1, result.Detail.CandidatePool.TopK)
	prioritySetting := findAdvancedSchedulerDiagnosticSetting(result.Detail.EffectiveSettings, "weight_priority")
	require.NotNil(t, prioritySetting)
	require.Equal(t, "process_default", prioritySetting.Source)
}

func TestDiagnosticPolicySignalsOnlyReturnsContextualOrEnabledStrategies(t *testing.T) {
	group := &Group{ID: 601, Platform: PlatformOpenAI, SchedulerType: GroupSchedulerTypeAdvanced}
	effective := advancedSchedulerEffectiveSettings{}

	// 基准诊断不展示所有请求都会执行的固定实现细节。
	require.Empty(t, diagnosticPolicySignals(group, AdvancedSchedulerScoreDiagnosticRequest{}, effective))

	effective.stickyWeightedEnabled = true
	effective.subscriptionPriorityEnabled = true
	signals := diagnosticPolicySignals(group, AdvancedSchedulerScoreDiagnosticRequest{StickyAccountID: 99}, effective)
	require.Len(t, signals, 2)
	require.Equal(t, "session_sticky", signals[0].Key)
	require.Equal(t, "weighted", signals[0].State)
	require.Equal(t, "subscription_priority", signals[1].Key)
	require.Equal(t, "enabled", signals[1].State)
}
