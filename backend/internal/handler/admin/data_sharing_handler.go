package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/pkg/response"
	"github.com/TokenFlux/TokenRouter/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

// DataSharingHandler 处理管理端数据共享须知、session 管理、导出和统计。
type DataSharingHandler struct {
	dataSharingService *service.DataSharingService
}

// NewDataSharingHandler 创建管理端数据共享处理器。
func NewDataSharingHandler(dataSharingService *service.DataSharingService) *DataSharingHandler {
	return &DataSharingHandler{dataSharingService: dataSharingService}
}

// UpdateDataSharingNoticeRequest 是管理端更新数据共享须知的请求。
type UpdateDataSharingNoticeRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateDataShareSkipRulesRequest 是管理端更新采集跳过规则的请求。
type UpdateDataShareSkipRulesRequest struct {
	Rules []service.DataShareCaptureSkipRule `json:"rules"`
}

// UpdateDataShareStorageLimitRequest 是管理端更新数据共享空间阈值的请求。
type UpdateDataShareStorageLimitRequest struct {
	LimitBytes int64 `json:"limit_bytes"`
}

// BatchDeleteDataShareSessionsRequest 是管理端批量删除数据共享 session 的请求。
type BatchDeleteDataShareSessionsRequest struct {
	IDs []int64 `json:"ids"`
}

