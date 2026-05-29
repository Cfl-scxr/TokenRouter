package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"go.uber.org/zap"
)

// DataSharingCaptureBufferFlush 负责把缓冲中合并后的 session 写入持久化层。
type DataSharingCaptureBufferFlush func(ctx context.Context, session *DataShareSession) error

// DataSharingCaptureBufferOptions 描述采集缓冲池的依赖与初始配置。
type DataSharingCaptureBufferOptions struct {
	Flush DataSharingCaptureBufferFlush
}

// DataSharingCaptureBufferStats 是管理端可见的采集缓冲池运行时统计。
type DataSharingCaptureBufferStats struct {
	Enabled                 bool   `json:"enabled"`
	IdleFlushSeconds        int    `json:"idle_flush_seconds"`
	MaxSessions             int    `json:"max_sessions"`
	MaxPendingEvents        int    `json:"max_pending_events"`
	BufferedSessions        int    `json:"buffered_sessions"`
	PendingEvents           int    `json:"pending_events"`
	FlushingSessions        int64  `json:"flushing_sessions"`
	SubmittedTotal          uint64 `json:"submitted_total"`
	FlushSuccessTotal       uint64 `json:"flush_success_total"`
	FlushFailedTotal        uint64 `json:"flush_failed_total"`
	DroppedTotal            uint64 `json:"dropped_total"`
	LastFlushDurationMillis int64  `json:"last_flush_duration_millis"`
	LastError               string `json:"last_error"`
}

// DataSharingCaptureBuffer 按 trajectory_id 聚合采集增量，降低热点 session 的重复落库成本。
type DataSharingCaptureBuffer struct {
	mu             sync.Mutex
	flush          DataSharingCaptureBufferFlush
	entries        map[string]*dataSharingCaptureBufferEntry
	stopped        bool
	enabled        bool
	idleFlush      time.Duration
	flushTimeout   time.Duration
	maxSessions    int
	maxPending     int
	pendingEvents  int
	flushWG        sync.WaitGroup
	flushing       atomic.Int64
	submittedTotal atomic.Uint64
	successTotal   atomic.Uint64
	failedTotal    atomic.Uint64
	droppedTotal   atomic.Uint64
	lastDurationMS atomic.Int64
	lastError      atomic.Value
}

type dataSharingCaptureBufferEntry struct {
	key                   string
	session               *DataShareSession
	eventCount            int
	lastUpdated           time.Time
	timer                 *time.Timer
	flushing              bool
	flushQueued           bool
	lastFlushed           *DataShareSession
	lastFlushedEventCount int
}

// NewDataSharingCaptureBuffer 创建进程内数据共享采集缓冲池。
func NewDataSharingCaptureBuffer(opts DataSharingCaptureBufferOptions) *DataSharingCaptureBuffer {
	b := &DataSharingCaptureBuffer{
		flush:        opts.Flush,
		entries:      map[string]*dataSharingCaptureBufferEntry{},
		enabled:      defaultDataSharingCaptureBufferEnabled,
		idleFlush:    time.Duration(defaultDataSharingCaptureBufferIdleSeconds) * time.Second,
		flushTimeout: time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second,
		maxSessions:  defaultDataSharingCaptureBufferMaxSessions,
		maxPending:   defaultDataSharingCaptureBufferMaxEvents,
	}
	return b
}

// UpdateRuntimeSettings 在线更新缓冲池配置，后续提交和 flush 调度立即使用新阈值。
func (b *DataSharingCaptureBuffer) UpdateRuntimeSettings(settings DataShareCaptureRuntimeSettings) {
	if b == nil {
		return
	}
	normalized := normalizeDataShareCaptureRuntimeSettings(settings)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = normalized.BufferEnabled
	b.idleFlush = time.Duration(normalized.BufferIdleFlushSeconds) * time.Second
	b.flushTimeout = time.Duration(normalized.TaskTimeoutSeconds) * time.Second
	b.maxSessions = normalized.BufferMaxSessions
	b.maxPending = normalized.BufferMaxPendingEvents
	for _, entry := range b.entries {
		if !b.enabled {
			b.startFlushLocked(entry, context.Background(), true)
			continue
		}
		b.scheduleEntryTimerLocked(entry, b.remainingIdleFlushLocked(entry))
	}
}

