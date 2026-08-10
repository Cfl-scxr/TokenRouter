package service

import (
	"container/heap"
	"context"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// advancedSchedulerNoSlotSelectionContextKey 标记只做账号选择、不会实际转发的调用。
// 可用性探测和 count_tokens 需要复用高级评分，但不能占用真实并发槽或注册会话数量。
type advancedSchedulerNoSlotSelectionContextKey struct{}

// withAdvancedSchedulerNoSlotSelection 为辅助选择入口保留完整硬过滤和评分语义，
// 同时跳过并发槽与会话数量的副作用。
func withAdvancedSchedulerNoSlotSelection(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, advancedSchedulerNoSlotSelectionContextKey{}, true)
}

func isAdvancedSchedulerNoSlotSelection(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(advancedSchedulerNoSlotSelectionContextKey{}).(bool)
	return enabled
}

// advancedAccountRuntimeStats 保存所有高级调度分组共享的运行时反馈。
// 账号没有反馈样本时，评分函数使用中性值，不会把它排除出候选集。
type advancedAccountRuntimeStats struct {
	accounts     sync.Map
	accountCount atomic.Int64
	switchCount  atomic.Int64
}

func (s *advancedAccountRuntimeStats) reportSwitch() {
	if s != nil {
		s.switchCount.Add(1)
	}
}

func (s *advancedAccountRuntimeStats) switches() int64 {
	if s == nil {
		return 0
	}
	return s.switchCount.Load()
}

type advancedAccountRuntimeStat struct {
	errorRateEWMABits atomic.Uint64
	ttftEWMABits      atomic.Uint64
}

func newAdvancedAccountRuntimeStats() *advancedAccountRuntimeStats {
	return &advancedAccountRuntimeStats{}
}

func (s *advancedAccountRuntimeStats) loadOrCreate(accountID int64) *advancedAccountRuntimeStat {
	if value, ok := s.accounts.Load(accountID); ok {
		stat, _ := value.(*advancedAccountRuntimeStat)
		if stat != nil {
			return stat
		}
	}

	stat := &advancedAccountRuntimeStat{}
	stat.ttftEWMABits.Store(math.Float64bits(math.NaN()))
	actual, loaded := s.accounts.LoadOrStore(accountID, stat)
	if !loaded {
		s.accountCount.Add(1)
		return stat
	}
	existing, _ := actual.(*advancedAccountRuntimeStat)
	if existing != nil {
		return existing
	}
	return stat
}

func updateAdvancedSchedulerEWMA(target *atomic.Uint64, sample float64, alpha float64) {
	for {
		oldBits := target.Load()
		oldValue := math.Float64frombits(oldBits)
		newValue := alpha*sample + (1-alpha)*oldValue
		if target.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
			return
		}
	}
}

func (s *advancedAccountRuntimeStats) report(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || accountID <= 0 {
		return
	}
	const alpha = 0.2
	stat := s.loadOrCreate(accountID)

	errorSample := 1.0
	if success {
		errorSample = 0.0
	}
	updateAdvancedSchedulerEWMA(&stat.errorRateEWMABits, errorSample, alpha)

	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttft := float64(*firstTokenMs)
		ttftBits := math.Float64bits(ttft)
		for {
			oldBits := stat.ttftEWMABits.Load()
			oldValue := math.Float64frombits(oldBits)
			if math.IsNaN(oldValue) {
				if stat.ttftEWMABits.CompareAndSwap(oldBits, ttftBits) {
					break
				}
				continue
			}
			newValue := alpha*ttft + (1-alpha)*oldValue
			if stat.ttftEWMABits.CompareAndSwap(oldBits, math.Float64bits(newValue)) {
				break
			}
		}
	}
}

func (s *advancedAccountRuntimeStats) snapshot(accountID int64) (errorRate float64, ttft float64, hasTTFT bool) {
	if s == nil || accountID <= 0 {
		return 0, 0, false
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return 0, 0, false
	}
	stat, _ := value.(*advancedAccountRuntimeStat)
	if stat == nil {
		return 0, 0, false
	}
	errorRate = clamp01(math.Float64frombits(stat.errorRateEWMABits.Load()))
	ttftValue := math.Float64frombits(stat.ttftEWMABits.Load())
	if math.IsNaN(ttftValue) {
		return errorRate, 0, false
	}
	return errorRate, ttftValue, true
}

// hasFeedback 判断账号是否已经产生过高级调度运行时反馈。
// 错误率的零值既可能表示全成功，也可能表示没有样本，因此不能仅凭数值判断。
func (s *advancedAccountRuntimeStats) hasFeedback(accountID int64) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	_, ok := s.accounts.Load(accountID)
	return ok
}

