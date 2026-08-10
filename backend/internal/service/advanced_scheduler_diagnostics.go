package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AdvancedSchedulerScoreCalculationVersion 标识诊断公式的兼容版本。
const AdvancedSchedulerScoreCalculationVersion = "v1"

// AdvancedSchedulerScoreDiagnosticRequest 描述管理员希望模拟的安全请求上下文。
// 不接受 session hash、响应正文或任何凭据相关字段。
type AdvancedSchedulerScoreDiagnosticRequest struct {
	GroupID                   int64  `json:"group_id"`
	RequestedModel            string `json:"requested_model,omitempty"`
	StickyAccountID           int64  `json:"sticky_account_id,omitempty"`
	PreviousResponseAccountID int64  `json:"previous_response_account_id,omitempty"`
}

// AdvancedSchedulerScoreDiagnosticAccount 是诊断接口返回的安全账号摘要。
type AdvancedSchedulerScoreDiagnosticAccount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Type     string `json:"type"`
	Status   string `json:"status"`
}

// AdvancedSchedulerScoreDiagnosticGroup 是高级调度分组的安全摘要。
type AdvancedSchedulerScoreDiagnosticGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// AdvancedSchedulerScoreDiagnosticGroupSummary 用于首次打开弹窗时的轻量分组 Tab 信息。
type AdvancedSchedulerScoreDiagnosticGroupSummary struct {
	AdvancedSchedulerScoreDiagnosticGroup
	Eligible   bool     `json:"eligible"`
	FinalScore *float64 `json:"final_score,omitempty"`
	Status     string   `json:"status"`
}

// AdvancedSchedulerScoreDiagnosticResponse 是管理员评分诊断接口的统一响应。
type AdvancedSchedulerScoreDiagnosticResponse struct {
	Account            AdvancedSchedulerScoreDiagnosticAccount        `json:"account"`
	GeneratedAt        time.Time                                      `json:"generated_at"`
	CalculationVersion string                                         `json:"calculation_version"`
	Groups             []AdvancedSchedulerScoreDiagnosticGroupSummary `json:"groups"`
	Detail             *AdvancedSchedulerScoreDiagnosticDetail        `json:"detail,omitempty"`
}

// AdvancedSchedulerScoreDiagnosticContext 表示本次评分使用的非敏感场景上下文。
type AdvancedSchedulerScoreDiagnosticContext struct {
	RequestedModel            string `json:"requested_model,omitempty"`
	StickyAccountID           int64  `json:"sticky_account_id,omitempty"`
	PreviousResponseAccountID int64  `json:"previous_response_account_id,omitempty"`
	Baseline                  bool   `json:"baseline"`
}

// AdvancedSchedulerScoreDiagnosticDetail 是单个分组的完整评分解释。
type AdvancedSchedulerScoreDiagnosticDetail struct {
	Group             AdvancedSchedulerScoreDiagnosticGroup          `json:"group"`
	Context           AdvancedSchedulerScoreDiagnosticContext        `json:"context"`
	Eligible          bool                                           `json:"eligible"`
	HardFilterReasons []string                                       `json:"hard_filter_reasons,omitempty"`
	CandidatePool     AdvancedSchedulerScoreDiagnosticCandidatePool  `json:"candidate_pool"`
	Score             *AdvancedSchedulerScoreDiagnosticScore         `json:"score,omitempty"`
	Metrics           []AdvancedSchedulerScoreDiagnosticMetric       `json:"metrics"`
	EffectiveSettings []AdvancedSchedulerScoreDiagnosticSetting      `json:"effective_settings"`
	PolicySignals     []AdvancedSchedulerScoreDiagnosticPolicySignal `json:"policy_signals"`
}

// AdvancedSchedulerScoreDiagnosticCandidatePool 汇总硬过滤后的候选池及 Top-K 选择数据。
type AdvancedSchedulerScoreDiagnosticCandidatePool struct {
	TotalCandidates     int                                         `json:"total_candidates"`
	EligibleCandidates  int                                         `json:"eligible_candidates"`
	ExcludedCandidates  int                                         `json:"excluded_candidates"`
	ExclusionReasons    map[string]int                              `json:"exclusion_reasons"`
	TopK                int                                         `json:"top_k"`
	TopKMinimumScore    *float64                                    `json:"top_k_minimum_score,omitempty"`
	TopKWeightSum       *float64                                    `json:"top_k_weight_sum,omitempty"`
	NormalizationRanges AdvancedSchedulerScoreDiagnosticRanges      `json:"normalization_ranges"`
	Candidates          []AdvancedSchedulerScoreDiagnosticCandidate `json:"candidates"`
}

// AdvancedSchedulerScoreDiagnosticRanges 记录评分核心实际使用的候选池归一化范围。
type AdvancedSchedulerScoreDiagnosticRanges struct {
	PriorityMin     int      `json:"priority_min"`
	PriorityMax     int      `json:"priority_max"`
	MaxWaitingCount int      `json:"max_waiting_count"`
	TTFTMinMs       *float64 `json:"ttft_min_ms,omitempty"`
	TTFTMaxMs       *float64 `json:"ttft_max_ms,omitempty"`
	ResetMinSeconds *float64 `json:"reset_min_seconds,omitempty"`
	ResetMaxSeconds *float64 `json:"reset_max_seconds,omitempty"`
}

