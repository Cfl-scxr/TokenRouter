package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/enttest"
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
		UserID:             1,
		APIKeyID:           2,
		GroupID:            3,
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
		UserID:             1,
		APIKeyID:           2,
		GroupID:            3,
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
	for _, item := range []struct {
		traj    string
		session string
		path    string
		ua      string
		storage int64
		tokens  int64
	}{
		{traj: "traj-responses", session: "sess-responses", path: "/v1/responses", ua: "codex-cli/1.0", storage: 100, tokens: 10},
		{traj: "traj-chat", session: "sess-chat", path: "/v1/chat/completions", ua: "claude-code/2.0", storage: 200, tokens: 20},
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
			StorageBytes:       item.storage,
			TotalTokens:        item.tokens,
			UserID:             1,
			APIKeyID:           2,
			GroupID:            3,
			CreatedAt:          now,
			EndedAt:            &now,
			UpdatedAt:          now,
		}))
	}

	points, err := repo.loadRequestPathBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	sort.Slice(points, func(i, j int) bool { return points[i].RequestPath < points[j].RequestPath })
	require.Len(t, points, 2)
	require.Equal(t, "/v1/chat/completions", points[0].RequestPath)
	require.Equal(t, int64(200), points[0].StorageBytes)
	require.Equal(t, int64(20), points[0].TotalTokens)
	require.Equal(t, "/v1/responses", points[1].RequestPath)
	require.Equal(t, int64(100), points[1].StorageBytes)
	require.Equal(t, int64(10), points[1].TotalTokens)

	modelPoints, err := repo.loadModelBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	require.Len(t, modelPoints, 1)
	require.Equal(t, "gpt-5.5", modelPoints[0].Model)
	require.Equal(t, int64(300), modelPoints[0].StorageBytes)
	require.Equal(t, int64(30), modelPoints[0].TotalTokens)

	uaPoints, err := repo.loadUserAgentBreakdown(ctx, repo.sql, "", nil)
	require.NoError(t, err)
	sort.Slice(uaPoints, func(i, j int) bool { return uaPoints[i].UserAgent < uaPoints[j].UserAgent })
	require.Len(t, uaPoints, 2)
	require.Equal(t, "claude-code/2.0", uaPoints[0].UserAgent)
	require.Equal(t, int64(200), uaPoints[0].StorageBytes)
	require.Equal(t, int64(20), uaPoints[0].TotalTokens)
	require.Equal(t, "codex-cli/1.0", uaPoints[1].UserAgent)
	require.Equal(t, int64(100), uaPoints[1].StorageBytes)
	require.Equal(t, int64(10), uaPoints[1].TotalTokens)
}
