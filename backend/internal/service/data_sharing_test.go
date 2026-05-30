package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type dataShareSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *dataShareSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *dataShareSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *dataShareSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.err != nil {
		return s.err
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *dataShareSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (s *dataShareSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	if s.err != nil {
		return s.err
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *dataShareSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *dataShareSettingRepoStub) Delete(_ context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.values, key)
	return nil
}

func resetDataShareCompressionLevel(t *testing.T) {
	t.Helper()
	previous := CurrentDataShareCompressionLevel()
	t.Cleanup(func() {
		SetDataShareCompressionLevel(previous)
	})
	SetDataShareCompressionLevel(string(defaultDataSharingCaptureCompressionLevel))
}

type dataShareCaptureRepoStub struct {
	mu       sync.Mutex
	saves    int
	sessions []*DataShareSession
	hydrated map[string]*DataShareSession
	stats    *DataShareStats
	err      error
	seenOpts []DataShareUpsertOptions
}

func (r *dataShareCaptureRepoStub) GetCaptureByTrajectoryIDWithPayload(_ context.Context, trajectoryID string) (*DataShareSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hydrated == nil || r.hydrated[trajectoryID] == nil {
		return nil, ErrDataShareSessionNotFound
	}
	return cloneBufferedDataShareSession(r.hydrated[trajectoryID]), nil
}

func (r *dataShareCaptureRepoStub) SaveCaptureSnapshot(ctx context.Context, session *DataShareSession, opts ...DataShareUpsertOptions) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves++
	r.sessions = append(r.sessions, cloneBufferedDataShareSession(session))
	if len(opts) > 0 {
		r.seenOpts = append(r.seenOpts, opts[0])
	}
	if r.err != nil {
		return r.err
	}
	return ctx.Err()
}

func (r *dataShareCaptureRepoStub) List(context.Context, pagination.PaginationParams, DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *dataShareCaptureRepoStub) ListWithPayload(context.Context, pagination.PaginationParams, DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error) {
	panic("unexpected ListWithPayload call")
}

func (r *dataShareCaptureRepoStub) GetByID(context.Context, int64) (*DataShareSession, error) {
	panic("unexpected GetByID call")
}

func (r *dataShareCaptureRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *dataShareCaptureRepoStub) BatchDelete(context.Context, []int64, DataShareSessionFilters) (int64, error) {
	panic("unexpected BatchDelete call")
}

func (r *dataShareCaptureRepoStub) Stats(context.Context, DataShareSessionFilters) (*DataShareStats, error) {
	if r.stats != nil {
		return r.stats, r.err
	}
	return &DataShareStats{SessionCount: 2}, r.err
}

func (r *dataShareCaptureRepoStub) FilterOptions(context.Context, DataShareSessionFilters) (*DataShareSessionFilterOptions, error) {
	panic("unexpected FilterOptions call")
}

func (r *dataShareCaptureRepoStub) TotalStorageBytes(context.Context) (int64, error) {
	return 0, nil
}

func (r *dataShareCaptureRepoStub) upsertCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saves
}

func (r *dataShareCaptureRepoStub) lastSession() *DataShareSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sessions) == 0 {
		return nil
	}
	return r.sessions[len(r.sessions)-1]
}

type dataShareExportRepoStub struct {
	items []DataShareSession
}

func (r *dataShareExportRepoStub) GetCaptureByTrajectoryIDWithPayload(context.Context, string) (*DataShareSession, error) {
	panic("unexpected GetCaptureByTrajectoryIDWithPayload call")
}

func (r *dataShareExportRepoStub) SaveCaptureSnapshot(context.Context, *DataShareSession, ...DataShareUpsertOptions) error {
	panic("unexpected SaveCaptureSnapshot call")
}

func (r *dataShareExportRepoStub) List(context.Context, pagination.PaginationParams, DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *dataShareExportRepoStub) ListWithPayload(_ context.Context, params pagination.PaginationParams, _ DataShareSessionFilters) ([]DataShareSession, *pagination.PaginationResult, error) {
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = len(r.items)
	}
	start := params.Offset()
	if start >= len(r.items) {
		return nil, &pagination.PaginationResult{Total: int64(len(r.items)), Page: params.Page, PageSize: pageSize, Pages: 1}, nil
	}
	end := start + pageSize
	if end > len(r.items) {
		end = len(r.items)
	}
	pages := 1
	if pageSize > 0 {
		pages = (len(r.items) + pageSize - 1) / pageSize
	}
	return r.items[start:end], &pagination.PaginationResult{Total: int64(len(r.items)), Page: params.Page, PageSize: pageSize, Pages: pages}, nil
}

func (r *dataShareExportRepoStub) GetByID(context.Context, int64) (*DataShareSession, error) {
	panic("unexpected GetByID call")
}

func (r *dataShareExportRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *dataShareExportRepoStub) BatchDelete(context.Context, []int64, DataShareSessionFilters) (int64, error) {
	panic("unexpected BatchDelete call")
}

func (r *dataShareExportRepoStub) Stats(context.Context, DataShareSessionFilters) (*DataShareStats, error) {
	panic("unexpected Stats call")
}

func (r *dataShareExportRepoStub) FilterOptions(context.Context, DataShareSessionFilters) (*DataShareSessionFilterOptions, error) {
	panic("unexpected FilterOptions call")
}

func (r *dataShareExportRepoStub) TotalStorageBytes(context.Context) (int64, error) {
	return 0, nil
}

