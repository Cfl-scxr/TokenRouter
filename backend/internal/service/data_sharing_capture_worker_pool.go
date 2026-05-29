package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"github.com/alitto/pond/v2"
	"go.uber.org/zap"
)

const (
	defaultDataSharingCaptureWorkerCount        = 16
	defaultDataSharingCaptureQueueSize          = 256
	defaultDataSharingCaptureTaskTimeoutSeconds = 15
	dataSharingCaptureDropLogInterval           = 5 * time.Second
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
	QueueDepth     uint64 `json:"queue_depth"`
	QueueCapacity  int    `json:"queue_capacity"`
	RunningWorkers int64  `json:"running_workers"`
	SubmittedTotal uint64 `json:"submitted_total"`
	CompletedTotal uint64 `json:"completed_total"`
	FailedTotal    uint64 `json:"failed_total"`
	TimeoutTotal   uint64 `json:"timeout_total"`
	DroppedTotal   uint64 `json:"dropped_total"`
	LastError      string `json:"last_error"`
}

// DataSharingCaptureWorkerPool 提供固定 worker + 有界队列的数据共享采集执行器。
type DataSharingCaptureWorkerPool struct {
	pool             pond.Pool
	taskTimeout      time.Duration
	handler          DataSharingCaptureHandler
	queueCapacity    int
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
		taskTimeout:   opts.TaskTimeout,
		handler:       opts.Handler,
		queueCapacity: opts.QueueSize,
	}
	p.pool = pond.NewPool(
		opts.WorkerCount,
		pond.WithQueueSize(opts.QueueSize),
	)
	return p
}

// SetHandler 绑定实际采集执行体，允许 wire 先构造池再注入 service。
func (p *DataSharingCaptureWorkerPool) SetHandler(handler DataSharingCaptureHandler) {
	if p == nil {
		return
	}
	p.handler = handler
}

// Submit 非阻塞提交采集任务；队列满或未就绪时直接丢弃。
func (p *DataSharingCaptureWorkerPool) Submit(job DataSharingCaptureJob) DataSharingCaptureSubmitMode {
	if p == nil {
		return DataSharingCaptureSubmitModeDropped
	}
	if p.pool == nil || p.pool.Stopped() || p.handler == nil {
		p.droppedTotal.Add(1)
		reason := "stopped"
		if p.pool == nil {
			reason = "nil_pool"
		} else if p.handler == nil {
			reason = "missing_handler"
		}
		p.logDrop(reason, job.Metadata)
		return DataSharingCaptureSubmitModeDropped
	}
	_, ok := p.pool.TrySubmit(func() {
		p.execute(job)
	})
	if ok {
		return DataSharingCaptureSubmitModeEnqueued
	}

	p.droppedTotal.Add(1)
	reason := "full"
	if p.pool.Stopped() {
		reason = "stopped"
	}
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
	if p == nil || p.pool == nil {
		return DataSharingCaptureWorkerPoolStats{}
	}
	lastError, _ := p.lastError.Load().(string)
	return DataSharingCaptureWorkerPoolStats{
		QueueDepth:     p.pool.WaitingTasks(),
		QueueCapacity:  p.queueCapacity,
		RunningWorkers: p.pool.RunningWorkers(),
		SubmittedTotal: p.pool.SubmittedTasks(),
		CompletedTotal: p.pool.CompletedTasks(),
		FailedTotal:    p.failedTotal.Load(),
		TimeoutTotal:   p.timeoutTotal.Load(),
		DroppedTotal:   p.droppedTotal.Load(),
		LastError:      lastError,
	}
}

// Stop 停止采集池并等待已入队任务完成。
func (p *DataSharingCaptureWorkerPool) Stop() {
	if p == nil || p.pool == nil {
		return
	}
	p.pool.StopAndWait()
}

func (p *DataSharingCaptureWorkerPool) execute(job DataSharingCaptureJob) {
	handler := p.handler
	if handler == nil {
		p.droppedTotal.Add(1)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.taskTimeout)
	defer cancel()

	defer func() {
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
	return normalizeDataSharingCapturePoolOptions(opts)
}

func normalizeDataSharingCapturePoolOptions(opts DataSharingCaptureWorkerPoolOptions) DataSharingCaptureWorkerPoolOptions {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = defaultDataSharingCaptureWorkerCount
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultDataSharingCaptureQueueSize
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = time.Duration(defaultDataSharingCaptureTaskTimeoutSeconds) * time.Second
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
