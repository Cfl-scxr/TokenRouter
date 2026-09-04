package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSaveGroupAvailabilityProbeResultMaintainsConsecutiveFailures(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	startedAt := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	nextRunAt := finishedAt.Add(15 * time.Minute)
	result := &service.GroupAvailabilityProbeResult{
		GroupID:    17,
		ModelID:    "gpt-5.6-sol",
		Status:     service.GroupAvailabilityProbeStatusFailed,
		Success:    false,
		LatencyMs:  2000,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO group_availability_probe_results")).
		WithArgs(result.GroupID, result.AccountID, result.ModelID, result.Status, result.Success, result.LatencyMs, nil, result.StartedAt, result.FinishedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?s)INSERT INTO group_availability_probe_states.*consecutive_failures.*CASE WHEN \$4 THEN 0 ELSE 1 END.*group_availability_probe_states\.consecutive_failures \+ 1`).
		WithArgs(result.GroupID, nextRunAt, result.Status, result.Success, result.LatencyMs, nil, result.FinishedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewGroupAvailabilityProbeRepository(db)
	require.NoError(t, repo.SaveResultAndScheduleNext(context.Background(), result, nextRunAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupAvailabilitySummaryReadsLatestState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 9, 4, 9, 7, 0, 0, time.UTC)
	checkedAt := now.Add(-2 * time.Minute)
	mock.ExpectQuery(`(?s)FROM group_availability_probe_results.*GROUP BY group_id, bucket_index`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 15).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "bucket_index", "success_count", "total_count"}))
	mock.ExpectQuery(`(?s)SELECT.*last_status.*last_checked_at.*last_latency_ms.*consecutive_failures.*FROM group_availability_probe_states`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "last_status", "last_checked_at", "last_latency_ms", "consecutive_failures"}).
			AddRow(int64(17), service.GroupAvailabilityProbeStatusFailed, checkedAt, int64(4321), int64(3)))

	repo := NewGroupAvailabilityProbeRepository(db)
	summaries, err := repo.GetSummaryByGroupIDs(context.Background(), []int64{17}, 1, 15, "UTC", now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	summary := summaries[17]
	require.NotNil(t, summary)
	require.Equal(t, service.GroupAvailabilityProbeStatusFailed, summary.LastStatus)
	require.Equal(t, checkedAt, *summary.LastCheckedAt)
	require.EqualValues(t, 4321, *summary.LastLatencyMs)
	require.EqualValues(t, 3, summary.ConsecutiveFailures)
	require.Len(t, summary.Days, 96)
}
