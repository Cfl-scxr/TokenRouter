package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultDataSharingCaptureWorkerCount        = 32
	maxDataSharingCaptureWorkerCount            = 1024
	defaultDataSharingCaptureQueueSize          = 32768
	maxDataSharingCaptureQueueSize              = 100000
	defaultDataSharingCaptureTaskTimeoutSeconds = 15
	maxDataSharingCaptureTaskTimeoutSeconds     = 300
	defaultDataSharingCaptureCompressionLevel   = DataShareCompressionLevelFastest
	dataSharingCaptureDropLogInterval           = 5 * time.Second
	dataSharingCaptureInitialQueueBufferSize    = 1024
)

// DataSharingCaptureProtocol 表示采集写入需要使用的上游协议解析方式。
type DataSharingCaptureProtocol string

const (
	DataSharingCaptureProtocolClaude DataSharingCaptureProtocol = "claude"
	DataSharingCaptureProtocolOpenAI DataSharingCaptureProtocol = "openai"
)

// DataSharingCaptureJobMetadata 保存日志与统计需要的轻量元信息，不依赖请求 ctx。
type DataSharingCaptureJobMetadata struct {
	Provider  string
	Model     string
	RequestID string
	APIKeyID  int64
	AccountID int64
	GroupID   int64
}

// DataSharingCaptureJob 是提交到数据共享采集 worker 的任务。
type DataSharingCaptureJob struct {
	Protocol DataSharingCaptureProtocol
	Input    DataShareCaptureInput
	Metadata DataSharingCaptureJobMetadata
}

// DataSharingCaptureHandler 执行一次具体的数据共享采集写入。
type DataSharingCaptureHandler func(ctx context.Context, job DataSharingCaptureJob) error

// DataSharingCaptureSubmitMode 表示采集任务提交结果。
type DataSharingCaptureSubmitMode string

const (
	DataSharingCaptureSubmitModeEnqueued DataSharingCaptureSubmitMode = "enqueued"
	DataSharingCaptureSubmitModeDropped  DataSharingCaptureSubmitMode = "dropped"
)

// DataSharingCaptureWorkerPoolOptions 数据共享采集池配置。
type DataSharingCaptureWorkerPoolOptions struct {
	WorkerCount int
	QueueSize   int
	TaskTimeout time.Duration
	Handler     DataSharingCaptureHandler
}

// DataSharingCaptureWorkerPoolStats 是管理端可见的采集池运行时统计。
type DataSharingCaptureWorkerPoolStats struct {
	QueueDepth         uint64 `json:"queue_depth"`
	QueueCapacity      int    `json:"queue_capacity"`
	WorkerCount        int    `json:"worker_count"`
	RunningWorkers     int64  `json:"running_workers"`
	AvailableWorkers   int64  `json:"available_workers"`
	TaskTimeoutSeconds int    `json:"task_timeout_seconds"`
	CompressionLevel   string `json:"compression_level"`
	SubmittedTotal     uint64 `json:"submitted_total"`
	CompletedTotal     uint64 `json:"completed_total"`
	FailedTotal        uint64 `json:"failed_total"`
	TimeoutTotal       uint64 `json:"timeout_total"`
	DroppedTotal       uint64 `json:"dropped_total"`
	LastError          string `json:"last_error"`
}

// DataSharingCaptureWorkerPool 提供可在线调整 worker 数与逻辑队列容量的数据共享采集执行器。
type DataSharingCaptureWorkerPool struct {
	mu               sync.Mutex
	cond             *sync.Cond
	queue            []DataSharingCaptureJob
	queueHead        int
	queueLen         int
	stopping         bool
	startedWorkers   int
	workerWG         sync.WaitGroup
	taskTimeoutNanos atomic.Int64
	handler          DataSharingCaptureHandler
	workerCount      int
	queueCapacity    int
	activeTotal      atomic.Int64
	submittedTotal   atomic.Uint64
	completedTotal   atomic.Uint64
	failedTotal      atomic.Uint64
	timeoutTotal     atomic.Uint64
	droppedTotal     atomic.Uint64
	lastDropLogNanos atomic.Int64
	lastError        atomic.Value
}

// NewDataSharingCaptureWorkerPool 从配置构建数据共享采集池。
func NewDataSharingCaptureWorkerPool(cfg *config.Config) *DataSharingCaptureWorkerPool {
	opts := dataSharingCapturePoolOptionsFromConfig(cfg)
	return NewDataSharingCaptureWorkerPoolWithOptions(opts)
}

