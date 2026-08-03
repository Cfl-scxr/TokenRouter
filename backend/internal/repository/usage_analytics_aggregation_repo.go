package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
)

// AggregateUsageAnalyticsRange 重建范围内完整 UTC 小时桶，并同步刷新受影响的 UTC 日桶。
func (r *dashboardAggregationRepository) AggregateUsageAnalyticsRange(ctx context.Context, start, end time.Time) error {
	if r == nil || r.sql == nil || !end.After(start) {
		return nil
	}
	hourStart := start.UTC().Truncate(time.Hour)
	hourEnd := end.UTC().Truncate(time.Hour)
	if end.UTC().After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}
	if !hourEnd.After(hourStart) {
		return nil
	}

	scanEnd := end.UTC()
	if scanEnd.After(hourEnd) {
		scanEnd = hourEnd
	}
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		if err := txRepo.aggregateUsageAnalyticsRangeInTx(ctx, hourStart, hourEnd, scanEnd); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.aggregateUsageAnalyticsRangeInTx(ctx, hourStart, hourEnd, scanEnd)
}

func (r *dashboardAggregationRepository) aggregateUsageAnalyticsRangeInTx(ctx context.Context, hourStart, hourEnd, scanEnd time.Time) error {
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_analytics_hourly
		WHERE bucket_start >= $1 AND bucket_start < $2
	`, hourStart, hourEnd); err != nil {
		return err
	}

	// 将历史请求类型和计费模式在写入聚合表时归一，查询层无需反复兼容旧字段。
	if _, err := r.sql.ExecContext(ctx, `
		INSERT INTO usage_analytics_hourly (
			bucket_start, user_id, billing_user_id, team_id, api_key_id, group_id,
			requested_model, request_type, stream, billing_type, billing_mode,
			platform, inbound_endpoint, total_requests, input_tokens, output_tokens,
			cache_creation_tokens, cache_read_tokens, total_cost, actual_cost,
			account_cost, total_duration_ms, duration_count, computed_at
		)
		SELECT
			date_trunc('hour', ul.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
			ul.user_id,
			COALESCE(ul.billing_user_id, ul.user_id),
			COALESCE(ul.team_id, 0),
			ul.api_key_id,
			COALESCE(ul.group_id, 0),
			COALESCE(NULLIF(TRIM(ul.requested_model), ''), ul.model, ''),
			CASE
				WHEN COALESCE(ul.request_type, 0) <> 0 THEN ul.request_type
				WHEN COALESCE(ul.openai_ws_mode, FALSE) THEN 3
				WHEN COALESCE(ul.stream, FALSE) THEN 2
				ELSE 1
			END,
			COALESCE(ul.stream, FALSE),
			COALESCE(ul.billing_type, 0),
			COALESCE(
				NULLIF(ul.billing_mode, ''),
				CASE
					WHEN COALESCE(ul.video_duration_seconds, 0) > 0 THEN 'video'
					WHEN COALESCE(ul.image_count, 0) > 0 THEN 'image'
					ELSE 'token'
				END
			),
			COALESCE(NULLIF(g.platform, ''), a.platform, ''),
			COALESCE(ul.inbound_endpoint, ''),
			COUNT(*),
			COALESCE(SUM(ul.input_tokens), 0),
			COALESCE(SUM(ul.output_tokens), 0),
			COALESCE(SUM(ul.cache_creation_tokens), 0),
			COALESCE(SUM(ul.cache_read_tokens), 0),
			COALESCE(SUM(ul.total_cost), 0),
			COALESCE(SUM(ul.actual_cost), 0),
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0),
			COALESCE(SUM(COALESCE(ul.duration_ms, 0)), 0),
			COUNT(ul.duration_ms),
			NOW()
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
		GROUP BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13
	`, hourStart, scanEnd); err != nil {
		return err
	}

	dayStart := time.Date(hourStart.Year(), hourStart.Month(), hourStart.Day(), 0, 0, 0, 0, time.UTC)
	lastHour := hourEnd.Add(-time.Nanosecond)
	dayEnd := time.Date(lastHour.Year(), lastHour.Month(), lastHour.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_analytics_daily
		WHERE bucket_date >= $1::date AND bucket_date < $2::date
	`, dayStart, dayEnd); err != nil {
		return err
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO usage_analytics_daily (
			bucket_date, user_id, billing_user_id, team_id, api_key_id, group_id,
			requested_model, request_type, stream, billing_type, billing_mode,
			platform, inbound_endpoint, total_requests, input_tokens, output_tokens,
			cache_creation_tokens, cache_read_tokens, total_cost, actual_cost,
			account_cost, total_duration_ms, duration_count, computed_at
		)
		SELECT
			(bucket_start AT TIME ZONE 'UTC')::date,
			user_id, billing_user_id, team_id, api_key_id, group_id,
			requested_model, request_type, stream, billing_type, billing_mode,
			platform, inbound_endpoint,
			SUM(total_requests), SUM(input_tokens), SUM(output_tokens),
			SUM(cache_creation_tokens), SUM(cache_read_tokens), SUM(total_cost),
			SUM(actual_cost), SUM(account_cost), SUM(total_duration_ms), SUM(duration_count), NOW()
		FROM usage_analytics_hourly
		WHERE bucket_start >= $1 AND bucket_start < $2
		GROUP BY 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13
	`, dayStart, dayEnd)
	return err
}

// GetUsageAnalyticsAggregationState 读取单行聚合状态。
func (r *dashboardAggregationRepository) GetUsageAnalyticsAggregationState(ctx context.Context) (*service.UsageAnalyticsAggregationState, error) {
	state := &service.UsageAnalyticsAggregationState{}
	var coverageStart, backfillCursor, sourceOldest, lastRun, lastSuccess, lastErrorAt sql.NullTime
	err := scanSingleRow(ctx, r.sql, `
		SELECT live_watermark, coverage_start, backfill_cursor, source_oldest_at,
		       phase, last_run_at, last_success_at, last_error_at, last_error, last_duration_ms
		FROM usage_analytics_aggregation_state WHERE id = 1
	`, nil, &state.LiveWatermark, &coverageStart, &backfillCursor, &sourceOldest,
		&state.Phase, &lastRun, &lastSuccess, &lastErrorAt, &state.LastError, &state.LastDurationMS)
	if err != nil {
		return nil, err
	}
	state.CoverageStart = nullableTimePointer(coverageStart)
	state.BackfillCursor = nullableTimePointer(backfillCursor)
	state.SourceOldestAt = nullableTimePointer(sourceOldest)
	state.LastRunAt = nullableTimePointer(lastRun)
	state.LastSuccessAt = nullableTimePointer(lastSuccess)
	state.LastErrorAt = nullableTimePointer(lastErrorAt)
	state.LiveWatermark = state.LiveWatermark.UTC()
	return state, nil
}

// SaveUsageAnalyticsAggregationState 完整保存单行聚合状态。
func (r *dashboardAggregationRepository) SaveUsageAnalyticsAggregationState(ctx context.Context, state *service.UsageAnalyticsAggregationState) error {
	if state == nil {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO usage_analytics_aggregation_state (
			id, live_watermark, coverage_start, backfill_cursor, source_oldest_at,
			phase, last_run_at, last_success_at, last_error_at, last_error,
			last_duration_ms, updated_at
		) VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (id) DO UPDATE SET
			live_watermark = EXCLUDED.live_watermark,
			coverage_start = EXCLUDED.coverage_start,
			backfill_cursor = EXCLUDED.backfill_cursor,
			source_oldest_at = EXCLUDED.source_oldest_at,
			phase = EXCLUDED.phase,
			last_run_at = EXCLUDED.last_run_at,
			last_success_at = EXCLUDED.last_success_at,
			last_error_at = EXCLUDED.last_error_at,
			last_error = EXCLUDED.last_error,
			last_duration_ms = EXCLUDED.last_duration_ms,
			updated_at = NOW()
	`, state.LiveWatermark.UTC(), state.CoverageStart, state.BackfillCursor, state.SourceOldestAt,
		state.Phase, state.LastRunAt, state.LastSuccessAt, state.LastErrorAt, state.LastError, state.LastDurationMS)
	return err
}

// GetOldestUsageLogTime 通过 created_at 索引读取当前原始数据最早时间。
func (r *dashboardAggregationRepository) GetOldestUsageLogTime(ctx context.Context) (*time.Time, error) {
	var oldest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, `
		SELECT (
			SELECT created_at
			FROM usage_logs
			ORDER BY created_at ASC
			LIMIT 1
		)
	`, nil, &oldest); err != nil {
		return nil, err
	}
	return nullableTimePointer(oldest), nil
}

// CleanupUsageAnalytics 清理超过部署保留期的多维聚合桶。
func (r *dashboardAggregationRepository) CleanupUsageAnalytics(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	if _, err := r.sql.ExecContext(ctx, `DELETE FROM usage_analytics_hourly WHERE bucket_start < $1`, hourlyCutoff.UTC()); err != nil {
		return err
	}
	_, err := r.sql.ExecContext(ctx, `DELETE FROM usage_analytics_daily WHERE bucket_date < $1::date`, dailyCutoff.UTC())
	return err
}

func nullableTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
