package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/errors"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/tidwall/gjson"
)

const (
	DataShareStatusCompleted  = "completed"
	DataShareStatusTerminated = "terminated"
	DataShareQualityComplete  = "complete"
	DataShareQualityPartial   = "partial"
	DataShareQualityInvalid   = "invalid"
	defaultDataShareDataset   = "tokenrouter-agent"
)

var (
	ErrDataShareSessionNotFound = infraerrors.NotFound("DATA_SHARE_SESSION_NOT_FOUND", "data share session not found")
	ErrDataShareNoticeMissing   = infraerrors.BadRequest("DATA_SHARE_NOTICE_MISSING", "data sharing notice content is required")
)

const defaultDataSharingNoticeContent = "该分组已启用数据共享。使用该分组产生的 Agent 对话数据会被保存，并可能用于训练、评估和改进模型。请确认你已理解并同意该数据共享安排。"
const dataShareSkipRulesCacheTTL = 30 * time.Second

var ErrDataShareSkipRulesInvalid = infraerrors.BadRequest("DATA_SHARE_SKIP_RULES_INVALID", "data sharing capture skip rules are invalid")

const (
	dataShareSkipRuleMatchContains = "contains"
	dataShareSkipRuleMatchEquals   = "equals"
)

// DataShareNotice 是用户切换到数据共享分组前需要确认的须知。
type DataShareNotice struct {
	Content   string    `json:"content"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DataShareCaptureSkipRule 描述数据共享采集前需要跳过的辅助请求模式。
type DataShareCaptureSkipRule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	ClientFamilies []string `json:"client_families"`
	RequestPaths   []string `json:"request_paths"`
	FieldScopes    []string `json:"field_scopes"`
	Patterns       []string `json:"patterns"`
	CaseSensitive  bool     `json:"case_sensitive"`
	MatchMode      string   `json:"match_mode"`
}

// DataShareSession 保存一条聚合后的 Agent session。
type DataShareSession struct {
	ID                 int64
	TrajectoryID       string
	SessionID          string
	Dataset            string
	Provider           string
	Model              string
	RequestPath        string
	UserAgent          string
	Status             string
	IsFinalSnapshot    bool
	SourceRequestCount int
	SystemPrompt       *string
	Tools              []map[string]any
	Messages           []map[string]any
	Usage              map[string]any
	Meta               map[string]any
	SessionJSON        map[string]any
	PayloadCompressed  []byte
	PayloadEncoding    string
	PayloadBytes       int64
	Exportable         bool
	QualityStatus      string
	QualityErrors      []string
	StorageBytes       int64
	InputTokens        int64
	OutputTokens       int64
	TotalTokens        int64
	UserID             int64
	UserName           string
	UserEmail          string
	APIKeyID           int64
	APIKeyName         string
	GroupID            int64
	GroupName          string
	CreatedAt          time.Time
	EndedAt            *time.Time
	UpdatedAt          time.Time
}

// DataShareSessionFilters 描述列表/统计/导出筛选条件。
type DataShareSessionFilters struct {
	IDs        []int64
	ExcludeIDs []int64
	// SelectAll 表示批量操作覆盖当前筛选条件下的全集，ExcludeIDs 用于排除用户取消勾选的记录。
	SelectAll     bool
	UserID        int64
	UserName      string
	APIKeyID      int64
	APIKeyName    string
	GroupID       int64
	GroupName     string
	Provider      string
	Model         string
	RequestPath   string
	UserAgent     string
	Exportable    *bool
	QualityStatus string
	StartTime     *time.Time
	EndTime       *time.Time
	Search        string
}

// DataShareStoragePoint 用于管理端展示空间增长趋势。
type DataShareStoragePoint struct {
	Date         string `json:"date"`
	StorageBytes int64  `json:"storage_bytes"`
	SessionCount int64  `json:"session_count"`
}

// DataShareGroupStoragePoint 用于管理端按分组展示空间占用。
type DataShareGroupStoragePoint struct {
	GroupID      int64  `json:"group_id"`
	GroupName    string `json:"group_name"`
	StorageBytes int64  `json:"storage_bytes"`
	SessionCount int64  `json:"session_count"`
}

// DataShareRequestPathPoint 用于管理端按用户请求路径展示分布。
type DataShareRequestPathPoint struct {
	RequestPath  string `json:"request_path"`
	StorageBytes int64  `json:"storage_bytes"`
	SessionCount int64  `json:"session_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

// DataShareModelPoint 用于管理端按模型展示分布。
type DataShareModelPoint struct {
	Model        string `json:"model"`
	StorageBytes int64  `json:"storage_bytes"`
	SessionCount int64  `json:"session_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

// DataShareUserAgentPoint 用于管理端按客户端 User-Agent 展示分布。
type DataShareUserAgentPoint struct {
	UserAgent    string `json:"user_agent"`
	StorageBytes int64  `json:"storage_bytes"`
	SessionCount int64  `json:"session_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

// DataShareStats 是管理端数据共享概览指标。
type DataShareStats struct {
	SessionCount          int64                        `json:"session_count"`
	ExportableCount       int64                        `json:"exportable_count"`
	NonExportableCount    int64                        `json:"non_exportable_count"`
	CompleteCount         int64                        `json:"complete_count"`
	PartialCount          int64                        `json:"partial_count"`
	InvalidCount          int64                        `json:"invalid_count"`
	TotalStorageBytes     int64                        `json:"total_storage_bytes"`
	TotalTokens           int64                        `json:"total_tokens"`
	AvgTokensPerSession   float64                      `json:"avg_tokens_per_session"`
	StorageTrend          []DataShareStoragePoint      `json:"storage_trend"`
	GroupStorageBreakdown []DataShareGroupStoragePoint `json:"group_storage_breakdown"`
	RequestPathBreakdown  []DataShareRequestPathPoint  `json:"request_path_breakdown"`
	ModelBreakdown        []DataShareModelPoint        `json:"model_breakdown"`
	UserAgentBreakdown    []DataShareUserAgentPoint    `json:"user_agent_breakdown"`
}

// DataShareCaptureInput 是网关成功完成请求后的采集输入。
type DataShareCaptureInput struct {
	APIKey            *APIKey
	User              *User
	Account           *Account
	Provider          string
	Model             string
	UpstreamModel     string
	SessionID         string
	RequestID         string
	RequestBody       []byte
	ResponseBody      []byte
	SystemPrompt      string
	Messages          []any
	Tools             []map[string]any
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	UserAgent         string
	IPAddress         string
	InboundEndpoint   string
	UpstreamEndpoint  string
}

// DataShareSessionRepository 定义数据共享 session 的持久化能力。
type DataShareSessionRepository interface {
	UpsertCapture(ctx context.Context, session *DataShareSession) error
	List(ctx context.Context, params pagination.PaginationParams, filters DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error)
	ListWithPayload(ctx context.Context, params pagination.PaginationParams, filters DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error)
	GetByID(ctx context.Context, id int64) (*DataShareSession, error)
	Delete(ctx context.Context, id int64) error
	BatchDelete(ctx context.Context, ids []int64, filters DataShareSessionFilters) (int64, error)
	Stats(ctx context.Context, filters DataShareSessionFilters) (*DataShareStats, error)
}

// DataSharingService 负责数据共享须知、采集、导出和统计。
type DataSharingService struct {
	repo                    DataShareSessionRepository
	settingRepo             SettingRepository
	skipRulesMu             sync.RWMutex
	skipRulesCache          []DataShareCaptureSkipRule
	skipRulesCacheExpiresAt time.Time
}

func NewDataSharingService(repo DataShareSessionRepository, settingRepo SettingRepository) *DataSharingService {
	return &DataSharingService{repo: repo, settingRepo: settingRepo}
}

// GetNotice 返回当前数据共享须知；未配置时返回默认模板和版本 1。
func (s *DataSharingService) GetNotice(ctx context.Context) (*DataShareNotice, error) {
	return defaultDataSharingNotice(ctx, s.settingRepo)
}

// UpdateNotice 更新数据共享须知并递增版本号。
func (s *DataSharingService) UpdateNotice(ctx context.Context, content string) (*DataShareNotice, error) {
	if s == nil || s.settingRepo == nil {
		return nil, ErrSettingNotFound
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrDataShareNoticeMissing
	}
	current, err := s.GetNotice(ctx)
	if err != nil {
		return nil, err
	}
	version := current.Version + 1
	if version < 1 {
		version = 1
	}
	updates := map[string]string{
		SettingKeyDataSharingNoticeContent: content,
		SettingKeyDataSharingNoticeVersion: strconv.Itoa(version),
	}
	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return nil, err
	}
	return &DataShareNotice{Content: content, Version: version, UpdatedAt: time.Now()}, nil
}

// ConfirmNotice 校验用户确认的数据共享须知版本。
func (s *DataSharingService) ConfirmNotice(ctx context.Context, version int) (*DataShareNotice, error) {
	notice, err := s.GetNotice(ctx)
	if err != nil {
		return nil, err
	}
	if version <= 0 || version != notice.Version {
		return nil, ErrDataSharingConsentRequired
	}
	return notice, nil
}

// GetCaptureSkipRules 返回当前生效的数据共享采集跳过规则。
func (s *DataSharingService) GetCaptureSkipRules(ctx context.Context) ([]DataShareCaptureSkipRule, error) {
	rules, err := s.loadCaptureSkipRules(ctx)
	if err != nil {
		return nil, err
	}
	return cloneDataShareCaptureSkipRules(rules), nil
}

// UpdateCaptureSkipRules 保存管理端维护的数据共享采集跳过规则。
func (s *DataSharingService) UpdateCaptureSkipRules(ctx context.Context, rules []DataShareCaptureSkipRule) ([]DataShareCaptureSkipRule, error) {
	if s == nil || s.settingRepo == nil {
		return nil, ErrSettingNotFound
	}
	normalized, err := normalizeDataShareCaptureSkipRules(rules)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyDataSharingCaptureSkipRules, string(data)); err != nil {
		return nil, err
	}
	s.clearCaptureSkipRulesCache()
	return cloneDataShareCaptureSkipRules(normalized), nil
}