// NewDataSharingCaptureWorkerPoolWithOptions 根据给定参数构建数据共享采集池。
func NewDataSharingCaptureWorkerPoolWithOptions(opts DataSharingCaptureWorkerPoolOptions) *DataSharingCaptureWorkerPool {
	opts = normalizeDataSharingCapturePoolOptions(opts)

	p := &DataSharingCaptureWorkerPool{
		handler:       opts.Handler,
		workerCount:   opts.WorkerCount,
		queueCapacity: opts.QueueSize,
	}
	p.cond = sync.NewCond(&p.mu)
	p.SetTaskTimeout(opts.TaskTimeout)
	p.mu.Lock()
	p.ensureWorkersLocked(opts.WorkerCount)
	p.mu.Unlock()
	return p
}

// SetHandler 绑定实际采集执行体，允许 wire 先构造池再注入 service。
func (p *DataSharingCaptureWorkerPool) SetHandler(handler DataSharingCaptureHandler) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCondLocked()
	p.handler = handler
	p.cond.Broadcast()
}

// SetTaskTimeout 在线更新后续采集任务使用的超时时间。
func (p *DataSharingCaptureWorkerPool) SetTaskTimeout(timeout time.Duration) time.Duration {
	if p == nil {
		return 0
	}
	if timeout <= 0 {
		timeout = time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
	}
	p.taskTimeoutNanos.Store(int64(timeout))
	return timeout
}

// TaskTimeout 返回后续采集任务使用的超时时间。
func (p *DataSharingCaptureWorkerPool) TaskTimeout() time.Duration {
	if p == nil {
		return time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
	}
	timeout := time.Duration(p.taskTimeoutNanos.Load())
	if timeout <= 0 {
		return time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
	}
	return timeout
}

// UpdateRuntimeSettings 在线调整 worker 数、队列容量和后续任务超时。
func (p *DataSharingCaptureWorkerPool) UpdateRuntimeSettings(workerCount, queueSize int, taskTimeout time.Duration) {
	if p == nil {
		return
	}
	opts := normalizeDataSharingCapturePoolOptions(DataSharingCaptureWorkerPoolOptions{
		WorkerCount: workerCount,
		QueueSize:   queueSize,
		TaskTimeout: taskTimeout,
	})
	p.SetTaskTimeout(opts.TaskTimeout)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCondLocked()
	if p.stopping {
		return
	}
	p.workerCount = opts.WorkerCount
	p.queueCapacity = opts.QueueSize
	p.ensureWorkersLocked(opts.WorkerCount)
	p.cond.Broadcast()
}

// Submit 非阻塞提交采集任务；队列满或未就绪时直接丢弃。
func (p *DataSharingCaptureWorkerPool) Submit(job DataSharingCaptureJob) DataSharingCaptureSubmitMode {
	if p == nil {
		return DataSharingCaptureSubmitModeDropped
	}
	p.mu.Lock()
	p.initCondLocked()
	reason := ""
	switch {
	case p.stopping:
		reason = "stopped"
	case p.handler == nil:
		reason = "missing_handler"
	case p.queueLen >= p.queueCapacity:
		reason = "full"
	default:
		p.enqueueLocked(job)
		p.submittedTotal.Add(1)
		p.cond.Broadcast()
		p.mu.Unlock()
		return DataSharingCaptureSubmitModeEnqueued
	}
	p.mu.Unlock()

	p.droppedTotal.Add(1)
	p.logDrop(reason, job.Metadata)
	return DataSharingCaptureSubmitModeDropped
}

// RecordDropped 记录未进入队列的 fail-open 丢弃场景。
func (p *DataSharingCaptureWorkerPool) RecordDropped(reason string, metadata DataSharingCaptureJobMetadata) {
	if p == nil {
		return
	}
	p.droppedTotal.Add(1)
	p.logDrop(reason, metadata)
}

// Stats 返回当前池状态与计数器快照。
func (p *DataSharingCaptureWorkerPool) Stats() DataSharingCaptureWorkerPoolStats {
	if p == nil {
		return DataSharingCaptureWorkerPoolStats{}
	}
	p.mu.Lock()
	p.initCondLocked()
	queueDepth := p.queueLen
	queueCapacity := p.queueCapacity
	workerCount := p.workerCount
	stopping := p.stopping
	p.mu.Unlock()
	lastError, _ := p.lastError.Load().(string)
	runningWorkers := p.activeTotal.Load()
	availableWorkers := int64(workerCount) - runningWorkers
	if availableWorkers < 0 || stopping {
		availableWorkers = 0
	}
	return DataSharingCaptureWorkerPoolStats{
		QueueDepth:         uint64(queueDepth),
		QueueCapacity:      queueCapacity,
		WorkerCount:        workerCount,
		RunningWorkers:     runningWorkers,
		AvailableWorkers:   availableWorkers,
		TaskTimeoutSeconds: durationSecondsCeil(p.TaskTimeout()),
		CompressionLevel:   CurrentDataShareCompressionLevel(),
		SubmittedTotal:     p.submittedTotal.Load(),
		CompletedTotal:     p.completedTotal.Load(),
		FailedTotal:        p.failedTotal.Load(),
		TimeoutTotal:       p.timeoutTotal.Load(),
		DroppedTotal:       p.droppedTotal.Load(),
		LastError:          lastError,
	}
}

