package service

import (
	"context"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func advancedSchedulerTestWeights() GatewayAdvancedSchedulerScoreWeightsView {
	return GatewayAdvancedSchedulerScoreWeightsView{
		Priority:  1,
		Load:      1,
		Queue:     1,
		ErrorRate: 1,
		TTFT:      1,
	}
}

func TestAdvancedSchedulerCoreUsesRuntimeFeedbackAndNeutralOptionalSignals(t *testing.T) {
	accounts := []*Account{
		{ID: 11, Priority: 1, Platform: PlatformGemini},
		{ID: 12, Priority: 1, Platform: PlatformGemini},
	}
	loadMap := map[int64]*AccountLoadInfo{
		11: {AccountID: 11, LoadRate: 20, WaitingCount: 0},
		12: {AccountID: 12, LoadRate: 20, WaitingCount: 0},
	}
	stats := newAdvancedAccountRuntimeStats()
	for range 8 {
		stats.report(11, false, nil)
		stats.report(12, true, nil)
	}

	candidates, _ := scoreAdvancedSchedulerCandidates(
		accounts,
		loadMap,
		stats,
		advancedSchedulerTestWeights(),
		advancedSchedulerSelectionInput{},
		time.Now(),
	)
	require.Len(t, candidates, 2)
	require.Greater(t, candidates[1].score, candidates[0].score)
	require.False(t, candidates[0].hasTTFT)
	require.False(t, candidates[1].hasTTFT)
}

func TestAdvancedSchedulerCoreTreatsMissingSignalsAsNeutral(t *testing.T) {
	accounts := []*Account{
		{ID: 21, Priority: 1, Platform: PlatformGemini},
		{ID: 22, Priority: 1, Platform: PlatformGemini},
	}
	weights := GatewayAdvancedSchedulerScoreWeightsView{
		Load:      1,
		ErrorRate: 1,
		TTFT:      1,
		Reset:     1,
	}

	// 账号 21 缺失负载，账号 22 的已知负载恰好处于中性位置。两者均没有
	// 运行时反馈、TTFT 或窗口信息，因此分数必须一致，不能把缺失值当作劣化。
	candidates, skew := scoreAdvancedSchedulerCandidates(
		accounts,
		map[int64]*AccountLoadInfo{22: {AccountID: 22, LoadRate: 50}},
		nil,
		weights,
		advancedSchedulerSelectionInput{},
		time.Now(),
	)

	require.Len(t, candidates, 2)
	require.InDelta(t, candidates[0].score, candidates[1].score, 0.000001)
	require.Equal(t, 0.0, skew, "只有一个已知负载样本时不能计算出偏斜")
	require.False(t, candidates[0].loadKnown)
	require.True(t, candidates[1].loadKnown)
}

func TestAdvancedSchedulerCoreSelectsNonOpenAIGroupAndMarksResult(t *testing.T) {
	groupID := int64(42)
	group := &Group{ID: groupID, Platform: PlatformGemini, SchedulerType: GroupSchedulerTypeAdvanced}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	service := &GatewayService{}

	selection, selected, err := service.tryAcquireByAdvancedScheduler(ctx, &groupID, "session", []accountWithLoad{
		{
			account:  &Account{ID: 101, Platform: PlatformGemini, Priority: 1, Schedulable: true, Status: StatusActive},
			loadInfo: &AccountLoadInfo{AccountID: 101, LoadRate: 0},
		},
	})

	require.NoError(t, err)
	require.True(t, selected)
	require.NotNil(t, selection)
	require.Equal(t, int64(101), selection.Account.ID)
	require.True(t, selection.AdvancedScheduler)

	basicCtx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 43, Platform: PlatformGemini, SchedulerType: GroupSchedulerTypeBasic})
	basicSelection, err := service.newSelectionResult(basicCtx, &Account{ID: 102}, true, func() {}, nil)
	require.NoError(t, err)
	require.False(t, basicSelection.AdvancedScheduler)
}

func TestAdvancedSchedulerCoreKeepsWeightedStickyInsideTopK(t *testing.T) {
	candidates := []advancedSchedulerCandidateScore{
		{account: &Account{ID: 1, Priority: 1}, loadInfo: &AccountLoadInfo{}, score: 10},
		{account: &Account{ID: 2, Priority: 1}, loadInfo: &AccountLoadInfo{}, score: 9},
		{account: &Account{ID: 3, Priority: 1}, loadInfo: &AccountLoadInfo{}, score: 1},
	}

	order := buildAdvancedSchedulerSelectionOrder(candidates, advancedSchedulerSelectionInput{
		StickyWeighted:  true,
		StickyAccountID: 2,
		TopK:            2,
	})

	require.Len(t, order, 2)
	require.Equal(t, int64(2), order[0].account.ID)
}