func (s *DataSharingService) shouldSkipDataShareCapture(ctx context.Context, input DataShareCaptureInput) bool {
	rules, err := s.loadCaptureSkipRules(ctx)
	if err != nil {
		slog.Warn("data sharing: failed to load capture skip rules", "error", err)
		return false
	}
	return dataShareCaptureSkipRulesMatch(input, rules)
}

func (s *DataSharingService) loadCaptureSkipRules(ctx context.Context) ([]DataShareCaptureSkipRule, error) {
	if s == nil || s.settingRepo == nil {
		return defaultDataShareCaptureSkipRules(), nil
	}
	now := time.Now()
	s.skipRulesMu.RLock()
	if now.Before(s.skipRulesCacheExpiresAt) && s.skipRulesCache != nil {
		cached := cloneDataShareCaptureSkipRules(s.skipRulesCache)
		s.skipRulesMu.RUnlock()
		return cached, nil
	}
	s.skipRulesMu.RUnlock()

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyDataSharingCaptureSkipRules)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			rules := defaultDataShareCaptureSkipRules()
			s.storeCaptureSkipRulesCache(rules)
			return rules, nil
		}
		return nil, err
	}
	var rules []DataShareCaptureSkipRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		slog.Warn("data sharing: invalid capture skip rules json, fallback to defaults", "error", err)
		rules = defaultDataShareCaptureSkipRules()
		s.storeCaptureSkipRulesCache(rules)
		return rules, nil
	}
	normalized, err := normalizeDataShareCaptureSkipRules(rules)
	if err != nil {
		slog.Warn("data sharing: invalid capture skip rules config, fallback to defaults", "error", err)
		normalized = defaultDataShareCaptureSkipRules()
	}
	s.storeCaptureSkipRulesCache(normalized)
	return cloneDataShareCaptureSkipRules(normalized), nil
}

func (s *DataSharingService) storeCaptureSkipRulesCache(rules []DataShareCaptureSkipRule) {
	if s == nil {
		return
	}
	s.skipRulesMu.Lock()
	defer s.skipRulesMu.Unlock()
	s.skipRulesCache = cloneDataShareCaptureSkipRules(rules)
	s.skipRulesCacheExpiresAt = time.Now().Add(dataShareSkipRulesCacheTTL)
}

func (s *DataSharingService) clearCaptureSkipRulesCache() {
	if s == nil {
		return
	}
	s.skipRulesMu.Lock()
	defer s.skipRulesMu.Unlock()
	s.skipRulesCache = nil
	s.skipRulesCacheExpiresAt = time.Time{}
}

func defaultDataShareCaptureSkipRules() []DataShareCaptureSkipRule {
	return []DataShareCaptureSkipRule{
		{
			ID:             "claude_code_title",
			Name:           "Claude Code 标题生成",
			Enabled:        true,
			ClientFamilies: []string{"claude-cli"},
			RequestPaths:   []string{"/v1/messages"},
			FieldScopes:    []string{"system"},
			Patterns:       []string{"Generate a concise, sentence-case title"},
			MatchMode:      dataShareSkipRuleMatchContains,
		},
		{
			ID:             "opencode_title_system",
			Name:           "opencode 标题生成系统提示",
			Enabled:        true,
			ClientFamilies: []string{"opencode"},
			RequestPaths:   []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			FieldScopes:    []string{"system"},
			Patterns: []string{
				"You are a title generator. You output ONLY a thread title. Nothing else.",
				"Generate a brief title that would help the user find this conversation later.",
				"NEVER respond to questions, just generate a title for the conversation",
			},
			MatchMode: dataShareSkipRuleMatchContains,
		},
		{
			ID:             "opencode_title_user_prompt",
			Name:           "opencode 标题生成用户提示",
			Enabled:        true,
			ClientFamilies: []string{"opencode"},
			RequestPaths:   []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			FieldScopes:    []string{"messages", "input"},
			Patterns:       []string{"Generate a title for this conversation:"},
			MatchMode:      dataShareSkipRuleMatchContains,
		},
		{
			ID:           "agent_title_from_messages",
			Name:         "Agent 会话标题生成",
			Enabled:      true,
			RequestPaths: []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			FieldScopes:  []string{"messages", "input"},
			Patterns:     []string{"Please write a 5-10 word title for the following conversation:"},
			MatchMode:    dataShareSkipRuleMatchContains,
		},
		{
			ID:           "agent_topic_title",
			Name:         "Agent 主题标题提取",
			Enabled:      true,
			RequestPaths: []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			FieldScopes:  []string{"system", "instructions"},
			Patterns:     []string{"extract a 2-3 word title"},
			MatchMode:    dataShareSkipRuleMatchContains,
		},
		{
			ID:           "agent_warmup",
			Name:         "Agent 预热请求",
			Enabled:      true,
			RequestPaths: []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"},
			FieldScopes:  []string{"messages", "input"},
			Patterns:     []string{"Warmup"},
			MatchMode:    dataShareSkipRuleMatchEquals,
		},
	}
}