// Stop 停止采集池并等待已入队任务完成。
func (p *DataSharingCaptureWorkerPool) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.initCondLocked()
	if p.stopping {
		p.mu.Unlock()
		p.workerWG.Wait()
		return
	}
	p.stopping = true
	p.cond.Broadcast()
	p.mu.Unlock()
	p.workerWG.Wait()
}

func (p *DataSharingCaptureWorkerPool) execute(job DataSharingCaptureJob) {
	p.mu.Lock()
	p.initCondLocked()
	handler := p.handler
	p.mu.Unlock()
	if handler == nil {
		p.droppedTotal.Add(1)
		return
	}
	p.activeTotal.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), p.TaskTimeout())
	defer cancel()

	defer func() {
		p.activeTotal.Add(-1)
		p.completedTotal.Add(1)
		if recovered := recover(); recovered != nil {
			p.failedTotal.Add(1)
			p.storeLastError("panic")
			logger.L().With(
				zap.String("component", "service.data_sharing_capture_worker_pool"),
				zap.Any("panic", recovered),
				zap.String("protocol", string(job.Protocol)),
				zap.String("provider", job.Metadata.Provider),
				zap.String("model", job.Metadata.Model),
				zap.String("request_id", job.Metadata.RequestID),
				zap.Int64("api_key_id", job.Metadata.APIKeyID),
				zap.Int64("account_id", job.Metadata.AccountID),
				zap.Int64("group_id", job.Metadata.GroupID),
			).Error("data_sharing.capture_panic")
		}
	}()

	if err := handler(ctx, job); err != nil {
		p.failedTotal.Add(1)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			p.timeoutTotal.Add(1)
		}
		p.storeLastError(err.Error())
		logger.L().With(
			zap.String("component", "service.data_sharing_capture_worker_pool"),
			zap.String("protocol", string(job.Protocol)),
			zap.String("provider", job.Metadata.Provider),
			zap.String("model", job.Metadata.Model),
			zap.String("request_id", job.Metadata.RequestID),
			zap.Int64("api_key_id", job.Metadata.APIKeyID),
			zap.Int64("account_id", job.Metadata.AccountID),
			zap.Int64("group_id", job.Metadata.GroupID),
			zap.Error(err),
		).Warn("data_sharing.capture_failed")
	}
}

func (p *DataSharingCaptureWorkerPool) ensureWorkersLocked(workerCount int) {
	p.initCondLocked()
	for p.startedWorkers < workerCount {
		workerID := p.startedWorkers
		p.startedWorkers++
		p.workerWG.Add(1)
		go p.worker(workerID)
	}
}

func (p *DataSharingCaptureWorkerPool) worker(workerID int) {
	defer p.workerWG.Done()
	for {
		job, ok := p.dequeue(workerID)
		if !ok {
			return
		}
		p.execute(job)
	}
}

func (p *DataSharingCaptureWorkerPool) dequeue(workerID int) (DataSharingCaptureJob, bool) {
	var zero DataSharingCaptureJob
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCondLocked()
	for {
		if p.stopping && p.queueLen == 0 {
			return zero, false
		}
		// 停机排空阶段允许已创建 worker 协助清空队列，避免缩容后停止等待过久。
		if !p.stopping && workerID >= p.workerCount {
			p.cond.Wait()
			continue
		}
		if p.queueLen > 0 {
			return p.dequeueLocked(), true
		}
		p.cond.Wait()
	}
}

func (p *DataSharingCaptureWorkerPool) initCondLocked() {
	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}
}

func (p *DataSharingCaptureWorkerPool) enqueueLocked(job DataSharingCaptureJob) {
	if len(p.queue) == 0 {
		p.queue = make([]DataSharingCaptureJob, initialDataSharingCaptureQueueBufferSize(p.queueCapacity))
	}
	if p.queueLen == len(p.queue) {
		p.growQueueLocked()
	}
	tail := (p.queueHead + p.queueLen) % len(p.queue)
	p.queue[tail] = job
	p.queueLen++
}

