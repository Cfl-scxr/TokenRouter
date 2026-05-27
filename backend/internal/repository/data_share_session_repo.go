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
	"github.com/lib/pq"

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
			SetRequestPath(session.RequestPath).
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
			SetQualityStatus(session.QualityStatus).
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

	messages := mergeDataShareMessages(existing.Messages, session.Messages)
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
	sessionJSON["request_path"] = firstNonBlankRepository(session.RequestPath, existing.RequestPath)
	sessionJSON["messages"] = messages
	sessionJSON["tools"] = tools
	sessionJSON["usage"] = usage
	sessionJSON["meta"] = meta
	sessionJSON["created_at"] = existing.CreatedAt.Format(time.RFC3339Nano)
	sessionJSON["ended_at"] = now.Format(time.RFC3339Nano)
	storageBytes := int64(len(mustRepositoryJSON(sessionJSON)))
	systemPrompt := firstSystemPrompt(existing.SystemPrompt, session.SystemPrompt)
	// 每次合并后都重新评估质量，避免早期空消息请求留下的错误阻止后续导出。
	qualityErrors := validateRepositoryQuality(session.Model, systemPrompt, messages, tools, usage)
	qualityStatus := repositoryQualityStatus(session.Model, systemPrompt, messages, tools, usage)
	status, finalSnapshot := repositoryCompletionState(qualityStatus)
	sessionJSON["status"] = status
	sessionJSON["is_final_snapshot"] = finalSnapshot
	sessionJSON["quality_status"] = qualityStatus

	_, err = client.DataShareSession.Update().
		Where(datasharesession.IDEQ(existing.ID)).
		SetModel(session.Model).
		SetRequestPath(firstNonBlankRepository(session.RequestPath, existing.RequestPath)).
		SetStatus(status).
		SetIsFinalSnapshot(finalSnapshot).
		SetSourceRequestCount(sourceRequestCount).
		SetNillableSystemPrompt(systemPrompt).
		SetTools(tools).
		SetMessages(messages).
		SetUsage(usage).
		SetMeta(meta).
		SetSessionJSON(sessionJSON).
		SetExportable(service.DataShareQualityExportable(qualityStatus)).
		SetQualityStatus(qualityStatus).
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
	if err := r.hydrateDisplayNames(ctx, out); err != nil {
		return nil, nil, err
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
	out := dataShareSessionEntityToService(item)
	if out == nil {
		return nil, service.ErrDataShareSessionNotFound
	}
	items := []service.DataShareSession{*out}
	if err := r.hydrateDisplayNames(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
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
	if len(ids) == 0 && !filters.SelectAll {
		return 0, nil
	}
	if filters.SelectAll {
		filters.IDs = nil
	} else {
		filters.IDs = ids
	}
	q := clientFromContext(ctx, r.client).DataShareSession.Delete()
	if preds := dataSharePredicates(filters); len(preds) > 0 {
		q = q.Where(preds...)
	}
	affected, err := q.Exec(ctx)
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
			COUNT(*) FILTER (WHERE quality_status = 'complete'),
			COUNT(*) FILTER (WHERE quality_status = 'partial'),
			COUNT(*) FILTER (WHERE quality_status = 'invalid'),
			COALESCE(SUM(storage_bytes), 0),
			COALESCE(SUM(total_tokens), 0)
		FROM data_share_sessions
		`+whereSQL,
		args,
		&stats.SessionCount,
		&stats.ExportableCount,
		&stats.NonExportableCount,
		&stats.CompleteCount,
		&stats.PartialCount,
		&stats.InvalidCount,
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
	pathBreakdown, err := r.loadRequestPathBreakdown(ctx, sqlq, whereSQL, args)
	if err != nil {
		return nil, err
	}
	stats.RequestPathBreakdown = pathBreakdown
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

func (r *dataShareSessionRepository) loadRequestPathBreakdown(ctx context.Context, sqlq sqlExecutor, whereSQL string, args []any) ([]service.DataShareRequestPathPoint, error) {
	rows, err := sqlq.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(request_path, ''), '(unknown)') AS request_path,
		       COALESCE(SUM(storage_bytes), 0),
		       COUNT(*),
		       COALESCE(SUM(total_tokens), 0)
		FROM data_share_sessions
		`+whereSQL+`
		GROUP BY COALESCE(NULLIF(request_path, ''), '(unknown)')
		ORDER BY COUNT(*) DESC, COALESCE(SUM(storage_bytes), 0) DESC
		LIMIT 20
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []service.DataShareRequestPathPoint
	for rows.Next() {
		var p service.DataShareRequestPathPoint
		if err := rows.Scan(&p.RequestPath, &p.StorageBytes, &p.SessionCount, &p.TotalTokens); err != nil {
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
		"request_path", alias+".request_path",
		"exportable", alias+".exportable",
		"quality_status", alias+".quality_status",
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
	if len(filters.IDs) > 0 {
		preds = append(preds, datasharesession.IDIn(filters.IDs...))
	}
	if len(filters.ExcludeIDs) > 0 {
		preds = append(preds, datasharesession.IDNotIn(filters.ExcludeIDs...))
	}
	if filters.UserID > 0 {
		preds = append(preds, datasharesession.UserIDEQ(filters.UserID))
	}
	if filters.UserName != "" {
		preds = append(preds, dataShareRelatedNamePredicate("users", "user_id", []string{"username", "email"}, filters.UserName))
	}
	if filters.APIKeyID > 0 {
		preds = append(preds, datasharesession.APIKeyIDEQ(filters.APIKeyID))
	}
	if filters.APIKeyName != "" {
		preds = append(preds, dataShareRelatedNamePredicate("api_keys", "api_key_id", []string{"name"}, filters.APIKeyName))
	}
	if filters.GroupID > 0 {
		preds = append(preds, datasharesession.GroupIDEQ(filters.GroupID))
	}
	if filters.GroupName != "" {
		preds = append(preds, dataShareRelatedNamePredicate("groups", "group_id", []string{"name"}, filters.GroupName))
	}
	if filters.Provider != "" {
		preds = append(preds, datasharesession.ProviderEQ(filters.Provider))
	}
	if filters.Model != "" {
		preds = append(preds, datasharesession.ModelContainsFold(filters.Model))
	}
	if filters.RequestPath != "" {
		preds = append(preds, datasharesession.RequestPathEQ(filters.RequestPath))
	}
	if filters.Exportable != nil {
		preds = append(preds, datasharesession.ExportableEQ(*filters.Exportable))
	}
	if filters.QualityStatus != "" {
		preds = append(preds, datasharesession.QualityStatusEQ(filters.QualityStatus))
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
			datasharesession.RequestPathContainsFold(filters.Search),
			dataShareRelatedNamePredicate("users", "user_id", []string{"username", "email"}, filters.Search),
			dataShareRelatedNamePredicate("api_keys", "api_key_id", []string{"name"}, filters.Search),
			dataShareRelatedNamePredicate("groups", "group_id", []string{"name"}, filters.Search),
		))
	}
	return preds
}

func dataShareRelatedNamePredicate(table string, localField string, columns []string, keyword string) predicate.DataShareSession {
	return predicate.DataShareSession(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("EXISTS (SELECT 1 FROM ").
				WriteString(table).
				WriteString(" rel WHERE rel.id = ").
				WriteString(s.C(localField)).
				WriteString(" AND (")
			for i, column := range columns {
				if i > 0 {
					b.WriteString(" OR ")
				}
				b.WriteString("LOWER(").
					WriteString("rel.").
					WriteString(column).
					WriteString(") LIKE '%' || LOWER(").
					Arg(keyword).
					WriteString(") || '%'")
			}
			b.WriteString("))")
		}))
	})
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
	if filters.UserName != "" {
		addDataShareRelatedNameStatsClause(&clauses, &args, "users", "user_id", []string{"username", "email"}, filters.UserName)
	}
	if filters.APIKeyID > 0 {
		add("api_key_id = $%d", filters.APIKeyID)
	}
	if filters.APIKeyName != "" {
		addDataShareRelatedNameStatsClause(&clauses, &args, "api_keys", "api_key_id", []string{"name"}, filters.APIKeyName)
	}
	if filters.GroupID > 0 {
		add("group_id = $%d", filters.GroupID)
	}
	if filters.GroupName != "" {
		addDataShareRelatedNameStatsClause(&clauses, &args, "groups", "group_id", []string{"name"}, filters.GroupName)
	}
	if filters.Provider != "" {
		add("provider = $%d", filters.Provider)
	}
	if filters.Model != "" {
		add("model ILIKE '%%' || $%d || '%%'", filters.Model)
	}
	if filters.RequestPath != "" {
		add("request_path = $%d", filters.RequestPath)
	}
	if filters.Exportable != nil {
		add("exportable = $%d", *filters.Exportable)
	}
	if filters.QualityStatus != "" {
		add("quality_status = $%d", filters.QualityStatus)
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
		clauses = append(clauses, fmt.Sprintf(`(
			trajectory_id ILIKE '%%' || $%d || '%%'
			OR session_id ILIKE '%%' || $%d || '%%'
			OR model ILIKE '%%' || $%d || '%%'
			OR request_path ILIKE '%%' || $%d || '%%'
			OR EXISTS (SELECT 1 FROM users u WHERE u.id = user_id AND (u.username ILIKE '%%' || $%d || '%%' OR u.email ILIKE '%%' || $%d || '%%'))
			OR EXISTS (SELECT 1 FROM api_keys ak WHERE ak.id = api_key_id AND ak.name ILIKE '%%' || $%d || '%%')
			OR EXISTS (SELECT 1 FROM groups g WHERE g.id = group_id AND g.name ILIKE '%%' || $%d || '%%')
		)`, idx, idx, idx, idx, idx, idx, idx, idx))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func addDataShareRelatedNameStatsClause(clauses *[]string, args *[]any, table string, localField string, columns []string, keyword string) {
	*args = append(*args, keyword)
	idx := len(*args)
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("LOWER(rel.%s) LIKE '%%' || LOWER($%d) || '%%'", column, idx))
	}
	*clauses = append(*clauses, fmt.Sprintf("EXISTS (SELECT 1 FROM %s rel WHERE rel.id = %s AND (%s))", table, localField, strings.Join(parts, " OR ")))
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
	case "request_path":
		field = datasharesession.FieldRequestPath
	case "quality_status":
		field = datasharesession.FieldQualityStatus
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
		RequestPath:        m.RequestPath,
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
		QualityStatus:      m.QualityStatus,
		QualityErrors:      m.QualityErrors,
		StorageBytes:       m.StorageBytes,
		InputTokens:        m.InputTokens,
		OutputTokens:       m.OutputTokens,
		TotalTokens:        m.TotalTokens,
		UserID:             m.UserID,
		UserName:           stringFromRepositoryAny(m.Meta["user_name"]),
		UserEmail:          stringFromRepositoryAny(m.Meta["user_email"]),
		APIKeyID:           m.APIKeyID,
		APIKeyName:         stringFromRepositoryAny(m.Meta["api_key_name"]),
		GroupID:            m.GroupID,
		GroupName:          stringFromRepositoryAny(m.Meta["group_name"]),
		CreatedAt:          m.CreatedAt,
		EndedAt:            m.EndedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func (r *dataShareSessionRepository) hydrateDisplayNames(ctx context.Context, items []service.DataShareSession) error {
	if len(items) == 0 {
		return nil
	}
	sqlq := r.sqlExecutorFromContext(ctx)
	userIDs := make([]int64, 0, len(items))
	apiKeyIDs := make([]int64, 0, len(items))
	groupIDs := make([]int64, 0, len(items))
	for i := range items {
		if items[i].UserID > 0 {
			userIDs = append(userIDs, items[i].UserID)
		}
		if items[i].APIKeyID > 0 {
			apiKeyIDs = append(apiKeyIDs, items[i].APIKeyID)
		}
		if items[i].GroupID > 0 {
			groupIDs = append(groupIDs, items[i].GroupID)
		}
	}
	users, err := dataShareLoadUserNames(ctx, sqlq, uniqueInt64s(userIDs))
	if err != nil {
		return err
	}
	keys, err := dataShareLoadIDNames(ctx, sqlq, "api_keys", "name", uniqueInt64s(apiKeyIDs))
	if err != nil {
		return err
	}
	groups, err := dataShareLoadIDNames(ctx, sqlq, "groups", "name", uniqueInt64s(groupIDs))
	if err != nil {
		return err
	}
	for i := range items {
		if info, ok := users[items[i].UserID]; ok {
			if strings.TrimSpace(items[i].UserName) == "" {
				items[i].UserName = info.Name
			}
			if strings.TrimSpace(items[i].UserEmail) == "" {
				items[i].UserEmail = info.Email
			}
		}
		if name := keys[items[i].APIKeyID]; strings.TrimSpace(items[i].APIKeyName) == "" && name != "" {
			items[i].APIKeyName = name
		}
		if name := groups[items[i].GroupID]; strings.TrimSpace(items[i].GroupName) == "" && name != "" {
			items[i].GroupName = name
		}
		if items[i].Meta == nil {
			items[i].Meta = map[string]any{}
		}
		items[i].Meta["user_name"] = items[i].UserName
		items[i].Meta["user_email"] = items[i].UserEmail
		items[i].Meta["api_key_name"] = items[i].APIKeyName
		items[i].Meta["group_name"] = items[i].GroupName
		if items[i].SessionJSON != nil {
			meta, _ := items[i].SessionJSON["meta"].(map[string]any)
			if meta == nil {
				meta = map[string]any{}
			}
			meta["user_name"] = items[i].UserName
			meta["user_email"] = items[i].UserEmail
			meta["api_key_name"] = items[i].APIKeyName
			meta["group_name"] = items[i].GroupName
			items[i].SessionJSON["meta"] = meta
		}
	}
	return nil
}

type dataShareUserDisplay struct {
	Name  string
	Email string
}

func dataShareLoadUserNames(ctx context.Context, sqlq sqlExecutor, ids []int64) (map[int64]dataShareUserDisplay, error) {
	out := make(map[int64]dataShareUserDisplay)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := sqlq.QueryContext(ctx, `
		SELECT id, COALESCE(NULLIF(username, ''), email, ''), COALESCE(email, '')
		FROM users
		WHERE id = ANY($1)
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var item dataShareUserDisplay
		if err := rows.Scan(&id, &item.Name, &item.Email); err != nil {
			return nil, err
		}
		out[id] = item
	}
	return out, rows.Err()
}