func (s *advancedAccountRuntimeStats) size() int {
	if s == nil {
		return 0
	}
	return int(s.accountCount.Load())
}

// advancedSchedulerCandidateScore 是完成平台硬过滤后的通用高级调度候选。
type advancedSchedulerCandidateScore struct {
	account     *Account
	loadInfo    *AccountLoadInfo
	loadKnown   bool
	score       float64
	priority    int
	errorRate   float64
	ttft        float64
	hasTTFT     bool
	hasFeedback bool
}

type advancedSchedulerCandidateHeap []advancedSchedulerCandidateScore

func (h advancedSchedulerCandidateHeap) Len() int {
	return len(h)
}

func (h advancedSchedulerCandidateHeap) Less(i, j int) bool {
	// 最小堆根节点保存最差候选，便于在线维护 Top-K。
	return isAdvancedSchedulerCandidateBetter(h[j], h[i])
}

func (h advancedSchedulerCandidateHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *advancedSchedulerCandidateHeap) Push(x any) {
	candidate, ok := x.(advancedSchedulerCandidateScore)
	if !ok {
		panic("advancedSchedulerCandidateHeap: invalid element type")
	}
	*h = append(*h, candidate)
}

func (h *advancedSchedulerCandidateHeap) Pop() any {
	old := *h
	n := len(old)
	last := old[n-1]
	*h = old[:n-1]
	return last
}

func isAdvancedSchedulerCandidateBetter(left, right advancedSchedulerCandidateScore) bool {
	if left.account == nil {
		return false
	}
	if right.account == nil {
		return true
	}
	if left.score != right.score {
		return left.score > right.score
	}
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
	}
	// 缺失负载是中性信号，不能因排序兜底而让未知负载账号额外失分。
	if left.loadKnown && right.loadKnown {
		if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
			return left.loadInfo.LoadRate < right.loadInfo.LoadRate
		}
		if left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
			return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
		}
	}
	return left.account.ID < right.account.ID
}

func selectTopKAdvancedSchedulerCandidates(candidates []advancedSchedulerCandidateScore, topK int) []advancedSchedulerCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 1
	}
	if topK >= len(candidates) {
		ranked := append([]advancedSchedulerCandidateScore(nil), candidates...)
		sortAdvancedSchedulerCandidates(ranked)
		return ranked
	}

	best := make(advancedSchedulerCandidateHeap, 0, topK)
	for _, candidate := range candidates {
		if len(best) < topK {
			heap.Push(&best, candidate)
			continue
		}
		if isAdvancedSchedulerCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			heap.Fix(&best, 0)
		}
	}

	ranked := make([]advancedSchedulerCandidateScore, len(best))
	copy(ranked, best)
	sortAdvancedSchedulerCandidates(ranked)
	return ranked
}

func sortAdvancedSchedulerCandidates(candidates []advancedSchedulerCandidateScore) {
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && isAdvancedSchedulerCandidateBetter(candidates[j], candidates[j-1]); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
}

// advancedSchedulerSelectionInput 只携带平台无关的可选调度信号。
type advancedSchedulerSelectionInput struct {
	GroupID                 *int64
	SessionHash             string
	PreviousResponseID      string
	RequestedModel          string
	StickyAccountID         int64
	StickyPreviousAccountID int64
	StickyWeighted          bool
	TopK                    int
	QuotaHeadroomFactor     func(*Account, time.Time) float64
}