// AdvancedSchedulerScoreDiagnosticCandidate 是安全的候选账号摘要，供排名展示和场景选择使用。
type AdvancedSchedulerScoreDiagnosticCandidate struct {
	ID                   int64    `json:"id"`
	Name                 string   `json:"name"`
	Platform             string   `json:"platform"`
	Priority             int      `json:"priority"`
	FinalScore           float64  `json:"final_score"`
	Rank                 int      `json:"rank"`
	InTopK               bool     `json:"in_top_k"`
	SelectionWeight      *float64 `json:"selection_weight,omitempty"`
	SelectionProbability *float64 `json:"selection_probability,omitempty"`
}

// AdvancedSchedulerScoreDiagnosticScore 是目标账号的最终分数与加权选择解释。
type AdvancedSchedulerScoreDiagnosticScore struct {
	BaseScore            float64  `json:"base_score"`
	StickyBonus          float64  `json:"sticky_bonus"`
	FinalScore           float64  `json:"final_score"`
	Rank                 int      `json:"rank"`
	InTopK               bool     `json:"in_top_k"`
	SelectionWeight      *float64 `json:"selection_weight,omitempty"`
	SelectionProbability *float64 `json:"selection_probability,omitempty"`
	SelectionMode        string   `json:"selection_mode"`
	Formula              string   `json:"formula"`
}

// AdvancedSchedulerScoreDiagnosticMetric 逐项描述归一化、权重及贡献。
type AdvancedSchedulerScoreDiagnosticMetric struct {
	Key                  string     `json:"key"`
	RawValue             string     `json:"raw_value"`
	Normalization        string     `json:"normalization"`
	NormalizedValue      float64    `json:"normalized_value"`
	Weight               float64    `json:"weight"`
	WeightedContribution float64    `json:"weighted_contribution"`
	Available            bool       `json:"available"`
	Neutral              bool       `json:"neutral"`
	Source               string     `json:"source"`
	ObservedAt           *time.Time `json:"observed_at,omitempty"`
}