func normalizeDataShareCaptureSkipRules(rules []DataShareCaptureSkipRule) ([]DataShareCaptureSkipRule, error) {
	out := make([]DataShareCaptureSkipRule, 0, len(rules))
	seenIDs := map[string]struct{}{}
	for _, rule := range rules {
		normalized, err := normalizeDataShareCaptureSkipRule(rule)
		if err != nil {
			return nil, err
		}
		if _, ok := seenIDs[normalized.ID]; ok {
			return nil, ErrDataShareSkipRulesInvalid
		}
		seenIDs[normalized.ID] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeDataShareCaptureSkipRule(rule DataShareCaptureSkipRule) (DataShareCaptureSkipRule, error) {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.MatchMode = strings.ToLower(strings.TrimSpace(rule.MatchMode))
	if rule.MatchMode == "" {
		rule.MatchMode = dataShareSkipRuleMatchContains
	}
	if rule.ID == "" || rule.Name == "" {
		return DataShareCaptureSkipRule{}, ErrDataShareSkipRulesInvalid
	}
	if rule.MatchMode != dataShareSkipRuleMatchContains && rule.MatchMode != dataShareSkipRuleMatchEquals {
		return DataShareCaptureSkipRule{}, ErrDataShareSkipRulesInvalid
	}
	rule.ClientFamilies = uniqueTrimmedStrings(rule.ClientFamilies, func(v string) string {
		return strings.ToLower(normalizeDataShareUserAgent(v))
	})
	rule.RequestPaths = uniqueTrimmedStrings(rule.RequestPaths, func(v string) string {
		return strings.ToLower(normalizeDataShareRequestPath(v))
	})
	rule.FieldScopes = uniqueTrimmedStrings(rule.FieldScopes, func(v string) string {
		return strings.ToLower(strings.TrimSpace(v))
	})
	rule.Patterns = uniqueTrimmedStrings(rule.Patterns, strings.TrimSpace)
	if len(rule.FieldScopes) == 0 || len(rule.Patterns) == 0 {
		return DataShareCaptureSkipRule{}, ErrDataShareSkipRulesInvalid
	}
	for _, scope := range rule.FieldScopes {
		if !isDataShareSkipScope(scope) {
			return DataShareCaptureSkipRule{}, ErrDataShareSkipRulesInvalid
		}
	}
	return rule, nil
}

func uniqueTrimmedStrings(values []string, normalize func(string) string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = normalize(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isDataShareSkipScope(scope string) bool {
	switch scope {
	case "system", "messages", "input", "instructions":
		return true
	default:
		return false
	}
}

func cloneDataShareCaptureSkipRules(rules []DataShareCaptureSkipRule) []DataShareCaptureSkipRule {
	out := make([]DataShareCaptureSkipRule, 0, len(rules))
	for _, rule := range rules {
		cloned := rule
		cloned.ClientFamilies = append([]string(nil), rule.ClientFamilies...)
		cloned.RequestPaths = append([]string(nil), rule.RequestPaths...)
		cloned.FieldScopes = append([]string(nil), rule.FieldScopes...)
		cloned.Patterns = append([]string(nil), rule.Patterns...)
		out = append(out, cloned)
	}
	return out
}

func dataShareCaptureSkipRulesMatch(input DataShareCaptureInput, rules []DataShareCaptureSkipRule) bool {
	texts := dataShareSkipCandidateTexts(input.RequestBody)
	clientFamily := strings.ToLower(normalizeDataShareUserAgent(input.UserAgent))
	requestPath := strings.ToLower(normalizeDataShareRequestPath(input.InboundEndpoint))
	for _, rule := range rules {
		if !rule.Enabled || !dataShareSkipRuleApplies(rule.ClientFamilies, clientFamily) || !dataShareSkipRuleApplies(rule.RequestPaths, requestPath) {
			continue
		}
		for _, scope := range rule.FieldScopes {
			for _, text := range texts[scope] {
				if dataShareSkipRuleTextMatches(rule, text) {
					return true
				}
			}
		}
	}
	return false
}

func dataShareSkipRuleApplies(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func dataShareSkipRuleTextMatches(rule DataShareCaptureSkipRule, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, pattern := range rule.Patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		left, right := text, pattern
		if !rule.CaseSensitive {
			left = strings.ToLower(left)
			right = strings.ToLower(right)
		}
		switch rule.MatchMode {
		case dataShareSkipRuleMatchEquals:
			if left == right {
				return true
			}
		default:
			if strings.Contains(left, right) {
				return true
			}
		}
	}
	return false
}

func dataShareSkipCandidateTexts(body []byte) map[string][]string {
	out := map[string][]string{
		"system":       {},
		"messages":     {},
		"input":        {},
		"instructions": {},
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return out
	}
	add := func(scope string, value any) {
		if text := strings.TrimSpace(dataShareContentText(value)); text != "" {
			out[scope] = append(out[scope], text)
		}
	}
	add("system", payload["system"])
	add("system", payload["system_instruction"])
	add("instructions", payload["instructions"])
	add("instructions", payload["system_instruction"])
	add("input", payload["input"])
	appendDataShareSkipResponsesInput(out, payload["input"])
	appendDataShareSkipMessages(out, payload["messages"])
	appendDataShareSkipContents(out, payload["contents"])
	return out
}

func appendDataShareSkipResponsesInput(out map[string][]string, raw any) {
	for _, item := range anySlice(raw) {
		msg, ok := mapFromAny(item)
		if !ok {
			continue
		}
		text := strings.TrimSpace(dataShareContentText(firstPresentAny(msg["content"], msg["text"])))
		if text == "" {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(stringFromAny(msg["role"])))
		if role == "system" || role == "developer" {
			out["system"] = append(out["system"], text)
		}
	}
}

func appendDataShareSkipMessages(out map[string][]string, raw any) {
	for _, item := range anySlice(raw) {
		msg, ok := mapFromAny(item)
		if !ok {
			if text := strings.TrimSpace(dataShareContentText(item)); text != "" {
				out["messages"] = append(out["messages"], text)
			}
			continue
		}
		text := strings.TrimSpace(dataShareContentText(firstPresentAny(msg["content"], msg["text"])))
		if text == "" {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(stringFromAny(msg["role"])))
		if role == "system" || role == "developer" {
			out["system"] = append(out["system"], text)
			continue
		}
		out["messages"] = append(out["messages"], text)
	}
}

func appendDataShareSkipContents(out map[string][]string, raw any) {
	for _, item := range anySlice(raw) {
		msg, ok := mapFromAny(item)
		if !ok {
			continue
		}
		text := strings.TrimSpace(dataShareContentText(firstPresentAny(msg["parts"], msg["content"], msg["text"])))
		if text == "" {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(stringFromAny(msg["role"])))
		if role == "system" || role == "developer" {
			out["system"] = append(out["system"], text)
			continue
		}
		out["messages"] = append(out["messages"], text)
	}
}

// CaptureClaudeRequest 采集 Claude/Gemini 兼容协议成功请求。
func (s *DataSharingService) CaptureClaudeRequest(ctx context.Context, input DataShareCaptureInput) error {
	if input.APIKey == nil || input.APIKey.Group == nil || !input.APIKey.Group.DataSharingEnabled {
		return nil
	}
	if s == nil || s.repo == nil {
		return nil
	}
	if s.shouldSkipDataShareCapture(ctx, input) {
		return nil
	}
	if input.Model == "" && input.UpstreamModel != "" {
		input.Model = input.UpstreamModel
	}
	session := s.buildSession(input)
	return s.repo.UpsertCapture(ctx, session)
}

// CaptureOpenAIRequest 采集 OpenAI 协议成功请求。
func (s *DataSharingService) CaptureOpenAIRequest(ctx context.Context, input DataShareCaptureInput) error {
	if input.APIKey == nil || input.APIKey.Group == nil || !input.APIKey.Group.DataSharingEnabled {
		return nil
	}
	if s == nil || s.repo == nil {
		return nil
	}
	if s.shouldSkipDataShareCapture(ctx, input) {
		return nil
	}
	session := s.buildSession(input)
	return s.repo.UpsertCapture(ctx, session)
}

// ListSessions 查询数据共享 session。
func (s *DataSharingService) ListSessions(ctx context.Context, params pagination.PaginationParams, filters DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, filters)
}

// GetSession 查询单条 session，并可选限制 userID。
func (s *DataSharingService) GetSession(ctx context.Context, id int64, userID int64) (*DataShareSession, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if userID > 0 && session.UserID != userID {
		return nil, ErrDataShareSessionNotFound
	}
	return session, nil
}

func (s *DataSharingService) DeleteSession(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *DataSharingService) BatchDeleteSessions(ctx context.Context, ids []int64, filters DataShareSessionFilters) (int64, error) {
	return s.repo.BatchDelete(ctx, ids, filters)
}

func (s *DataSharingService) Stats(ctx context.Context, filters DataShareSessionFilters) (*DataShareStats, error) {
	return s.repo.Stats(ctx, filters)
}

// ExportJSONL 导出选中的数据共享 session；显式选中的记录保留原始快照，不再因质量状态跳过。
func (s *DataSharingService) ExportJSONL(ctx context.Context, w io.Writer, filters DataShareSessionFilters, includeNonExportable bool) error {
	_ = includeNonExportable
	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "created_at", SortOrder: pagination.SortOrderAsc}
	for {
		items, result, err := s.repo.ListWithPayload(ctx, params, filters)
		if err != nil {
			return err
		}
		for i := range items {
			payload := exportPayloadFromSession(&items[i])
			line, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if _, err := w.Write(append(line, '\n')); err != nil {
				return err
			}
		}
		if result == nil || params.Page >= result.Pages || len(items) == 0 {
			return nil
		}
		params.Page++
	}
}

func defaultDataSharingNotice(ctx context.Context, repo SettingRepository) (*DataShareNotice, error) {
	if repo == nil {
		return &DataShareNotice{Content: defaultDataSharingNoticeContent, Version: 1, UpdatedAt: time.Now()}, nil
	}
	settings, err := repo.GetMultiple(ctx, []string{SettingKeyDataSharingNoticeContent, SettingKeyDataSharingNoticeVersion})
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(settings[SettingKeyDataSharingNoticeContent])
	if content == "" {
		content = defaultDataSharingNoticeContent
	}
	version, _ := strconv.Atoi(strings.TrimSpace(settings[SettingKeyDataSharingNoticeVersion]))
	if version <= 0 {
		version = 1
	}
	return &DataShareNotice{Content: content, Version: version, UpdatedAt: time.Now()}, nil
}

func (s *DataSharingService) buildSession(input DataShareCaptureInput) *DataShareSession {
	now := time.Now()
	groupID := int64(0)
	if input.APIKey != nil && input.APIKey.GroupID != nil {
		groupID = *input.APIKey.GroupID
	} else if input.APIKey != nil && input.APIKey.Group != nil {
		groupID = input.APIKey.Group.ID
	}
	userID := int64(0)
	if input.User != nil {
		userID = input.User.ID
	} else if input.APIKey != nil {
		userID = input.APIKey.UserID
	}
	apiKeyID := int64(0)
	if input.APIKey != nil {
		apiKeyID = input.APIKey.ID
	}
	provider := normalizeDataShareProvider(input.Provider, input.APIKey)
	model := resolveDataShareActualModel(input)
	requestPath := normalizeDataShareRequestPath(input.InboundEndpoint)
	userAgent := normalizeDataShareUserAgent(input.UserAgent)
	sessionID := normalizeDataShareSessionID(input.SessionID, input.RequestID, input.RequestBody, apiKeyID)
	trajectoryID := buildTrajectoryID(provider, sessionID, apiKeyID, groupID)
	messages := normalizeCaptureMessages(input)
	tools := normalizeCaptureTools(input)
	usage := buildCaptureUsage(input)
	meta := buildCaptureMeta(input)
	systemPrompt := strings.TrimSpace(input.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = extractSystemPromptFromRequest(input.RequestBody)
	}
	if systemPrompt == "" {
		systemPrompt = extractSystemPromptFromMessages(messages)
	}
	qualityErrors := ValidateDataShareSessionQuality(model, systemPrompt, messages, tools, usage)
	qualityStatus := DataSharePayloadQualityStatus(model, systemPrompt, messages, tools, usage)
	status, finalSnapshot := dataShareCompletionState(qualityStatus)
	sessionJSON := map[string]any{
		"trajectory_id":        trajectoryID,
		"session_id":           sessionID,
		"dataset":              defaultDataShareDataset,
		"provider":             provider,
		"model":                model,
		"request_path":         requestPath,
		"user_agent":           userAgent,
		"created_at":           now.Format(time.RFC3339Nano),
		"ended_at":             now.Format(time.RFC3339Nano),
		"status":               status,
		"is_final_snapshot":    finalSnapshot,
		"source_request_count": 1,
		"quality_status":       qualityStatus,
		"system_prompt":        systemPrompt,
		"tools":                tools,
		"messages":             messages,
		"usage":                usage,
		"meta":                 meta,
	}
	storageBytes := int64(len(mustJSON(sessionJSON)))
	var sysPtr *string
	if systemPrompt != "" {
		sysPtr = &systemPrompt
	}
	inputTokens := int64(input.InputTokens + input.CacheReadTokens + input.CacheCreateTokens)
	outputTokens := int64(input.OutputTokens)
	return &DataShareSession{
		TrajectoryID:       trajectoryID,
		SessionID:          sessionID,
		Dataset:            defaultDataShareDataset,
		Provider:           provider,
		Model:              model,
		RequestPath:        requestPath,
		UserAgent:          userAgent,
		Status:             status,
		IsFinalSnapshot:    finalSnapshot,
		SourceRequestCount: 1,
		SystemPrompt:       sysPtr,
		Tools:              tools,
		Messages:           messages,
		Usage:              usage,
		Meta:               meta,
		SessionJSON:        sessionJSON,
		Exportable:         DataShareQualityExportable(qualityStatus),
		QualityStatus:      qualityStatus,
		QualityErrors:      qualityErrors,
		StorageBytes:       storageBytes,
		InputTokens:        inputTokens,
		OutputTokens:       outputTokens,
		TotalTokens:        inputTokens + outputTokens,
		UserID:             userID,
		APIKeyID:           apiKeyID,
		GroupID:            groupID,
		CreatedAt:          now,
		EndedAt:            &now,
		UpdatedAt:          now,
	}
}

func normalizeCaptureMessages(input DataShareCaptureInput) []map[string]any {
	var out []map[string]any
	if len(input.Messages) > 0 {
		out = appendAnyMessages(out, input.Messages)
	}
	if len(out) == 0 && len(input.RequestBody) > 0 {
		out = appendRequestMessages(out, input.RequestBody)
	}
	if len(input.ResponseBody) > 0 {
		out = appendAssistantMessageFromResponse(out, input.ResponseBody)
	}
	return normalizeDataShareMessages(out)
}

func appendAnyMessages(out []map[string]any, messages []any) []map[string]any {
	for _, msg := range messages {
		switch v := msg.(type) {
		case map[string]any:
			out = append(out, v)
		default:
			out = append(out, map[string]any{"role": "unknown", "content": v})
		}
	}
	return out
}

func appendRequestMessages(out []map[string]any, body []byte) []map[string]any {
	startLen := len(out)
	if arr := gjson.GetBytes(body, "messages"); arr.IsArray() {
		for _, item := range arr.Array() {
			out = append(out, rawJSONToMap(item.Raw))
		}
	}
	if arr := gjson.GetBytes(body, "contents"); arr.IsArray() {
		for _, item := range arr.Array() {
			msg := rawJSONToMap(item.Raw)
			if role, ok := msg["role"].(string); ok && role == "model" {
				msg["role"] = "assistant"
			}
			out = append(out, msg)
		}
	}
	if len(out) == startLen {
		// OpenAI Responses 使用 input 承载对话上下文，Codex CLI 会走这条协议。
		out = appendResponsesInputMessages(out, gjson.GetBytes(body, "input"))
	}
	return out
}

func appendResponsesInputMessages(out []map[string]any, input gjson.Result) []map[string]any {
	if !input.Exists() {
		return out
	}
	if input.Type == gjson.String {
		return append(out, map[string]any{"role": "user", "content": input.String()})
	}
	if input.IsObject() {
		return append(out, normalizeResponsesInputItem(input))
	}
	if !input.IsArray() {
		return out
	}
	for _, item := range input.Array() {
		if item.Type == gjson.String {
			out = append(out, map[string]any{"role": "user", "content": item.String()})
			continue
		}
		if item.IsObject() {
			out = append(out, normalizeResponsesInputItem(item))
		}
	}
	return out
}

func normalizeResponsesInputItem(item gjson.Result) map[string]any {
	msg := rawJSONToMap(item.Raw)
	role := normalizeResponsesInputRole(item.Get("role").String(), item.Get("type").String())
	if role != "" {
		msg["role"] = role
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	switch itemType {
	case "function_call":
		// 工具调用在对话中等价于 assistant 发起的 tool_call。
		return normalizeResponsesFunctionCallMessage(msg)
	case "function_call_output":
		// 工具执行结果按 tool 消息保存，便于后续训练流水线识别。
		return normalizeToolResultMessage(msg)
	case "input_text", "text":
		msg["role"] = "user"
		if text := item.Get("text"); text.Exists() {
			msg["content"] = text.String()
		}
	}
	if _, ok := msg["content"]; !ok {
		if content := item.Get("content"); content.Exists() {
			msg["content"] = responseInputContentValue(content)
		} else if text := item.Get("text"); text.Exists() {
			msg["content"] = text.String()
		}
	}
	return msg
}

func normalizeResponsesInputRole(role string, itemType string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "developer":
		return "system"
	case "model":
		return "assistant"
	case "":
		switch strings.TrimSpace(itemType) {
		case "function_call":
			return "assistant"
		case "function_call_output":
			return "tool"
		default:
			return "user"
		}
	default:
		return role
	}
}

func responseInputContentValue(value gjson.Result) any {
	if value.Type == gjson.String {
		return value.String()
	}
	return normalizeDataShareContentValue(rawJSONToAny(value.Raw))
}

func appendAssistantMessageFromResponse(out []map[string]any, body []byte) []map[string]any {
	if msg := gjson.GetBytes(body, "choices.0.message"); msg.Exists() {
		out = append(out, rawJSONToMap(msg.Raw))
	}
	if output := gjson.GetBytes(body, "output"); output.IsArray() {
		for _, item := range output.Array() {
			if item.IsObject() {
				out = append(out, normalizeResponsesOutputItem(item))
			}
		}
	}
	if content := gjson.GetBytes(body, "content"); content.IsArray() {
		out = append(out, map[string]any{"role": "assistant", "content": responseInputContentValue(content)})
	}
	if candidates := gjson.GetBytes(body, "candidates.0.content"); candidates.Exists() {
		msg := rawJSONToMap(candidates.Raw)
		msg["role"] = "assistant"
		out = append(out, msg)
	}
	return out
}

func normalizeResponsesOutputItem(item gjson.Result) map[string]any {
	msg := rawJSONToMap(item.Raw)
	switch strings.TrimSpace(item.Get("type").String()) {
	case "function_call":
		// Responses API 的 output 也可能直接携带工具调用，需要转成统一 tool_calls。
		return normalizeResponsesFunctionCallMessage(msg)
	case "function_call_output":
		return normalizeToolResultMessage(msg)
	case "message":
		role := normalizeResponsesInputRole(item.Get("role").String(), item.Get("type").String())
		if strings.TrimSpace(item.Get("role").String()) == "" {
			role = "assistant"
		}
		out := map[string]any{"role": role}
		if content := item.Get("content"); content.Exists() {
			out["content"] = responseInputContentValue(content)
		}
		return out
	default:
		return normalizeDataShareMessage(msg)
	}
}

func normalizeCaptureTools(input DataShareCaptureInput) []map[string]any {
	if len(input.Tools) > 0 {
		return normalizeDataShareTools(input.Tools)
	}
	body := input.RequestBody
	var out []map[string]any
	for _, path := range []string{"tools", "functions"} {
		if arr := gjson.GetBytes(body, path); arr.IsArray() {
			for _, item := range arr.Array() {
				out = append(out, rawJSONToMap(item.Raw))
			}
		}
	}
	return normalizeDataShareTools(out)
}

func buildCaptureUsage(input DataShareCaptureInput) map[string]any {
	totalInput := input.InputTokens + input.CacheReadTokens + input.CacheCreateTokens
	total := totalInput + input.OutputTokens
	return map[string]any{
		"input_tokens":                input.InputTokens,
		"output_tokens":               input.OutputTokens,
		"cache_read_input_tokens":     input.CacheReadTokens,
		"cache_creation_input_tokens": input.CacheCreateTokens,
		"total_tokens":                total,
	}
}

func buildCaptureMeta(input DataShareCaptureInput) map[string]any {
	requestID := resolveDataShareRequestID(input)
	requestPath := normalizeDataShareRequestPath(input.InboundEndpoint)
	sourceRequestIDs := []string{}
	if requestID != "" {
		sourceRequestIDs = append(sourceRequestIDs, requestID)
	}
	meta := map[string]any{
		"api_key_id":         int64(0),
		"group_id":           int64(0),
		"account_id":         int64(0),
		"request_id":         requestID,
		"source_request_ids": sourceRequestIDs,
		"requested_model":    firstNonBlank(input.Model, gjson.GetBytes(input.RequestBody, "model").String()),
		"inbound_endpoint":   requestPath,
		"request_path":       requestPath,
		"upstream_endpoint":  input.UpstreamEndpoint,
		"user_agent":         input.UserAgent,
		"user_agent_family":  normalizeDataShareUserAgent(input.UserAgent),
		"ip_address":         input.IPAddress,
	}
	if input.APIKey != nil {
		meta["user_id"] = input.APIKey.UserID
		meta["api_key_id"] = input.APIKey.ID
		meta["api_key_name"] = input.APIKey.Name
		if input.APIKey.GroupID != nil {
			meta["group_id"] = *input.APIKey.GroupID
		}
		if input.APIKey.User != nil {
			meta["user_name"] = input.APIKey.User.Username
			meta["user_email"] = input.APIKey.User.Email
		}
		if input.APIKey.Group != nil {
			meta["group_id"] = input.APIKey.Group.ID
			meta["group_name"] = input.APIKey.Group.Name
		}
	}
	if input.User != nil {
		meta["user_id"] = input.User.ID
		meta["user_name"] = input.User.Username
		meta["user_email"] = input.User.Email
	}
	if input.Account != nil {
		meta["account_id"] = input.Account.ID
	}
	return meta
}

func resolveDataShareRequestID(input DataShareCaptureInput) string {
	return firstNonBlank(
		input.RequestID,
		gjson.GetBytes(input.ResponseBody, "id").String(),
		gjson.GetBytes(input.RequestBody, "request_id").String(),
		gjson.GetBytes(input.RequestBody, "metadata.request_id").String(),
	)
}

func resolveDataShareActualModel(input DataShareCaptureInput) string {
	// 正式交付要求 model 等于实际生成模型；映射后的上游模型优先，客户端请求模型只放入 meta。
	return firstNonBlank(input.UpstreamModel, input.Model, gjson.GetBytes(input.RequestBody, "model").String())
}

// ValidateDataShareSessionQuality 按附件交付规则检查 session 是否可进入正式导出。
func ValidateDataShareSessionQuality(model string, systemPrompt string, messages []map[string]any, tools []map[string]any, usage map[string]any) []string {
	var errs []string
	seenErrs := map[string]struct{}{}
	addErr := func(code string) {
		if _, ok := seenErrs[code]; ok {
			return
		}
		seenErrs[code] = struct{}{}
		errs = append(errs, code)
	}
	systemPrompt = firstNonBlank(systemPrompt, extractSystemPromptFromMessages(messages))
	messages = CompactDataShareMessages(messages)
	if strings.TrimSpace(systemPrompt) == "" {
		addErr("missing_system_prompt")
	}
	if len(messages) < 2 {
		addErr("effective_turns_lt_2")
	}
	toolDefs, invalidToolCount := collectDataShareToolDefinitions(tools)
	if len(toolDefs) == 0 {
		addErr("missing_tool_definitions")
	}
	if invalidToolCount > 0 {
		addErr("invalid_tool_definition")
	}
	toolCalls := collectDataShareToolCalls(messages)
	toolResults := collectDataShareToolResults(messages)
	if len(toolCalls) == 0 {
		addErr("missing_structured_tool_call")
	}
	for _, call := range toolCalls {
		if call.id == "" || call.name == "" {
			addErr("invalid_tool_call")
			continue
		}
		if _, ok := toolDefs[call.name]; !ok {
			addErr("tool_definition_missing")
		}
		if toolResults[call.id] != 1 {
			addErr("tool_call_result_unpaired")
		}
	}
	for id, count := range toolResults {
		if id == "" || count != 1 {
			addErr("tool_result_unpaired")
		}
	}
	if len(toolCalls) > 0 && !hasFinalAssistantMessage(messages) {
		addErr("missing_final_assistant")
	}
	if !dataShareModelAllowed(model) {
		addErr("model_not_allowed")
	}
	if intFromAny(usage["total_tokens"]) <= 0 {
		addErr("missing_usage_tokens")
	}
	return errs
}

func dataShareModelAllowed(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	return strings.Contains(model, "gpt-5") ||
		strings.Contains(model, "claude") && (strings.Contains(model, "4.5") || strings.Contains(model, "4-5")) ||
		strings.Contains(model, "gemini-3")
}

type dataShareToolCall struct {
	id   string
	name string
}

func collectDataShareToolDefinitions(tools []map[string]any) (map[string]struct{}, int) {
	defs := make(map[string]struct{}, len(tools))
	invalid := 0
	for _, tool := range tools {
		name := strings.TrimSpace(stringFromAny(tool["name"]))
		description := strings.TrimSpace(stringFromAny(tool["description"]))
		parameters, ok := mapFromAny(tool["parameters"])
		if name == "" || description == "" || !ok || len(parameters) == 0 {
			invalid++
			continue
		}
		defs[name] = struct{}{}
	}
	return defs, invalid
}

func collectDataShareToolCalls(messages []map[string]any) []dataShareToolCall {
	var out []dataShareToolCall
	for _, msg := range messages {
		for _, call := range anySlice(msg["tool_calls"]) {
			m, ok := mapFromAny(call)
			if !ok {
				continue
			}
			out = append(out, dataShareToolCall{
				id:   strings.TrimSpace(stringFromAny(m["id"])),
				name: strings.TrimSpace(stringFromAny(m["name"])),
			})
		}
	}
	return out
}

func collectDataShareToolResults(messages []map[string]any) map[string]int {
	out := map[string]int{}
	for _, msg := range messages {
		if strings.TrimSpace(stringFromAny(msg["role"])) != "tool" {
			continue
		}
		id := strings.TrimSpace(stringFromAny(msg["tool_call_id"]))
		out[id]++
	}
	return out
}

func hasFinalAssistantMessage(messages []map[string]any) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		role := strings.TrimSpace(stringFromAny(msg["role"]))
		if role == "" {
			continue
		}
		if role != "assistant" {
			return false
		}
		if len(anySlice(msg["tool_calls"])) > 0 {
			return false
		}
		return strings.TrimSpace(dataShareContentText(msg["content"])) != ""
	}
	return false
}