func TestDataSharingService_CaptureAsyncUsesWorkerContext(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(repo, nil, pool)
	svc.SetDefaultCaptureRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              4,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          false,
		BufferIdleFlushSeconds: 5,
		BufferMaxSessions:      4096,
		BufferMaxPendingEvents: 65536,
	})

	mode := svc.CaptureOpenAIRequestAsync(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group:   &Group{ID: gid, DataSharingEnabled: true},
		},
		Provider:        PlatformOpenAI,
		Model:           "gpt-5-alias",
		UpstreamModel:   "gpt-5-2026-05-01",
		SessionID:       "session-async",
		RequestID:       "request-async",
		RequestBody:     []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`),
		InboundEndpoint: "/v1/chat/completions",
	})
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, mode)

	require.Eventually(t, func() bool {
		return svc.CaptureWorkerStats().CompletedTotal == 1 && svc.CaptureBufferStats().PendingEvents == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 0, repo.upsertCount())
	svc.captureBuffer.FlushAll(context.Background())
	require.Equal(t, 1, repo.upsertCount())
}

func TestDataSharingService_CaptureAsyncDisabledGroupDoesNotSubmit(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(repo, nil, pool)

	mode := svc.CaptureClaudeRequestAsync(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group:   &Group{ID: gid, DataSharingEnabled: false},
		},
	})

	require.Equal(t, DataSharingCaptureSubmitModeDropped, mode)
	require.Equal(t, uint64(0), svc.CaptureWorkerStats().SubmittedTotal)
	require.Equal(t, 0, repo.upsertCount())
}

func TestDataSharingService_CaptureAsyncMissingPoolDoesNotSyncFallback(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	svc := NewDataSharingService(repo, nil)

	mode := svc.CaptureOpenAIRequestAsync(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group:   &Group{ID: gid, DataSharingEnabled: true},
		},
		Provider: PlatformOpenAI,
		Model:    "gpt-5",
	})

	require.Equal(t, DataSharingCaptureSubmitModeDropped, mode)
	require.Equal(t, 0, repo.upsertCount())
	require.Equal(t, uint64(1), svc.CaptureWorkerStats().DroppedTotal)
}

func TestDataSharingService_StatsIncludesCaptureWorker(t *testing.T) {
	repo := &dataShareCaptureRepoStub{stats: &DataShareStats{SessionCount: 3}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   7,
		TaskTimeout: time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(repo, nil, pool)

	stats, err := svc.Stats(context.Background(), DataShareSessionFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.SessionCount)
	require.Equal(t, 7, stats.CaptureWorker.QueueCapacity)
	require.True(t, stats.CaptureBuffer.Enabled)
	require.Equal(t, defaultDataSharingCaptureBufferIdleSeconds, stats.CaptureBuffer.IdleFlushSeconds)
}

func TestDataSharingService_UpdateCaptureRuntimeSettingsAppliesWorkerTimeout(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.UpdateCaptureRuntimeSettings(context.Background(), DataShareCaptureRuntimeSettings{
		WorkerCount:            2,
		QueueSize:              9,
		FlushQueueSize:         12,
		TaskTimeoutSeconds:     45,
		CompressionLevel:       string(DataShareCompressionLevelDefault),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 7,
		BufferMaxSessions:      123,
		BufferMaxPendingEvents: 456,
	})
	require.NoError(t, err)
	require.Equal(t, 2, settings.WorkerCount)
	require.Equal(t, 9, settings.QueueSize)
	require.Equal(t, 12, settings.FlushQueueSize)
	require.Equal(t, 45, settings.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelDefault), settings.CompressionLevel)
	require.True(t, settings.BufferEnabled)
	require.Equal(t, 7, settings.BufferIdleFlushSeconds)
	require.Equal(t, 123, settings.BufferMaxSessions)
	require.Equal(t, 456, settings.BufferMaxPendingEvents)
	require.JSONEq(t, `{"worker_count":2,"queue_size":9,"flush_queue_size":12,"task_timeout_seconds":45,"compression_level":"default","buffer_enabled":true,"buffer_idle_flush_seconds":7,"buffer_max_sessions":123,"buffer_max_pending_events":456}`, repo.values[SettingKeyDataSharingCaptureRuntime])
	require.Equal(t, 2, svc.CaptureWorkerStats().WorkerCount)
	require.Equal(t, 9, svc.CaptureWorkerStats().QueueCapacity)
	require.Equal(t, 12, svc.CaptureWorkerStats().FlushQueueCapacity)
	require.Equal(t, 45, svc.CaptureWorkerStats().TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelDefault), svc.CaptureWorkerStats().CompressionLevel)
}

func TestDataSharingService_UpdateCaptureRuntimeSettingsClampsUpperBounds(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.UpdateCaptureRuntimeSettings(context.Background(), DataShareCaptureRuntimeSettings{
		WorkerCount:            maxDataSharingCaptureWorkerCount + 100,
		QueueSize:              maxDataSharingCaptureQueueSize + 100,
		FlushQueueSize:         maxDataSharingCaptureQueueSize + 200,
		TaskTimeoutSeconds:     maxDataSharingCaptureTaskTimeoutSeconds + 100,
		BufferIdleFlushSeconds: maxDataSharingCaptureBufferIdleSeconds + 100,
		BufferMaxSessions:      maxDataSharingCaptureBufferMaxSessions + 100,
		BufferMaxPendingEvents: maxDataSharingCaptureBufferMaxEvents + 100,
	})
	require.NoError(t, err)
	require.Equal(t, maxDataSharingCaptureWorkerCount, settings.WorkerCount)
	require.Equal(t, maxDataSharingCaptureQueueSize, settings.QueueSize)
	require.Equal(t, maxDataSharingCaptureQueueSize, settings.FlushQueueSize)
	require.Equal(t, maxDataSharingCaptureTaskTimeoutSeconds, settings.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelFastest), settings.CompressionLevel)
	require.Equal(t, maxDataSharingCaptureBufferIdleSeconds, settings.BufferIdleFlushSeconds)
	require.Equal(t, maxDataSharingCaptureBufferMaxSessions, settings.BufferMaxSessions)
	require.Equal(t, maxDataSharingCaptureBufferMaxEvents, settings.BufferMaxPendingEvents)
	require.JSONEq(t, `{"worker_count":1024,"queue_size":100000,"flush_queue_size":100000,"task_timeout_seconds":300,"compression_level":"fastest","buffer_enabled":true,"buffer_idle_flush_seconds":300,"buffer_max_sessions":100000,"buffer_max_pending_events":1000000}`, repo.values[SettingKeyDataSharingCaptureRuntime])
	require.Equal(t, maxDataSharingCaptureWorkerCount, pool.Stats().WorkerCount)
	require.Equal(t, maxDataSharingCaptureQueueSize, pool.Stats().QueueCapacity)
	require.Equal(t, maxDataSharingCaptureQueueSize, pool.Stats().FlushQueueCapacity)
	require.Equal(t, maxDataSharingCaptureTaskTimeoutSeconds, pool.Stats().TaskTimeoutSeconds)
}

func TestDataSharingService_LoadRuntimeSettingsAppliesStoredTimeout(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{SettingKeyDataSharingCaptureRuntime: `{"worker_count":4,"queue_size":10,"flush_queue_size":11,"task_timeout_seconds":90,"compression_level":"better","buffer_enabled":true,"buffer_idle_flush_seconds":9,"buffer_max_sessions":100,"buffer_max_pending_events":200}`}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.LoadRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, settings.WorkerCount)
	require.Equal(t, 10, settings.QueueSize)
	require.Equal(t, 11, settings.FlushQueueSize)
	require.Equal(t, 90, settings.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelBetter), settings.CompressionLevel)
	require.True(t, settings.BufferEnabled)
	require.Equal(t, 9, settings.BufferIdleFlushSeconds)
	require.Equal(t, 100, settings.BufferMaxSessions)
	require.Equal(t, 200, settings.BufferMaxPendingEvents)
	require.Equal(t, 4, pool.Stats().WorkerCount)
	require.Equal(t, 10, pool.Stats().QueueCapacity)
	require.Equal(t, 11, pool.Stats().FlushQueueCapacity)
	require.Equal(t, 90, pool.Stats().TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelBetter), pool.Stats().CompressionLevel)
}

func TestDataSharingService_LoadRuntimeSettingsUsesCurrentCompressionDefault(t *testing.T) {
	resetDataShareCompressionLevel(t)
	SetDataShareCompressionLevel(string(DataShareCompressionLevelBest))
	repo := &dataShareSettingRepoStub{values: map[string]string{}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.LoadRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, string(DataShareCompressionLevelBest), settings.CompressionLevel)
	require.Equal(t, string(DataShareCompressionLevelBest), pool.Stats().CompressionLevel)
}

func TestDataSharingService_LoadRuntimeSettingsClampsStoredUpperBounds(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{SettingKeyDataSharingCaptureRuntime: `{"worker_count":2048,"queue_size":999999,"task_timeout_seconds":999,"buffer_enabled":true,"buffer_idle_flush_seconds":999,"buffer_max_sessions":999999,"buffer_max_pending_events":9999999}`}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.LoadRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, maxDataSharingCaptureWorkerCount, settings.WorkerCount)
	require.Equal(t, maxDataSharingCaptureQueueSize, settings.QueueSize)
	require.Equal(t, maxDataSharingCaptureQueueSize, settings.FlushQueueSize)
	require.Equal(t, maxDataSharingCaptureTaskTimeoutSeconds, settings.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelFastest), settings.CompressionLevel)
	require.Equal(t, maxDataSharingCaptureBufferIdleSeconds, settings.BufferIdleFlushSeconds)
	require.Equal(t, maxDataSharingCaptureBufferMaxSessions, settings.BufferMaxSessions)
	require.Equal(t, maxDataSharingCaptureBufferMaxEvents, settings.BufferMaxPendingEvents)
	require.Equal(t, maxDataSharingCaptureWorkerCount, pool.Stats().WorkerCount)
	require.Equal(t, maxDataSharingCaptureQueueSize, pool.Stats().QueueCapacity)
	require.Equal(t, maxDataSharingCaptureQueueSize, pool.Stats().FlushQueueCapacity)
	require.Equal(t, maxDataSharingCaptureTaskTimeoutSeconds, pool.Stats().TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelFastest), pool.Stats().CompressionLevel)
}

func TestDataSharingService_LoadRuntimeSettingsBackfillsLegacyBufferDefaults(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{SettingKeyDataSharingCaptureRuntime: `{"worker_count":4,"queue_size":10,"task_timeout_seconds":90,"compression_level":"better"}`}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.LoadRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, settings.WorkerCount)
	require.Equal(t, 10, settings.QueueSize)
	require.Equal(t, 10, settings.FlushQueueSize)
	require.Equal(t, 90, settings.TaskTimeoutSeconds)
	require.Equal(t, string(DataShareCompressionLevelBetter), settings.CompressionLevel)
	require.True(t, settings.BufferEnabled)
	require.Equal(t, defaultDataSharingCaptureBufferIdleSeconds, settings.BufferIdleFlushSeconds)
	require.Equal(t, defaultDataSharingCaptureBufferMaxSessions, settings.BufferMaxSessions)
	require.Equal(t, defaultDataSharingCaptureBufferMaxEvents, settings.BufferMaxPendingEvents)
}

func TestDataSharingService_UpdateRuntimeSettingsBackfillsLegacyRequest(t *testing.T) {
	resetDataShareCompressionLevel(t)
	repo := &dataShareSettingRepoStub{values: map[string]string{}}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(nil, repo, pool)

	settings, err := svc.UpdateCaptureRuntimeSettings(context.Background(), DataShareCaptureRuntimeSettings{
		WorkerCount:        3,
		QueueSize:          8,
		TaskTimeoutSeconds: 60,
		CompressionLevel:   string(DataShareCompressionLevelDefault),
	})
	require.NoError(t, err)
	require.Equal(t, 8, settings.FlushQueueSize)
	require.True(t, settings.BufferEnabled)
	require.Equal(t, defaultDataSharingCaptureBufferIdleSeconds, settings.BufferIdleFlushSeconds)
	require.Equal(t, defaultDataSharingCaptureBufferMaxSessions, settings.BufferMaxSessions)
	require.Equal(t, defaultDataSharingCaptureBufferMaxEvents, settings.BufferMaxPendingEvents)
	require.JSONEq(t, `{"worker_count":3,"queue_size":8,"flush_queue_size":8,"task_timeout_seconds":60,"compression_level":"default","buffer_enabled":true,"buffer_idle_flush_seconds":30,"buffer_max_sessions":4096,"buffer_max_pending_events":65536}`, repo.values[SettingKeyDataSharingCaptureRuntime])
}

func TestDataSharingService_CaptureAsyncBuffersUntilFlush(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	pool := NewDataSharingCaptureWorkerPoolWithOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   4,
		TaskTimeout: time.Second,
	})
	t.Cleanup(pool.Stop)
	svc := NewDataSharingService(repo, nil, pool)
	svc.SetDefaultCaptureRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              4,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      4096,
		BufferMaxPendingEvents: 65536,
	})

	mode := svc.CaptureOpenAIRequestAsync(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group:   &Group{ID: gid, DataSharingEnabled: true},
		},
		Provider:        PlatformOpenAI,
		Model:           "gpt-5",
		SessionID:       "session-buffer",
		RequestID:       "request-buffer-1",
		RequestBody:     []byte(`{"messages":[{"role":"system","content":"你是编码助手"},{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"exec_command"}}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec_command","arguments":"{}"}}]}}]}`),
		InboundEndpoint: "/v1/chat/completions",
	})
	require.Equal(t, DataSharingCaptureSubmitModeEnqueued, mode)

	require.Eventually(t, func() bool {
		return svc.CaptureBufferStats().PendingEvents == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 0, repo.upsertCount())

	svc.captureBuffer.FlushAll(context.Background())
	require.Equal(t, 1, repo.upsertCount())
	require.Equal(t, uint64(1), svc.CaptureBufferStats().FlushSuccessTotal)
}

