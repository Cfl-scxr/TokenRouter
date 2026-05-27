package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/datasharesession"
	"github.com/TokenFlux/TokenRouter/ent/enttest"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newDataShareSessionRepoSQLite(t *testing.T) (*dataShareSessionRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &dataShareSessionRepository{client: client, sql: db}, client
}

func TestDataShareSessionRepository_RequestPathFilter(t *testing.T) {
	repo, _ := newDataShareSessionRepoSQLite(t)
	ctx := context.Background()

	now := time.Now().UTC()
	require.NoError(t, repo.UpsertCapture(ctx, &service.DataShareSession{
		TrajectoryID:       "traj-responses",
		SessionID:          "sess-responses",
		Dataset:            "tokenrouter-agent",
		Provider:           service.PlatformOpenAI,
		Model:              "gpt-5.5",
		RequestPath:        "/v1/responses",
		UserAgent:          "codex-cli/1.0",
		Status:             service.DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		Tools:              []map[string]any{},
		Messages:           []map[string]any{},
		Usage:              map[string]any{},
		Meta:               map[string]any{"request_path": "/v1/responses", "user_agent": "codex-cli/1.0"},
		SessionJSON:        map[string]any{"request_path": "/v1/responses", "user_agent": "codex-cli/1.0"},
		QualityStatus:      service.DataShareQualityInvalid,
		QualityErrors:      []string{},
		StorageBytes:       100,
		TotalTokens:        10,
		UserID:             0,
		APIKeyID:           0,
		GroupID:            0,
		CreatedAt:          now,
		EndedAt:            &now,
		UpdatedAt:          now,
	}))
	require.NoError(t, repo.UpsertCapture(ctx, &service.DataShareSession{
		TrajectoryID:       "traj-chat",
		SessionID:          "sess-chat",
		Dataset:            "tokenrouter-agent",
		Provider:           service.PlatformOpenAI,
		Model:              "gpt-5.5",
		RequestPath:        "/v1/chat/completions",
		UserAgent:          "claude-code/2.0",
		Status:             service.DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		Tools:              []map[string]any{},
		Messages:           []map[string]any{},
		Usage:              map[string]any{},
		Meta:               map[string]any{"request_path": "/v1/chat/completions", "user_agent": "claude-code/2.0"},
		SessionJSON:        map[string]any{"request_path": "/v1/chat/completions", "user_agent": "claude-code/2.0"},
		QualityStatus:      service.DataShareQualityInvalid,
		QualityErrors:      []string{},
		StorageBytes:       200,
		TotalTokens:        20,
		UserID:             0,
		APIKeyID:           0,
		GroupID:            0,
		CreatedAt:          now,
		EndedAt:            &now,
		UpdatedAt:          now,
	}))

	q := applyDataShareFilters(repo.client.DataShareSession.Query(), service.DataShareSessionFilters{RequestPath: "/v1/chat/completions"})
	total, err := q.Clone().Count(ctx)
	require.NoError(t, err)
	items, err := q.All(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "/v1/chat/completions", items[0].RequestPath)

	searchQ := applyDataShareFilters(repo.client.DataShareSession.Query(), service.DataShareSessionFilters{Search: "responses"})
	searchItems, err := searchQ.All(ctx)
	require.NoError(t, err)
	require.Len(t, searchItems, 1)
	require.Equal(t, "/v1/responses", searchItems[0].RequestPath)

	uaQ := applyDataShareFilters(repo.client.DataShareSession.Query(), service.DataShareSessionFilters{UserAgent: "claude-code/2.0"})
	uaItems, err := uaQ.All(ctx)
	require.NoError(t, err)
	require.Len(t, uaItems, 1)
	require.Equal(t, "claude-code/2.0", uaItems[0].UserAgent)

	modelQ := applyDataShareFilters(repo.client.DataShareSession.Query(), service.DataShareSessionFilters{Model: "gpt-5.5"})
	modelItems, err := modelQ.All(ctx)
	require.NoError(t, err)
	require.Len(t, modelItems, 2)
}