func normalizeDataShareMessages(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		for _, expanded := range expandAnthropicDataShareMessage(msg) {
			normalized := normalizeDataShareMessage(expanded)
			if len(normalized) == 0 {
				continue
			}
			out = append(out, normalized)
		}
	}
	return out
}

// expandAnthropicDataShareMessage 将 Anthropic content block 展开成统一的 message/tool 结构。
func expandAnthropicDataShareMessage(msg map[string]any) []map[string]any {
	if msg == nil {
		return nil
	}
	content := anySlice(msg["content"])
	if len(content) == 0 {
		return []map[string]any{msg}
	}
	role := normalizeResponsesInputRole(stringFromAny(msg["role"]), stringFromAny(msg["type"]))
	switch role {
	case "assistant":
		return expandAnthropicAssistantMessage(msg, content)
	case "user":
		return expandAnthropicUserMessage(msg, content)
	default:
		return []map[string]any{msg}
	}
}

// expandAnthropicAssistantMessage 把 assistant 的 tool_use block 转成标准 tool_calls。
func expandAnthropicAssistantMessage(msg map[string]any, content []any) []map[string]any {
	calls := make([]map[string]any, 0)
	textBlocks := make([]any, 0, len(content))
	for _, raw := range content {
		block, ok := mapFromAny(raw)
		if !ok || strings.TrimSpace(stringFromAny(block["type"])) != "tool_use" {
			textBlocks = append(textBlocks, raw)
			continue
		}
		calls = append(calls, map[string]any{
			"id":        firstNonBlank(stringFromAny(block["id"]), stringFromAny(block["tool_use_id"])),
			"name":      stringFromAny(block["name"]),
			"arguments": firstPresentAny(block["input"], block["arguments"]),
		})
	}
	if len(calls) == 0 {
		return []map[string]any{msg}
	}
	out := cloneDataShareMap(msg)
	out["role"] = "assistant"
	out["tool_calls"] = calls
	out["finish_reason"] = "tool_calls"
	out["content"] = contentValueFromAnthropicBlocks(textBlocks)
	return []map[string]any{out}
}