func TestDataSharingService_CaptureBufferMergesSameTrajectory(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	svc := NewDataSharingService(repo, nil)
	svc.SetDefaultCaptureRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              4,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      4096,
		BufferMaxPendingEvents: 65536,
	})

	input := func(requestID string, messages string, inputTokens int) DataShareCaptureInput {
		return DataShareCaptureInput{
			APIKey: &APIKey{
				ID:      34,
				UserID:  56,
				GroupID: &gid,
				Group:   &Group{ID: gid, DataSharingEnabled: true},
			},
			Provider:        PlatformOpenAI,
			Model:           "gpt-5",
			SessionID:       "session-buffer-merge",
			RequestID:       requestID,
			RequestBody:     []byte(`{"messages":` + messages + `,"tools":[{"type":"function","function":{"name":"exec_command"}}]}`),
			ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`),
			InboundEndpoint: "/v1/chat/completions",
			InputTokens:     inputTokens,
		}
	}
	require.NoError(t, svc.captureRequestFromJob(context.Background(), DataSharingCaptureJob{Input: input("request-1", `[{"role":"system","content":"你是编码助手"},{"role":"user","content":"hi"}]`, 10)}))
	require.NoError(t, svc.captureRequestFromJob(context.Background(), DataSharingCaptureJob{Input: input("request-2", `[{"role":"system","content":"你是编码助手"},{"role":"user","content":"hi"},{"role":"assistant","content":"ok"},{"role":"user","content":"again"}]`, 20)}))

	require.Equal(t, 0, repo.upsertCount())
	require.Equal(t, 2, svc.CaptureBufferStats().PendingEvents)
	svc.captureBuffer.FlushAll(context.Background())

	require.Equal(t, 1, repo.upsertCount())
	session := repo.lastSession()
	require.NotNil(t, session)
	require.Equal(t, 2, session.SourceRequestCount)
	require.Equal(t, int64(30), session.InputTokens)
	require.Equal(t, []string{"request-1", "request-2"}, stringsFromAny(session.Meta["source_request_ids"]))
	require.Len(t, session.Messages, 5)
}

func TestDataSharingService_CaptureBufferIdleHotUpdateFlushesSooner(t *testing.T) {
	gid := int64(12)
	repo := &dataShareCaptureRepoStub{}
	svc := NewDataSharingService(repo, &dataShareSettingRepoStub{values: map[string]string{}})
	svc.SetDefaultCaptureRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              4,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      4096,
		BufferMaxPendingEvents: 65536,
	})

	require.NoError(t, svc.captureRequestFromJob(context.Background(), DataSharingCaptureJob{Input: DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group:   &Group{ID: gid, DataSharingEnabled: true},
		},
		Provider:        PlatformOpenAI,
		Model:           "gpt-5",
		SessionID:       "session-buffer-hot-update",
		RequestID:       "request-hot-update",
		RequestBody:     []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`),
		InboundEndpoint: "/v1/chat/completions",
	}}))
	require.Equal(t, 0, repo.upsertCount())

	_, err := svc.UpdateCaptureRuntimeSettings(context.Background(), DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              4,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 1,
		BufferMaxSessions:      4096,
		BufferMaxPendingEvents: 65536,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return repo.upsertCount() == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestDataSharingCaptureBuffer_ForcedEnabledIgnoresDisabledSetting(t *testing.T) {
	var got *DataShareSession
	systemPrompt := "你是编码助手"
	buffer := NewDataSharingCaptureBuffer(DataSharingCaptureBufferOptions{
		Flush: func(_ context.Context, session *DataShareSession) error {
			got = session
			return nil
		},
	})
	buffer.UpdateRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          false,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      16,
		BufferMaxPendingEvents: 16,
	})

	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-bypass",
		SessionID:          "sess-bypass",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		SystemPrompt:       &systemPrompt,
		Tools:              []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}},
		Messages: []map[string]any{
			{"role": "system", "content": "你是编码助手"},
			{"role": "user", "content": "hi"},
			{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command"}}},
			{"role": "tool", "tool_call_id": "call_1", "content": "ok"},
			{"role": "assistant", "content": "ok"},
		},
		Usage: map[string]any{"total_tokens": 3},
		Meta:  map[string]any{"request_id": "req-bypass"},
	}))
	require.Nil(t, got)
	require.True(t, buffer.Stats().Enabled)

	buffer.FlushAll(context.Background())
	require.NotNil(t, got)
	require.Equal(t, DataShareQualityComplete, got.QualityStatus)
	require.True(t, got.Exportable)
	require.NotEmpty(t, got.SessionJSON)
	require.Greater(t, got.StorageBytes, int64(0))
}