// Submit 合并一次采集结果；缓冲关闭时会同步调用 flush，保留原有立即落库语义。
func (b *DataSharingCaptureBuffer) Submit(ctx context.Context, session *DataShareSession) error {
	if b == nil || session == nil {
		return nil
	}
	if b.flush == nil {
		return errors.New("data sharing capture buffer flush is nil")
	}
	key := session.TrajectoryID
	if key == "" {
		key = session.SessionID
	}
	if key == "" {
		key = time.Now().Format(time.RFC3339Nano)
	}

	b.mu.Lock()
	if b.stopped || !b.enabled {
		b.mu.Unlock()
		// 绕过缓冲池时仍要完成质量评估和 payload 构建，避免写入半成品 session。
		return b.flush(ctx, finalizeBufferedDataShareSession(session))
	}
	if b.pendingEvents >= b.maxPending || (b.entries[key] == nil && len(b.entries) >= b.maxSessions) {
		if !b.flushOldestLocked() {
			b.droppedTotal.Add(1)
			b.mu.Unlock()
			return nil
		}
	}
	entry := b.entries[key]
	if entry == nil {
		entry = &dataSharingCaptureBufferEntry{key: key}
		b.entries[key] = entry
	}
	entry.session = mergeBufferedDataShareSession(entry.session, session)
	entry.eventCount++
	entry.lastUpdated = time.Now()
	b.pendingEvents++
	b.submittedTotal.Add(1)
	b.scheduleEntryTimerLocked(entry, b.idleFlush)
	b.mu.Unlock()
	return nil
}

// FlushAll 立即落库当前缓冲内容；用于正常停机和测试。
func (b *DataSharingCaptureBuffer) FlushAll(ctx context.Context) {
	if b == nil {
		return
	}
	for {
		b.mu.Lock()
		var selected *dataSharingCaptureBufferEntry
		for _, entry := range b.entries {
			if !entry.flushing {
				selected = entry
				break
			}
		}
		if selected == nil {
			b.mu.Unlock()
			b.flushWG.Wait()
			b.mu.Lock()
			empty := len(b.entries) == 0
			b.mu.Unlock()
			if empty {
				return
			}
			continue
		}
		session, key := b.detachFlushLocked(selected)
		b.mu.Unlock()
		err := b.flushEntry(ctx, key, session)
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if err != nil {
			return
		}
	}
}

// Stop 停止接收新缓冲并尽量 drain 已缓冲内容。
func (b *DataSharingCaptureBuffer) Stop(ctx context.Context) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		b.flushWG.Wait()
		return
	}
	b.stopped = true
	for _, entry := range b.entries {
		if entry.timer != nil {
			entry.timer.Stop()
			entry.timer = nil
		}
	}
	b.mu.Unlock()
	b.FlushAll(ctx)
}

// Stats 返回缓冲池当前状态和累计计数。
func (b *DataSharingCaptureBuffer) Stats() DataSharingCaptureBufferStats {
	if b == nil {
		return DataSharingCaptureBufferStats{}
	}
	b.mu.Lock()
	stats := DataSharingCaptureBufferStats{
		Enabled:                 b.enabled,
		IdleFlushSeconds:        durationSecondsCeil(b.idleFlush),
		MaxSessions:             b.maxSessions,
		MaxPendingEvents:        b.maxPending,
		BufferedSessions:        len(b.entries),
		PendingEvents:           b.pendingEvents,
		FlushingSessions:        b.flushing.Load(),
		SubmittedTotal:          b.submittedTotal.Load(),
		FlushSuccessTotal:       b.successTotal.Load(),
		FlushFailedTotal:        b.failedTotal.Load(),
		DroppedTotal:            b.droppedTotal.Load(),
		LastFlushDurationMillis: b.lastDurationMS.Load(),
	}
	b.mu.Unlock()
	lastError, _ := b.lastError.Load().(string)
	stats.LastError = lastError
	return stats
}

