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
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/errors"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/tidwall/gjson"
)

const (
	DataShareStatusCompleted = "completed"
	defaultDataShareDataset  = "tokenrouter-agent"
)

var (
	ErrDataShareSessionNotFound = infraerrors.NotFound("DATA_SHARE_SESSION_NOT_FOUND", "data share session not found")
	ErrDataShareNoticeMissing   = infraerrors.BadRequest("DATA_SHARE_NOTICE_MISSING", "data sharing notice content is required")
)

const defaultDataSharingNoticeContent = "该分组已启用数据共享。使用该分组产生的 Agent 对话数据会被保存，并可能用于训练、评估和改进模型。请确认你已理解并同意该数据共享安排。"

// DataShareNotice 是用户切换到数据共享分组前需要确认的须知。
type DataShareNotice struct {
	Content   string    `json:"content"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DataShareSession 保存一条聚合后的 Agent session。
type DataShareSession struct {
	ID                 int64
	TrajectoryID       string
	SessionID          string
	Dataset            string
	Provider           string
	Model              string
	Status             string
	IsFinalSnapshot    bool
	SourceRequestCount int
	SystemPrompt       *string
	Tools              []map[string]any
	Messages           []map[string]any
	Usage              map[string]any
	Meta               map[string]any
	SessionJSON        map[string]any
	Exportable         bool
	QualityErrors      []string
	StorageBytes       int64
	InputTokens        int64
	OutputTokens       int64
	TotalTokens        int64
	UserID             int64
	APIKeyID           int64
	GroupID            int64
	CreatedAt          time.Time
	EndedAt            *time.Time
	UpdatedAt          time.Time
}

// DataShareSessionFilters 描述列表/统计/导出筛选条件。
type DataShareSessionFilters struct {
	UserID     int64
	APIKeyID   int64
	GroupID    int64
	Provider   string
	Model      string
	Exportable *bool
	StartTime  *time.Time
	EndTime    *time.Time
	Search     string
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

// DataShareStats 是管理端数据共享概览指标。
type DataShareStats struct {
	SessionCount          int64                        `json:"session_count"`
	ExportableCount       int64                        `json:"exportable_count"`
	NonExportableCount    int64                        `json:"non_exportable_count"`
	TotalStorageBytes     int64                        `json:"total_storage_bytes"`
	TotalTokens           int64                        `json:"total_tokens"`
	AvgTokensPerSession   float64                      `json:"avg_tokens_per_session"`
	StorageTrend          []DataShareStoragePoint      `json:"storage_trend"`
	GroupStorageBreakdown []DataShareGroupStoragePoint `json:"group_storage_breakdown"`
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
	GetByID(ctx context.Context, id int64) (*DataShareSession, error)
	Delete(ctx context.Context, id int64) error
	BatchDelete(ctx context.Context, ids []int64, filters DataShareSessionFilters) (int64, error)
	Stats(ctx context.Context, filters DataShareSessionFilters) (*DataShareStats, error)
}

// DataSharingService 负责数据共享须知、采集、导出和统计。
type DataSharingService struct {
	repo        DataShareSessionRepository
	settingRepo SettingRepository
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

// CaptureClaudeRequest 采集 Claude/Gemini 兼容协议成功请求。
func (s *DataSharingService) CaptureClaudeRequest(ctx context.Context, input DataShareCaptureInput) error {
	if input.APIKey == nil || input.APIKey.Group == nil || !input.APIKey.Group.DataSharingEnabled {
		return nil
	}
	if s == nil || s.repo == nil {
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

// ExportJSONL 按附件要求导出 JSONL。默认只导出 exportable=true 的记录。
func (s *DataSharingService) ExportJSONL(ctx context.Context, w io.Writer, filters DataShareSessionFilters, includeNonExportable bool) error {
	if !includeNonExportable && filters.Exportable == nil {
		v := true
		filters.Exportable = &v
	}
	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "created_at", SortOrder: pagination.SortOrderAsc}
	for {
		items, result, err := s.repo.List(ctx, params, filters)
		if err != nil {
			return err
		}
		for i := range items {
			line, err := json.Marshal(exportPayloadFromSession(&items[i]))
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
	qualityErrors := validateDataShareQuality(model, messages, tools, usage)
	sessionJSON := map[string]any{
		"trajectory_id":        trajectoryID,
		"session_id":           sessionID,
		"dataset":              defaultDataShareDataset,
		"provider":             provider,
		"model":                model,
		"created_at":           now.Format(time.RFC3339Nano),
		"ended_at":             now.Format(time.RFC3339Nano),
		"status":               DataShareStatusCompleted,
		"is_final_snapshot":    true,
		"source_request_count": 1,
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
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		SystemPrompt:       sysPtr,
		Tools:              tools,
		Messages:           messages,
		Usage:              usage,
		Meta:               meta,
		SessionJSON:        sessionJSON,
		Exportable:         len(qualityErrors) == 0,
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
	return out
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
		msg["role"] = "assistant"
		if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
			msg["tool_call_id"] = callID
		}
	case "function_call_output":
		// 工具执行结果按 tool 消息保存，便于后续训练流水线识别。
		msg["role"] = "tool"
		if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
			msg["tool_call_id"] = callID
		}
		if output := item.Get("output"); output.Exists() {
			msg["content"] = responseInputContentValue(output)
		}
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
	return rawJSONToAny(value.Raw)
}

func appendAssistantMessageFromResponse(out []map[string]any, body []byte) []map[string]any {
	if msg := gjson.GetBytes(body, "choices.0.message"); msg.Exists() {
		out = append(out, rawJSONToMap(msg.Raw))
	}
	if output := gjson.GetBytes(body, "output"); output.IsArray() {
		out = append(out, map[string]any{"role": "assistant", "content": rawJSONToAny(output.Raw)})
	}
	if content := gjson.GetBytes(body, "content"); content.IsArray() {
		out = append(out, map[string]any{"role": "assistant", "content": rawJSONToAny(content.Raw)})
	}
	if candidates := gjson.GetBytes(body, "candidates.0.content"); candidates.Exists() {
		msg := rawJSONToMap(candidates.Raw)
		msg["role"] = "assistant"
		out = append(out, msg)
	}
	return out
}

func normalizeCaptureTools(input DataShareCaptureInput) []map[string]any {
	if len(input.Tools) > 0 {
		return input.Tools
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
	return out
}

func buildCaptureUsage(input DataShareCaptureInput) map[string]any {
	totalInput := input.InputTokens + input.CacheReadTokens + input.CacheCreateTokens
	total := totalInput + input.OutputTokens
	return map[string]any{
		"input_tokens":          input.InputTokens,
		"output_tokens":         input.OutputTokens,
		"cache_read_tokens":     input.CacheReadTokens,
		"cache_creation_tokens": input.CacheCreateTokens,
		"total_tokens":          total,
	}
}

func buildCaptureMeta(input DataShareCaptureInput) map[string]any {
	meta := map[string]any{
		"api_key_id":        int64(0),
		"group_id":          int64(0),
		"account_id":        int64(0),
		"request_id":        input.RequestID,
		"requested_model":   firstNonBlank(input.Model, gjson.GetBytes(input.RequestBody, "model").String()),
		"inbound_endpoint":  input.InboundEndpoint,
		"upstream_endpoint": input.UpstreamEndpoint,
		"user_agent":        input.UserAgent,
		"ip_address":        input.IPAddress,
	}
	if input.APIKey != nil {
		meta["api_key_id"] = input.APIKey.ID
		if input.APIKey.GroupID != nil {
			meta["group_id"] = *input.APIKey.GroupID
		}
	}
	if input.Account != nil {
		meta["account_id"] = input.Account.ID
	}
	return meta
}

func resolveDataShareActualModel(input DataShareCaptureInput) string {
	// 正式交付要求 model 等于实际生成模型；映射后的上游模型优先，客户端请求模型只放入 meta。
	return firstNonBlank(input.UpstreamModel, input.Model, gjson.GetBytes(input.RequestBody, "model").String())
}

func validateDataShareQuality(model string, messages []map[string]any, tools []map[string]any, usage map[string]any) []string {
	var errs []string
	if len(messages) < 2 {
		errs = append(errs, "effective_turns_lt_2")
	}
	if len(tools) == 0 {
		errs = append(errs, "missing_structured_tool_call")
	}
	if !dataShareModelAllowed(model) {
		errs = append(errs, "model_not_allowed")
	}
	if intFromAny(usage["total_tokens"]) <= 0 {
		errs = append(errs, "missing_usage_tokens")
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

func exportPayloadFromSession(session *DataShareSession) map[string]any {
	if session == nil {
		return map[string]any{}
	}
	payload := session.SessionJSON
	if payload == nil {
		payload = map[string]any{}
	}
	payload["trajectory_id"] = session.TrajectoryID
	payload["session_id"] = session.SessionID
	payload["dataset"] = session.Dataset
	payload["provider"] = session.Provider
	payload["model"] = session.Model
	payload["created_at"] = session.CreatedAt.Format(time.RFC3339Nano)
	if session.EndedAt != nil {
		payload["ended_at"] = session.EndedAt.Format(time.RFC3339Nano)
	}
	payload["status"] = session.Status
	payload["is_final_snapshot"] = session.IsFinalSnapshot
	payload["source_request_count"] = session.SourceRequestCount
	if session.SystemPrompt != nil {
		payload["system_prompt"] = *session.SystemPrompt
	}
	payload["tools"] = session.Tools
	payload["messages"] = session.Messages
	payload["usage"] = session.Usage
	payload["meta"] = session.Meta
	return payload
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

// WriteSingleSessionJSONL 输出单条 session 的 JSONL，供详情页下载使用。
func WriteSingleSessionJSONL(w io.Writer, session *DataShareSession) error {
	if session == nil {
		return ErrDataShareSessionNotFound
	}
	var buf bytes.Buffer
	line, err := json.Marshal(exportPayloadFromSession(session))
	if err != nil {
		return err
	}
	buf.Write(line)
	buf.WriteByte('\n')
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