func TestDataSharingCaptureBuffer_RetainsSessionWhenFlushFails(t *testing.T) {
	wantErr := errors.New("boom")
	buffer := NewDataSharingCaptureBuffer(DataSharingCaptureBufferOptions{
		Flush: func(context.Context, *DataShareSession) error {
			return wantErr
		},
	})
	buffer.UpdateRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      16,
		BufferMaxPendingEvents: 16,
	})

	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-failed",
		SessionID:          "sess-failed",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "user", "content": "hi"}},
		Meta:               map[string]any{"request_id": "req-failed"},
	}))
	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-failed",
		SessionID:          "sess-failed",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "assistant", "content": "ok"}},
		Meta:               map[string]any{"request_id": "req-failed-2"},
	}))

	buffer.FlushAll(context.Background())
	stats := buffer.Stats()
	require.Equal(t, uint64(1), stats.FlushFailedTotal)
	require.Equal(t, 1, stats.BufferedSessions)
	require.Equal(t, 2, stats.PendingEvents)
	require.Contains(t, stats.LastError, "boom")
}

func TestDataSharingCaptureBuffer_HydratesColdSessionBeforeMerge(t *testing.T) {
	systemPrompt := "你是编码助手"
	var got *DataShareSession
	buffer := NewDataSharingCaptureBuffer(DataSharingCaptureBufferOptions{
		Hydrate: func(_ context.Context, key string) (*DataShareSession, error) {
			require.Equal(t, "traj-cold", key)
			return &DataShareSession{
				TrajectoryID:       "traj-cold",
				SessionID:          "sess-cold",
				Dataset:            defaultDataShareDataset,
				Provider:           PlatformOpenAI,
				Model:              "gpt-5",
				SourceRequestCount: 1,
				SystemPrompt:       &systemPrompt,
				Messages:           []map[string]any{{"role": "user", "content": "old"}},
				Usage:              map[string]any{"input_tokens": 3},
				InputTokens:        3,
			}, nil
		},
		Flush: func(_ context.Context, session *DataShareSession) error {
			got = cloneBufferedDataShareSession(session)
			return nil
		},
	})
	buffer.UpdateRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      16,
		BufferMaxPendingEvents: 16,
	})

	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-cold",
		SessionID:          "sess-cold",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "assistant", "content": "new"}},
		Usage:              map[string]any{"input_tokens": 7},
		InputTokens:        7,
	}))
	buffer.FlushAll(context.Background())

	require.NotNil(t, got)
	require.Equal(t, 2, got.SourceRequestCount)
	require.Equal(t, int64(10), got.InputTokens)
	require.Len(t, got.Messages, 2)
}

func TestDataSharingCaptureBuffer_FlushAllWaitsForHydrate(t *testing.T) {
	hydrateStarted := make(chan struct{})
	releaseHydrate := make(chan struct{})
	flushDone := make(chan struct{})
	var flushed *DataShareSession
	buffer := NewDataSharingCaptureBuffer(DataSharingCaptureBufferOptions{
		Hydrate: func(_ context.Context, key string) (*DataShareSession, error) {
			require.Equal(t, "traj-wait-hydrate", key)
			close(hydrateStarted)
			<-releaseHydrate
			return nil, ErrDataShareSessionNotFound
		},
		Flush: func(_ context.Context, session *DataShareSession) error {
			flushed = cloneBufferedDataShareSession(session)
			close(flushDone)
			return nil
		},
	})
	buffer.UpdateRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      16,
		BufferMaxPendingEvents: 16,
	})

	submitDone := make(chan error, 1)
	go func() {
		submitDone <- buffer.Submit(context.Background(), &DataShareSession{
			TrajectoryID:       "traj-wait-hydrate",
			SessionID:          "sess-wait-hydrate",
			Dataset:            defaultDataShareDataset,
			Provider:           PlatformOpenAI,
			Model:              "gpt-5",
			SourceRequestCount: 1,
			Messages:           []map[string]any{{"role": "user", "content": "after hydrate"}},
		})
	}()
	<-hydrateStarted

	flushAllDone := make(chan struct{})
	go func() {
		buffer.FlushAll(context.Background())
		close(flushAllDone)
	}()

	select {
	case <-flushAllDone:
		t.Fatal("FlushAll returned before hydrate completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseHydrate)
	require.NoError(t, <-submitDone)
	select {
	case <-flushDone:
	case <-time.After(time.Second):
		t.Fatal("FlushAll did not flush after hydrate completed")
	}
	select {
	case <-flushAllDone:
	case <-time.After(time.Second):
		t.Fatal("FlushAll did not return after flush")
	}
	require.NotNil(t, flushed)
	require.Equal(t, "after hydrate", flushed.Messages[0]["content"])
}

func TestDataSharingCaptureBuffer_MergesNewEventsWhileFlushInFlight(t *testing.T) {
	flushStarted := make(chan struct{})
	releaseFlush := make(chan struct{})
	var flushed []*DataShareSession
	buffer := NewDataSharingCaptureBuffer(DataSharingCaptureBufferOptions{
		Flush: func(_ context.Context, session *DataShareSession) error {
			flushed = append(flushed, cloneBufferedDataShareSession(session))
			if len(flushed) == 1 {
				close(flushStarted)
				<-releaseFlush
			}
			return nil
		},
	})
	buffer.UpdateRuntimeSettings(DataShareCaptureRuntimeSettings{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeoutSeconds:     1,
		CompressionLevel:       string(DataShareCompressionLevelFastest),
		BufferEnabled:          true,
		BufferIdleFlushSeconds: 30,
		BufferMaxSessions:      16,
		BufferMaxPendingEvents: 16,
	})

	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-inflight",
		SessionID:          "sess-inflight",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "user", "content": "first"}},
		Meta:               map[string]any{"request_id": "first"},
	}))
	buffer.mu.Lock()
	buffer.startFlushLocked(buffer.entries["traj-inflight"], context.Background(), true)
	buffer.mu.Unlock()

	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		t.Fatal("flush did not start")
	}
	require.NoError(t, buffer.Submit(context.Background(), &DataShareSession{
		TrajectoryID:       "traj-inflight",
		SessionID:          "sess-inflight",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5",
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "assistant", "content": "second"}},
		Meta:               map[string]any{"request_id": "second"},
	}))
	close(releaseFlush)
	buffer.flushWG.Wait()

	stats := buffer.Stats()
	require.Equal(t, 1, stats.BufferedSessions)
	require.Equal(t, 1, stats.PendingEvents)
	buffer.FlushAll(context.Background())
	require.Len(t, flushed, 2)
	require.Equal(t, 2, flushed[1].SourceRequestCount)
	require.Len(t, flushed[1].Messages, 2)
	require.Equal(t, "first", flushed[1].Messages[0]["content"])
	require.Equal(t, "second", flushed[1].Messages[1]["content"])
}

func TestBuildSessionUsesActualUpstreamModel(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider:        PlatformOpenAI,
		Model:           "gpt-5-alias",
		UpstreamModel:   "gpt-5-2026-05-01",
		SessionID:       "session-1",
		RequestID:       "request-1",
		RequestBody:     []byte(`{"model":"gpt-5-alias","messages":[{"role":"system","content":"你是编码助手"},{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"README.md"}],"tools":[{"type":"function","function":{"name":"exec_command","description":"运行命令","parameters":{"type":"object"}}}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"id":"resp_1"}`),
		InboundEndpoint: "v1/chat/completions",
		UserAgent:       "codex-cli/1.0",
		InputTokens:     10,
		OutputTokens:    5,
	})

	if session.Model != "gpt-5-2026-05-01" {
		t.Fatalf("model = %q, want actual upstream model", session.Model)
	}
	if got := session.SessionJSON["model"]; got != "gpt-5-2026-05-01" {
		t.Fatalf("session_json.model = %v, want actual upstream model", got)
	}
	if got := session.Meta["requested_model"]; got != "gpt-5-alias" {
		t.Fatalf("meta.requested_model = %v, want client requested model", got)
	}
	if got := session.RequestPath; got != "/v1/chat/completions" {
		t.Fatalf("request_path = %q, want normalized inbound path", got)
	}
	if got := session.SessionJSON["request_path"]; got != "/v1/chat/completions" {
		t.Fatalf("session_json.request_path = %v, want normalized inbound path", got)
	}
	if got := session.Meta["inbound_endpoint"]; got != "/v1/chat/completions" {
		t.Fatalf("meta.inbound_endpoint = %v, want normalized inbound path", got)
	}
	if got := session.UserAgent; got != "codex-cli" {
		t.Fatalf("user_agent = %q, want captured client user agent", got)
	}
	if got := session.SessionJSON["user_agent"]; got != "codex-cli" {
		t.Fatalf("session_json.user_agent = %v, want captured client user agent", got)
	}
	if got := session.Meta["user_agent"]; got != "codex-cli/1.0" {
		t.Fatalf("meta.user_agent = %v, want captured client user agent", got)
	}
	if got := session.Meta["user_agent_family"]; got != "codex-cli" {
		t.Fatalf("meta.user_agent_family = %v, want normalized client family", got)
	}
	if session.Exportable != true {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
}

