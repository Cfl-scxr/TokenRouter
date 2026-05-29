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
	cfg := &config.Config{}
	cfg.Gateway.DataSharingCapture.WorkerCount = 3
	cfg.Gateway.DataSharingCapture.QueueSize = 9
	cfg.Gateway.DataSharingCapture.TaskTimeoutSeconds = 7

	pool := NewDataSharingCaptureWorkerPool(cfg)
	t.Cleanup(pool.Stop)

	stats := pool.Stats()
	require.Equal(t, 9, stats.QueueCapacity)
	require.Equal(t, 7*time.Second, pool.taskTimeout)
}

func TestDataSharingCaptureWorkerPool_OptionsFromNilConfig(t *testing.T) {
	opts := dataSharingCapturePoolOptionsFromConfig(nil)
	require.Equal(t, defaultDataSharingCaptureWorkerCount, opts.WorkerCount)
	require.Equal(t, defaultDataSharingCaptureQueueSize, opts.QueueSize)
	require.Equal(t, time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds)*time.Second, opts.TaskTimeout)
}