func scoreAdvancedSchedulerCandidates(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
	stats *advancedAccountRuntimeStats,
	weights GatewayAdvancedSchedulerScoreWeightsView,
	input advancedSchedulerSelectionInput,
	now time.Time,
) ([]advancedSchedulerCandidateScore, float64) {
	candidates := make([]advancedSchedulerCandidateScore, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		loadInfo, loadKnown := loadMap[account.ID]
		if !loadKnown || loadInfo == nil {
			loadInfo = &AccountLoadInfo{AccountID: account.ID}
			loadKnown = false
		}
		errorRate, ttft, hasTTFT := 0.0, 0.0, false
		hasFeedback := false
		if stats != nil {
			errorRate, ttft, hasTTFT = stats.snapshot(account.ID)
			hasFeedback = stats.hasFeedback(account.ID)
		}
		candidates = append(candidates, advancedSchedulerCandidateScore{
			account:     account,
			loadInfo:    loadInfo,
			loadKnown:   loadKnown,
			priority:    account.Priority,
			errorRate:   errorRate,
			ttft:        ttft,
			hasTTFT:     hasTTFT,
			hasFeedback: hasFeedback,
		})
	}
	if len(candidates) == 0 {
		return nil, 0
	}

	minPriority, maxPriority := candidates[0].priority, candidates[0].priority
	maxWaiting := 1
	loadRateSum := 0.0
	loadRateSumSquares := 0.0
	knownLoadCount := 0
	minTTFT, maxTTFT := 0.0, 0.0
	hasTTFTSample := false
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.priority < minPriority {
			minPriority = candidate.priority
		}
		if candidate.priority > maxPriority {
			maxPriority = candidate.priority
		}
		if candidate.loadKnown && candidate.loadInfo.WaitingCount > maxWaiting {
			maxWaiting = candidate.loadInfo.WaitingCount
		}
		if candidate.hasTTFT && candidate.ttft > 0 {
			if !hasTTFTSample {
				minTTFT, maxTTFT, hasTTFTSample = candidate.ttft, candidate.ttft, true
			} else {
				if candidate.ttft < minTTFT {
					minTTFT = candidate.ttft
				}
				if candidate.ttft > maxTTFT {
					maxTTFT = candidate.ttft
				}
			}
		}
		if candidate.loadKnown {
			loadRate := float64(candidate.loadInfo.LoadRate)
			loadRateSum += loadRate
			loadRateSumSquares += loadRate * loadRate
			knownLoadCount++
		}
	}

	minResetRemaining, maxResetRemaining := 0.0, 0.0
	hasResetSample := false
	if weights.Reset > 0 {
		for _, candidate := range candidates {
			end := candidate.account.SessionWindowEnd
			if end == nil || !now.Before(*end) {
				continue
			}
			remaining := end.Sub(now).Seconds()
			if !hasResetSample {
				minResetRemaining, maxResetRemaining, hasResetSample = remaining, remaining, true
				continue
			}
			if remaining < minResetRemaining {
				minResetRemaining = remaining
			}
			if remaining > maxResetRemaining {
				maxResetRemaining = remaining
			}
		}
	}

	quotaFactor := input.QuotaHeadroomFactor
	if quotaFactor == nil {
		quotaFactor = func(*Account, time.Time) float64 { return 0.5 }
	}
	for i := range candidates {
		item := &candidates[i]
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(item.priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := 0.5
		queueFactor := 0.5
		if item.loadKnown {
			loadFactor = 1 - clamp01(float64(item.loadInfo.LoadRate)/100.0)
			queueFactor = 1 - clamp01(float64(item.loadInfo.WaitingCount)/float64(maxWaiting))
		}
		errorFactor := 0.5
		if item.hasFeedback {
			errorFactor = 1 - clamp01(item.errorRate)
		}
		ttftFactor := 0.5
		if item.hasTTFT && hasTTFTSample && maxTTFT > minTTFT {
			ttftFactor = 1 - clamp01((item.ttft-minTTFT)/(maxTTFT-minTTFT))
		}
		resetFactor := 0.5
		if weights.Reset > 0 && hasResetSample {
			if end := item.account.SessionWindowEnd; end != nil && now.Before(*end) {
				if maxResetRemaining > minResetRemaining {
					resetFactor = 1 - clamp01((end.Sub(now).Seconds()-minResetRemaining)/(maxResetRemaining-minResetRemaining))
				} else {
					resetFactor = 1
				}
			}
		}
		quotaHeadroomFactor := 0.0
		if weights.QuotaHeadroom > 0 {
			quotaHeadroomFactor = clamp01(quotaFactor(item.account, now))
		}
		item.score = weights.Priority*priorityFactor +
			weights.Load*loadFactor +
			weights.Queue*queueFactor +
			weights.ErrorRate*errorFactor +
			weights.TTFT*ttftFactor +
			weights.Reset*resetFactor +
			weights.QuotaHeadroom*quotaHeadroomFactor
		if input.StickyWeighted {
			if input.StickyPreviousAccountID > 0 && item.account.ID == input.StickyPreviousAccountID {
				item.score += weights.Previous
			}
			if input.StickyAccountID > 0 && item.account.ID == input.StickyAccountID {
				item.score += weights.SessionSticky
			}
		}
	}

	return candidates, calcLoadSkewByMoments(loadRateSum, loadRateSumSquares, knownLoadCount)
}

type advancedSchedulerRNG struct {
	state uint64
}

func newAdvancedSchedulerRNG(seed uint64) advancedSchedulerRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return advancedSchedulerRNG{state: seed}
}