func TestBuildSessionCapturesOpenAIResponsesInputAndOutput(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformOpenAI,
		Model:    "gpt-5.5",
		RequestBody: []byte(`{
			"model":"gpt-5.5",
				"input":[
					{"type":"message","role":"system","content":[{"type":"input_text","text":"你是编码助手"}]},
					{"type":"message","role":"user","content":[{"type":"input_text","text":"请列目录"}]},
					{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"},
					{"type":"function_call_output","call_id":"call_1","output":"README.md"}
				],
			"tools":[{"type":"function","name":"exec_command","description":"运行命令","parameters":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"resp_1",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"看到了 README.md"}]}],
			"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}
		}`),
		InputTokens:  10,
		OutputTokens: 5,
	})

	if len(session.Messages) != 5 {
		t.Fatalf("message count = %d, want 5: %#v", len(session.Messages), session.Messages)
	}
	if got := session.Messages[0]["role"]; got != "system" {
		t.Fatalf("first role = %v, want system", got)
	}
	if got := session.Messages[2]["role"]; got != "assistant" {
		t.Fatalf("function_call role = %v, want assistant", got)
	}
	if calls, ok := session.Messages[2]["tool_calls"].([]map[string]any); !ok || len(calls) != 1 || calls[0]["id"] != "call_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool_calls not normalized: %#v", session.Messages[2]["tool_calls"])
	}
	if got := session.Messages[3]["role"]; got != "tool" {
		t.Fatalf("function_call_output role = %v, want tool", got)
	}
	if got := session.Messages[3]["status"]; got != "success" {
		t.Fatalf("tool status = %v, want success", got)
	}
	if got := session.Messages[3]["is_error"]; got != false {
		t.Fatalf("tool is_error = %v, want false", got)
	}
	if got := session.Messages[4]["role"]; got != "assistant" {
		t.Fatalf("response role = %v, want assistant", got)
	}
	if session.SystemPrompt == nil || *session.SystemPrompt == "" {
		t.Fatalf("system_prompt missing")
	}
	if got := session.Meta["source_request_ids"]; got == nil {
		t.Fatalf("source_request_ids missing")
	}
	if !session.Exportable {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
	if session.QualityStatus != DataShareQualityComplete {
		t.Fatalf("quality_status = %q, want complete", session.QualityStatus)
	}
}

func TestBuildSessionCapturesAnthropicResponseBody(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformAnthropic,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformAnthropic,
		Model:    "claude-sonnet-4-5-20250929",
		RequestBody: []byte(`{
			"model":"claude-sonnet-4-5-20250929",
			"system":"你是编码助手",
			"messages":[
				{"role":"user","content":"列目录"},
				{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"exec_command","input":{"cmd":"ls"}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"README.md"}]}
			],
			"tools":[{"name":"exec_command","description":"运行命令","input_schema":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"content":[{"type":"text","text":"看到了 README.md"}],
			"usage":{"input_tokens":10,"output_tokens":5}
		}`),
		InboundEndpoint: "/v1/messages",
		InputTokens:     10,
		OutputTokens:    5,
	})

	if got := session.RequestPath; got != "/v1/messages" {
		t.Fatalf("request_path = %q, want /v1/messages", got)
	}
	if len(session.Messages) != 4 {
		t.Fatalf("message count = %d, want 4: %#v", len(session.Messages), session.Messages)
	}
	if got := session.Messages[1]["role"]; got != "assistant" {
		t.Fatalf("tool_use role = %v, want assistant", got)
	}
	calls, ok := session.Messages[1]["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 || calls[0]["id"] != "toolu_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool_calls not normalized: %#v", session.Messages[1]["tool_calls"])
	}
	if got := session.Messages[2]["role"]; got != "tool" {
		t.Fatalf("tool_result role = %v, want tool", got)
	}
	if got := session.Messages[2]["tool_call_id"]; got != "toolu_1" {
		t.Fatalf("tool_call_id = %v, want toolu_1", got)
	}
	if got := session.Messages[3]["role"]; got != "assistant" {
		t.Fatalf("response role = %v, want assistant", got)
	}
	if got := dataShareContentText(session.Messages[3]["content"]); got != "看到了 README.md" {
		t.Fatalf("response content = %q, want assistant text", got)
	}
	if !session.Exportable {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
}

func TestAnthropicStreamAccumulatorBuildsFinalMessage(t *testing.T) {
	acc := &anthropicStreamResponseAccumulator{}
	acc.ObserveData("", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":10}}}`)
	acc.ObserveData("", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"exec_command","input":{}}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\""}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"ls\"}"}}`)
	acc.ObserveData("", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"看到了 README.md"}}`)
	body := acc.ObserveData("", `{"type":"message_delta","usage":{"output_tokens":5},"delta":{"stop_reason":"end_turn"}}`)

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid accumulated body: %v", err)
	}
	messages := normalizeDataShareMessages([]map[string]any{got})
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1: %#v", len(messages), messages)
	}
	calls, ok := messages[0]["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls not normalized: %#v", messages[0]["tool_calls"])
	}
	if calls[0]["id"] != "toolu_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool call mismatch: %#v", calls[0])
	}
	args, ok := calls[0]["arguments"].(map[string]any)
	if !ok || args["cmd"] != "ls" {
		t.Fatalf("tool arguments mismatch: %#v", calls[0]["arguments"])
	}
	if got := dataShareContentText(got["content"]); got != "看到了 README.md" {
		t.Fatalf("content text = %q, want assistant text", got)
	}
}

func TestBuildSessionFiltersOrdinaryResponsesChat(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformOpenAI,
		Model:    "gpt-5.5",
		RequestBody: []byte(`{
			"model":"gpt-5.5",
			"input":[
				{"type":"message","role":"system","content":[{"type":"input_text","text":"你是编码助手"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
			],
			"tools":[{"type":"function","name":"exec_command","description":"运行命令","parameters":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"resp_ordinary",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],
			"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}
		}`),
		InputTokens:  10,
		OutputTokens: 5,
	})

	if session.Exportable {
		t.Fatalf("ordinary no-tool session should not be exportable")
	}
	if session.QualityStatus != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", session.QualityStatus)
	}
	if !containsString(session.QualityErrors, "missing_structured_tool_call") {
		t.Fatalf("quality_errors = %v, want missing_structured_tool_call", session.QualityErrors)
	}
}