// AdvancedSchedulerScoreDiagnosticSetting 表示有效配置值及其优先级来源。
type AdvancedSchedulerScoreDiagnosticSetting struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// AdvancedSchedulerScoreDiagnosticPolicySignal 展示未混入普通评分项的策略与硬约束。
type AdvancedSchedulerScoreDiagnosticPolicySignal struct {
	Key    string `json:"key"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// AdvancedSchedulerScoreDiagnosticSource 是诊断服务所需的最小账号读取能力。
// 通过窄接口保持服务可独立测试，也避免诊断路径触及凭据读取或写入接口。
type AdvancedSchedulerScoreDiagnosticSource interface {
	GetAccount(ctx context.Context, id int64) (*Account, error)
	GetGroup(ctx context.Context, id int64) (*Group, error)
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]Account, int64, error)
	ListSchedulableAccountsForAdvancedSchedulerScore(ctx context.Context, groupID *int64, platform string) ([]Account, error)
}

// AdvancedSchedulerScoreDiagnosticService 编排只读评分诊断。
// 它复用高级评分核心，但绝不获取并发槽、绑定会话或回写运行时状态。
type AdvancedSchedulerScoreDiagnosticService struct {
	source             AdvancedSchedulerScoreDiagnosticSource
	concurrencyService *ConcurrencyService
	rateLimitService   *RateLimitService
}

// NewAdvancedSchedulerScoreDiagnosticService 创建高级评分诊断服务。
func NewAdvancedSchedulerScoreDiagnosticService(
	source AdvancedSchedulerScoreDiagnosticSource,
	concurrencyService *ConcurrencyService,
	rateLimitService *RateLimitService,
) *AdvancedSchedulerScoreDiagnosticService {
	return &AdvancedSchedulerScoreDiagnosticService{
		source:             source,
		concurrencyService: concurrencyService,
		rateLimitService:   rateLimitService,
	}
}

// GetOverview 返回账号所属高级分组的轻量摘要。
func (s *AdvancedSchedulerScoreDiagnosticService) GetOverview(ctx context.Context, accountID int64) (*AdvancedSchedulerScoreDiagnosticResponse, error) {
	account, groups, err := s.loadAccountAndAdvancedGroups(ctx, accountID)
	if err != nil {
		return nil, err
	}
	response := s.newResponse(account)
	for _, group := range groups {
		summary, err := s.buildGroupSummary(ctx, account, group)
		if err != nil {
			return nil, err
		}
		response.Groups = append(response.Groups, summary)
	}
	return response, nil
}

// GetDetail 返回指定高级分组在给定安全场景下的完整解释。
func (s *AdvancedSchedulerScoreDiagnosticService) GetDetail(ctx context.Context, accountID int64, request AdvancedSchedulerScoreDiagnosticRequest) (*AdvancedSchedulerScoreDiagnosticResponse, error) {
	account, groups, err := s.loadAccountAndAdvancedGroups(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if request.GroupID <= 0 {
		return nil, fmt.Errorf("group_id is required")
	}
	request.RequestedModel = strings.TrimSpace(request.RequestedModel)
	if len(request.RequestedModel) > 512 {
		return nil, fmt.Errorf("requested_model is too long")
	}
	if request.StickyAccountID < 0 || request.PreviousResponseAccountID < 0 {
		return nil, fmt.Errorf("sticky account id must not be negative")
	}

	var selected *Group
	for _, group := range groups {
		if group.ID == request.GroupID {
			selected = group
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("account is not in an advanced scheduler group")
	}

	response := s.newResponse(account)
	for _, group := range groups {
		if group.ID == selected.ID {
			detail, buildErr := s.buildDetail(ctx, account, selected, request)
			if buildErr != nil {
				return nil, buildErr
			}
			response.Detail = detail
			response.Groups = append(response.Groups, summaryFromDiagnosticDetail(detail))
			continue
		}
		summary, buildErr := s.buildGroupSummary(ctx, account, group)
		if buildErr != nil {
			return nil, buildErr
		}
		response.Groups = append(response.Groups, summary)
	}
	return response, nil
}

func (s *AdvancedSchedulerScoreDiagnosticService) loadAccountAndAdvancedGroups(ctx context.Context, accountID int64) (*Account, []*Group, error) {
	if s == nil || s.source == nil {
		return nil, nil, fmt.Errorf("advanced scheduler diagnostics is unavailable")
	}
	account, err := s.source.GetAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	if account == nil {
		return nil, nil, fmt.Errorf("account not found")
	}
	groupsByID := make(map[int64]*Group)
	for _, accountGroup := range account.AccountGroups {
		if accountGroup.Group != nil && accountGroup.Group.UsesAdvancedScheduler() {
			groupsByID[accountGroup.Group.ID] = accountGroup.Group
		}
	}
	for _, group := range account.Groups {
		if group != nil && group.UsesAdvancedScheduler() {
			groupsByID[group.ID] = group
		}
	}
	for _, groupID := range account.GroupIDs {
		if groupID <= 0 {
			continue
		}
		if _, exists := groupsByID[groupID]; exists {
			continue
		}
		group, groupErr := s.source.GetGroup(ctx, groupID)
		if groupErr != nil {
			return nil, nil, groupErr
		}
		if group != nil && group.UsesAdvancedScheduler() {
			groupsByID[group.ID] = group
		}
	}

	groups := make([]*Group, 0, len(groupsByID))
	for _, group := range groupsByID {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].SortOrder != groups[j].SortOrder {
			return groups[i].SortOrder < groups[j].SortOrder
		}
		return groups[i].ID < groups[j].ID
	})
	return account, groups, nil
}

func (s *AdvancedSchedulerScoreDiagnosticService) newResponse(account *Account) *AdvancedSchedulerScoreDiagnosticResponse {
	return &AdvancedSchedulerScoreDiagnosticResponse{
		Account:            diagnosticAccountSummary(account),
		GeneratedAt:        time.Now().UTC(),
		CalculationVersion: AdvancedSchedulerScoreCalculationVersion,
		Groups:             make([]AdvancedSchedulerScoreDiagnosticGroupSummary, 0),
	}
}

func diagnosticAccountSummary(account *Account) AdvancedSchedulerScoreDiagnosticAccount {
	if account == nil {
		return AdvancedSchedulerScoreDiagnosticAccount{}
	}
	return AdvancedSchedulerScoreDiagnosticAccount{
		ID:       account.ID,
		Name:     account.Name,
		Platform: account.Platform,
		Type:     account.Type,
		Status:   account.Status,
	}
}

func diagnosticGroupSummary(group *Group) AdvancedSchedulerScoreDiagnosticGroup {
	if group == nil {
		return AdvancedSchedulerScoreDiagnosticGroup{}
	}
	return AdvancedSchedulerScoreDiagnosticGroup{ID: group.ID, Name: group.Name, Platform: group.Platform}
}

func summaryFromDiagnosticDetail(detail *AdvancedSchedulerScoreDiagnosticDetail) AdvancedSchedulerScoreDiagnosticGroupSummary {
	summary := AdvancedSchedulerScoreDiagnosticGroupSummary{
		AdvancedSchedulerScoreDiagnosticGroup: detail.Group,
		Eligible:                              detail.Eligible,
		Status:                                "eligible",
	}
	if !detail.Eligible {
		summary.Status = "filtered"
		return summary
	}
	if detail.Score != nil {
		score := detail.Score.FinalScore
		summary.FinalScore = &score
	}
	return summary
}

// buildGroupSummary 只计算分组 Tab 所需的资格和最终分数。
// 它刻意不读取完整分组账号清单、不组装指标或策略解释，避免首次打开诊断弹窗时放大读取负载。
func (s *AdvancedSchedulerScoreDiagnosticService) buildGroupSummary(
	ctx context.Context,
	target *Account,
	group *Group,
) (AdvancedSchedulerScoreDiagnosticGroupSummary, error) {
	summary := AdvancedSchedulerScoreDiagnosticGroupSummary{
		AdvancedSchedulerScoreDiagnosticGroup: diagnosticGroupSummary(group),
		Status:                                "eligible",
	}
	if target == nil || group == nil {
		summary.Status = "filtered"
		return summary, nil
	}
	now := time.Now()
	effective, _ := s.effectiveSettings(ctx, group)
	poolAccounts, err := s.source.ListSchedulableAccountsForAdvancedSchedulerScore(ctx, &group.ID, group.Platform)
	if err != nil {
		return summary, err
	}
	filtered := make([]*Account, 0, len(poolAccounts))
	for index := range poolAccounts {
		candidate := poolAccounts[index]
		if diagnosticHardFilterReason(&candidate, group, AdvancedSchedulerScoreDiagnosticRequest{GroupID: group.ID}, now) == "" {
			filtered = append(filtered, &candidate)
		}
	}
	loadMap := s.loadMap(ctx, filtered)
	var stats *advancedAccountRuntimeStats
	if s.rateLimitService != nil {
		stats = s.rateLimitService.AdvancedSchedulerRuntimeStats()
	}
	candidates, _, _ := scoreAdvancedSchedulerCandidatesWithRanges(
		filtered,
		loadMap,
		stats,
		effective.weights,
		advancedSchedulerSelectionInput{
			GroupID:             &group.ID,
			StickyWeighted:      effective.stickyWeightedEnabled,
			TopK:                effective.topK,
			QuotaHeadroomFactor: openAIQuotaHeadroomFactor,
		},
		now,
	)
	targetCandidate, _ := findDiagnosticCandidate(candidates, target.ID)
	if targetCandidate == nil {
		summary.Status = "filtered"
		return summary, nil
	}
	summary.Eligible = true
	finalScore := targetCandidate.score
	summary.FinalScore = &finalScore
	return summary, nil
}

func (s *AdvancedSchedulerScoreDiagnosticService) buildDetail(
	ctx context.Context,
	target *Account,
	group *Group,
	request AdvancedSchedulerScoreDiagnosticRequest,
) (*AdvancedSchedulerScoreDiagnosticDetail, error) {
	now := time.Now()
	effective, runtime := s.effectiveSettings(ctx, group)
	allAccounts, _, err := s.source.ListAccounts(ctx, 1, 10000, "", "", "", "", group.ID, "", "", "")
	if err != nil {
		return nil, err
	}
	poolAccounts, err := s.source.ListSchedulableAccountsForAdvancedSchedulerScore(ctx, &group.ID, group.Platform)
	if err != nil {
		return nil, err
	}

	exclusions := make(map[string]int)
	filtered := make([]*Account, 0, len(poolAccounts))
	seenPoolIDs := make(map[int64]struct{}, len(poolAccounts))
	for i := range poolAccounts {
		candidate := poolAccounts[i]
		seenPoolIDs[candidate.ID] = struct{}{}
		if reason := diagnosticHardFilterReason(&candidate, group, request, now); reason != "" {
			exclusions[reason]++
			continue
		}
		filtered = append(filtered, &candidate)
	}
	for i := range allAccounts {
		account := &allAccounts[i]
		if _, found := seenPoolIDs[account.ID]; found {
			continue
		}
		reason := diagnosticHardFilterReason(account, group, request, now)
		if reason == "" {
			reason = "not_in_schedulable_pool"
		}
		exclusions[reason]++
	}

	loadMap := s.loadMap(ctx, filtered)
	stats := (*advancedAccountRuntimeStats)(nil)
	if s.rateLimitService != nil {
		stats = s.rateLimitService.AdvancedSchedulerRuntimeStats()
	}
	input := advancedSchedulerSelectionInput{
		GroupID:                 &group.ID,
		RequestedModel:          request.RequestedModel,
		StickyAccountID:         request.StickyAccountID,
		StickyPreviousAccountID: request.PreviousResponseAccountID,
		StickyWeighted:          effective.stickyWeightedEnabled,
		TopK:                    effective.topK,
		QuotaHeadroomFactor:     openAIQuotaHeadroomFactor,
	}
	candidates, _, ranges := scoreAdvancedSchedulerCandidatesWithRanges(filtered, loadMap, stats, effective.weights, input, now)
	sortAdvancedSchedulerCandidates(candidates)
	topKCandidates := selectTopKAdvancedSchedulerCandidates(candidates, effective.topK)
	selection := diagnosticSelectionStats(topKCandidates, input)

	detail := &AdvancedSchedulerScoreDiagnosticDetail{
		Group:             diagnosticGroupSummary(group),
		Context:           diagnosticContext(request),
		CandidatePool:     diagnosticCandidatePool(candidates, topKCandidates, ranges, exclusions, selection),
		EffectiveSettings: diagnosticEffectiveSettings(group, runtime, effective),
		PolicySignals:     diagnosticPolicySignals(group, request, effective),
		Metrics:           make([]AdvancedSchedulerScoreDiagnosticMetric, 0),
	}
	targetReason := diagnosticHardFilterReason(target, group, request, now)
	if targetReason != "" {
		detail.HardFilterReasons = append(detail.HardFilterReasons, targetReason)
	}
	targetCandidate, targetRank := findDiagnosticCandidate(candidates, target.ID)
	if targetCandidate == nil {
		if targetReason == "" {
			detail.HardFilterReasons = append(detail.HardFilterReasons, "not_in_schedulable_pool")
		}
		detail.Eligible = false
		return detail, nil
	}
	detail.Eligible = true
	detail.Score = diagnosticScore(targetCandidate, targetRank, topKCandidates, selection, effective, request)
	detail.Metrics = diagnosticMetrics(targetCandidate, ranges, effective.weights, now)
	return detail, nil
}

func (s *AdvancedSchedulerScoreDiagnosticService) effectiveSettings(ctx context.Context, group *Group) (advancedSchedulerEffectiveSettings, advancedSchedulerRuntimeSettings) {
	gateway := &OpenAIGatewayService{rateLimitService: s.rateLimitService}
	if s != nil && s.rateLimitService != nil {
		gateway.cfg = s.rateLimitService.cfg
	}
	runtime := gateway.advancedSchedulerRuntimeSettings(ctx)
	return gateway.advancedSchedulerEffectiveSettingsForGroup(ctx, group), runtime
}

func (s *AdvancedSchedulerScoreDiagnosticService) loadMap(ctx context.Context, accounts []*Account) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil || len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}
	}
	loads := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			loads = append(loads, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.Concurrency})
		}
	}
	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, loads)
	if err != nil || loadMap == nil {
		return map[int64]*AccountLoadInfo{}
	}
	return loadMap
}

func diagnosticContext(request AdvancedSchedulerScoreDiagnosticRequest) AdvancedSchedulerScoreDiagnosticContext {
	return AdvancedSchedulerScoreDiagnosticContext{
		RequestedModel:            strings.TrimSpace(request.RequestedModel),
		StickyAccountID:           request.StickyAccountID,
		PreviousResponseAccountID: request.PreviousResponseAccountID,
		Baseline: strings.TrimSpace(request.RequestedModel) == "" &&
			request.StickyAccountID == 0 && request.PreviousResponseAccountID == 0,
	}
}

func diagnosticHardFilterReason(account *Account, group *Group, request AdvancedSchedulerScoreDiagnosticRequest, now time.Time) string {
	if account == nil {
		return "account_missing"
	}
	if !diagnosticPlatformMatchesGroup(account, group) {
		return "platform_mismatch"
	}
	if account.Status != StatusActive {
		return "account_inactive"
	}
	if !account.Schedulable {
		return "account_disabled"
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt) {
		return "account_expired"
	}
	if account.OverloadUntil != nil && now.Before(*account.OverloadUntil) {
		return "account_overloaded"
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		return "account_rate_limited"
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		return "account_temporarily_unschedulable"
	}
	if group != nil && group.RequirePrivacySet && !account.IsPrivacySet() {
		return "privacy_not_set"
	}
	if model := strings.TrimSpace(request.RequestedModel); model != "" && !account.IsModelSupported(model) {
		return "model_unsupported"
	}
	return ""
}

func diagnosticPlatformMatchesGroup(account *Account, group *Group) bool {
	if account == nil || group == nil {
		return false
	}
	if account.Platform == group.Platform {
		return true
	}
	return (group.Platform == PlatformAnthropic || group.Platform == PlatformGemini) &&
		account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()
}

type diagnosticTopKSelection struct {
	minimumScore    float64
	weightSum       float64
	weights         map[int64]float64
	probabilities   map[int64]float64
	forcedAccountID int64
}

func diagnosticSelectionStats(topK []advancedSchedulerCandidateScore, input advancedSchedulerSelectionInput) diagnosticTopKSelection {
	selection := diagnosticTopKSelection{
		weights:       make(map[int64]float64, len(topK)),
		probabilities: make(map[int64]float64, len(topK)),
	}
	if len(topK) == 0 {
		return selection
	}
	selection.minimumScore = topK[0].score
	for _, candidate := range topK[1:] {
		if candidate.score < selection.minimumScore {
			selection.minimumScore = candidate.score
		}
	}
	for _, candidate := range topK {
		if candidate.account == nil {
			continue
		}
		weight := candidate.score - selection.minimumScore + 1
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1
		}
		selection.weights[candidate.account.ID] = weight
		selection.weightSum += weight
	}
	if selection.weightSum > 0 {
		for accountID, weight := range selection.weights {
			selection.probabilities[accountID] = weight / selection.weightSum
		}
	}
	if input.StickyWeighted {
		for _, accountID := range []int64{input.StickyPreviousAccountID, input.StickyAccountID} {
			if _, found := selection.weights[accountID]; found && accountID > 0 {
				selection.forcedAccountID = accountID
				break
			}
		}
	}
	return selection
}

func diagnosticCandidatePool(
	candidates []advancedSchedulerCandidateScore,
	topK []advancedSchedulerCandidateScore,
	ranges advancedSchedulerScoreRanges,
	exclusions map[string]int,
	selection diagnosticTopKSelection,
) AdvancedSchedulerScoreDiagnosticCandidatePool {
	pool := AdvancedSchedulerScoreDiagnosticCandidatePool{
		EligibleCandidates:  len(candidates),
		ExcludedCandidates:  0,
		ExclusionReasons:    exclusions,
		TopK:                len(topK),
		NormalizationRanges: diagnosticRanges(ranges),
		Candidates:          make([]AdvancedSchedulerScoreDiagnosticCandidate, 0, len(candidates)),
	}
	for _, count := range exclusions {
		pool.ExcludedCandidates += count
	}
	pool.TotalCandidates = pool.EligibleCandidates + pool.ExcludedCandidates
	if len(topK) > 0 {
		minimum := selection.minimumScore
		weightSum := selection.weightSum
		pool.TopKMinimumScore = &minimum
		pool.TopKWeightSum = &weightSum
	}
	topKIDs := make(map[int64]struct{}, len(topK))
	for _, candidate := range topK {
		if candidate.account != nil {
			topKIDs[candidate.account.ID] = struct{}{}
		}
	}
	for index, candidate := range candidates {
		if candidate.account == nil {
			continue
		}
		_, inTopK := topKIDs[candidate.account.ID]
		item := AdvancedSchedulerScoreDiagnosticCandidate{
			ID:         candidate.account.ID,
			Name:       candidate.account.Name,
			Platform:   candidate.account.Platform,
			Priority:   candidate.account.Priority,
			FinalScore: candidate.score,
			Rank:       index + 1,
			InTopK:     inTopK,
		}
		if inTopK {
			weight := selection.weights[candidate.account.ID]
			probability := selection.probabilities[candidate.account.ID]
			item.SelectionWeight = &weight
			item.SelectionProbability = &probability
		}
		pool.Candidates = append(pool.Candidates, item)
	}
	return pool
}

func diagnosticRanges(ranges advancedSchedulerScoreRanges) AdvancedSchedulerScoreDiagnosticRanges {
	result := AdvancedSchedulerScoreDiagnosticRanges{
		PriorityMin:     ranges.MinPriority,
		PriorityMax:     ranges.MaxPriority,
		MaxWaitingCount: ranges.MaxWaiting,
	}
	if ranges.HasTTFTSample {
		minValue, maxValue := ranges.MinTTFT, ranges.MaxTTFT
		result.TTFTMinMs = &minValue
		result.TTFTMaxMs = &maxValue
	}
	if ranges.HasResetSample {
		minValue, maxValue := ranges.MinResetRemaining, ranges.MaxResetRemaining
		result.ResetMinSeconds = &minValue
		result.ResetMaxSeconds = &maxValue
	}
	return result
}

func findDiagnosticCandidate(candidates []advancedSchedulerCandidateScore, accountID int64) (*advancedSchedulerCandidateScore, int) {
	for index := range candidates {
		if candidates[index].account != nil && candidates[index].account.ID == accountID {
			return &candidates[index], index + 1
		}
	}
	return nil, 0
}

func diagnosticScore(
	candidate *advancedSchedulerCandidateScore,
	rank int,
	topK []advancedSchedulerCandidateScore,
	selection diagnosticTopKSelection,
	effective advancedSchedulerEffectiveSettings,
	request AdvancedSchedulerScoreDiagnosticRequest,
) *AdvancedSchedulerScoreDiagnosticScore {
	if candidate == nil {
		return nil
	}
	score := &AdvancedSchedulerScoreDiagnosticScore{
		BaseScore:     candidate.baseScore,
		StickyBonus:   candidate.stickyBonus,
		FinalScore:    candidate.score,
		Rank:          rank,
		Formula:       diagnosticFormula(candidate, effective.weights),
		SelectionMode: "top_k_weighted",
	}
	for _, item := range topK {
		if item.account == nil || item.account.ID != candidate.account.ID {
			continue
		}
		score.InTopK = true
		weight := selection.weights[candidate.account.ID]
		probability := selection.probabilities[candidate.account.ID]
		if selection.forcedAccountID > 0 {
			score.SelectionMode = "sticky_forced_first"
		}
		score.SelectionWeight = &weight
		score.SelectionProbability = &probability
		break
	}
	if !effective.stickyWeightedEnabled || (request.StickyAccountID == 0 && request.PreviousResponseAccountID == 0) {
		score.SelectionMode = "top_k_weighted"
	}
	return score
}

func diagnosticFormula(candidate *advancedSchedulerCandidateScore, weights GatewayAdvancedSchedulerScoreWeightsView) string {
	if candidate == nil {
		return ""
	}
	terms := []string{
		diagnosticFormulaTerm(weights.Priority, candidate.factors.Priority),
		diagnosticFormulaTerm(weights.Load, candidate.factors.Load),
		diagnosticFormulaTerm(weights.Queue, candidate.factors.Queue),
		diagnosticFormulaTerm(weights.ErrorRate, candidate.factors.ErrorRate),
		diagnosticFormulaTerm(weights.TTFT, candidate.factors.TTFT),
		diagnosticFormulaTerm(weights.Reset, candidate.factors.Reset),
		diagnosticFormulaTerm(weights.QuotaHeadroom, candidate.factors.QuotaHeadroom),
	}
	base := strings.Join(terms, " + ") + " = " + diagnosticFloat(candidate.baseScore)
	if candidate.stickyBonus == 0 {
		return base
	}
	bonus := make([]string, 0, 2)
	if candidate.previousBonus != 0 {
		bonus = append(bonus, diagnosticFloat(candidate.previousBonus))
	}
	if candidate.sessionStickyBonus != 0 {
		bonus = append(bonus, diagnosticFloat(candidate.sessionStickyBonus))
	}
	return base + "；粘性加成 " + strings.Join(bonus, " + ") + "；最终 = " + diagnosticFloat(candidate.score)
}

func diagnosticFormulaTerm(weight, factor float64) string {
	return diagnosticFloat(weight) + "×" + diagnosticFloat(factor)
}

func diagnosticFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 4, 64)
}

func diagnosticMetrics(
	candidate *advancedSchedulerCandidateScore,
	ranges advancedSchedulerScoreRanges,
	weights GatewayAdvancedSchedulerScoreWeightsView,
	now time.Time,
) []AdvancedSchedulerScoreDiagnosticMetric {
	if candidate == nil || candidate.account == nil {
		return []AdvancedSchedulerScoreDiagnosticMetric{}
	}
	metrics := []AdvancedSchedulerScoreDiagnosticMetric{
		{
			Key:                  "priority",
			RawValue:             strconv.Itoa(candidate.account.Priority),
			Normalization:        diagnosticPriorityNormalization(candidate.account.Priority, ranges),
			NormalizedValue:      candidate.factors.Priority,
			Weight:               weights.Priority,
			WeightedContribution: weights.Priority * candidate.factors.Priority,
			Available:            true,
			Source:               "account.priority",
		},
		diagnosticLoadMetric(candidate, weights),
		diagnosticQueueMetric(candidate, ranges, weights),
		diagnosticErrorRateMetric(candidate, weights),
		diagnosticTTFTMetric(candidate, ranges, weights),
		diagnosticResetMetric(candidate, ranges, weights, now),
		diagnosticQuotaMetric(candidate, weights, now),
	}
	return metrics
}

func diagnosticPriorityNormalization(priority int, ranges advancedSchedulerScoreRanges) string {
	if ranges.MaxPriority <= ranges.MinPriority {
		return "候选优先级相同，使用 1.0000"
	}
	return "1 - (" + strconv.Itoa(priority) + " - " + strconv.Itoa(ranges.MinPriority) + ") / (" + strconv.Itoa(ranges.MaxPriority) + " - " + strconv.Itoa(ranges.MinPriority) + ")"
}

func diagnosticLoadMetric(candidate *advancedSchedulerCandidateScore, weights GatewayAdvancedSchedulerScoreWeightsView) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "load",
		NormalizedValue:      candidate.factors.Load,
		Weight:               weights.Load,
		WeightedContribution: weights.Load * candidate.factors.Load,
		Source:               "concurrency.load_rate",
	}
	if !candidate.loadKnown {
		metric.RawValue = "未观测"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	metric.Available = true
	metric.RawValue = strconv.Itoa(candidate.loadInfo.LoadRate) + "%"
	metric.Normalization = "1 - clamp(" + strconv.Itoa(candidate.loadInfo.LoadRate) + " / 100)"
	return metric
}

func diagnosticQueueMetric(candidate *advancedSchedulerCandidateScore, ranges advancedSchedulerScoreRanges, weights GatewayAdvancedSchedulerScoreWeightsView) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "queue",
		NormalizedValue:      candidate.factors.Queue,
		Weight:               weights.Queue,
		WeightedContribution: weights.Queue * candidate.factors.Queue,
		Source:               "concurrency.waiting_count",
	}
	if !candidate.loadKnown {
		metric.RawValue = "未观测"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	metric.Available = true
	metric.RawValue = strconv.Itoa(candidate.loadInfo.WaitingCount)
	metric.Normalization = "1 - clamp(" + strconv.Itoa(candidate.loadInfo.WaitingCount) + " / " + strconv.Itoa(ranges.MaxWaiting) + ")"
	return metric
}

func diagnosticErrorRateMetric(candidate *advancedSchedulerCandidateScore, weights GatewayAdvancedSchedulerScoreWeightsView) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "error_rate",
		NormalizedValue:      candidate.factors.ErrorRate,
		Weight:               weights.ErrorRate,
		WeightedContribution: weights.ErrorRate * candidate.factors.ErrorRate,
		Source:               "advanced_scheduler.error_rate_ewma",
	}
	if !candidate.feedback.HasFeedback {
		metric.RawValue = "未观测"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	metric.Available = true
	metric.RawValue = "EWMA " + diagnosticFloat(candidate.feedback.ErrorRate) + "（" + strconv.FormatInt(candidate.feedback.ErrorSamples, 10) + " 样本）"
	metric.Normalization = "1 - clamp(" + diagnosticFloat(candidate.feedback.ErrorRate) + ")"
	metric.ObservedAt = candidate.feedback.LastObservedAt
	return metric
}

func diagnosticTTFTMetric(candidate *advancedSchedulerCandidateScore, ranges advancedSchedulerScoreRanges, weights GatewayAdvancedSchedulerScoreWeightsView) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "ttft",
		NormalizedValue:      candidate.factors.TTFT,
		Weight:               weights.TTFT,
		WeightedContribution: weights.TTFT * candidate.factors.TTFT,
		Source:               "advanced_scheduler.ttft_ewma",
	}
	if !candidate.feedback.HasTTFT {
		metric.RawValue = "未观测"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	metric.Available = true
	metric.RawValue = diagnosticFloat(candidate.feedback.TTFT) + " ms（" + strconv.FormatInt(candidate.feedback.TTFTSamples, 10) + " 样本）"
	if !ranges.HasTTFTSample || ranges.MaxTTFT <= ranges.MinTTFT {
		metric.Normalization = "候选 TTFT 相同，使用中性值 0.5000"
		metric.Neutral = true
	} else {
		metric.Normalization = "1 - clamp((" + diagnosticFloat(candidate.feedback.TTFT) + " - " + diagnosticFloat(ranges.MinTTFT) + ") / (" + diagnosticFloat(ranges.MaxTTFT) + " - " + diagnosticFloat(ranges.MinTTFT) + "))"
	}
	metric.ObservedAt = candidate.feedback.LastTTFTAt
	return metric
}

func diagnosticResetMetric(candidate *advancedSchedulerCandidateScore, ranges advancedSchedulerScoreRanges, weights GatewayAdvancedSchedulerScoreWeightsView, now time.Time) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "reset",
		NormalizedValue:      candidate.factors.Reset,
		Weight:               weights.Reset,
		WeightedContribution: weights.Reset * candidate.factors.Reset,
		Source:               "account.session_window_end",
	}
	if weights.Reset <= 0 {
		metric.RawValue = "权重为 0，未参与评分"
		metric.Normalization = "不适用，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	if candidate.account.SessionWindowEnd == nil || !now.Before(*candidate.account.SessionWindowEnd) {
		metric.RawValue = "未观测"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	remaining := candidate.account.SessionWindowEnd.Sub(now).Seconds()
	metric.Available = true
	metric.RawValue = diagnosticFloat(remaining) + " 秒"
	if !ranges.HasResetSample || ranges.MaxResetRemaining <= ranges.MinResetRemaining {
		metric.Normalization = "候选窗口重置时间相同，使用 1.0000"
	} else {
		metric.Normalization = "1 - clamp((" + diagnosticFloat(remaining) + " - " + diagnosticFloat(ranges.MinResetRemaining) + ") / (" + diagnosticFloat(ranges.MaxResetRemaining) + " - " + diagnosticFloat(ranges.MinResetRemaining) + "))"
	}
	return metric
}

func diagnosticQuotaMetric(candidate *advancedSchedulerCandidateScore, weights GatewayAdvancedSchedulerScoreWeightsView, now time.Time) AdvancedSchedulerScoreDiagnosticMetric {
	metric := AdvancedSchedulerScoreDiagnosticMetric{
		Key:                  "quota_headroom",
		NormalizedValue:      candidate.factors.QuotaHeadroom,
		Weight:               weights.QuotaHeadroom,
		WeightedContribution: weights.QuotaHeadroom * candidate.factors.QuotaHeadroom,
		Source:               "platform.quota_headroom",
	}
	if weights.QuotaHeadroom <= 0 {
		metric.RawValue = "权重为 0，未参与评分"
		metric.Normalization = "不适用，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	if candidate.account.Platform != PlatformOpenAI && candidate.account.Platform != PlatformGrok {
		metric.RawValue = "当前平台未提供配额余量信号"
		metric.Normalization = "未观测，使用中性值 0.5000"
		metric.Neutral = true
		return metric
	}
	factor := openAIQuotaHeadroomFactor(candidate.account, now)
	metric.Available = factor != openAIQuotaHeadroomNeutralFactor
	metric.Neutral = !metric.Available
	metric.RawValue = diagnosticFloat(factor)
	if metric.Neutral {
		metric.RawValue = "未观测或过期，0.5000"
		metric.Normalization = "未观测，使用中性值 0.5000"
	} else {
		metric.Normalization = "平台适配器提供的 0..1 quota headroom"
	}
	return metric
}

func diagnosticEffectiveSettings(group *Group, runtime advancedSchedulerRuntimeSettings, effective advancedSchedulerEffectiveSettings) []AdvancedSchedulerScoreDiagnosticSetting {
	overrides := GroupAdvancedSchedulerOverrides{}
	if group != nil {
		overrides = group.AdvancedSchedulerOverrides
	}
	settingSource := func(groupOverride bool, runtimeOverride bool) string {
		if groupOverride {
			return "group_override"
		}
		if runtimeOverride {
			return "global_runtime"
		}
		return "process_default"
	}
	settings := []AdvancedSchedulerScoreDiagnosticSetting{
		{Key: "sticky_weighted_enabled", Value: strconv.FormatBool(effective.stickyWeightedEnabled), Source: settingSource(overrides.StickyWeightedEnabled != nil, true)},
		{Key: "subscription_priority_enabled", Value: strconv.FormatBool(effective.subscriptionPriorityEnabled), Source: settingSource(overrides.SubscriptionPriorityEnabled != nil, true)},
		{Key: "lb_top_k", Value: strconv.Itoa(effective.topK), Source: settingSource(overrides.LBTopK != nil, runtime.lbTopKOverride > 0)},
	}
	for _, item := range []struct {
		key           string
		value         float64
		groupOverride *float64
		runtimeKey    string
	}{
		{"weight_priority", effective.weights.Priority, overrides.WeightPriority, "priority"},
		{"weight_load", effective.weights.Load, overrides.WeightLoad, "load"},
		{"weight_queue", effective.weights.Queue, overrides.WeightQueue, "queue"},
		{"weight_error_rate", effective.weights.ErrorRate, overrides.WeightErrorRate, "error_rate"},
		{"weight_ttft", effective.weights.TTFT, overrides.WeightTTFT, "ttft"},
		{"weight_reset", effective.weights.Reset, overrides.WeightReset, "reset"},
		{"weight_quota_headroom", effective.weights.QuotaHeadroom, overrides.WeightQuotaHeadroom, "quota_headroom"},
		{"weight_previous_response", effective.weights.Previous, overrides.WeightPreviousResponse, "previous_response"},
		{"weight_session_sticky", effective.weights.SessionSticky, overrides.WeightSessionSticky, "session_sticky"},
	} {
		_, hasRuntimeOverride := runtime.weightOverrides[item.runtimeKey]
		settings = append(settings, AdvancedSchedulerScoreDiagnosticSetting{
			Key: item.key, Value: diagnosticFloat(item.value), Source: settingSource(item.groupOverride != nil, hasRuntimeOverride),
		})
	}
	return settings
}

func diagnosticPolicySignals(
	group *Group,
	request AdvancedSchedulerScoreDiagnosticRequest,
	effective advancedSchedulerEffectiveSettings,
) []AdvancedSchedulerScoreDiagnosticPolicySignal {
	signals := make([]AdvancedSchedulerScoreDiagnosticPolicySignal, 0, 3)
	if request.PreviousResponseAccountID > 0 {
		state := "ignored"
		if effective.stickyWeightedEnabled {
			state = "weighted"
		}
		signals = append(signals, AdvancedSchedulerScoreDiagnosticPolicySignal{
			Key: "previous_response_binding", State: state,
			Detail: "仅使用管理员输入的账号 ID 模拟上一响应粘性；不会读取响应正文或响应标识。",
		})
	}
	if request.StickyAccountID > 0 {
		state := "ignored"
		if effective.stickyWeightedEnabled {
			state = "weighted"
		}
		signals = append(signals, AdvancedSchedulerScoreDiagnosticPolicySignal{
			Key: "session_sticky", State: state,
			Detail: "仅使用管理员输入的账号 ID 模拟会话粘性；不会读取或写入 session hash。",
		})
	}
	if group != nil && (group.Platform == PlatformOpenAI || group.Platform == PlatformGrok) && effective.subscriptionPriorityEnabled {
		signals = append(signals,
			AdvancedSchedulerScoreDiagnosticPolicySignal{
				Key: "subscription_priority", State: "enabled",
				Detail: "订阅优先属于候选策略，不作为普通加权指标展示。",
			},
		)
	}
	return signals
}