// expandAnthropicUserMessage 把 user 消息里的 tool_result block 转成标准 tool 消息。
func expandAnthropicUserMessage(msg map[string]any, content []any) []map[string]any {
	out := make([]map[string]any, 0, len(content))
	textBlocks := make([]any, 0, len(content))
	sawToolResult := false
	flushText := func() {
		if len(textBlocks) == 0 {
			return
		}
		textMsg := cloneDataShareMap(msg)
		textMsg["role"] = "user"
		textMsg["content"] = contentValueFromAnthropicBlocks(textBlocks)
		out = append(out, textMsg)
		textBlocks = nil
	}
	for _, raw := range content {
		block, ok := mapFromAny(raw)
		if !ok || strings.TrimSpace(stringFromAny(block["type"])) != "tool_result" {
			textBlocks = append(textBlocks, raw)
			continue
		}
		sawToolResult = true
		flushText()
		out = append(out, map[string]any{
			"role":         "tool",
			"tool_call_id": firstNonBlank(stringFromAny(block["tool_use_id"]), stringFromAny(block["tool_call_id"]), stringFromAny(block["id"])),
			"content":      firstPresentAny(block["content"], block["output"]),
			"is_error":     firstPresentAny(block["is_error"], block["error"]),
			"status":       stringFromAny(block["status"]),
		})
	}
	if !sawToolResult {
		return []map[string]any{msg}
	}
	flushText()
	return out
}