func (r *advancedSchedulerRNG) nextUint64() uint64 {
	// xorshift64*
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func (r *advancedSchedulerRNG) nextFloat64() float64 {
	return float64(r.nextUint64()>>11) / (1 << 53)
}

func deriveAdvancedSchedulerSelectionSeed(input advancedSchedulerSelectionInput) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0})
	}
	writeValue(input.SessionHash)
	writeValue(input.PreviousResponseID)
	writeValue(input.RequestedModel)
	if input.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*input.GroupID, 10)))
	}
	seed := hasher.Sum64()
	if strings.TrimSpace(input.SessionHash) == "" && strings.TrimSpace(input.PreviousResponseID) == "" {
		seed ^= uint64(time.Now().UnixNano())
	}
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
	}
	return seed
}

func buildAdvancedWeightedSelectionOrder(candidates []advancedSchedulerCandidateScore, input advancedSchedulerSelectionInput) []advancedSchedulerCandidateScore {
	if len(candidates) <= 1 {
		return append([]advancedSchedulerCandidateScore(nil), candidates...)
	}

	pool := append([]advancedSchedulerCandidateScore(nil), candidates...)
	weights := make([]float64, len(pool))
	minScore := pool[0].score
	for i := 1; i < len(pool); i++ {
		if pool[i].score < minScore {
			minScore = pool[i].score
		}
	}
	for i := range pool {
		// 将 Top-K 分值平移到正区间，避免单个账号长期垄断。
		weight := (pool[i].score - minScore) + 1.0
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1.0
		}
		weights[i] = weight
	}

	order := make([]advancedSchedulerCandidateScore, 0, len(pool))
	rng := newAdvancedSchedulerRNG(deriveAdvancedSchedulerSelectionSeed(input))
	for len(pool) > 0 {
		total := 0.0
		for _, weight := range weights {
			total += weight
		}
		selectedIdx := 0
		if total > 0 {
			random := rng.nextFloat64() * total
			accumulated := 0.0
			for i, weight := range weights {
				accumulated += weight
				if random <= accumulated {
					selectedIdx = i
					break
				}
			}
		} else {
			selectedIdx = int(rng.nextUint64() % uint64(len(pool)))
		}
		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
		weights = append(weights[:selectedIdx], weights[selectedIdx+1:]...)
	}
	return order
}

func buildAdvancedSchedulerSelectionOrder(candidates []advancedSchedulerCandidateScore, input advancedSchedulerSelectionInput) []advancedSchedulerCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	topK := input.TopK
	if topK <= 0 {
		topK = 1
	}
	ranked := selectTopKAdvancedSchedulerCandidates(candidates, topK)
	if input.StickyWeighted {
		for _, stickyID := range []int64{input.StickyPreviousAccountID, input.StickyAccountID} {
			if stickyID <= 0 {
				continue
			}
			for i, candidate := range ranked {
				if candidate.account != nil && candidate.account.ID == stickyID {
					order := append([]advancedSchedulerCandidateScore{candidate}, ranked[:i]...)
					return append(order, ranked[i+1:]...)
				}
			}
		}
	}
	return buildAdvancedWeightedSelectionOrder(ranked, input)
}

// AdvancedAccountSchedulerScoreSnapshot 是管理端和高级调度器共用的评分展示结构。
type AdvancedAccountSchedulerScoreSnapshot struct {
	BaseScore             float64
	StickyScore           float64
	StickyScoreInfinity   bool
	StickyWeightedEnabled bool
}

func buildAdvancedAccountSchedulerScoreSnapshot(
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
	stats *advancedAccountRuntimeStats,
	weights GatewayAdvancedSchedulerScoreWeightsView,
	stickyWeightedEnabled bool,
	quotaHeadroomFactor func(*Account, time.Time) float64,
) map[int64]AdvancedAccountSchedulerScoreSnapshot {
	candidates, _ := scoreAdvancedSchedulerCandidates(accounts, loadMap, stats, weights, advancedSchedulerSelectionInput{
		QuotaHeadroomFactor: quotaHeadroomFactor,
	}, time.Now())
	if len(candidates) == 0 {
		return nil
	}
	result := make(map[int64]AdvancedAccountSchedulerScoreSnapshot, len(candidates))
	for _, candidate := range candidates {
		score := AdvancedAccountSchedulerScoreSnapshot{
			BaseScore:             candidate.score,
			StickyWeightedEnabled: stickyWeightedEnabled,
			StickyScoreInfinity:   !stickyWeightedEnabled,
		}
		if stickyWeightedEnabled {
			score.StickyScore = candidate.score + weights.Previous + weights.SessionSticky
		}
		result[candidate.account.ID] = score
	}
	return result
}