func (b *DataSharingCaptureBuffer) resetEntryTimerLocked(entry *dataSharingCaptureBufferEntry) {
	if b == nil || entry == nil || !b.enabled || b.stopped {
		return
	}
	if b.idleFlush <= 0 {
		b.idleFlush = time.Duration(defaultDataSharingCaptureBufferIdleSeconds) * time.Second
	}
	entry.lastUpdated = time.Now()
	b.scheduleEntryTimerLocked(entry, b.idleFlush)
}

func (b *DataSharingCaptureBuffer) scheduleEntryTimerLocked(entry *dataSharingCaptureBufferEntry, delay time.Duration) {
	if b == nil || entry == nil || !b.enabled || b.stopped || entry.flushing {
		return
	}
	if delay <= 0 {
		delay = time.Millisecond
	}
	if entry.timer == nil {
		entry.timer = time.AfterFunc(delay, func() {
			b.flushByKey(entry.key)
		})
		return
	}
	entry.timer.Reset(delay)
}

func (b *DataSharingCaptureBuffer) remainingIdleFlushLocked(entry *dataSharingCaptureBufferEntry) time.Duration {
	if b == nil || entry == nil {
		return 0
	}
	if entry.lastUpdated.IsZero() {
		return b.idleFlush
	}
	return entry.lastUpdated.Add(b.idleFlush).Sub(time.Now())
}

func (b *DataSharingCaptureBuffer) flushByKey(key string) {
	b.mu.Lock()
	entry := b.entries[key]
	if entry == nil {
		b.mu.Unlock()
		return
	}
	if b.enabled && !b.stopped {
		if remaining := b.remainingIdleFlushLocked(entry); remaining > 0 {
			b.scheduleEntryTimerLocked(entry, remaining)
			b.mu.Unlock()
			return
		}
	}
	b.startFlushLocked(entry, context.Background(), true)
	b.mu.Unlock()
}

func (b *DataSharingCaptureBuffer) flushOldestLocked() bool {
	var selected *dataSharingCaptureBufferEntry
	for _, entry := range b.entries {
		if entry.flushing {
			continue
		}
		if selected == nil || entry.eventCount > selected.eventCount {
			selected = entry
		}
	}
	if selected == nil {
		return false
	}
	b.startFlushLocked(selected, context.Background(), true)
	return true
}

func (b *DataSharingCaptureBuffer) startFlushLocked(entry *dataSharingCaptureBufferEntry, ctx context.Context, async bool) {
	if b == nil || entry == nil || entry.flushing {
		return
	}
	session, key := b.detachFlushLocked(entry)
	run := func() {
		_ = b.flushEntry(ctx, key, session)
	}
	if !async {
		run()
		return
	}
	b.flushWG.Add(1)
	go func() {
		defer b.flushWG.Done()
		run()
	}()
}

func (b *DataSharingCaptureBuffer) detachFlushLocked(entry *dataSharingCaptureBufferEntry) (*DataShareSession, string) {
	if entry.timer != nil {
		entry.timer.Stop()
		entry.timer = nil
	}
	session := entry.session
	eventCount := entry.eventCount
	entry.session = nil
	entry.eventCount = 0
	entry.lastFlushed = cloneBufferedDataShareSession(session)
	entry.lastFlushedEventCount = eventCount
	entry.flushing = true
	b.pendingEvents -= eventCount
	if b.pendingEvents < 0 {
		b.pendingEvents = 0
	}
	return session, entry.key
}