func TestCaptureSkipRulesDefaultMatching(t *testing.T) {
	ctx := context.Background()
	svc := NewDataSharingService(nil, &dataShareSettingRepoStub{values: map[string]string{}})

	cases := []struct {
		name  string
		input DataShareCaptureInput
		want  bool
	}{
		{
			name: "Claude Code title generator",
			input: DataShareCaptureInput{
				UserAgent:       "claude-cli/2.1.142 (external, cli)",
				InboundEndpoint: "/v1/messages",
				RequestBody: []byte(`{
					"model":"gpt-5.5",
					"system":[
						{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},
						{"type":"text","text":"Generate a concise, sentence-case title (3-7 words) that captures the main topic."}
					],
					"messages":[{"role":"user","content":"<session>看看 Documents 里面有什么</session>"}]
				}`),
			},
			want: true,
		},
		{
			name: "opencode chat completions title generator",
			input: DataShareCaptureInput{
				UserAgent:       "opencode/0.7.0",
				InboundEndpoint: "/v1/chat/completions",
				RequestBody: []byte(`{
					"model":"claude-sonnet-4-5",
					"messages":[
						{"role":"system","content":"You are a title generator. You output ONLY a thread title. Nothing else.\nGenerate a brief title that would help the user find this conversation later.\nNEVER respond to questions, just generate a title for the conversation"},
						{"role":"user","content":"Generate a title for this conversation:"},
						{"role":"user","content":"hi"}
					]
				}`),
			},
			want: true,
		},
		{
			name: "opencode messages title generator",
			input: DataShareCaptureInput{
				UserAgent:       "opencode/0.7.0",
				InboundEndpoint: "/v1/messages",
				RequestBody: []byte(`{
					"model":"claude-sonnet-4-5",
					"system":"You are a title generator. You output ONLY a thread title. Nothing else.",
					"messages":[{"role":"user","content":"Generate a title for this conversation:\n\nhi"}]
				}`),
			},
			want: true,
		},
		{
			name: "opencode responses title generator",
			input: DataShareCaptureInput{
				UserAgent:       "opencode/1.15.11 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14",
				InboundEndpoint: "/v1/responses",
				RequestBody: []byte(`{
					"model":"gpt-5.5",
					"input":[
						{"role":"system","content":"You are a title generator. You output ONLY a thread title. Nothing else.\n\n<task>\nGenerate a brief title that would help the user find this conversation later.\n</task>\n\n<rules>\n- NEVER respond to questions, just generate a title for the conversation\n</rules>"},
						{"role":"user","content":"Generate a title for this conversation:\n"},
						{"role":"user","content":"hi"}
					]
				}`),
			},
			want: true,
		},
		{
			name: "opencode normal task",
			input: DataShareCaptureInput{
				UserAgent:       "opencode/0.7.0",
				InboundEndpoint: "/v1/messages",
				RequestBody:     []byte(`{"model":"claude-sonnet-4-5","system":"你是编码助手","messages":[{"role":"user","content":"帮我检查这个函数"}]}`),
			},
			want: false,
		},
		{
			name: "default excluded requested model",
			input: DataShareCaptureInput{
				UserAgent:       "codex-cli/1.0",
				InboundEndpoint: "/v1/responses",
				Model:           "gpt-5.4-mini",
				UpstreamModel:   "gpt-5.4-mini-openai-compact",
				RequestBody:     []byte(`{"model":"gpt-5.4-mini","input":"帮我检查这个函数"}`),
			},
			want: true,
		},
		{
			name: "default excluded model from request body",
			input: DataShareCaptureInput{
				UserAgent:       "codex-cli/1.0",
				InboundEndpoint: "/v1/responses",
				RequestBody:     []byte(`{"model":"codex-auto-review","input":"review this change"}`),
			},
			want: true,
		},
		{
			name: "ordinary title request",
			input: DataShareCaptureInput{
				UserAgent:       "curl/8.0",
				InboundEndpoint: "/v1/chat/completions",
				RequestBody:     []byte(`{"model":"gpt-5.5","messages":[{"role":"system","content":"你是写作助手"},{"role":"user","content":"帮我写一个标题"}]}`),
			},
			want: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.shouldSkipDataShareCapture(ctx, tt.input); got != tt.want {
				t.Fatalf("shouldSkipDataShareCapture = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCaptureSkipRulesFallbackAndUpdate(t *testing.T) {
	ctx := context.Background()
	repo := &dataShareSettingRepoStub{values: map[string]string{
		SettingKeyDataSharingCaptureSkipRules: "{not-json",
	}}
	svc := NewDataSharingService(nil, repo)

	rules, err := svc.GetCaptureSkipRules(ctx)
	if err != nil {
		t.Fatalf("GetCaptureSkipRules error = %v", err)
	}
	if len(rules) == 0 || rules[0].ID != "claude_code_title" {
		t.Fatalf("rules fallback mismatch: %#v", rules)
	}

	custom := []DataShareCaptureSkipRule{{
		ID:           "custom_warmup",
		Name:         "自定义预热",
		Enabled:      true,
		RequestPaths: []string{"v1/responses"},
		FieldScopes:  []string{"input"},
		Patterns:     []string{"Warmup"},
		MatchMode:    "equals",
	}}
	updated, err := svc.UpdateCaptureSkipRules(ctx, custom)
	if err != nil {
		t.Fatalf("UpdateCaptureSkipRules error = %v", err)
	}
	if len(updated) != 1 || updated[0].RequestPaths[0] != "/v1/responses" {
		t.Fatalf("updated rules mismatch: %#v", updated)
	}
	if repo.values[SettingKeyDataSharingCaptureSkipRules] == "" {
		t.Fatalf("rules were not persisted")
	}
	if !svc.shouldSkipDataShareCapture(ctx, DataShareCaptureInput{
		UserAgent:       "custom-client/1.0",
		InboundEndpoint: "/v1/responses",
		RequestBody:     []byte(`{"model":"gpt-5.5","input":"Warmup"}`),
	}) {
		t.Fatalf("custom warmup rule should skip matching request")
	}

	modelOnly := []DataShareCaptureSkipRule{{
		ID:      "custom_model",
		Name:    "自定义模型",
		Enabled: true,
		Models:  []string{"codex-auto-review"},
	}}
	updated, err = svc.UpdateCaptureSkipRules(ctx, modelOnly)
	if err != nil {
		t.Fatalf("UpdateCaptureSkipRules model-only error = %v", err)
	}
	if len(updated) != 1 || len(updated[0].Models) != 1 || updated[0].Models[0] != "codex-auto-review" {
		t.Fatalf("model-only rule mismatch: %#v", updated)
	}
	if !svc.shouldSkipDataShareCapture(ctx, DataShareCaptureInput{
		UserAgent:       "custom-client/1.0",
		InboundEndpoint: "/v1/responses",
		RequestBody:     []byte(`{"model":"codex-auto-review","input":"real task"}`),
	}) {
		t.Fatalf("custom model-only rule should skip matching request")
	}
}

func TestDataShareExportTicketRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := &dataShareSettingRepoStub{values: map[string]string{}}
	svc := NewDataSharingService(nil, repo)

	ticket, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:    DataShareExportScopeUser,
		UserID:   42,
		Filters:  DataShareSessionFilters{UserID: 42, IDs: []int64{1, 2}},
		Filename: "my-export",
	})
	if err != nil {
		t.Fatalf("CreateExportTicket error = %v", err)
	}
	if !strings.HasSuffix(ticket.Filename, ".jsonl.zst") {
		t.Fatalf("filename = %q, want .jsonl.zst suffix", ticket.Filename)
	}
	if !strings.Contains(ticket.DownloadURL, "/data-sharing/export/download?ticket=") {
		t.Fatalf("download_url = %q", ticket.DownloadURL)
	}
	if repo.values[SettingKeyDataSharingExportTicketKey] == "" {
		t.Fatalf("ticket signing key was not persisted")
	}

	claims, err := svc.ParseExportTicket(ctx, DataShareExportScopeUser, ticket.Token)
	if err != nil {
		t.Fatalf("ParseExportTicket error = %v", err)
	}
	if claims.UserID != 42 || len(claims.Filters.IDs) != 2 {
		t.Fatalf("claims mismatch: %#v", claims)
	}
}

func TestDataShareExportTicketFilenameEncoding(t *testing.T) {
	ctx := context.Background()
	svc := NewDataSharingService(nil, &dataShareSettingRepoStub{values: map[string]string{}})

	zstdTicket, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:    DataShareExportScopeAdmin,
		Filters:  DataShareSessionFilters{IDs: []int64{1}},
		Filename: "admin-data-sharing.jsonl",
	})
	if err != nil {
		t.Fatalf("CreateExportTicket zstd error = %v", err)
	}
	if zstdTicket.Filename != "admin-data-sharing.jsonl.zst" {
		t.Fatalf("zstd filename = %q, want admin-data-sharing.jsonl.zst", zstdTicket.Filename)
	}
	if zstdTicket.Encoding != string(DataShareExportEncodingZstd) {
		t.Fatalf("zstd encoding = %q", zstdTicket.Encoding)
	}

	plainTicket, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:    DataShareExportScopeAdmin,
		Filters:  DataShareSessionFilters{IDs: []int64{1}},
		Filename: "admin-data-sharing-session-1.jsonl.zst",
		Encoding: DataShareExportEncodingJSONL,
	})
	if err != nil {
		t.Fatalf("CreateExportTicket jsonl error = %v", err)
	}
	if plainTicket.Filename != "admin-data-sharing-session-1.jsonl" {
		t.Fatalf("plain filename = %q, want admin-data-sharing-session-1.jsonl", plainTicket.Filename)
	}
	if plainTicket.Encoding != string(DataShareExportEncodingJSONL) {
		t.Fatalf("plain encoding = %q", plainTicket.Encoding)
	}

	jsonTicket, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:    DataShareExportScopeAdmin,
		Filters:  DataShareSessionFilters{IDs: []int64{1}},
		Filename: "admin-data-sharing-session-1.jsonl.zst",
		Encoding: DataShareExportEncodingJSON,
	})
	if err != nil {
		t.Fatalf("CreateExportTicket json error = %v", err)
	}
	if jsonTicket.Filename != "admin-data-sharing-session-1.json" {
		t.Fatalf("json filename = %q, want admin-data-sharing-session-1.json", jsonTicket.Filename)
	}
	if jsonTicket.Encoding != string(DataShareExportEncodingJSON) {
		t.Fatalf("json encoding = %q", jsonTicket.Encoding)
	}
}