func dataShareLoadIDNames(ctx context.Context, sqlq sqlExecutor, table string, nameColumn string, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if len(ids) == 0 {
		return out, nil
	}
	query := fmt.Sprintf("SELECT id, COALESCE(%s, '') FROM %s WHERE id = ANY($1)", nameColumn, table)
	rows, err := sqlq.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
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

func mergeDataShareMessages(existing, incoming []map[string]any) []map[string]any {
	out := append([]map[string]any{}, existing...)
	seenToolCalls := map[string]struct{}{}
	seenToolResults := map[string]struct{}{}
	for _, msg := range out {
		rememberDataShareMessage(msg, seenToolCalls, seenToolResults)
	}
	for len(incoming) > 0 {
		if prefix := dataShareCommonPrefixLen(out, incoming); prefix >= 2 {
			incoming = incoming[prefix:]
			continue
		}
		msg := incoming[0]
		if !dataShareMessageAlreadySeen(msg, seenToolCalls, seenToolResults) {
			rememberDataShareMessage(msg, seenToolCalls, seenToolResults)
			out = append(out, msg)
		}
		incoming = incoming[1:]
	}
	return out
}

func dataShareMessageAlreadySeen(msg map[string]any, seenToolCalls map[string]struct{}, seenToolResults map[string]struct{}) bool {
	if strings.TrimSpace(stringFromRepositoryAny(msg["role"])) == "tool" {
		id := strings.TrimSpace(stringFromRepositoryAny(msg["tool_call_id"]))
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
	if strings.TrimSpace(stringFromRepositoryAny(msg["role"])) == "tool" {
		if id := strings.TrimSpace(stringFromRepositoryAny(msg["tool_call_id"])); id != "" {
			seenToolResults[id] = struct{}{}
		}
		return
	}
	for _, id := range dataShareToolCallIDs(msg) {
		seenToolCalls[id] = struct{}{}
	}
}

func dataShareToolCallIDs(msg map[string]any) []string {
	switch calls := msg["tool_calls"].(type) {
	case []map[string]any:
		out := make([]string, 0, len(calls))
		for _, call := range calls {
			if id := strings.TrimSpace(stringFromRepositoryAny(call["id"])); id != "" {
				out = append(out, id)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(calls))
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if id := strings.TrimSpace(stringFromRepositoryAny(call["id"])); id != "" {
				out = append(out, id)
			}
		}
		return out
	default:
		return nil
	}
}

func dataShareMessageIdentity(msg map[string]any) string {
	role := strings.TrimSpace(stringFromRepositoryAny(msg["role"]))
	if role == "" {
		return string(mustRepositoryJSON(msg))
	}
	if role == "tool" {
		if id := strings.TrimSpace(stringFromRepositoryAny(msg["tool_call_id"])); id != "" {
			return "tool:" + id
		}
	}
	if role == "assistant" {
		if calls, ok := msg["tool_calls"].([]any); ok && len(calls) > 0 {
			return "assistant_tool_calls:" + string(mustRepositoryJSON(calls))
		}
		if calls, ok := msg["tool_calls"].([]map[string]any); ok && len(calls) > 0 {
			return "assistant_tool_calls:" + string(mustRepositoryJSON(calls))
		}
	}
	return role + ":" + string(mustRepositoryJSON(msg))
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
	sourceIDs := appendStringAny(nil, stringsFromRepositoryAny(existing["source_request_ids"])...)
	sourceIDs = appendStringAny(sourceIDs, stringsFromRepositoryAny(existing["request_ids"])...)
	sourceIDs = appendStringAny(sourceIDs, stringsFromRepositoryAny(incoming["source_request_ids"])...)
	sourceIDs = appendStringAny(sourceIDs, stringsFromRepositoryAny(incoming["request_ids"])...)
	sourceIDs = appendStringAny(sourceIDs, stringFromRepositoryAny(existing["request_id"]), stringFromRepositoryAny(incoming["request_id"]))
	out["source_request_ids"] = sourceIDs
	delete(out, "request_ids")
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

func stringsFromRepositoryAny(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(stringFromRepositoryAny(item)); text != "" {
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

func stringFromRepositoryAny(v any) string {
	if text, ok := v.(string); ok {
		return text
	}
	return ""
}

func firstSystemPrompt(existing, incoming *string) *string {
	if existing != nil && strings.TrimSpace(*existing) != "" {
		return existing
	}
	return incoming
}

func firstNonBlankRepository(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func validateRepositoryQuality(model string, systemPrompt *string, messages []map[string]any, tools []map[string]any, usage map[string]any) []string {
	prompt := ""
	if systemPrompt != nil {
		prompt = *systemPrompt
	}
	return service.ValidateDataShareSessionQuality(model, prompt, messages, tools, usage)
}

func repositoryQualityStatus(model string, systemPrompt *string, messages []map[string]any, tools []map[string]any, usage map[string]any) string {
	prompt := ""
	if systemPrompt != nil {
		prompt = *systemPrompt
	}
	return service.DataSharePayloadQualityStatus(model, prompt, messages, tools, usage)
}

func repositoryCompletionState(qualityStatus string) (string, bool) {
	if qualityStatus == service.DataShareQualityComplete {
		return service.DataShareStatusCompleted, true
	}
	return service.DataShareStatusTerminated, false
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