func (b *DataSharingCaptureBuffer) flushEntry(ctx context.Context, key string, session *DataShareSession) error {
	if session == nil {
		b.finishFlush(key, nil)
		return nil
	}
	session = finalizeBufferedDataShareSession(session)
	start := time.Now()
	b.flushing.Add(1)
	flushCtx, cancel := b.flushContext(ctx)
	err := b.flush(flushCtx, session)
	cancel()
	b.flushing.Add(-1)
	b.lastDurationMS.Store(time.Since(start).Milliseconds())
	if err != nil {
		b.failedTotal.Add(1)
		b.lastError.Store(truncateDataSharingCaptureError(err.Error()))
		logger.L().With(
			zap.String("component", "service.data_sharing_capture_buffer"),
			zap.String("trajectory_id", session.TrajectoryID),
			zap.Error(err),
		).Warn("data_sharing.capture_buffer_flush_failed")
	} else {
		b.successTotal.Add(1)
	}
	b.finishFlush(key, err)
	return err
}

func (b *DataSharingCaptureBuffer) flushContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	timeout := b.flushTimeout
	b.mu.Unlock()
	if timeout <= 0 {
		timeout = time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (b *DataSharingCaptureBuffer) finishFlush(key string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := b.entries[key]
	if entry == nil {
		return
	}
	entry.flushing = false
	if err != nil && entry.lastFlushed != nil {
		lastFlushedEventCount := entry.lastFlushedEventCount
		if lastFlushedEventCount <= 0 {
			lastFlushedEventCount = 1
		}
		entry.session = mergeBufferedDataShareSession(entry.lastFlushed, entry.session)
		entry.lastFlushed = nil
		entry.lastFlushedEventCount = 0
		entry.eventCount += lastFlushedEventCount
		b.pendingEvents += lastFlushedEventCount
		entry.lastUpdated = time.Now()
		if !b.stopped {
			b.scheduleEntryTimerLocked(entry, b.idleFlush)
		}
		return
	}
	entry.lastFlushed = nil
	entry.lastFlushedEventCount = 0
	if entry.eventCount == 0 {
		delete(b.entries, key)
		return
	}
	b.scheduleEntryTimerLocked(entry, b.remainingIdleFlushLocked(entry))
}

func mergeBufferedDataShareSession(existing *DataShareSession, incoming *DataShareSession) *DataShareSession {
	if existing == nil {
		return cloneBufferedDataShareSession(incoming)
	}
	if incoming == nil {
		return existing
	}
	now := time.Now()
	existing.Model = firstNonBlank(incoming.Model, existing.Model)
	existing.RequestPath = firstNonBlank(incoming.RequestPath, existing.RequestPath)
	existing.UserAgent = firstNonBlank(incoming.UserAgent, existing.UserAgent)
	incomingCount := incoming.SourceRequestCount
	if incomingCount <= 0 {
		incomingCount = 1
	}
	existing.SourceRequestCount += incomingCount
	existing.InputTokens += incoming.InputTokens
	existing.OutputTokens += incoming.OutputTokens
	existing.TotalTokens += incoming.TotalTokens
	if incoming.EndedAt != nil {
		existing.EndedAt = incoming.EndedAt
	} else if existing.EndedAt == nil {
		existing.EndedAt = &now
	}
	if existing.SystemPrompt == nil || firstNonBlank(optionalStringValue(existing.SystemPrompt)) == "" {
		existing.SystemPrompt = incoming.SystemPrompt
	}
	existing.Messages = mergeBufferedDataShareMessages(existing.Messages, incoming.Messages)
	existing.Tools = mergeBufferedDataShareTools(existing.Tools, incoming.Tools)
	existing.Usage = mergeBufferedDataShareUsage(existing.Usage, incoming.Usage)
	existing.Meta = mergeBufferedDataShareMeta(existing.Meta, incoming.Meta)
	existing.UpdatedAt = now
	return existing
}

func finalizeBufferedDataShareSession(session *DataShareSession) *DataShareSession {
	if session == nil {
		return nil
	}
	session.Messages = CompactDataShareMessages(normalizeDataShareMessages(session.Messages))
	session.Tools = normalizeDataShareTools(session.Tools)
	session.Usage = normalizeDataShareUsage(session.Usage)
	session.Meta = normalizeDataShareMeta(session.Meta)
	qualityStatus, qualityErrors := DataShareSessionQuality(session.Model, optionalStringValue(session.SystemPrompt), session.Messages, session.Tools, session.Usage)
	status, finalSnapshot := dataShareCompletionState(qualityStatus)
	session.Status = status
	session.IsFinalSnapshot = finalSnapshot
	session.QualityStatus = qualityStatus
	session.QualityErrors = qualityErrors
	session.Exportable = DataShareQualityExportable(qualityStatus)
	session.SessionJSON = BuildDataShareSessionPayload(session)
	session.StorageBytes = int64(len(mustJSON(session.SessionJSON)))
	return session
}

func cloneBufferedDataShareSession(session *DataShareSession) *DataShareSession {
	if session == nil {
		return nil
	}
	clone := *session
	if session.SystemPrompt != nil {
		prompt := *session.SystemPrompt
		clone.SystemPrompt = &prompt
	}
	if session.EndedAt != nil {
		endedAt := *session.EndedAt
		clone.EndedAt = &endedAt
	}
	clone.Messages = cloneBufferedDataShareMaps(session.Messages)
	clone.Tools = cloneBufferedDataShareMaps(session.Tools)
	clone.Usage = cloneDataShareMap(session.Usage)
	clone.Meta = cloneDataShareMap(session.Meta)
	clone.SessionJSON = cloneDataShareMap(session.SessionJSON)
	return &clone
}

func cloneBufferedDataShareMaps(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, cloneDataShareMap(item))
	}
	return out
}