// contentValueFromAnthropicBlocks 提取 Anthropic 文本块中的可读内容。
func contentValueFromAnthropicBlocks(blocks []any) any {
	if len(blocks) == 0 {
		return ""
	}
	return normalizeDataShareContentValue(blocks)
}

// CompactDataShareMessages 压缩 Responses/Codex 每轮请求重复携带的历史消息。
func CompactDataShareMessages(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	seenToolCalls := map[string]struct{}{}
	seenToolResults := map[string]struct{}{}
	for i := 0; i < len(messages); {
		if len(out) > 0 {
			if prefix := dataShareCommonPrefixLen(out, messages[i:]); prefix >= 2 {
				i += prefix
				continue
			}
		}
		msg := messages[i]
		if dataShareMessageAlreadySeen(msg, seenToolCalls, seenToolResults) {
			i++
			continue
		}
		rememberDataShareMessage(msg, seenToolCalls, seenToolResults)
		out = append(out, msg)
		i++
	}
	return out
}

func dataShareMessageAlreadySeen(msg map[string]any, seenToolCalls map[string]struct{}, seenToolResults map[string]struct{}) bool {
	if strings.TrimSpace(stringFromAny(msg["role"])) == "tool" {
		id := strings.TrimSpace(stringFromAny(msg["tool_call_id"]))
		if id == "" {
			return false
		}
		_, ok := seenToolResults[id]
		return ok
	}
	callIDs := dataShareToolCallIDs(msg)
	if len(callIDs) == 0 {
		return false
	}
	for _, id := range callIDs {
		if _, ok := seenToolCalls[id]; !ok {
			return false
		}
	}
	return true
}

func rememberDataShareMessage(msg map[string]any, seenToolCalls map[string]struct{}, seenToolResults map[string]struct{}) {
	if strings.TrimSpace(stringFromAny(msg["role"])) == "tool" {
		if id := strings.TrimSpace(stringFromAny(msg["tool_call_id"])); id != "" {
			seenToolResults[id] = struct{}{}
		}
		return
	}
	for _, id := range dataShareToolCallIDs(msg) {
		seenToolCalls[id] = struct{}{}
	}
}

func dataShareToolCallIDs(msg map[string]any) []string {
	calls := anySlice(msg["tool_calls"])
	out := make([]string, 0, len(calls))
	for _, raw := range calls {
		call, ok := mapFromAny(raw)
		if !ok {
			continue
		}
		if id := strings.TrimSpace(stringFromAny(call["id"])); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func dataShareMessageIdentity(msg map[string]any) string {
	role := strings.TrimSpace(stringFromAny(msg["role"]))
	if role == "" {
		return string(mustJSON(msg))
	}
	if role == "tool" {
		if id := strings.TrimSpace(stringFromAny(msg["tool_call_id"])); id != "" {
			return "tool:" + id
		}
	}
	if role == "assistant" {
		if calls := anySlice(msg["tool_calls"]); len(calls) > 0 {
			return "assistant_tool_calls:" + string(mustJSON(calls))
		}
	}
	return role + ":" + string(mustJSON(msg))
}

func dataShareCommonPrefixLen(left, right []map[string]any) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		if dataShareMessageIdentity(left[i]) != dataShareMessageIdentity(right[i]) {
			return i
		}
	}
	return limit
}

func normalizeDataShareMessage(msg map[string]any) map[string]any {
	if msg == nil {
		return nil
	}
	msgType := strings.TrimSpace(stringFromAny(msg["type"]))
	switch msgType {
	case "function_call":
		return normalizeResponsesFunctionCallMessage(msg)
	case "function_call_output":
		return normalizeToolResultMessage(msg)
	}
	out := cloneDataShareMap(msg)
	role := normalizeResponsesInputRole(stringFromAny(out["role"]), msgType)
	if role != "" {
		out["role"] = role
	}
	if role == "tool" {
		return normalizeToolResultMessage(out)
	}
	if role == "assistant" {
		if calls := normalizeToolCalls(out["tool_calls"]); len(calls) > 0 {
			out["tool_calls"] = calls
			if _, ok := out["finish_reason"]; !ok {
				out["finish_reason"] = "tool_calls"
			}
		} else {
			delete(out, "tool_calls")
		}
	}
	if content, ok := out["content"]; ok {
		out["content"] = normalizeDataShareContentValue(content)
	} else if text := strings.TrimSpace(stringFromAny(out["text"])); text != "" {
		out["content"] = text
	}
	delete(out, "type")
	return out
}