func TestDataShareSessionRepository_RequestPathStats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &dataShareSessionRepository{sql: db}
	ctx := context.Background()

	mock.ExpectQuery(`SELECT\s+COUNT\(\*\),\s+COUNT\(\*\) FILTER \(WHERE exportable = TRUE\)`).
		WillReturnRows(sqlmock.NewRows([]string{
			"count",
			"exportable",
			"non_exportable",
			"complete",
			"partial",
			"invalid",
			"storage",
			"tokens",
		}).AddRow(int64(2), int64(1), int64(1), int64(1), int64(0), int64(1), int64(300), int64(30)))
	mock.ExpectQuery(`SELECT to_char\(date_trunc\('day', created_at\), 'YYYY-MM-DD'\) AS day`).
		WillReturnRows(sqlmock.NewRows([]string{"day", "storage_bytes", "session_count"}).
			AddRow("2026-05-27", int64(300), int64(2)))
	mock.ExpectQuery(`LEFT JOIN groups g ON g.id = d.group_id`).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "group_name", "storage_bytes", "session_count"}).
			AddRow(int64(3), "共享分组", int64(300), int64(2)))
	mock.ExpectQuery(`SELECT COALESCE\(NULLIF\(request_path, ''\), '\(unknown\)'\) AS request_path`).
		WillReturnRows(sqlmock.NewRows([]string{"request_path", "storage_bytes", "session_count", "total_tokens"}).
			AddRow("/v1/responses", int64(100), int64(1), int64(10)).
			AddRow("/v1/chat/completions", int64(200), int64(1), int64(20)))
	mock.ExpectQuery(`SELECT COALESCE\(NULLIF\(model, ''\), '\(unknown\)'\) AS model`).
		WillReturnRows(sqlmock.NewRows([]string{"model", "storage_bytes", "session_count", "total_tokens"}).
			AddRow("gpt-5.5", int64(300), int64(2), int64(30)))
	mock.ExpectQuery(`SELECT COALESCE\(NULLIF\(user_agent, ''\), '\(unknown\)'\) AS user_agent`).
		WillReturnRows(sqlmock.NewRows([]string{"user_agent", "storage_bytes", "session_count", "total_tokens"}).
			AddRow("codex-cli/1.0", int64(100), int64(1), int64(10)).
			AddRow("claude-code/2.0", int64(200), int64(1), int64(20)))

	stats, err := repo.Stats(ctx, service.DataShareSessionFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.SessionCount)
	require.Equal(t, float64(15), stats.AvgTokensPerSession)
	require.Len(t, stats.RequestPathBreakdown, 2)
	require.Equal(t, "/v1/responses", stats.RequestPathBreakdown[0].RequestPath)
	require.Equal(t, int64(100), stats.RequestPathBreakdown[0].StorageBytes)
	require.Equal(t, int64(10), stats.RequestPathBreakdown[0].TotalTokens)
	require.Len(t, stats.ModelBreakdown, 1)
	require.Equal(t, "gpt-5.5", stats.ModelBreakdown[0].Model)
	require.Len(t, stats.UserAgentBreakdown, 2)
	require.Equal(t, "codex-cli/1.0", stats.UserAgentBreakdown[0].UserAgent)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDataShareSessionRepository_RequestPathBreakdownLoader(t *testing.T) {
	repo, _ := newDataShareSessionRepoSQLite(t)
	ctx := context.Background()

	now := time.Now().UTC()
	storageByPath := map[string]int64{}
	storageByUserAgent := map[string]int64{}
	totalStorage := int64(0)
	for _, item := range []struct {
		traj    string
		session string
		path    string
		ua      string
		tokens  int64
	}{
		{traj: "traj-responses", session: "sess-responses", path: "/v1/responses", ua: "codex-cli/1.0", tokens: 10},
		{traj: "traj-chat", session: "sess-chat", path: "/v1/chat/completions", ua: "claude-code/2.0", tokens: 20},
	} {
		require.NoError(t, repo.UpsertCapture(ctx, &service.DataShareSession{
			TrajectoryID:       item.traj,
			SessionID:          item.session,
			Dataset:            "tokenrouter-agent",
			Provider:           service.PlatformOpenAI,
			Model:              "gpt-5.5",
			RequestPath:        item.path,
			UserAgent:          item.ua,
			Status:             service.DataShareStatusCompleted,
			IsFinalSnapshot:    true,
			SourceRequestCount: 1,
			Tools:              []map[string]any{},
			Messages:           []map[string]any{},
			Usage:              map[string]any{},
			Meta:               map[string]any{"request_path": item.path, "user_agent": item.ua},
			SessionJSON:        map[string]any{"request_path": item.path, "user_agent": item.ua},
			QualityStatus:      service.DataShareQualityInvalid,
			QualityErrors:      []string{},
			TotalTokens:        item.tokens,
			UserID:             1,
			APIKeyID:           2,
			GroupID:            3,
			CreatedAt:          now,
			EndedAt:            &now,
			UpdatedAt:          now,
		}))
		stored, err := repo.client.DataShareSession.Query().Where(datasharesession.TrajectoryIDEQ(item.traj)).Only(ctx)
		require.NoError(t, err)
		storageByPath[item.path] = stored.StorageBytes
		storageByUserAgent[item.ua] = stored.StorageBytes
		totalStorage += stored.StorageBytes
	}

	points, err := repo.loadRequestPathBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	sort.Slice(points, func(i, j int) bool { return points[i].RequestPath < points[j].RequestPath })
	require.Len(t, points, 2)
	require.Equal(t, "/v1/chat/completions", points[0].RequestPath)
	require.Equal(t, storageByPath["/v1/chat/completions"], points[0].StorageBytes)
	require.Equal(t, int64(20), points[0].TotalTokens)
	require.Equal(t, "/v1/responses", points[1].RequestPath)
	require.Equal(t, storageByPath["/v1/responses"], points[1].StorageBytes)
	require.Equal(t, int64(10), points[1].TotalTokens)

	modelPoints, err := repo.loadModelBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	require.Len(t, modelPoints, 1)
	require.Equal(t, "gpt-5.5", modelPoints[0].Model)
	require.Equal(t, totalStorage, modelPoints[0].StorageBytes)
	require.Equal(t, int64(30), modelPoints[0].TotalTokens)

	uaPoints, err := repo.loadUserAgentBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	sort.Slice(uaPoints, func(i, j int) bool { return uaPoints[i].UserAgent < uaPoints[j].UserAgent })
	require.Len(t, uaPoints, 2)
	require.Equal(t, "claude-code/2.0", uaPoints[0].UserAgent)
	require.Equal(t, storageByUserAgent["claude-code/2.0"], uaPoints[0].StorageBytes)
	require.Equal(t, int64(20), uaPoints[0].TotalTokens)
	require.Equal(t, "codex-cli/1.0", uaPoints[1].UserAgent)
	require.Equal(t, storageByUserAgent["codex-cli/1.0"], uaPoints[1].StorageBytes)
	require.Equal(t, int64(10), uaPoints[1].TotalTokens)
}