func mergeBufferedDataShareMessages(existing, incoming []map[string]any) []map[string]any {
	if len(existing) == 0 {
		return cloneBufferedDataShareMaps(incoming)
	}
	if len(incoming) == 0 {
		return cloneBufferedDataShareMaps(existing)
	}
	// Agent 客户端通常每轮都会带上完整历史；保留最新快照可避免热点 session 在内存中重复累积大段历史。
	if bufferedMessagesLookLikeSnapshot(existing, incoming) {
		return cloneBufferedDataShareMaps(incoming)
	}
	return append(cloneBufferedDataShareMaps(existing), cloneBufferedDataShareMaps(incoming)...)
}

func bufferedMessagesLookLikeSnapshot(existing, incoming []map[string]any) bool {
	if len(incoming) < len(existing) || len(existing) == 0 {
		return false
	}
	limit := len(existing)
	if limit > 2 {
		limit = 2
	}
	for i := 0; i < limit; i++ {
		if dataShareMessageIdentity(existing[i]) != dataShareMessageIdentity(incoming[i]) {
			return false
		}
	}
	return true
}

func mergeBufferedDataShareTools(existing, incoming []map[string]any) []map[string]any {
	out := cloneBufferedDataShareMaps(existing)
	seen := make(map[string]struct{}, len(out))
	for _, tool := range out {
		seen[string(mustJSON(tool))] = struct{}{}
	}
	for _, tool := range incoming {
		key := string(mustJSON(tool))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneDataShareMap(tool))
	}
	return out
}

func mergeBufferedDataShareUsage(existing, incoming map[string]any) map[string]any {
	out := cloneDataShareMap(existing)
	for k, v := range incoming {
		out[k] = intFromAny(out[k]) + intFromAny(v)
	}
	return normalizeDataShareUsage(out)
}

func mergeBufferedDataShareMeta(existing, incoming map[string]any) map[string]any {
	out := cloneDataShareMap(existing)
	for k, v := range incoming {
		out[k] = v
	}
	sourceIDs := appendStringValues(nil, stringsFromAny(existing["source_request_ids"])...)
	sourceIDs = appendStringValues(sourceIDs, stringsFromAny(incoming["source_request_ids"])...)
	sourceIDs = appendStringValues(sourceIDs, stringFromAny(existing["request_id"]), stringFromAny(incoming["request_id"]))
	out["source_request_ids"] = sourceIDs
	delete(out, "request_ids")
	return normalizeDataShareMeta(out)
}