func normalizeResponsesFunctionCallMessage(msg map[string]any) map[string]any {
	functionMap, _ := mapFromAny(msg["function"])
	call := map[string]any{
		"id":        firstNonBlank(stringFromAny(msg["call_id"]), stringFromAny(msg["id"]), stringFromAny(msg["tool_call_id"])),
		"name":      firstNonBlank(stringFromAny(msg["name"]), stringFromAny(functionMap["name"])),
		"arguments": normalizeToolArguments(firstPresentAny(msg["arguments"], functionMap["arguments"], msg["input"])),
	}
	return map[string]any{
		"role":          "assistant",
		"content":       normalizeDataShareContentValue(msg["content"]),
		"tool_calls":    []map[string]any{call},
		"finish_reason": "tool_calls",
	}
}

func normalizeToolResultMessage(msg map[string]any) map[string]any {
	callID := firstNonBlank(
		stringFromAny(msg["tool_call_id"]),
		stringFromAny(msg["call_id"]),
		stringFromAny(msg["tool_use_id"]),
		stringFromAny(msg["id"]),
	)
	content := normalizeDataShareContentValue(firstPresentAny(msg["content"], msg["output"], msg["result"], msg["error"]))
	isError := boolFromAny(msg["is_error"]) || dataShareStatusIsError(stringFromAny(msg["status"])) || dataShareToolContentLooksError(content) || msg["error"] != nil
	status := strings.TrimSpace(stringFromAny(msg["status"]))
	if status == "" {
		if isError {
			status = "error"
		} else {
			status = "success"
		}
	}
	out := map[string]any{
		"role":         "tool",
		"tool_call_id": callID,
		"content":      content,
		"status":       status,
		"is_error":     isError,
	}
	if errMsg := strings.TrimSpace(stringFromAny(msg["error_message"])); errMsg != "" {
		out["error_message"] = errMsg
	}
	return out
}

func normalizeToolCalls(value any) []map[string]any {
	rawCalls := anySlice(value)
	out := make([]map[string]any, 0, len(rawCalls))
	for _, raw := range rawCalls {
		call, ok := mapFromAny(raw)
		if !ok {
			continue
		}
		functionMap, _ := mapFromAny(call["function"])
		arguments := firstPresentAny(call["arguments"], functionMap["arguments"], call["input"])
		if arguments == nil && len(functionMap) > 0 {
			arguments = functionMap
		}
		out = append(out, map[string]any{
			"id":        firstNonBlank(stringFromAny(call["id"]), stringFromAny(call["call_id"]), stringFromAny(call["tool_call_id"])),
			"name":      firstNonBlank(stringFromAny(call["name"]), stringFromAny(functionMap["name"]), stringFromAny(call["type"])),
			"arguments": normalizeToolArguments(arguments),
		})
	}
	return out
}

func normalizeToolArguments(value any) any {
	value = firstPresentAny(value)
	if value == nil {
		return map[string]any{}
	}
	if raw, ok := value.(string); ok {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return map[string]any{}
		}
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return parsed
		}
		return raw
	}
	return value
}

func dataShareStatusIsError(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure":
		return true
	default:
		return false
	}
}

func dataShareToolContentLooksError(content any) bool {
	text := dataShareContentText(content)
	if !strings.Contains(text, "Process exited with code ") {
		return false
	}
	return !strings.Contains(text, "Process exited with code 0")
}

func normalizeDataShareTools(tools []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	seen := map[string]struct{}{}
	var visit func(map[string]any)
	visit = func(tool map[string]any) {
		if nested := mapsFromAny(tool["tools"]); len(nested) > 0 {
			for _, item := range nested {
				visit(item)
			}
		}
		normalized, ok := normalizeDataShareTool(tool)
		if !ok {
			return
		}
		name := stringFromAny(normalized["name"])
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		out = append(out, normalized)
	}
	for _, tool := range tools {
		visit(tool)
	}
	return out
}

func normalizeDataShareTool(tool map[string]any) (map[string]any, bool) {
	if tool == nil {
		return nil, false
	}
	functionMap, _ := mapFromAny(tool["function"])
	name := firstNonBlank(stringFromAny(tool["name"]), stringFromAny(functionMap["name"]), dataShareToolNameFromType(stringFromAny(tool["type"])))
	description := firstNonBlank(stringFromAny(tool["description"]), stringFromAny(functionMap["description"]), defaultDataShareToolDescription(name, stringFromAny(tool["type"])))
	parameters := firstPresentAny(tool["parameters"], functionMap["parameters"], tool["input_schema"], defaultDataShareToolParameters(name, stringFromAny(tool["type"])))
	parameterMap, ok := mapFromAny(parameters)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" || !ok || len(parameterMap) == 0 {
		return nil, false
	}
	out := map[string]any{
		"name":        strings.TrimSpace(name),
		"description": strings.TrimSpace(description),
		"parameters":  parameterMap,
	}
	if toolType := normalizeDataShareToolType(stringFromAny(tool["type"])); toolType != "" {
		out["type"] = toolType
	}
	if strict, ok := tool["strict"]; ok {
		out["strict"] = strict
	}
	return out, true
}

func dataShareToolNameFromType(toolType string) string {
	switch strings.TrimSpace(toolType) {
	case "tool_search":
		return "tool_search"
	case "web_search", "web_search_preview", "web_search_20250305":
		return "web_search"
	default:
		return ""
	}
}

func defaultDataShareToolDescription(name string, toolType string) string {
	switch firstNonBlank(name, toolType) {
	case "apply_patch":
		return "Apply a structured patch to files in the workspace."
	case "tool_search":
		return "Search available deferred tools by text query."
	case "web_search", "web_search_preview", "web_search_20250305":
		return "Search the web for relevant information."
	default:
		return ""
	}
}

func defaultDataShareToolParameters(name string, toolType string) map[string]any {
	switch firstNonBlank(name, dataShareToolNameFromType(toolType)) {
	case "apply_patch":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{"type": "string", "description": "符合 apply_patch 语法的补丁内容。"},
			},
			"required": []string{"patch"},
		}
	case "web_search":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "搜索关键词。"},
			},
			"required": []string{"query"},
		}
	default:
		return nil
	}
}

func normalizeDataShareToolType(toolType string) string {
	switch strings.TrimSpace(toolType) {
	case "function", "custom", "namespace":
		return strings.TrimSpace(toolType)
	case "tool_search", "web_search", "web_search_preview", "web_search_20250305":
		return "function"
	default:
		return ""
	}
}

func normalizeDataShareContentValue(value any) any {
	if value == nil {
		return ""
	}
	if text := dataShareContentText(value); text != "" {
		return text
	}
	return value
}

func dataShareContentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := dataShareContentText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case []map[string]any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := dataShareContentText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "parts", "output", "summary"} {
			if text := dataShareContentText(v[key]); strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

func extractSystemPromptFromMessages(messages []map[string]any) string {
	for _, msg := range messages {
		role := strings.TrimSpace(stringFromAny(msg["role"]))
		if role == "system" || role == "developer" {
			if text := strings.TrimSpace(dataShareContentText(msg["content"])); text != "" {
				return text
			}
		}
	}
	return ""
}

func normalizeDataShareUsage(usage map[string]any) map[string]any {
	out := cloneDataShareMap(usage)
	inputTokens := intFromAny(out["input_tokens"])
	outputTokens := intFromAny(out["output_tokens"])
	cacheReadTokens := intFromAny(firstPresentAny(out["cache_read_input_tokens"], out["cache_read_tokens"]))
	cacheCreateTokens := intFromAny(firstPresentAny(out["cache_creation_input_tokens"], out["cache_creation_tokens"]))
	totalTokens := intFromAny(out["total_tokens"])
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens + cacheReadTokens + cacheCreateTokens
	}
	return map[string]any{
		"input_tokens":                inputTokens,
		"output_tokens":               outputTokens,
		"total_tokens":                totalTokens,
		"cache_creation_input_tokens": cacheCreateTokens,
		"cache_read_input_tokens":     cacheReadTokens,
	}
}