func (p *DataSharingCaptureWorkerPool) dequeueLocked() DataSharingCaptureJob {
	job := p.queue[p.queueHead]
	p.queue[p.queueHead] = DataSharingCaptureJob{}
	p.queueHead = (p.queueHead + 1) % len(p.queue)
	p.queueLen--
	if p.queueLen == 0 {
		p.queueHead = 0
	}
	return job
}

func (p *DataSharingCaptureWorkerPool) growQueueLocked() {
	nextSize := len(p.queue) * 2
	if nextSize <= 0 {
		nextSize = 1
	}
	if nextSize > p.queueCapacity {
		nextSize = p.queueCapacity
	}
	next := make([]DataSharingCaptureJob, nextSize)
	for i := 0; i < p.queueLen; i++ {
		next[i] = p.queue[(p.queueHead+i)%len(p.queue)]
	}
	p.queue = next
	p.queueHead = 0
}

func (p *DataSharingCaptureWorkerPool) storeLastError(msg string) {
	if p == nil {
		return
	}
	p.lastError.Store(truncateDataSharingCaptureError(msg))
}

func (p *DataSharingCaptureWorkerPool) logDrop(reason string, metadata DataSharingCaptureJobMetadata) {
	now := time.Now().UnixNano()
	last := p.lastDropLogNanos.Load()
	if now-last < int64(dataSharingCaptureDropLogInterval) {
		return
	}
	if !p.lastDropLogNanos.CompareAndSwap(last, now) {
		return
	}

	stats := p.Stats()
	logger.L().With(
		zap.String("component", "service.data_sharing_capture_worker_pool"),
		zap.String("reason", reason),
		zap.String("provider", metadata.Provider),
		zap.String("model", metadata.Model),
		zap.String("request_id", metadata.RequestID),
		zap.Int64("api_key_id", metadata.APIKeyID),
		zap.Int64("account_id", metadata.AccountID),
		zap.Int64("group_id", metadata.GroupID),
		zap.Uint64("queue_depth", stats.QueueDepth),
		zap.Int("queue_capacity", stats.QueueCapacity),
		zap.Uint64("dropped_total", stats.DroppedTotal),
	).Warn("data_sharing.capture_dropped")
}

func dataSharingCapturePoolOptionsFromConfig(cfg *config.Config) DataSharingCaptureWorkerPoolOptions {
	opts := DataSharingCaptureWorkerPoolOptions{
		WorkerCount: defaultDataSharingCaptureWorkerCount,
		QueueSize:   defaultDataSharingCaptureQueueSize,
		TaskTimeout: time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second,
	}
	if cfg == nil {
		return opts
	}
	if cfg.Gateway.DataSharingCapture.WorkerCount > 0 {
		opts.WorkerCount = cfg.Gateway.DataSharingCapture.WorkerCount
	}
	if cfg.Gateway.DataSharingCapture.QueueSize > 0 {
		opts.QueueSize = cfg.Gateway.DataSharingCapture.QueueSize
	}
	if cfg.Gateway.DataSharingCapture.TaskTimeoutSeconds > 0 {
		opts.TaskTimeout = time.Duration(cfg.Gateway.DataSharingCapture.TaskTimeoutSeconds) * time.Second
	}
	SetDataShareCompressionLevel(cfg.Gateway.DataSharingCapture.CompressionLevel)
	return normalizeDataSharingCapturePoolOptions(opts)
}

func normalizeDataSharingCapturePoolOptions(opts DataSharingCaptureWorkerPoolOptions) DataSharingCaptureWorkerPoolOptions {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = defaultDataSharingCaptureWorkerCount
	}
	if opts.WorkerCount > maxDataSharingCaptureWorkerCount {
		opts.WorkerCount = maxDataSharingCaptureWorkerCount
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultDataSharingCaptureQueueSize
	}
	if opts.QueueSize > maxDataSharingCaptureQueueSize {
		opts.QueueSize = maxDataSharingCaptureQueueSize
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
	}
	maxTimeout := time.Duration(maxDataSharingCaptureTaskTimeoutSeconds) * time.Second
	if opts.TaskTimeout > maxTimeout {
		opts.TaskTimeout = maxTimeout
	}
	return opts
}

func truncateDataSharingCaptureError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= 512 {
		return msg
	}
	return msg[:512]
}

func durationSecondsCeil(d time.Duration) int {
	if d <= 0 {
		return defaultDataSharingCaptureTaskTimeoutSeconds
	}
	return int((d + time.Second - 1) / time.Second)
}

func initialDataSharingCaptureQueueBufferSize(queueCapacity int) int {
	if queueCapacity <= 0 {
		return 1
	}
	if queueCapacity < dataSharingCaptureInitialQueueBufferSize {
		return queueCapacity
	}
	return dataSharingCaptureInitialQueueBufferSize
}