func TestDataShareSessionRepository_CompressesPayloadAndOmitsListPayload(t *testing.T) {
	repo, client := newDataShareSessionRepoSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	systemPrompt := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": strings.Repeat("请分析这个文件。", 20)},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
		{"role": "tool", "tool_call_id": "call_1", "content": strings.Repeat("README.md\n", 40), "status": "success", "is_error": false},
		{"role": "assistant", "content": "分析完成。"},
	}
	tools := []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}}
	sessionJSON := map[string]any{
		"trajectory_id":        "traj-compress",
		"session_id":           "sess-compress",
		"dataset":              "tokenrouter-agent",
		"provider":             service.PlatformOpenAI,
		"model":                "gpt-5.5",
		"request_path":         "/v1/responses",
		"user_agent":           "codex-cli",
		"created_at":           now.Format(time.RFC3339Nano),
		"ended_at":             now.Format(time.RFC3339Nano),
		"status":               service.DataShareStatusCompleted,
		"is_final_snapshot":    true,
		"source_request_count": 1,
		"system_prompt":        systemPrompt,
		"tools":                tools,
		"messages":             messages,
		"usage":                map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		"meta":                 map[string]any{"request_path": "/v1/responses", "user_agent": "codex-cli/1.0"},
	}

	require.NoError(t, repo.UpsertCapture(ctx, &service.DataShareSession{
		TrajectoryID:       "traj-compress",
		SessionID:          "sess-compress",
		Dataset:            "tokenrouter-agent",
		Provider:           service.PlatformOpenAI,
		Model:              "gpt-5.5",
		RequestPath:        "/v1/responses",
		UserAgent:          "codex-cli",
		Status:             service.DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		SystemPrompt:       &systemPrompt,
		Tools:              tools,
		Messages:           messages,
		Usage:              map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		Meta:               map[string]any{"request_path": "/v1/responses", "user_agent": "codex-cli/1.0"},
		SessionJSON:        sessionJSON,
		QualityStatus:      service.DataShareQualityComplete,
		QualityErrors:      []string{},
		StorageBytes:       int64(len(mustRepositoryJSON(sessionJSON))),
		InputTokens:        10,
		OutputTokens:       5,
		TotalTokens:        15,
		UserID:             0,
		APIKeyID:           0,
		GroupID:            0,
		CreatedAt:          now,
		EndedAt:            &now,
		UpdatedAt:          now,
	}))

	stored, err := client.DataShareSession.Query().Where(datasharesession.TrajectoryIDEQ("traj-compress")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, stored.PayloadCompressed)
	require.NotEmpty(t, *stored.PayloadCompressed)
	require.Equal(t, dataSharePayloadEncodingZstd, stored.PayloadEncoding)
	require.Greater(t, stored.PayloadBytes, int64(0))
	require.Equal(t, int64(len(*stored.PayloadCompressed)), stored.StorageBytes)
	require.Less(t, stored.StorageBytes, stored.PayloadBytes)
	require.Empty(t, stored.Messages)
	require.Empty(t, stored.Tools)
	require.Empty(t, stored.Usage)
	require.Empty(t, stored.Meta)
	require.Empty(t, stored.SessionJSON)
	require.Nil(t, stored.SystemPrompt)

	listItems, _, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.DataShareSessionFilters{})
	require.NoError(t, err)
	require.Len(t, listItems, 1)
	require.Empty(t, listItems[0].PayloadCompressed)
	require.Empty(t, listItems[0].Messages)
	require.Empty(t, listItems[0].SessionJSON)

	detail, err := repo.GetByID(ctx, stored.ID)
	require.NoError(t, err)
	require.Len(t, detail.Messages, len(messages))
	require.Equal(t, "system", detail.Messages[0]["role"])
	require.Equal(t, "tool_calls", detail.Messages[2]["finish_reason"])
	require.Equal(t, "tool", detail.Messages[3]["role"])
	require.Equal(t, "分析完成。", detail.Messages[4]["content"])
	require.Equal(t, tools, detail.Tools)
	require.Equal(t, systemPrompt, *detail.SystemPrompt)
	require.Equal(t, "/v1/responses", detail.SessionJSON["request_path"])

	payloadItems, _, err := repo.ListWithPayload(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, service.DataShareSessionFilters{})
	require.NoError(t, err)
	require.Len(t, payloadItems, 1)
	require.Len(t, payloadItems[0].Messages, len(messages))
	require.Equal(t, tools, payloadItems[0].Tools)
}