type adminDataShareSessionResponse struct {
	ID                 int64            `json:"id"`
	TrajectoryID       string           `json:"trajectory_id"`
	SessionID          string           `json:"session_id"`
	Dataset            string           `json:"dataset"`
	Provider           string           `json:"provider"`
	Model              string           `json:"model"`
	RequestPath        string           `json:"request_path"`
	UserAgent          string           `json:"user_agent"`
	Status             string           `json:"status"`
	IsFinalSnapshot    bool             `json:"is_final_snapshot"`
	SourceRequestCount int              `json:"source_request_count"`
	SystemPrompt       *string          `json:"system_prompt,omitempty"`
	Tools              []map[string]any `json:"tools,omitempty"`
	Messages           []map[string]any `json:"messages,omitempty"`
	Usage              map[string]any   `json:"usage,omitempty"`
	Meta               map[string]any   `json:"meta,omitempty"`
	SessionJSON        map[string]any   `json:"session_json,omitempty"`
	PayloadEncoding    string           `json:"payload_encoding,omitempty"`
	PayloadBytes       int64            `json:"payload_bytes,omitempty"`
	Exportable         bool             `json:"exportable"`
	QualityStatus      string           `json:"quality_status"`
	QualityErrors      []string         `json:"quality_errors"`
	StorageBytes       int64            `json:"storage_bytes"`
	InputTokens        int64            `json:"input_tokens"`
	OutputTokens       int64            `json:"output_tokens"`
	TotalTokens        int64            `json:"total_tokens"`
	UserID             int64            `json:"user_id"`
	UserName           string           `json:"user_name,omitempty"`
	UserEmail          string           `json:"user_email,omitempty"`
	APIKeyID           int64            `json:"api_key_id"`
	APIKeyName         string           `json:"api_key_name,omitempty"`
	GroupID            int64            `json:"group_id"`
	GroupName          string           `json:"group_name,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	EndedAt            *time.Time       `json:"ended_at,omitempty"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// adminDataShareExportTicketResponse 是管理端触发浏览器原生下载所需的票据。
type adminDataShareExportTicketResponse struct {
	Token       string    `json:"token"`
	DownloadURL string    `json:"download_url"`
	Filename    string    `json:"filename"`
	Encoding    string    `json:"encoding"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// GetNotice 返回当前数据共享须知。
func (h *DataSharingHandler) GetNotice(c *gin.Context) {
	notice, err := h.dataSharingService.GetNotice(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, notice)
}

// UpdateNotice 更新数据共享须知并递增版本。
func (h *DataSharingHandler) UpdateNotice(c *gin.Context) {
	var req UpdateDataSharingNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	notice, err := h.dataSharingService.UpdateNotice(c.Request.Context(), req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, notice)
}

// GetSkipRules 返回当前生效的数据共享采集跳过规则。
func (h *DataSharingHandler) GetSkipRules(c *gin.Context) {
	rules, err := h.dataSharingService.GetCaptureSkipRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

// UpdateSkipRules 保存管理端维护的数据共享采集跳过规则。
func (h *DataSharingHandler) UpdateSkipRules(c *gin.Context) {
	var req UpdateDataShareSkipRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rules, err := h.dataSharingService.UpdateCaptureSkipRules(c.Request.Context(), req.Rules)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

// GetStorageLimit 返回数据共享采集空间阈值和当前占用。
func (h *DataSharingHandler) GetStorageLimit(c *gin.Context) {
	limit, err := h.dataSharingService.GetStorageLimit(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, limit)
}

// UpdateStorageLimit 保存数据共享采集空间阈值。
func (h *DataSharingHandler) UpdateStorageLimit(c *gin.Context) {
	var req UpdateDataShareStorageLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	limit, err := h.dataSharingService.UpdateStorageLimit(c.Request.Context(), req.LimitBytes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, limit)
}

// ListSessions 查询所有数据共享 session。
func (h *DataSharingHandler) ListSessions(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filters, ok := parseAdminDataShareFilters(c)
	if !ok {
		return
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.dataSharingService.ListSessions(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]adminDataShareSessionResponse, 0, len(items))
	for i := range items {
		out = append(out, adminDataShareSessionToResponse(&items[i], false))
	}
	total := int64(0)
	if result != nil {
		total = result.Total
	}
	response.Paginated(c, out, total, page, pageSize)
}

// FilterOptions 返回管理端数据共享列表的全量筛选选项。
func (h *DataSharingHandler) FilterOptions(c *gin.Context) {
	options, err := h.dataSharingService.FilterOptions(c.Request.Context(), service.DataShareSessionFilters{})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, options)
}

// GetSession 返回单条数据共享 session 详情。
func (h *DataSharingHandler) GetSession(c *gin.Context) {
	id, err := parseAdminDataShareIDParam(c)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}
	session, err := h.dataSharingService.GetSession(c.Request.Context(), id, 0)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, adminDataShareSessionToResponse(session, true))
}

// DeleteSession 删除单条数据共享 session。
func (h *DataSharingHandler) DeleteSession(c *gin.Context) {
	id, err := parseAdminDataShareIDParam(c)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}
	if err := h.dataSharingService.DeleteSession(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// BatchDeleteSessions 按 ID 或当前筛选条件批量删除数据共享 session。
func (h *DataSharingHandler) BatchDeleteSessions(c *gin.Context) {
	var req BatchDeleteDataShareSessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	filters, ok := parseAdminDataShareFilters(c)
	if !ok {
		return
	}
	if filters.SelectAll {
		req.IDs = nil
	} else if len(req.IDs) == 0 {
		response.BadRequest(c, "ids or select_all is required")
		return
	}
	affected, err := h.dataSharingService.BatchDeleteSessions(c.Request.Context(), req.IDs, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": affected})
}

// CreateExportTicket 按筛选条件签发管理端数据共享下载票据。
func (h *DataSharingHandler) CreateExportTicket(c *gin.Context) {
	filters, ok := parseAdminDataShareFilters(c)
	if !ok {
		return
	}
	if filters.SelectAll {
		filters.IDs = nil
	} else if len(filters.IDs) == 0 {
		response.BadRequest(c, "ids or select_all is required")
		return
	}
	ticket, err := h.dataSharingService.CreateExportTicket(c.Request.Context(), service.DataShareExportTicketRequest{
		Scope:    service.DataShareExportScopeAdmin,
		Filters:  filters,
		Filename: fmt.Sprintf("admin-data-sharing-%s", time.Now().Format("20060102-150405")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, adminDataShareExportTicketToResponse(ticket))
}

// CreateSessionExportTicket 为管理端单条 session 签发未压缩 JSON 下载票据。
func (h *DataSharingHandler) CreateSessionExportTicket(c *gin.Context) {
	id, err := parseAdminDataShareIDParam(c)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}
	ticket, err := h.dataSharingService.CreateExportTicket(c.Request.Context(), service.DataShareExportTicketRequest{
		Scope:    service.DataShareExportScopeAdmin,
		Filters:  service.DataShareSessionFilters{IDs: []int64{id}},
		Filename: fmt.Sprintf("admin-data-sharing-session-%d", id),
		Encoding: service.DataShareExportEncodingJSON,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, adminDataShareExportTicketToResponse(ticket))
}

// DownloadExport 使用短期票据下载 JSONL 或 zstd 压缩后的 JSONL。
func (h *DataSharingHandler) DownloadExport(c *gin.Context) {
	claims, err := h.dataSharingService.ParseExportTicket(c.Request.Context(), service.DataShareExportScopeAdmin, strings.TrimSpace(c.Query("ticket")))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if claims.Encoding == service.DataShareExportEncodingJSON {
		writeAdminDataSharePlainJSON(c, claims.Filename, func() error {
			return h.dataSharingService.ExportJSONL(c.Request.Context(), c.Writer, claims.Filters, false)
		})
		return
	}
	if claims.Encoding == service.DataShareExportEncodingJSONL {
		writeAdminDataSharePlainJSONL(c, claims.Filename, func() error {
			return h.dataSharingService.ExportJSONL(c.Request.Context(), c.Writer, claims.Filters, false)
		})
		return
	}
	writeAdminDataShareZstdJSONL(c, claims.Filename, func(zw *zstd.Encoder) error {
		return h.dataSharingService.ExportJSONL(c.Request.Context(), zw, claims.Filters, false)
	})
}

// Stats 返回管理端数据共享统计和图表数据。
func (h *DataSharingHandler) Stats(c *gin.Context) {
	filters, ok := parseAdminDataShareFilters(c)
	if !ok {
		return
	}
	stats, err := h.dataSharingService.Stats(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func parseAdminDataShareIDParam(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func parseAdminDataShareFilters(c *gin.Context) (service.DataShareSessionFilters, bool) {
	var filters service.DataShareSessionFilters
	ids, ok := parseAdminDataShareIDsQuery(c)
	if !ok {
		return filters, false
	}
	filters.IDs = ids
	for _, item := range []struct {
		key string
		set func(int64)
	}{
		{key: "user_id", set: func(v int64) { filters.UserID = v }},
		{key: "api_key_id", set: func(v int64) { filters.APIKeyID = v }},
		{key: "group_id", set: func(v int64) { filters.GroupID = v }},
	} {
		raw := strings.TrimSpace(c.Query(item.key))
		if raw == "" {
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid "+item.key)
			return filters, false
		}
		item.set(v)
	}
	filters.Provider = strings.TrimSpace(c.Query("provider"))
	filters.Model = strings.TrimSpace(c.Query("model"))
	filters.RequestPath = strings.TrimSpace(c.Query("request_path"))
	filters.UserAgent = strings.TrimSpace(c.Query("user_agent"))
	filters.Search = strings.TrimSpace(c.Query("search"))
	filters.UserName = strings.TrimSpace(c.Query("user_name"))
	filters.APIKeyName = strings.TrimSpace(c.Query("api_key_name"))
	filters.GroupName = strings.TrimSpace(c.Query("group_name"))
	if selectAll, ok := parseAdminDataShareBoolQuery(c, "select_all"); ok {
		filters.SelectAll = selectAll
	} else {
		return filters, false
	}
	excludeIDs, ok := parseAdminDataShareIDsQueryKey(c, "exclude_ids")
	if !ok {
		return filters, false
	}
	filters.ExcludeIDs = excludeIDs
	if raw := strings.TrimSpace(c.Query("quality_status")); raw != "" && raw != "all" {
		filters.QualityStatus = raw
	}
	if raw := strings.TrimSpace(c.Query("exportable")); raw != "" && raw != "all" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid exportable value, use true or false")
			return filters, false
		}
		filters.Exportable = &v
	}
	start, err := parseAdminDataShareTimeQuery(c, "start_date", "start_at")
	if err != nil {
		response.BadRequest(c, "Invalid start_date format")
		return filters, false
	}
	end, err := parseAdminDataShareTimeQuery(c, "end_date", "end_at")
	if err != nil {
		response.BadRequest(c, "Invalid end_date format")
		return filters, false
	}
	filters.StartTime = start
	filters.EndTime = end
	return filters, true
}

func parseAdminDataShareIDsQuery(c *gin.Context) ([]int64, bool) {
	return parseAdminDataShareIDsQueryKey(c, "ids")
}

func parseAdminDataShareIDsQueryKey(c *gin.Context, key string) ([]int64, bool) {
	rawValues := c.QueryArray(key)
	seen := make(map[int64]struct{}, len(rawValues))
	ids := make([]int64, 0, len(rawValues))
	for _, rawValue := range rawValues {
		for _, raw := range strings.Split(rawValue, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				response.BadRequest(c, "Invalid "+key)
				return nil, false
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, true
}

func parseAdminDataShareBoolQuery(c *gin.Context, key string) (bool, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return false, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		response.BadRequest(c, "Invalid "+key)
		return false, false
	}
	return v, true
}

func parseAdminDataShareTimeQuery(c *gin.Context, keys ...string) (*time.Time, error) {
	raw := ""
	for _, key := range keys {
		raw = strings.TrimSpace(c.Query(key))
		if raw != "" {
			break
		}
	}
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			if layout == "2006-01-02" && strings.Contains(keys[0], "end") {
				t = t.AddDate(0, 0, 1)
			}
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid time")
}

func adminDataShareSessionToResponse(session *service.DataShareSession, includePayload bool) adminDataShareSessionResponse {
	if session == nil {
		return adminDataShareSessionResponse{}
	}
	resp := adminDataShareSessionResponse{
		ID:                 session.ID,
		TrajectoryID:       session.TrajectoryID,
		SessionID:          session.SessionID,
		Dataset:            session.Dataset,
		Provider:           session.Provider,
		Model:              session.Model,
		RequestPath:        session.RequestPath,
		UserAgent:          session.UserAgent,
		Status:             session.Status,
		IsFinalSnapshot:    session.IsFinalSnapshot,
		SourceRequestCount: session.SourceRequestCount,
		SystemPrompt:       session.SystemPrompt,
		PayloadEncoding:    session.PayloadEncoding,
		PayloadBytes:       session.PayloadBytes,
		Exportable:         session.Exportable,
		QualityStatus:      session.QualityStatus,
		QualityErrors:      session.QualityErrors,
		StorageBytes:       session.StorageBytes,
		InputTokens:        session.InputTokens,
		OutputTokens:       session.OutputTokens,
		TotalTokens:        session.TotalTokens,
		UserID:             session.UserID,
		UserName:           session.UserName,
		UserEmail:          session.UserEmail,
		APIKeyID:           session.APIKeyID,
		APIKeyName:         session.APIKeyName,
		GroupID:            session.GroupID,
		GroupName:          session.GroupName,
		CreatedAt:          session.CreatedAt,
		EndedAt:            session.EndedAt,
		UpdatedAt:          session.UpdatedAt,
	}
	if includePayload {
		resp.Tools = session.Tools
		resp.Messages = session.Messages
		resp.Usage = session.Usage
		resp.Meta = session.Meta
		resp.SessionJSON = session.SessionJSON
	}
	return resp
}

func adminDataShareExportTicketToResponse(ticket *service.DataShareExportTicket) adminDataShareExportTicketResponse {
	if ticket == nil {
		return adminDataShareExportTicketResponse{}
	}
	return adminDataShareExportTicketResponse{
		Token:       ticket.Token,
		DownloadURL: ticket.DownloadURL,
		Filename:    ticket.Filename,
		Encoding:    ticket.Encoding,
		ExpiresAt:   ticket.ExpiresAt,
	}
}

func writeAdminDataSharePlainJSON(c *gin.Context, filename string, write func() error) {
	if filename == "" {
		filename = fmt.Sprintf("admin-data-sharing-%s.json", time.Now().Format("20060102-150405"))
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if err := write(); err != nil {
		_ = c.Error(err)
	}
}

func writeAdminDataSharePlainJSONL(c *gin.Context, filename string, write func() error) {
	if filename == "" {
		filename = fmt.Sprintf("admin-data-sharing-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if err := write(); err != nil {
		_ = c.Error(err)
	}
}

func writeAdminDataShareZstdJSONL(c *gin.Context, filename string, write func(*zstd.Encoder) error) {
	if filename == "" {
		filename = fmt.Sprintf("admin-data-sharing-%s.jsonl.zst", time.Now().Format("20060102-150405"))
	}
	c.Header("Content-Type", "application/zstd")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	zw, err := zstd.NewWriter(c.Writer)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := write(zw); err != nil {
		_ = zw.Close()
		_ = c.Error(err)
		return
	}
	if err := zw.Close(); err != nil {
		_ = c.Error(err)
		return
	}
}