func TestDataShareExportTicketRejectsScopeMismatch(t *testing.T) {
	ctx := context.Background()
	svc := NewDataSharingService(nil, &dataShareSettingRepoStub{values: map[string]string{}})

	ticket, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:   DataShareExportScopeAdmin,
		Filters: DataShareSessionFilters{IDs: []int64{1}},
	})
	if err != nil {
		t.Fatalf("CreateExportTicket error = %v", err)
	}
	if _, err := svc.ParseExportTicket(ctx, DataShareExportScopeUser, ticket.Token); err == nil {
		t.Fatalf("ParseExportTicket should reject scope mismatch")
	}
}

func TestDataShareExportTicketRejectsUserFilterMismatch(t *testing.T) {
	ctx := context.Background()
	svc := NewDataSharingService(nil, &dataShareSettingRepoStub{values: map[string]string{}})

	_, err := svc.CreateExportTicket(ctx, DataShareExportTicketRequest{
		Scope:   DataShareExportScopeUser,
		UserID:  42,
		Filters: DataShareSessionFilters{UserID: 7, IDs: []int64{1}},
	})
	if err == nil {
		t.Fatalf("CreateExportTicket should reject mismatched user filter")
	}
}

func TestExportPayloadNormalizesLegacyRecord(t *testing.T) {
	sys := ""
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}, "type": "function"},
			{"name": "apply_patch", "description": "Use the apply_patch tool", "type": "custom"},
			{"name": "mcp__node_repl__", "description": "Node namespace", "type": "namespace", "tools": []any{
				map[string]any{"name": "js", "description": "运行 JavaScript", "parameters": map[string]any{"type": "object"}, "type": "function"},
			}},
			{"type": "web_search"},
		},
		Messages: []map[string]any{
			{"role": "system", "content": "你是编码助手"},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "type": "function_call", "call_id": "call_1", "name": "exec_command", "arguments": `{"cmd":"ls"}`},
			{"role": "tool", "type": "function_call_output", "call_id": "call_1", "output": "README.md\nProcess exited with code 0"},
			{"role": "assistant", "content": "看到了 README.md"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_read_tokens": 2, "total_tokens": 17},
		Meta:  map[string]any{"request_id": "req_1"},
	}

	payload := exportPayloadFromSession(session)
	if got := payload["system_prompt"]; got != "你是编码助手" {
		t.Fatalf("system_prompt = %v", got)
	}
	tools, ok := payload["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools type = %T", payload["tools"])
	}
	for _, tool := range tools {
		if tool["name"] == "" || tool["description"] == "" || tool["parameters"] == nil {
			t.Fatalf("invalid tool after normalize: %#v", tool)
		}
	}
	messages, ok := payload["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("messages type = %T", payload["messages"])
	}
	calls, ok := messages[2]["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("tool_calls type = %T", messages[2]["tool_calls"])
	}
	if calls[0]["id"] != "call_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool call not normalized: %#v", calls[0])
	}
	if messages[3]["status"] != "success" || messages[3]["is_error"] != false {
		t.Fatalf("tool result not normalized: %#v", messages[3])
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type = %T", payload["meta"])
	}
	sourceIDs, ok := meta["source_request_ids"].([]string)
	if !ok {
		t.Fatalf("source_request_ids type = %T", meta["source_request_ids"])
	}
	if len(sourceIDs) != 1 || sourceIDs[0] != "req_1" {
		t.Fatalf("source_request_ids = %#v", sourceIDs)
	}
	if errs := validateDataSharePayloadQuality(payload); len(errs) != 0 {
		t.Fatalf("payload quality errors = %v", errs)
	}
}

func TestExportPayloadDedupesRepeatedResponsesHistory(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 7,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}, "type": "function"},
		},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_dup", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_dup", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_dup", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_dup", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "看到了 README.md"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		Meta:  map[string]any{"source_request_ids": []any{"req_1", "req_2"}},
	}

	payload := exportPayloadFromSession(session)
	messages, ok := payload["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("messages type = %T", payload["messages"])
	}
	callCount := 0
	resultCount := 0
	for _, msg := range messages {
		callCount += len(anySlice(msg["tool_calls"]))
		if msg["role"] == "tool" {
			resultCount++
		}
	}
	if callCount != 1 || resultCount != 1 {
		t.Fatalf("dedupe failed, callCount=%d resultCount=%d messages=%#v", callCount, resultCount, messages)
	}
	if errs := validateDataSharePayloadQuality(payload); len(errs) != 0 {
		t.Fatalf("payload quality errors = %v", errs)
	}
}

func TestCompactDataShareMessagesKeepsUnfinishedTail(t *testing.T) {
	sys := map[string]any{"role": "system", "content": "你是编码助手"}
	user := map[string]any{"role": "user", "content": "列目录"}
	firstCall := map[string]any{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}}
	firstResult := map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false}
	final := map[string]any{"role": "assistant", "content": "看到了 README.md"}
	tailCall := map[string]any{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_2", "name": "exec_command", "arguments": map[string]any{"cmd": "pwd"}}}}
	messages := []map[string]any{sys, user, firstCall, firstResult, sys, user, firstCall, firstResult, final, tailCall}

	compact := CompactDataShareMessages(messages)
	if len(compact) != 6 {
		t.Fatalf("compact len = %d, want 6: %#v", len(compact), compact)
	}
	if compact[5]["tool_calls"] == nil {
		t.Fatalf("unfinished tail call should be retained")
	}
	errs := ValidateDataShareSessionQuality("gpt-5.5", "你是编码助手", compact, []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}}, map[string]any{"total_tokens": 1})
	if !containsString(errs, "tool_call_result_unpaired") {
		t.Fatalf("quality_errors = %v, want unfinished tail to remain unpaired", errs)
	}
	if status := DataSharePayloadQualityStatus("gpt-5.5", "你是编码助手", compact, []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}}, map[string]any{"total_tokens": 1}); status != DataShareQualityPartial {
		t.Fatalf("quality_status = %q, want partial", status)
	}
}

func TestDataShareQualityAllowsMissingUsageTokens(t *testing.T) {
	sys := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": "列目录"},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
		{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false},
		{"role": "assistant", "content": "看到了 README.md"},
	}
	tools := []map[string]any{
		{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
	}
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}

	// 交付说明允许 token 用量无法聚合时为空/为 0，不能因此误判完整工具 session 无效。
	if errs := ValidateDataShareSessionQuality("gpt-5.5", sys, messages, tools, usage); len(errs) != 0 {
		t.Fatalf("quality_errors = %v, want none", errs)
	}
	if status := DataSharePayloadQualityStatus("gpt-5.5", sys, messages, tools, usage); status != DataShareQualityComplete {
		t.Fatalf("quality_status = %q, want complete", status)
	}
}

