package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/datasharesession"
	"github.com/TokenFlux/TokenRouter/ent/predicate"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type dataShareSessionRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewDataShareSessionRepository(client *dbent.Client, sqlDB *sql.DB) service.DataShareSessionRepository {
	return &dataShareSessionRepository{client: client, sql: sqlDB}
}

func (r *dataShareSessionRepository) sqlExecutorFromContext(ctx context.Context) sqlExecutor {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.sql
}

func (r *dataShareSessionRepository) UpsertCapture(ctx context.Context, session *service.DataShareSession) error {
	if session == nil {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	existing, err := client.DataShareSession.Query().
		Where(datasharesession.TrajectoryIDEQ(session.TrajectoryID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return err
	}
	if dbent.IsNotFound(err) || existing == nil {
		builder := client.DataShareSession.Create().
			SetTrajectoryID(session.TrajectoryID).
			SetSessionID(session.SessionID).
			SetDataset(session.Dataset).
			SetProvider(session.Provider).
			SetModel(session.Model).
			SetStatus(session.Status).
			SetIsFinalSnapshot(session.IsFinalSnapshot).
			SetSourceRequestCount(session.SourceRequestCount).
			SetNillableSystemPrompt(session.SystemPrompt).
			SetTools(session.Tools).
			SetMessages(session.Messages).
			SetUsage(session.Usage).
			SetMeta(session.Meta).
			SetSessionJSON(session.SessionJSON).
			SetExportable(session.Exportable).
			SetQualityErrors(session.QualityErrors).
			SetStorageBytes(session.StorageBytes).
			SetInputTokens(session.InputTokens).
			SetOutputTokens(session.OutputTokens).
			SetTotalTokens(session.TotalTokens).
			SetUserID(session.UserID).
			SetAPIKeyID(session.APIKeyID).
			SetGroupID(session.GroupID).
			SetCreatedAt(now).
			SetNillableEndedAt(session.EndedAt).
			SetUpdatedAt(now)
		_, err := builder.Save(ctx)
		return err
	}

	messages := append(existing.Messages, session.Messages...)
	tools := mergeDataShareTools(existing.Tools, session.Tools)
	usage := mergeDataShareUsage(existing.Usage, session.Usage)
	meta := mergeDataShareMeta(existing.Meta, session.Meta)
	sourceRequestCount := existing.SourceRequestCount + 1
	inputTokens := existing.InputTokens + session.InputTokens
	outputTokens := existing.OutputTokens + session.OutputTokens
	totalTokens := existing.TotalTokens + session.TotalTokens
	sessionJSON := session.SessionJSON
	if sessionJSON == nil {
		sessionJSON = map[string]any{}
	}
	sessionJSON["source_request_count"] = sourceRequestCount
	sessionJSON["messages"] = messages
	sessionJSON["tools"] = tools
	sessionJSON["usage"] = usage
	sessionJSON["meta"] = meta
	sessionJSON["created_at"] = existing.CreatedAt.Format(time.RFC3339Nano)
	sessionJSON["ended_at"] = now.Format(time.RFC3339Nano)
	storageBytes := int64(len(mustRepositoryJSON(sessionJSON)))
	// 每次合并后都重新评估质量，避免早期空消息请求留下的错误阻止后续导出。
	qualityErrors := validateRepositoryQuality(session.Model, messages, tools, usage)

	_, err = client.DataShareSession.Update().
		Where(datasharesession.IDEQ(existing.ID)).
		SetModel(session.Model).
		SetStatus(session.Status).
		SetIsFinalSnapshot(session.IsFinalSnapshot).
		SetSourceRequestCount(sourceRequestCount).
		SetNillableSystemPrompt(firstSystemPrompt(existing.SystemPrompt, session.SystemPrompt)).
		SetTools(tools).
		SetMessages(messages).
		SetUsage(usage).
		SetMeta(meta).
		SetSessionJSON(sessionJSON).
		SetExportable(len(qualityErrors) == 0).
		SetQualityErrors(qualityErrors).
		SetStorageBytes(storageBytes).
		SetInputTokens(inputTokens).
		SetOutputTokens(outputTokens).
		SetTotalTokens(totalTokens).
		SetNillableEndedAt(session.EndedAt).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *dataShareSessionRepository) List(ctx context.Context, params pagination.PaginationParams, filters service.DataShareSessionFilters) ([]service.DataShareSession, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := applyDataShareFilters(client.DataShareSession.Query(), filters)
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	query := q.Offset(params.Offset()).Limit(params.Limit())
	for _, order := range dataShareListOrder(params) {
		query = query.Order(order)
	}
	items, err := query.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.DataShareSession, 0, len(items))
	for i := range items {
		out = append(out, *dataShareSessionEntityToService(items[i]))
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *dataShareSessionRepository) GetByID(ctx context.Context, id int64) (*service.DataShareSession, error) {
	item, err := clientFromContext(ctx, r.client).DataShareSession.Query().
		Where(datasharesession.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrDataShareSessionNotFound, nil)
	}
	return dataShareSessionEntityToService(item), nil
}

func (r *dataShareSessionRepository) Delete(ctx context.Context, id int64) error {
	affected, err := clientFromContext(ctx, r.client).DataShareSession.Delete().
		Where(datasharesession.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrDataShareSessionNotFound
	}
	return nil
}

func (r *dataShareSessionRepository) BatchDelete(ctx context.Context, ids []int64, filters service.DataShareSessionFilters) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	q := clientFromContext(ctx, r.client).DataShareSession.Delete()
	if preds := dataSharePredicates(filters); len(preds) > 0 {
		q = q.Where(preds...)
	}
	affected, err := q.Where(datasharesession.IDIn(ids...)).Exec(ctx)
	return int64(affected), err
}