func TestDataShareSessionRepository_LegacyPayloadLazyCompression(t *testing.T) {
	repo, client := newDataShareSessionRepoSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	systemPrompt := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": "请列目录"},
	}
	created, err := client.DataShareSession.Create().
		SetTrajectoryID("traj-legacy").
		SetSessionID("sess-legacy").
		SetDataset("tokenrouter-agent").
		SetProvider(service.PlatformOpenAI).
		SetModel("gpt-5.5").
		SetRequestPath("/v1/chat/completions").
		SetUserAgent("codex-cli").
		SetStatus(service.DataShareStatusTerminated).
		SetIsFinalSnapshot(false).
		SetSourceRequestCount(1).
		SetNillableSystemPrompt(&systemPrompt).
		SetTools([]map[string]any{}).
		SetMessages(messages).
		SetUsage(map[string]any{"total_tokens": 2}).
		SetMeta(map[string]any{"request_path": "/v1/chat/completions"}).
		SetSessionJSON(map[string]any{
			"trajectory_id":        "traj-legacy",
			"session_id":           "sess-legacy",
			"dataset":              "tokenrouter-agent",
			"provider":             service.PlatformOpenAI,
			"model":                "gpt-5.5",
			"request_path":         "/v1/chat/completions",
			"user_agent":           "codex-cli",
			"created_at":           now.Format(time.RFC3339Nano),
			"ended_at":             now.Format(time.RFC3339Nano),
			"status":               service.DataShareStatusTerminated,
			"is_final_snapshot":    false,
			"source_request_count": 1,
			"system_prompt":        systemPrompt,
			"tools":                []map[string]any{},
			"messages":             messages,
			"usage":                map[string]any{"total_tokens": 2},
			"meta":                 map[string]any{"request_path": "/v1/chat/completions"},
		}).
		SetExportable(false).
		SetQualityStatus(service.DataShareQualityInvalid).
		SetQualityErrors([]string{}).
		SetStorageBytes(999).
		SetInputTokens(1).
		SetOutputTokens(1).
		SetTotalTokens(2).
		SetUserID(0).
		SetAPIKeyID(0).
		SetGroupID(0).
		SetCreatedAt(now).
		SetNillableEndedAt(&now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	detail, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, messages, detail.Messages)
	require.Equal(t, "/v1/chat/completions", detail.SessionJSON["request_path"])

	stored, err := client.DataShareSession.Get(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.PayloadCompressed)
	require.NotEmpty(t, *stored.PayloadCompressed)
	require.Equal(t, dataSharePayloadEncodingZstd, stored.PayloadEncoding)
	require.Greater(t, stored.PayloadBytes, int64(0))
	require.Equal(t, int64(len(*stored.PayloadCompressed)), stored.StorageBytes)
	require.Empty(t, stored.Messages)
	require.Empty(t, stored.SessionJSON)
	require.Nil(t, stored.SystemPrompt)
}
