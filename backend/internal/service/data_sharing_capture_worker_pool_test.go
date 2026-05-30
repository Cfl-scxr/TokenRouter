package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDataSharingCaptureWorkerPool_SubmitEnqueued(t *testing.T) {
	done := make(chan struct{})
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			require.Equal(t, DataSharingCaptureProtocolOpenAI, job.Protocol)
			close(done)
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	mode := pool.Submit(DataSharingCaptureJob{Protocol: DataSharingCaptureProtocolOpenAI})
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, mode)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture job not executed")
	}

	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return stats.SubmittedTotal == 1 && stats.CompletedTotal == 1 && stats.FailedTotal == 0
	}, time.Second, 10*time.Millisecond)
}

func TestDataSharingCaptureWorkerPool_TimeoutAndFailureStats(t *testing.T) {
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: 20 * time.Millisecond,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{}))

	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return stats.FailedTotal == 1 && stats.TimeoutTotal == 1 && stats.LastError != ""
	}, time.Second, 10*time.Millisecond)
}

func TestDataSharingCaptureWorkerPool_QueueFullDropsWithoutSyncFallback(t *testing.T) {
	var overflowExecuted atomic.Bool
	block := make(chan struct{})
	started := make(chan struct{})
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			if job.Metadata.RequestID == "overflow" {
				overflowExecuted.Store(true)
				return nil
			}
			if job.Metadata.RequestID == "running" {
				close(started)
				<-block
			}
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "running"}}))
	<-started
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "queued"}}))

	mode := pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "overflow"}})
	require.Equal(t, DataSharingCaptureSubmitModeDropped, mode)
	require.False(t, overflowExecuted.Load())
	require.GreaterOrEqual(t, pool.Stats().DroppedTotal, uint64(1))

	close(block)
}

func TestDataSharingCaptureWorkerPool_FlushQueueHasPriority(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	order := make(chan string, 2)
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			if job.Metadata.RequestID == "running" {
				close(started)
				<-block
				return nil
			}
			order <- job.Metadata.RequestID
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "running"}}))
	<-started
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "capture"}}))
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.SubmitFlush(DataSharingCaptureJob{
		Metadata: DataSharingCaptureJobMetadata{RequestID: "flush"},
		Flush: func(context.Context) error {
			order <- "flush"
			return nil
		},
	}))

	close(block)

	select {
	case got := <-order:
		require.Equal(t, "flush", got)
	case <-time.After(time.Second):
		t.Fatal("flush job not executed")
	}
}

func TestDataSharingCaptureWorkerPool_FlushQueueUsesIndependentCapacity(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount:    1,
		QueueSize:      4,
		FlushQueueSize: 1,
		TaskTimeout:    time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			if job.Metadata.RequestID == "running" {
				close(started)
				<-block
			}
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "running"}}))
	<-started
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.SubmitFlush(DataSharingCaptureJob{Flush: func(context.Context) error { return nil }}))
	require.Equal(t, DataSharingCaptureSubmitModeDropped, pool.SubmitFlush(DataSharingCaptureJob{Flush: func(context.Context) error { return nil }}))
	require.Equal(t, 1, pool.Stats().FlushQueueCapacity)

	pool.UpdateRuntimeSettings(1, 4, 2, time.Second)
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.SubmitFlush(DataSharingCaptureJob{Flush: func(context.Context) error { return nil }}))
	require.Equal(t, 2, pool.Stats().FlushQueueCapacity)

	close(block)
}

func TestDataSharingCaptureWorkerPool_StatsExposeWorkerJobKinds(t *testing.T) {
	captureBlock := make(chan struct{})
	flushBlock := make(chan struct{})
	captureStarted := make(chan struct{})
	flushStarted := make(chan struct{})
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount:    2,
		QueueSize:      2,
		FlushQueueSize: 2,
		TaskTimeout:    time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			close(captureStarted)
			<-captureBlock
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{}))
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.SubmitFlush(DataSharingCaptureJob{
		Flush: func(context.Context) error {
			close(flushStarted)
			<-flushBlock
			return nil
		},
	}))
	<-captureStarted
	<-flushStarted

	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return countDataSharingCaptureWorkerKind(stats.WorkerStates, DataSharingCaptureJobKindCapture) == 1 &&
			countDataSharingCaptureWorkerKind(stats.WorkerStates, DataSharingCaptureJobKindFlush) == 1
	}, time.Second, 10*time.Millisecond)

	close(captureBlock)
	close(flushBlock)
	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return stats.RunningWorkers == 0 &&
			countDataSharingCaptureWorkerKind(stats.WorkerStates, DataSharingCaptureJobKindCapture) == 0 &&
			countDataSharingCaptureWorkerKind(stats.WorkerStates, DataSharingCaptureJobKindFlush) == 0
	}, time.Second, 10*time.Millisecond)
}

func countDataSharingCaptureWorkerKind(states []DataSharingCaptureWorkerState, kind DataSharingCaptureJobKind) int {
	count := 0
	for _, state := range states {
		if state.JobKind == string(kind) {
			count++
		}
	}
	return count
}