func normalizeDataShareMeta(meta map[string]any) map[string]any {
	out := cloneDataShareMap(meta)
	sourceIDs := appendStringValues(nil, stringsFromAny(out["source_request_ids"])...)
	sourceIDs = appendStringValues(sourceIDs, stringsFromAny(out["request_ids"])...)
	sourceIDs = appendStringValues(sourceIDs, stringFromAny(out["request_id"]))
	out["source_request_ids"] = sourceIDs
	delete(out, "request_ids")
	return out
}

func validateDataSharePayloadQuality(payload map[string]any) []string {
	return ValidateDataShareSessionQuality(
		stringFromAny(payload["model"]),
		stringFromAny(payload["system_prompt"]),
		mapsFromAny(payload["messages"]),
		mapsFromAny(payload["tools"]),
		normalizeDataShareUsage(mapAnyFromAny(payload["usage"])),
	)
}

// DataSharePayloadQualityStatus 把附件质量规则归纳成完整、部分完整、无效三态。
func DataSharePayloadQualityStatus(model string, systemPrompt string, messages []map[string]any, tools []map[string]any, usage map[string]any) string {
	if len(ValidateDataShareSessionQuality(model, systemPrompt, messages, tools, usage)) == 0 {
		return DataShareQualityComplete
	}
	if _, errs := exportableDataShareMessages(model, systemPrompt, messages, tools, usage); len(errs) == 0 {
		return DataShareQualityPartial
	}
	return DataShareQualityInvalid
}

func dataShareCompletionState(qualityStatus string) (string, bool) {
	if qualityStatus == DataShareQualityComplete {
		return DataShareStatusCompleted, true
	}
	return DataShareStatusTerminated, false
}

// DataShareQualityExportable 表示默认导出是否应包含该质量状态。
func DataShareQualityExportable(qualityStatus string) bool {
	return qualityStatus == DataShareQualityComplete || qualityStatus == DataShareQualityPartial
}

// exportableDataShareMessages 仅裁掉尾部未闭合工具链，裁切后仍需完整通过同一套交付校验。
func exportableDataShareMessages(model string, systemPrompt string, messages []map[string]any, tools []map[string]any, usage map[string]any) ([]map[string]any, []string) {
	compact := CompactDataShareMessages(normalizeDataShareMessages(messages))
	for end := len(compact); end >= 0; end-- {
		candidate := compact[:end]
		if !hasFinalAssistantMessage(candidate) {
			continue
		}
		errs := ValidateDataShareSessionQuality(model, systemPrompt, candidate, tools, usage)
		if len(errs) == 0 {
			return append([]map[string]any{}, candidate...), nil
		}
	}
	return nil, ValidateDataShareSessionQuality(model, systemPrompt, compact, tools, usage)
}

func exportPayloadFromSession(session *DataShareSession) map[string]any {
	if session == nil {
		return map[string]any{}
	}
	payload := cloneDataShareMap(session.SessionJSON)
	payload["trajectory_id"] = session.TrajectoryID
	payload["session_id"] = session.SessionID
	payload["dataset"] = session.Dataset
	payload["provider"] = session.Provider
	payload["model"] = session.Model
	payload["request_path"] = firstNonBlank(session.RequestPath, stringFromAny(payload["request_path"]), stringFromAny(session.Meta["request_path"]), stringFromAny(session.Meta["inbound_endpoint"]))
	payload["user_agent"] = firstNonBlank(session.UserAgent, stringFromAny(payload["user_agent"]), stringFromAny(session.Meta["user_agent"]))
	payload["created_at"] = session.CreatedAt.Format(time.RFC3339Nano)
	if session.EndedAt != nil {
		payload["ended_at"] = session.EndedAt.Format(time.RFC3339Nano)
	}
	payload["status"] = session.Status
	payload["is_final_snapshot"] = session.IsFinalSnapshot
	payload["source_request_count"] = session.SourceRequestCount
	messages := CompactDataShareMessages(normalizeDataShareMessages(firstNonEmptyMaps(session.Messages, mapsFromAny(payload["messages"]))))
	tools := normalizeDataShareTools(firstNonEmptyMaps(session.Tools, mapsFromAny(payload["tools"])))
	usage := normalizeDataShareUsage(firstNonEmptyMap(session.Usage, mapAnyFromAny(payload["usage"])))
	meta := normalizeDataShareMeta(firstNonEmptyMap(session.Meta, mapAnyFromAny(payload["meta"])))
	systemPrompt := firstNonBlank(optionalStringValue(session.SystemPrompt), stringFromAny(payload["system_prompt"]), extractSystemPromptFromMessages(messages))
	payload["system_prompt"] = systemPrompt
	payload["tools"] = tools
	payload["messages"] = messages
	payload["usage"] = usage
	payload["meta"] = meta
	delete(payload, "quality_status")
	return payload
}

// BuildDataShareSessionPayload 生成可导出、可压缩持久化的规范 session payload。
func BuildDataShareSessionPayload(session *DataShareSession) map[string]any {
	return exportPayloadFromSession(session)
}

func normalizeDataShareProvider(provider string, apiKey *APIKey) string {
	provider = strings.TrimSpace(provider)
	if provider != "" {
		return provider
	}
	if apiKey != nil && apiKey.Group != nil {
		return apiKey.Group.Platform
	}
	return "unknown"
}

func normalizeDataShareRequestPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func normalizeDataShareUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	// 统计维度只保留客户端产品名，避免版本号、系统架构把同一客户端打散成大量分组。
	if idx := strings.Index(userAgent, "/"); idx > 0 {
		userAgent = strings.TrimSpace(userAgent[:idx])
	}
	if len(userAgent) > 512 {
		return userAgent[:512]
	}
	return userAgent
}

func normalizeDataShareSessionID(sessionID string, requestID string, body []byte, apiKeyID int64) string {
	for _, candidate := range []string{
		sessionID,
		gjson.GetBytes(body, "session_id").String(),
		gjson.GetBytes(body, "conversation_id").String(),
		gjson.GetBytes(body, "metadata.session_id").String(),
		gjson.GetBytes(body, "metadata.conversation_id").String(),
		gjson.GetBytes(body, "metadata.prompt_cache_key").String(),
		gjson.GetBytes(body, "prompt_cache_key").String(),
		gjson.GetBytes(body, "metadata.user_id").String(),
		requestID,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	sum := sha256.Sum256(append(body, []byte(strconv.FormatInt(apiKeyID, 10))...))
	return hex.EncodeToString(sum[:16])
}

func buildTrajectoryID(provider string, sessionID string, apiKeyID int64, groupID int64) string {
	seed := fmt.Sprintf("%s:%s:%d:%d", provider, sessionID, apiKeyID, groupID)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

func extractSystemPromptFromRequest(body []byte) string {
	sys := gjson.GetBytes(body, "system")
	if sys.Exists() {
		if sys.Type == gjson.String {
			return sys.String()
		}
		return sys.Raw
	}
	if sys = gjson.GetBytes(body, "system_instruction"); sys.Exists() {
		return sys.Raw
	}
	return ""
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func optionalStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func firstPresentAny(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func boolFromAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	default:
		return false
	}
}

func mapFromAny(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		return x, true
	default:
		return nil, false
	}
}

func mapAnyFromAny(v any) map[string]any {
	if m, ok := mapFromAny(v); ok {
		return m
	}
	return nil
}

func mapsFromAny(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m, ok := mapFromAny(item); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func anySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func stringsFromAny(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(stringFromAny(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		return []string{x}
	default:
		return nil
	}
}

func appendStringValues(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	out := make([]string, 0, len(existing)+len(values))
	for _, item := range existing {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func cloneDataShareMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmptyMaps(values ...[]map[string]any) []map[string]any {
	for _, v := range values {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

func firstNonEmptyMap(values ...map[string]any) map[string]any {
	for _, v := range values {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

func rawJSONToMap(raw string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{"raw": raw}
	}
	return out
}

func rawJSONToAny(raw string) any {
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return raw
	}
	return out
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	default:
		return 0
	}
}

// WriteSingleSessionJSONL 输出单条 session 的原始 JSONL，供详情页下载和问题排查使用。
func WriteSingleSessionJSONL(w io.Writer, session *DataShareSession) error {
	if session == nil {
		return ErrDataShareSessionNotFound
	}
	var buf bytes.Buffer
	payload := exportPayloadFromSession(session)
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := buf.Write(line); err != nil {
		return err
	}
	if err := buf.WriteByte('\n'); err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

func IsDataShareNotFound(err error) bool {
	return errors.Is(err, ErrDataShareSessionNotFound)
}

func cloneDataSharingRequestBody(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	return append([]byte(nil), body...)
}