func (r *dataShareSessionRepository) Stats(ctx context.Context, filters service.DataShareSessionFilters) (*service.DataShareStats, error) {
	sqlq := r.sqlExecutorFromContext(ctx)
	whereSQL, args := dataShareStatsWhere(filters)
	stats := &service.DataShareStats{}
	if err := scanSingleRow(ctx, sqlq, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE exportable = TRUE),
			COUNT(*) FILTER (WHERE exportable = FALSE),
			COALESCE(SUM(storage_bytes), 0),
			COALESCE(SUM(total_tokens), 0)
		FROM data_share_sessions
		`+whereSQL,
		args,
		&stats.SessionCount,
		&stats.ExportableCount,
		&stats.NonExportableCount,
		&stats.TotalStorageBytes,
		&stats.TotalTokens,
	); err != nil {
		return nil, err
	}
	if stats.SessionCount > 0 {
		stats.AvgTokensPerSession = float64(stats.TotalTokens) / float64(stats.SessionCount)
	}
	trend, err := r.loadStorageTrend(ctx, sqlq, whereSQL, args)
	if err != nil {
		return nil, err
	}
	stats.StorageTrend = trend
	breakdown, err := r.loadGroupStorageBreakdown(ctx, sqlq, whereSQL, args)
	if err != nil {
		return nil, err
	}
	stats.GroupStorageBreakdown = breakdown
	return stats, nil
}

func (r *dataShareSessionRepository) loadStorageTrend(ctx context.Context, sqlq sqlExecutor, whereSQL string, args []any) ([]service.DataShareStoragePoint, error) {
	rows, err := sqlq.QueryContext(ctx, `
		SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS day,
		       COALESCE(SUM(storage_bytes), 0),
		       COUNT(*)
		FROM data_share_sessions
		`+whereSQL+`
		GROUP BY day
		ORDER BY day ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []service.DataShareStoragePoint
	for rows.Next() {
		var p service.DataShareStoragePoint
		if err := rows.Scan(&p.Date, &p.StorageBytes, &p.SessionCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *dataShareSessionRepository) loadGroupStorageBreakdown(ctx context.Context, sqlq sqlExecutor, whereSQL string, args []any) ([]service.DataShareGroupStoragePoint, error) {
	whereSQL = prefixDataShareWhereAlias(whereSQL, "d")
	rows, err := sqlq.QueryContext(ctx, `
		SELECT d.group_id, COALESCE(g.name, ''), COALESCE(SUM(d.storage_bytes), 0), COUNT(*)
		FROM data_share_sessions d
		LEFT JOIN groups g ON g.id = d.group_id
		`+whereSQL+`
		GROUP BY d.group_id, g.name
		ORDER BY COALESCE(SUM(d.storage_bytes), 0) DESC
		LIMIT 20
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []service.DataShareGroupStoragePoint
	for rows.Next() {
		var p service.DataShareGroupStoragePoint
		if err := rows.Scan(&p.GroupID, &p.GroupName, &p.StorageBytes, &p.SessionCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func prefixDataShareWhereAlias(whereSQL, alias string) string {
	if strings.TrimSpace(whereSQL) == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"user_id", alias+".user_id",
		"api_key_id", alias+".api_key_id",
		"group_id", alias+".group_id",
		"provider", alias+".provider",
		"model", alias+".model",
		"exportable", alias+".exportable",
		"created_at", alias+".created_at",
		"trajectory_id", alias+".trajectory_id",
		"session_id", alias+".session_id",
	)
	return replacer.Replace(whereSQL)
}

func applyDataShareFilters(q *dbent.DataShareSessionQuery, filters service.DataShareSessionFilters) *dbent.DataShareSessionQuery {
	preds := dataSharePredicates(filters)
	if len(preds) > 0 {
		q = q.Where(preds...)
	}
	return q
}

func dataSharePredicates(filters service.DataShareSessionFilters) []predicate.DataShareSession {
	var preds []predicate.DataShareSession
	if filters.UserID > 0 {
		preds = append(preds, datasharesession.UserIDEQ(filters.UserID))
	}
	if filters.APIKeyID > 0 {
		preds = append(preds, datasharesession.APIKeyIDEQ(filters.APIKeyID))
	}
	if filters.GroupID > 0 {
		preds = append(preds, datasharesession.GroupIDEQ(filters.GroupID))
	}
	if filters.Provider != "" {
		preds = append(preds, datasharesession.ProviderEQ(filters.Provider))
	}
	if filters.Model != "" {
		preds = append(preds, datasharesession.ModelContainsFold(filters.Model))
	}
	if filters.Exportable != nil {
		preds = append(preds, datasharesession.ExportableEQ(*filters.Exportable))
	}
	if filters.StartTime != nil {
		preds = append(preds, datasharesession.CreatedAtGTE(*filters.StartTime))
	}
	if filters.EndTime != nil {
		preds = append(preds, datasharesession.CreatedAtLT(*filters.EndTime))
	}
	if filters.Search != "" {
		preds = append(preds, datasharesession.Or(
			datasharesession.TrajectoryIDContainsFold(filters.Search),
			datasharesession.SessionIDContainsFold(filters.Search),
			datasharesession.ModelContainsFold(filters.Search),
		))
	}
	return preds
}

func dataShareStatsWhere(filters service.DataShareSessionFilters) (string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if filters.UserID > 0 {
		add("user_id = $%d", filters.UserID)
	}
	if filters.APIKeyID > 0 {
		add("api_key_id = $%d", filters.APIKeyID)
	}
	if filters.GroupID > 0 {
		add("group_id = $%d", filters.GroupID)
	}
	if filters.Provider != "" {
		add("provider = $%d", filters.Provider)
	}
	if filters.Model != "" {
		add("model ILIKE '%%' || $%d || '%%'", filters.Model)
	}
	if filters.Exportable != nil {
		add("exportable = $%d", *filters.Exportable)
	}
	if filters.StartTime != nil {
		add("created_at >= $%d", *filters.StartTime)
	}
	if filters.EndTime != nil {
		add("created_at < $%d", *filters.EndTime)
	}
	if filters.Search != "" {
		// 搜索条件复用同一个参数，避免手工补多个占位符时出现编号错位。
		args = append(args, filters.Search)
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf("(trajectory_id ILIKE '%%' || $%d || '%%' OR session_id ILIKE '%%' || $%d || '%%' OR model ILIKE '%%' || $%d || '%%')", idx, idx, idx))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func dataShareListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	field := datasharesession.FieldCreatedAt
	switch sortBy {
	case "storage_bytes":
		field = datasharesession.FieldStorageBytes
	case "total_tokens":
		field = datasharesession.FieldTotalTokens
	case "updated_at":
		field = datasharesession.FieldUpdatedAt
	case "model":
		field = datasharesession.FieldModel
	case "provider":
		field = datasharesession.FieldProvider
	case "id":
		field = datasharesession.FieldID
	}
	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(datasharesession.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(datasharesession.FieldID)}
}

func dataShareSessionEntityToService(m *dbent.DataShareSession) *service.DataShareSession {
	if m == nil {
		return nil
	}
	return &service.DataShareSession{
		ID:                 m.ID,
		TrajectoryID:       m.TrajectoryID,
		SessionID:          m.SessionID,
		Dataset:            m.Dataset,
		Provider:           m.Provider,
		Model:              m.Model,
		Status:             m.Status,
		IsFinalSnapshot:    m.IsFinalSnapshot,
		SourceRequestCount: m.SourceRequestCount,
		SystemPrompt:       m.SystemPrompt,
		Tools:              m.Tools,
		Messages:           m.Messages,
		Usage:              m.Usage,
		Meta:               m.Meta,
		SessionJSON:        m.SessionJSON,
		Exportable:         m.Exportable,
		QualityErrors:      m.QualityErrors,
		StorageBytes:       m.StorageBytes,
		InputTokens:        m.InputTokens,
		OutputTokens:       m.OutputTokens,
		TotalTokens:        m.TotalTokens,
		UserID:             m.UserID,
		APIKeyID:           m.APIKeyID,
		GroupID:            m.GroupID,
		CreatedAt:          m.CreatedAt,
		EndedAt:            m.EndedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func mergeDataShareTools(existing, incoming []map[string]any) []map[string]any {
	out := append([]map[string]any{}, existing...)
	seen := make(map[string]struct{}, len(existing))
	for _, tool := range existing {
		seen[string(mustRepositoryJSON(tool))] = struct{}{}
	}
	for _, tool := range incoming {
		key := string(mustRepositoryJSON(tool))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tool)
	}
	return out
}

func mergeDataShareUsage(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(incoming))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range incoming {
		out[k] = int64FromAny(out[k]) + int64FromAny(v)
	}
	return out
}

func mergeDataShareMeta(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(incoming))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range incoming {
		out[k] = v
	}
	if existingID, ok := existing["request_id"].(string); ok {
		if incomingID, ok := incoming["request_id"].(string); ok && incomingID != "" && incomingID != existingID {
			out["request_ids"] = appendStringAny(out["request_ids"], existingID, incomingID)
		}
	}
	return out
}

func appendStringAny(v any, values ...string) []string {
	seen := make(map[string]struct{})
	var out []string
	if arr, ok := v.([]string); ok {
		for _, item := range arr {
			if item == "" {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	for _, item := range values {
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

func firstSystemPrompt(existing, incoming *string) *string {
	if existing != nil && strings.TrimSpace(*existing) != "" {
		return existing
	}
	return incoming
}

func validateRepositoryQuality(model string, messages []map[string]any, tools []map[string]any, usage map[string]any) []string {
	var errs []string
	if len(messages) < 2 {
		errs = append(errs, "effective_turns_lt_2")
	}
	if len(tools) == 0 {
		errs = append(errs, "missing_structured_tool_call")
	}
	if !dataShareRepositoryModelAllowed(model) {
		errs = append(errs, "model_not_allowed")
	}
	if int64FromAny(usage["total_tokens"]) <= 0 {
		errs = append(errs, "missing_usage_tokens")
	}
	return errs
}

func dataShareRepositoryModelAllowed(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	// 与 service 层数据共享导出白名单保持一致，避免 upsert 合并后放宽质量门槛。
	return strings.Contains(model, "gpt-5") ||
		strings.Contains(model, "claude") && (strings.Contains(model, "4.5") || strings.Contains(model, "4-5")) ||
		strings.Contains(model, "gemini-3")
}

func int64FromAny(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case jsonNumber:
		i, _ := x.Int64()
		return i
	default:
		return 0
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}

func mustRepositoryJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}
