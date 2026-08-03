package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

type dashboardAggregationRepoTestStub struct {
	aggregateCalls       int
	recomputeCalls       int
	cleanupUsageCalls    int
	cleanupDedupCalls    int
	ensurePartitionCalls int
	lastStart            time.Time
	lastEnd              time.Time
	watermark            time.Time
	aggregateErr         error
	cleanupAggregatesErr error
	cleanupUsageErr      error
	cleanupDedupErr      error
	ensurePartitionErr   error
	aggregateStarted     chan struct{}
}

func (s *dashboardAggregationRepoTestStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	s.aggregateCalls++
	s.lastStart = start
	s.lastEnd = end
	if s.aggregateStarted != nil {
		select {
		case s.aggregateStarted <- struct{}{}:
		default:
		}
	}
	return s.aggregateErr
}

func (s *dashboardAggregationRepoTestStub) RecomputeRange(ctx context.Context, start, end time.Time) error {
	s.recomputeCalls++
	return s.AggregateRange(ctx, start, end)
}

func (s *dashboardAggregationRepoTestStub) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	return s.watermark, nil
}

func (s *dashboardAggregationRepoTestStub) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	return nil
}

func (s *dashboardAggregationRepoTestStub) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	return s.cleanupAggregatesErr
}

func (s *dashboardAggregationRepoTestStub) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	s.cleanupUsageCalls++
	return s.cleanupUsageErr
}

func (s *dashboardAggregationRepoTestStub) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	s.cleanupDedupCalls++
	return s.cleanupDedupErr
}

func (s *dashboardAggregationRepoTestStub) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	s.ensurePartitionCalls++
	return s.ensurePartitionErr
}

func TestDashboardAggregationService_RunScheduledAggregation_EpochUsesLiveLookback(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{watermark: time.Unix(0, 0).UTC()}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.runScheduledAggregation()

	require.Equal(t, 1, repo.aggregateCalls)
	require.False(t, repo.lastEnd.IsZero())
	require.WithinDuration(t, repo.lastEnd.Add(-120*time.Second), repo.lastStart, time.Second)
}

// TestDashboardAggregationServiceRuntimeDisabledStopsWrites 验证运行时关闭后定时轮次不会写聚合表。
func TestDashboardAggregationServiceRuntimeDisabledStopsWrites(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{watermark: time.Unix(0, 0).UTC()}
	settingRepo := newRuntimeSettingRepoStub()
	settingRepo.values[SettingKeyPreAggregationSettings] = `{"usage":{"enabled":false,"interval_seconds":60},"ops":{"enabled":true}}`
	cfg := &config.Config{DashboardAgg: config.DashboardAggregationConfig{Enabled: true, IntervalSeconds: 60}}
	settings := NewPreAggregationSettingsService(settingRepo, cfg)
	svc := NewDashboardAggregationService(repo, nil, cfg)
	svc.SetPreAggregationSettings(settings)

	svc.runScheduledAggregation()

	require.Zero(t, repo.aggregateCalls)
}

// TestDashboardAggregationServiceRuntimeEnableTriggersImmediately 验证开启运行时开关会立即唤醒任务。
func TestDashboardAggregationServiceRuntimeEnableTriggersImmediately(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{
		watermark:        time.Unix(0, 0).UTC(),
		aggregateStarted: make(chan struct{}, 1),
	}
	settingRepo := newRuntimeSettingRepoStub()
	settingRepo.values[SettingKeyPreAggregationSettings] = `{"usage":{"enabled":false,"interval_seconds":60},"ops":{"enabled":true}}`
	cfg := &config.Config{
		DashboardAgg: config.DashboardAggregationConfig{
			Enabled: true, IntervalSeconds: 60, LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{HourlyDays: 1, DailyDays: 1},
		},
	}
	settings := NewPreAggregationSettingsService(settingRepo, cfg)
	svc := NewDashboardAggregationService(repo, nil, cfg)
	svc.SetPreAggregationSettings(settings)

	_, err := settings.Update(context.Background(), PreAggregationSettings{
		Usage: PreAggregationUsageSettings{Enabled: true, IntervalSeconds: 60},
		Ops:   PreAggregationOpsSettings{Enabled: true},
	})
	require.NoError(t, err)
	select {
	case <-repo.aggregateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("开启预聚合后未立即触发任务")
	}
}

// TestDashboardAggregationServiceRecomputeRunsWhileRuntimeDisabled 验证删除后的内部重算不会留下陈旧聚合。
func TestDashboardAggregationServiceRecomputeRunsWhileRuntimeDisabled(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{aggregateStarted: make(chan struct{}, 1)}
	settingRepo := newRuntimeSettingRepoStub()
	settingRepo.values[SettingKeyPreAggregationSettings] = `{"usage":{"enabled":false,"interval_seconds":60},"ops":{"enabled":true}}`
	cfg := &config.Config{DashboardAgg: config.DashboardAggregationConfig{Enabled: true, IntervalSeconds: 60}}
	settings := NewPreAggregationSettingsService(settingRepo, cfg)
	svc := NewDashboardAggregationService(repo, nil, cfg)
	svc.SetPreAggregationSettings(settings)

	end := time.Now().UTC()
	require.NoError(t, svc.TriggerRecomputeRange(end.Add(-time.Hour), end))
	select {
	case <-repo.aggregateStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("运行时关闭后内部一致性重算未执行")
	}
}

func TestDashboardAggregationService_CleanupRetentionFailure_DoesNotRecord(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{cleanupAggregatesErr: errors.New("清理失败")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.maybeCleanupRetention(context.Background(), time.Now().UTC())

	require.Nil(t, svc.lastRetentionCleanup.Load())
	require.Equal(t, 1, repo.cleanupUsageCalls)
	require.Equal(t, 1, repo.cleanupDedupCalls)
}

func TestDashboardAggregationService_CleanupDedupFailure_DoesNotRecord(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{cleanupDedupErr: errors.New("dedup cleanup failed")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays: 1,
				HourlyDays:    1,
				DailyDays:     1,
			},
		},
	}

	svc.maybeCleanupRetention(context.Background(), time.Now().UTC())

	require.Nil(t, svc.lastRetentionCleanup.Load())
	require.Equal(t, 1, repo.cleanupDedupCalls)
}

func TestDashboardAggregationService_PartitionFailure_DoesNotAggregate(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{ensurePartitionErr: errors.New("partition failed")}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			LookbackSeconds: 120,
			Retention: config.DashboardAggregationRetentionConfig{
				UsageLogsDays:         1,
				UsageBillingDedupDays: 2,
				HourlyDays:            1,
				DailyDays:             1,
			},
		},
	}

	svc.runScheduledAggregation()

	require.Equal(t, 1, repo.ensurePartitionCalls)
	require.Equal(t, 1, repo.aggregateCalls)
}

func TestDashboardAggregationService_TriggerBackfill_TooLarge(t *testing.T) {
	repo := &dashboardAggregationRepoTestStub{}
	svc := &DashboardAggregationService{
		repo: repo,
		cfg: config.DashboardAggregationConfig{
			BackfillEnabled: true,
			BackfillMaxDays: 1,
		},
	}

	start := time.Now().AddDate(0, 0, -3)
	end := time.Now()
	err := svc.TriggerBackfill(start, end)
	require.ErrorIs(t, err, ErrDashboardBackfillTooLarge)
	require.Equal(t, 0, repo.aggregateCalls)
}
