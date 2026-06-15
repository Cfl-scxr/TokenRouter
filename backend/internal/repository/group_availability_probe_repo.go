package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/lib/pq"
)

type groupAvailabilityProbeRepository struct {
	db *sql.DB
}

func NewGroupAvailabilityProbeRepository(db *sql.DB) service.GroupAvailabilityProbeRepository {
	return &groupAvailabilityProbeRepository{db: db}
}

func (r *groupAvailabilityProbeRepository) ClaimDue(ctx context.Context, now time.Time, lockUntil time.Time, lockedBy string, limit int) ([]service.GroupAvailabilityProbeDueGroup, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 先将启用探测的分组补进状态表；分组被禁用时再清掉状态，避免长期扫描无效记录。
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_availability_probe_states (group_id, next_run_at, created_at, updated_at)
		SELECT id, $1, NOW(), NOW()
		FROM groups
		WHERE deleted_at IS NULL
		  AND status = 'active'
		  AND availability_probe_config @> '{"enabled": true}'::jsonb
		ON CONFLICT (group_id) DO NOTHING
	`, now); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM group_availability_probe_states s
		WHERE NOT EXISTS (
			SELECT 1
			FROM groups g
			WHERE g.id = s.group_id
			  AND g.deleted_at IS NULL
			  AND g.status = 'active'
			  AND g.availability_probe_config @> '{"enabled": true}'::jsonb
		)
	`); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		WITH due AS (
			SELECT s.group_id
			FROM group_availability_probe_states s
			JOIN groups g ON g.id = s.group_id
			WHERE g.deleted_at IS NULL
			  AND g.status = 'active'
			  AND g.availability_probe_config @> '{"enabled": true}'::jsonb
			  AND (s.next_run_at IS NULL OR s.next_run_at <= $1)
			  AND (s.locked_until IS NULL OR s.locked_until <= $1)
			ORDER BY COALESCE(s.next_run_at, 'epoch'::timestamptz), s.group_id
			LIMIT $4
			FOR UPDATE OF s SKIP LOCKED
		),
		claimed AS (
			UPDATE group_availability_probe_states s
			SET locked_until = $2,
				locked_by = $3,
				updated_at = NOW()
			FROM due
			WHERE s.group_id = due.group_id
			RETURNING s.group_id
		)
		SELECT g.id, g.name, g.platform, g.availability_probe_config
		FROM claimed c
		JOIN groups g ON g.id = c.group_id
		ORDER BY g.id
	`, now, lockUntil, lockedBy, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	dueGroups := make([]service.GroupAvailabilityProbeDueGroup, 0)
	for rows.Next() {
		var item service.GroupAvailabilityProbeDueGroup
		var rawConfig []byte
		if err := rows.Scan(&item.GroupID, &item.Name, &item.Platform, &rawConfig); err != nil {
			return nil, err
		}
		if len(rawConfig) > 0 {
			if err := json.Unmarshal(rawConfig, &item.Config); err != nil {
				return nil, fmt.Errorf("decode availability probe config: %w", err)
			}
		}
		dueGroups = append(dueGroups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return dueGroups, nil
}

func (r *groupAvailabilityProbeRepository) SaveResultAndScheduleNext(ctx context.Context, result *service.GroupAvailabilityProbeResult, nextRunAt time.Time) error {
	if r == nil || r.db == nil || result == nil {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_availability_probe_results (
			group_id, account_id, model_id, status, success, latency_ms,
			error_message, started_at, finished_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, result.GroupID, result.AccountID, result.ModelID, result.Status, result.Success, result.LatencyMs, nullableProbeString(result.ErrorMessage), result.StartedAt, result.FinishedAt); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_availability_probe_states (
			group_id, next_run_at, locked_until, locked_by, last_status,
			last_success, last_latency_ms, last_error, last_checked_at, created_at, updated_at
		)
		VALUES ($1, $2, NULL, NULL, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (group_id) DO UPDATE SET
			next_run_at = EXCLUDED.next_run_at,
			locked_until = NULL,
			locked_by = NULL,
			last_status = EXCLUDED.last_status,
			last_success = EXCLUDED.last_success,
			last_latency_ms = EXCLUDED.last_latency_ms,
			last_error = EXCLUDED.last_error,
			last_checked_at = EXCLUDED.last_checked_at,
			updated_at = NOW()
	`, result.GroupID, nextRunAt, result.Status, result.Success, result.LatencyMs, nullableProbeString(result.ErrorMessage), result.FinishedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *groupAvailabilityProbeRepository) GetSummaryByGroupIDs(ctx context.Context, groupIDs []int64, days int, timezoneName string, now time.Time) (map[int64]*service.GroupAvailabilitySummary, error) {
	out := make(map[int64]*service.GroupAvailabilitySummary, len(groupIDs))
	if r == nil || r.db == nil || len(groupIDs) == 0 {
		return out, nil
	}
	if days <= 0 {
		days = 30
	}

	loc := time.UTC
	sqlTimezoneName := "UTC"
	if timezoneName != "" {
		if parsed, err := time.LoadLocation(timezoneName); err == nil && parsed != nil {
			loc = parsed
			sqlTimezoneName = timezoneName
		}
	}
	localNow := now.In(loc)
	startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	endLocal := startLocal.AddDate(0, 0, days)
	startUTC := startLocal.UTC()
	endUTC := endLocal.UTC()

	for _, groupID := range groupIDs {
		summary := &service.GroupAvailabilitySummary{
			WindowDays: days,
			Days:       make([]service.GroupAvailabilityDailyPoint, 0, days),
		}
		for i := 0; i < days; i++ {
			day := startLocal.AddDate(0, 0, i)
			summary.Days = append(summary.Days, service.GroupAvailabilityDailyPoint{
				Date: day.Format("2006-01-02"),
			})
		}
		out[groupID] = summary
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			group_id,
			to_char(started_at AT TIME ZONE $4, 'YYYY-MM-DD') AS day,
			COUNT(*) FILTER (WHERE success = true) AS success_count,
			COUNT(*) AS total_count
		FROM group_availability_probe_results
		WHERE group_id = ANY($1)
		  AND started_at >= $2
		  AND started_at < $3
		GROUP BY group_id, day
	`, pq.Array(groupIDs), startUTC, endUTC, sqlTimezoneName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	dayIndex := make(map[int64]map[string]int, len(groupIDs))
	for _, groupID := range groupIDs {
		dayIndex[groupID] = make(map[string]int, days)
		for i, day := range out[groupID].Days {
			dayIndex[groupID][day.Date] = i
		}
	}

	for rows.Next() {
		var groupID int64
		var day string
		var successCount int64
		var totalCount int64
		if err := rows.Scan(&groupID, &day, &successCount, &totalCount); err != nil {
			return nil, err
		}
		summary, ok := out[groupID]
		if !ok {
			continue
		}
		idx, ok := dayIndex[groupID][day]
		if !ok {
			continue
		}
		rate := availabilityRate(successCount, totalCount)
		summary.Days[idx].SuccessCount = successCount
		summary.Days[idx].TotalCount = totalCount
		summary.Days[idx].AvailabilityRate = rate
		summary.SuccessCount += successCount
		summary.TotalCount += totalCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, summary := range out {
		summary.AvailabilityRate = availabilityRate(summary.SuccessCount, summary.TotalCount)
	}

	lastRows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (group_id)
			group_id, status, finished_at
		FROM group_availability_probe_results
		WHERE group_id = ANY($1)
		ORDER BY group_id, started_at DESC, id DESC
	`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = lastRows.Close() }()

	for lastRows.Next() {
		var groupID int64
		var status string
		var checkedAt time.Time
		if err := lastRows.Scan(&groupID, &status, &checkedAt); err != nil {
			return nil, err
		}
		if summary, ok := out[groupID]; ok {
			summary.LastStatus = status
			t := checkedAt
			summary.LastCheckedAt = &t
		}
	}
	if err := lastRows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *groupAvailabilityProbeRepository) CleanupOldResults(ctx context.Context, before time.Time) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM group_availability_probe_results
		WHERE created_at < $1
	`, before)
	return err
}

func availabilityRate(successCount int64, totalCount int64) *float64 {
	if totalCount <= 0 {
		return nil
	}
	value := float64(successCount) / float64(totalCount)
	return &value
}

func nullableProbeString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