func TestDataShareQualityStatusNormalizesLegacyPartialPayload(t *testing.T) {
	sys := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": "列目录"},
		{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "id": "toolu_1", "name": "exec_command", "input": map[string]any{"cmd": "ls"}},
			},
		},
		{
			"role": "user",
			"content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "content": "README.md"},
			},
		},
		{"role": "assistant", "content": "看到了 README.md"},
		{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "id": "toolu_tail", "name": "exec_command", "input": map[string]any{"cmd": "pwd"}},
			},
		},
	}
	tools := []map[string]any{
		{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
	}

	if status := DataSharePayloadQualityStatus("gpt-5.5", sys, messages, tools, map[string]any{"total_tokens": 15}); status != DataShareQualityPartial {
		t.Fatalf("quality_status = %q, want partial", status)
	}
}

func TestDataShareQualityStatusDoesNotTrimUnfixableErrors(t *testing.T) {
	sys := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": "列目录"},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
		{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false},
		{"role": "assistant", "content": "看到了 README.md"},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_tail", "name": "exec_command", "arguments": map[string]any{"cmd": "pwd"}}}},
	}

	if status := DataSharePayloadQualityStatus("gpt-5.5", sys, messages, nil, map[string]any{"total_tokens": 15}); status != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", status)
	}
}

func TestDataShareQualityStatusDoesNotNormalizeUnfixableErrors(t *testing.T) {
	sys := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": "列目录"},
		{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "id": "toolu_1", "name": "exec_command", "input": map[string]any{"cmd": "ls"}},
			},
		},
		{
			"role": "user",
			"content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "content": "README.md"},
			},
		},
		{"role": "assistant", "content": "看到了 README.md"},
	}
	tools := []map[string]any{
		{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
	}

	if status := DataSharePayloadQualityStatus("gpt-4.1", sys, messages, tools, map[string]any{"total_tokens": 15}); status != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", status)
	}
}

func TestExportableDataShareMessagesAllowsCompleteSnapshot(t *testing.T) {
	sys := "你是编码助手"
	messages := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": "列目录"},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
		{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false},
		{"role": "assistant", "content": "看到了 README.md"},
	}
	tools := []map[string]any{
		{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
	}

	exported, errs := exportableDataShareMessages("gpt-5.5", sys, messages, tools, map[string]any{"total_tokens": 15})
	if len(errs) != 0 {
		t.Fatalf("quality_errors = %v, want none", errs)
	}
	if len(exported) != len(messages) {
		t.Fatalf("exported len = %d, want %d", len(exported), len(messages))
	}
}

func TestPartialSessionExportsRawSnapshot(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusTerminated,
		IsFinalSnapshot:    false,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
		},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "看到了 README.md"},
			{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_tail", "name": "exec_command", "arguments": map[string]any{"cmd": "pwd"}}}},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		Meta:  map[string]any{"source_request_ids": []string{"req_1"}},
	}
	status := DataSharePayloadQualityStatus(session.Model, sys, session.Messages, session.Tools, session.Usage)
	if status != DataShareQualityPartial {
		t.Fatalf("quality_status = %q, want partial", status)
	}

	var buf bytes.Buffer
	if err := WriteSingleSessionJSONL(&buf, session); err != nil {
		t.Fatalf("WriteSingleSessionJSONL returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload); err != nil {
		t.Fatalf("invalid jsonl: %v", err)
	}
	messages := mapsFromAny(payload["messages"])
	if len(messages) != 6 {
		t.Fatalf("exported messages len = %d, want raw len 6: %#v", len(messages), messages)
	}
	if got := payload["status"]; got != DataShareStatusTerminated {
		t.Fatalf("exported status = %v, want terminated", got)
	}
	if got := payload["is_final_snapshot"]; got != false {
		t.Fatalf("exported is_final_snapshot = %v, want false", got)
	}
	if _, ok := payload["quality_status"]; ok {
		t.Fatalf("quality_status should not be included in JSONL payload")
	}
	if errs := validateDataSharePayloadQuality(payload); !containsString(errs, "tool_call_result_unpaired") {
		t.Fatalf("payload quality errors = %v, want unpaired tail retained", errs)
	}
}

func TestInvalidSessionCanExportWhenSelected(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusTerminated,
		IsFinalSnapshot:    false,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools:              []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hello"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
	if status := DataSharePayloadQualityStatus(session.Model, sys, session.Messages, session.Tools, session.Usage); status != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", status)
	}
	var buf bytes.Buffer
	if err := WriteSingleSessionJSONL(&buf, session); err != nil {
		t.Fatalf("WriteSingleSessionJSONL returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload); err != nil {
		t.Fatalf("invalid jsonl: %v", err)
	}
	if got := payload["session_id"]; got != "sess" {
		t.Fatalf("session_id = %v, want sess", got)
	}
}

func TestDataShareExportRedactsSensitiveFields(t *testing.T) {
	sys := "你是编码助手"
	session := DataShareSession{
		TrajectoryID:       "traj-redact",
		SessionID:          "sess-redact",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Messages: []map[string]any{
			{"role": "user", "content": "hello", "metadata": map[string]any{"user_id": "client-user", "trace_id": "trace-1"}},
			{"role": "assistant", "content": "hi"},
		},
		Usage: map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		Meta: map[string]any{
			"request_id":   "req-redact",
			"user_email":   "alice@example.com",
			"ip_address":   "127.0.0.1",
			"api_key_id":   int64(10),
			"api_key_name": "main-key",
			"account_id":   int64(20),
			"user_id":      int64(30),
			"user_name":    "alice",
			"group_id":     int64(40),
			"group_name":   "共享分组",
			"nested":       map[string]any{"api_key_name": "nested-key", "kept": true},
		},
		SessionJSON: map[string]any{
			"user_id": "legacy-user",
			"metadata": map[string]any{
				"user_email": "legacy@example.com",
				"trace_id":   "legacy-trace",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	var buf bytes.Buffer
	err := NewDataSharingService(&dataShareExportRepoStub{items: []DataShareSession{session}}, nil).
		ExportJSONL(context.Background(), &buf, DataShareSessionFilters{}, false)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload))
	requireNoDataShareExportSensitiveFields(t, payload)
	require.Equal(t, "trace-1", payload["messages"].([]any)[0].(map[string]any)["metadata"].(map[string]any)["trace_id"])
	require.Equal(t, "legacy-trace", payload["metadata"].(map[string]any)["trace_id"])
	meta := payload["meta"].(map[string]any)
	require.Equal(t, "req-redact", meta["request_id"])
	require.Equal(t, "共享分组", meta["group_name"])
	require.Equal(t, true, meta["nested"].(map[string]any)["kept"])
}

func TestWriteSingleSessionJSONLRedactsSensitiveFields(t *testing.T) {
	session := &DataShareSession{
		TrajectoryID:       "traj-single-redact",
		SessionID:          "sess-single-redact",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		Messages:           []map[string]any{{"role": "user", "content": "hello"}},
		Usage:              map[string]any{"total_tokens": 1},
		Meta:               map[string]any{"api_key_id": int64(10), "request_id": "req-single"},
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	var buf bytes.Buffer
	require.NoError(t, WriteSingleSessionJSONL(&buf, session))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload))
	requireNoDataShareExportSensitiveFields(t, payload)
	require.Equal(t, "req-single", payload["meta"].(map[string]any)["request_id"])
}

func requireNoDataShareExportSensitiveFields(t *testing.T, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if _, excluded := dataShareExportExcludedFields[key]; excluded {
				t.Fatalf("export payload contains sensitive field %q in %#v", key, v)
			}
			requireNoDataShareExportSensitiveFields(t, item)
		}
	case []any:
		for _, item := range v {
			requireNoDataShareExportSensitiveFields(t, item)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
