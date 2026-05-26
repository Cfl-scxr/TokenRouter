package admin

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/pkg/response"
	"github.com/TokenFlux/TokenRouter/internal/service"

	"github.com/gin-gonic/gin"
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

// BatchDeleteDataShareSessionsRequest 是管理端批量删除数据共享 session 的请求。
type BatchDeleteDataShareSessionsRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

type adminDataShareSessionResponse struct {
	ID                 int64            `json:"id"`
	TrajectoryID       string           `json:"trajectory_id"`
	SessionID          string           `json:"session_id"`
	Dataset            string           `json:"dataset"`
	Provider           string           `json:"provider"`
	Model              string           `json:"model"`
	Status             string           `json:"status"`
	IsFinalSnapshot    bool             `json:"is_final_snapshot"`
	SourceRequestCount int              `json:"source_request_count"`
	SystemPrompt       *string          `json:"system_prompt,omitempty"`
	Tools              []map[string]any `json:"tools,omitempty"`
	Messages           []map[string]any `json:"messages,omitempty"`
	Usage              map[string]any   `json:"usage,omitempty"`
	Meta               map[string]any   `json:"meta,omitempty"`
	SessionJSON        map[string]any   `json:"session_json,omitempty"`
	Exportable         bool             `json:"exportable"`
	QualityStatus      string           `json:"quality_status"`
	QualityErrors      []string         `json:"quality_errors"`
	StorageBytes       int64            `json:"storage_bytes"`
	InputTokens        int64            `json:"input_tokens"`
	OutputTokens       int64            `json:"output_tokens"`
	TotalTokens        int64            `json:"total_tokens"`
	UserID             int64            `json:"user_id"`
	APIKeyID           int64            `json:"api_key_id"`
	GroupID            int64            `json:"group_id"`
	CreatedAt          time.Time        `json:"created_at"`
	EndedAt            *time.Time       `json:"ended_at,omitempty"`
	UpdatedAt          time.Time        `json:"updated_at"`
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

// BatchDeleteSessions 按 ID 批量删除数据共享 session。
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
	affected, err := h.dataSharingService.BatchDeleteSessions(c.Request.Context(), req.IDs, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": affected})
}

// ExportSessions 按筛选条件导出 JSONL。
func (h *DataSharingHandler) ExportSessions(c *gin.Context) {
	filters, ok := parseAdminDataShareFilters(c)
	if !ok {
		return
	}
	var buf bytes.Buffer
	if err := h.dataSharingService.ExportJSONL(c.Request.Context(), &buf, filters, false); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	writeAdminDataShareJSONL(c, "admin-data-sharing", buf.Bytes())
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
	filters.Search = strings.TrimSpace(c.Query("search"))
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
		Status:             session.Status,
		IsFinalSnapshot:    session.IsFinalSnapshot,
		SourceRequestCount: session.SourceRequestCount,
		SystemPrompt:       session.SystemPrompt,
		Exportable:         session.Exportable,
		QualityStatus:      session.QualityStatus,
		QualityErrors:      session.QualityErrors,
		StorageBytes:       session.StorageBytes,
		InputTokens:        session.InputTokens,
		OutputTokens:       session.OutputTokens,
		TotalTokens:        session.TotalTokens,
		UserID:             session.UserID,
		APIKeyID:           session.APIKeyID,
		GroupID:            session.GroupID,
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

func writeAdminDataShareJSONL(c *gin.Context, prefix string, data []byte) {
	filename := fmt.Sprintf("%s-%s.jsonl", prefix, time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(200, "application/x-ndjson; charset=utf-8", data)
}