func TestDataSharingCaptureWorkerPool_SubmitAfterStop(t *testing.T) {
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			return errors.New("unexpected")
		},
	})
	pool.Stop()

	mode := pool.Submit(DataSharingCaptureJob{})
	require.Equal(t, DataSharingCaptureSubmitModeDropped, mode)
	require.GreaterOrEqual(t, pool.Stats().DroppedTotal, uint64(1))
}

func TestDataSharingCaptureWorkerPool_OptionsFromConfig(t *testing.T) {
	resetDataShareCompressionLevel(t)
	cfg := &config.Config{}
	cfg.Gateway.DataSharingCapture.WorkerCount = 3
	cfg.Gateway.DataSharingCapture.QueueSize = 9
	cfg.Gateway.DataSharingCapture.TaskTimeoutSeconds = 7
	cfg.Gateway.DataSharingCapture.CompressionLevel = string(DataShareCompressionLevelBetter)

	pool := NewDataSharingCaptureWorkerPool(cfg)
	t.Cleanup(pool.Stop)

	stats := pool.Stats()
	require.Equal(t, 9, stats.QueueCapacity)
	require.Equal(t, 9, stats.FlushQueueCapacity)
	require.Equal(t, 7*time.Second, pool.TaskTimeout())
	require.Equal(t, 7, stats.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelBetter), stats.CompressionLevel)
}

func TestDataSharingCaptureWorkerPool_OptionsFromNilConfig(t *testing.T) {
	opts := dataSharingCapturePoolOptionsFromConfig(nil)
	require.Equal(t, defaultDataSharingCaptureWorkerCount, opts.WorkerCount)
	require.Equal(t, defaultDataSharingCaptureQueueSize, opts.QueueSize)
	require.Equal(t, defaultDataSharingCaptureQueueSize, opts.FlushQueueSize)
	require.Equal(t, time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds)*time.Second, opts.TaskTimeout)
}

func TestDataSharingCaptureWorkerPool_OptionsClampUpperBounds(t *testing.T) {
	opts := normalizeDataSharingCapturePoolOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount:    maxDataSharingCaptureWorkerCount + 1,
		QueueSize:      maxDataSharingCaptureQueueSize + 1,
		FlushQueueSize: maxDataSharingCaptureQueueSize + 2,
		TaskTimeout:    time.Duration(maxDataSharingCaptureTaskTimeoutSeconds+1) * time.Second,
	})
	require.Equal(t, maxDataSharingCaptureWorkerCount, opts.WorkerCount)
	require.Equal(t, maxDataSharingCaptureQueueSize, opts.QueueSize)
	require.Equal(t, maxDataSharingCaptureQueueSize, opts.FlushQueueSize)
	require.Equal(t, time.Duration(maxDataSharingCaptureTaskTimeoutSeconds)*time.Second, opts.TaskTimeout)
}

func TestDataSharingCaptureWorkerPool_SetTaskTimeoutUpdatesLaterJobs(t *testing.T) {
	seen := make(chan time.Duration, 1)
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			seen <- time.Until(deadline)
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	pool.SetTaskTimeout(5 * time.Second)
	require.Equal(t, 5, pool.Stats().TaskTimeoutSeconds)
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{}))

	select {
	case got := <-seen:
		require.Greater(t, got, 4*time.Second)
	case <-time.After(time.Second):
		t.Fatal("capture job not executed")
	}
}

func TestDataSharingCaptureWorkerPool_UpdateRuntimeSettingsChangesLogicalCapacity(t *testing.T) {
	var overflowExecuted atomic.Bool
	block := make(chan struct{})
	started := make(chan struct{})
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   3,
		TaskTimeout: time.Second,
		Handler: func(ctx context.Context, job DataSharingCaptureJob) error {
			switch job.Metadata.RequestID {
			case "running":
				close(started)
				<-block
			case "overflow":
				overflowExecuted.Store(true)
			}
			return nil
		},
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "running"}}))
	<-started
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "queued-1"}}))
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "queued-2"}}))

	pool.UpdateRuntimeSettings(1, 2, 5, 30*time.Second)
	require.Equal(t, DataSharingCaptureSubmitModeDropped, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "overflow"}}))
	require.False(t, overflowExecuted.Load())

	pool.UpdateRuntimeSettings(3, 8, 9, 30*time.Second)
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, pool.Submit(DataSharingCaptureJob{Metadata: DataSharingCaptureJobMetadata{RequestID: "accepted-after-grow"}}))

	stats := pool.Stats()
	require.Equal(t, 3, stats.WorkerCount)
	require.Equal(t, 8, stats.QueueCapacity)
	require.Equal(t, 9, stats.FlushQueueCapacity)
	require.Equal(t, 30, stats.TaskTimeoutSeconds)
	require.GreaterOrEqual(t, stats.AvailableWorkers, int64(2))

	close(block)
}
