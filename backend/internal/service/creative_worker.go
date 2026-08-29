package service

import (
	"context"
	"errors"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"go.uber.org/zap"
)

// CreativeOutput 是执行器返回的一张输出图片。
type CreativeOutput struct {
	Index int
	Bytes []byte
	Mime  string
}

// CreativeExecuteResult 是任务执行结果，由 CreativeRunExecutor 返回。
type CreativeExecuteResult struct {
	Outputs      []CreativeOutput
	AccountID    int64
	ProviderCost float64
}

// CreativeRunExecutor 抽象创作台任务的上游执行能力。
// 由按平台分派的 HTTP 执行器实现（openai/grok/gemini），不经过本地 HTTP 回环。
type CreativeRunExecutor interface {
	Execute(ctx context.Context, run CreativeRun, payload CreativeRunPayload) (*CreativeExecuteResult, error)
	// IsRetryable 判断瞬时错误是否值得有限重试。
	IsRetryable(err error) bool
}

const (
	defaultCreativeWorkerLockTTL             = 5 * time.Minute
	defaultCreativeWorkerLockConflictDelay   = 5 * time.Second
	defaultCreativeWorkerErrorRetryDelay     = time.Minute
	defaultCreativeWorkerRequeueDelay        = 30 * time.Second
	defaultCreativeWorkerDelayedPollInterval = 5 * time.Second
	defaultCreativeWorkerRecoveryInterval    = 5 * time.Minute
	defaultCreativeWorkerStaleActiveAfter    = 10 * time.Minute
	defaultCreativeWorkerDelayedMoveLimit    = 100
	defaultCreativeWorkerRecoverLimit        = 100
	defaultCreativeWorkerErrorBackoff        = time.Second
	defaultCreativeWorkerReserveBlockTimeout = 5 * time.Second
)

// CreativeWorkerOptions 是创作台 worker 的运行参数（全部可由配置覆盖）。
type CreativeWorkerOptions struct {
	ReserveBlockTimeout time.Duration
	JobLockTTL          time.Duration
	LockConflictDelay   time.Duration
	DefaultRequeueDelay time.Duration
	ErrorRetryDelay     time.Duration
	ErrorBackoff        time.Duration
	DelayedPollInterval time.Duration
	RecoveryInterval    time.Duration
	StaleActiveAfter    time.Duration
	DelayedMoveLimit    int
	RecoverLimit        int
	MaxAttempts         int
}

// NewCreativeWorkerOptionsFromConfig 从配置构造 worker 运行参数。
func NewCreativeWorkerOptionsFromConfig(cfg *config.Config) CreativeWorkerOptions {
	if cfg == nil {
		return normalizeCreativeWorkerOptions(CreativeWorkerOptions{})
	}
	return normalizeCreativeWorkerOptions(CreativeWorkerOptions{
		JobLockTTL:          time.Duration(cfg.Creative.JobLockTTLSeconds) * time.Second,
		LockConflictDelay:   time.Duration(cfg.Creative.LockConflictDelaySeconds) * time.Second,
		DefaultRequeueDelay: time.Duration(cfg.Creative.DefaultRequeueDelaySeconds) * time.Second,
		ErrorRetryDelay:     time.Duration(cfg.Creative.ErrorRetryDelaySeconds) * time.Second,
		DelayedPollInterval: time.Duration(cfg.Creative.DelayedMoverIntervalSeconds) * time.Second,
		RecoveryInterval:    time.Duration(cfg.Creative.RecoveryIntervalSeconds) * time.Second,
		StaleActiveAfter:    time.Duration(cfg.Creative.StaleActiveAfterSeconds) * time.Second,
		DelayedMoveLimit:    cfg.Creative.DelayedMoveLimit,
		RecoverLimit:        cfg.Creative.RecoverLimit,
		MaxAttempts:         cfg.Creative.MaxExecuteAttempts,
	})
}

func normalizeCreativeWorkerOptions(opts CreativeWorkerOptions) CreativeWorkerOptions {
	if opts.ReserveBlockTimeout <= 0 {
		opts.ReserveBlockTimeout = defaultCreativeWorkerReserveBlockTimeout
	}
	if opts.JobLockTTL <= 0 {
		opts.JobLockTTL = defaultCreativeWorkerLockTTL
	}
	if opts.LockConflictDelay <= 0 {
		opts.LockConflictDelay = defaultCreativeWorkerLockConflictDelay
	}
	if opts.DefaultRequeueDelay <= 0 {
		opts.DefaultRequeueDelay = defaultCreativeWorkerRequeueDelay
	}
	if opts.ErrorRetryDelay <= 0 {
		opts.ErrorRetryDelay = defaultCreativeWorkerErrorRetryDelay
	}
	if opts.ErrorBackoff <= 0 {
		opts.ErrorBackoff = defaultCreativeWorkerErrorBackoff
	}
	if opts.DelayedPollInterval <= 0 {
		opts.DelayedPollInterval = defaultCreativeWorkerDelayedPollInterval
	}
	if opts.RecoveryInterval <= 0 {
		opts.RecoveryInterval = defaultCreativeWorkerRecoveryInterval
	}
	if opts.StaleActiveAfter <= 0 {
		opts.StaleActiveAfter = defaultCreativeWorkerStaleActiveAfter
	}
	if opts.DelayedMoveLimit <= 0 {
		opts.DelayedMoveLimit = defaultCreativeWorkerDelayedMoveLimit
	}
	if opts.RecoverLimit <= 0 {
		opts.RecoverLimit = defaultCreativeWorkerRecoverLimit
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultCreativeMaxAttempts
	}
	return opts
}

// CreativeProcessResult 是单次任务处理结果：Terminal 表示任务可 Ack，否则按 RequeueAfter 重排。
type CreativeProcessResult struct {
	RequeueAfter time.Duration
	Terminal     bool
}

// CreativeRunWorker 是创作台队列 worker：Reserve → 锁 → 执行 → 结算 → Ack/Requeue。
type CreativeRunWorker struct {
	queue    CreativeRunQueue
	repo     CreativeRunRepository
	store    CreativeTransientStore
	executor CreativeRunExecutor
	service  *CreativePublicService
	opts     CreativeWorkerOptions
}

// NewCreativeRunWorker 创建创作台 worker。
func NewCreativeRunWorker(queue CreativeRunQueue, repo CreativeRunRepository, store CreativeTransientStore, executor CreativeRunExecutor, service *CreativePublicService, opts CreativeWorkerOptions) *CreativeRunWorker {
	return &CreativeRunWorker{
		queue:    queue,
		repo:     repo,
		store:    store,
		executor: executor,
		service:  service,
		opts:     normalizeCreativeWorkerOptions(opts),
	}
}

// Run 是 worker 主循环；ctx 取消后退出。
func (w *CreativeRunWorker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			sleepOrDone(ctx, w.opts.ErrorBackoff)
		}
	}
}

// RunOnce 处理一个队列任务。
func (w *CreativeRunWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.queue == nil || w.repo == nil || w.service == nil || w.executor == nil {
		return nil
	}

	reserved, err := w.queue.Reserve(ctx, w.opts.ReserveBlockTimeout)
	if errors.Is(err, ErrCreativeQueueEmpty) {
		return nil
	}
	if err != nil {
		return err
	}

	lock, ok, err := w.queue.TryAcquireJobLock(ctx, reserved.RunID, w.opts.JobLockTTL)
	if err != nil {
		if requeueErr := w.queue.RequeueAfter(ctx, reserved.RunID, w.opts.LockConflictDelay); requeueErr != nil {
			return requeueErr
		}
		return err
	}
	if !ok {
		// 锁被其他实例持有：按冲突延迟重新入队，避免任务滞留 active 停摆。
		return w.queue.RequeueAfter(ctx, reserved.RunID, w.opts.LockConflictDelay)
	}
	defer func() {
		_ = lock.Release(ctx)
	}()

	// 处理期间持续心跳：刷新 active 时间戳防止 stale 恢复误重投，并续期锁。
	hbStop := make(chan struct{})
	hbDone := make(chan struct{})
	go w.runJobHeartbeat(ctx, reserved.RunID, lock, hbStop, hbDone)

	result, processErr := w.process(ctx, reserved.RunID)
	close(hbStop)
	<-hbDone
	if processErr != nil {
		logger.L().Warn("creative.worker_process_failed",
			zap.String("run_id", reserved.RunID),
			zap.Error(processErr),
		)
		return w.queue.RequeueAfter(ctx, reserved.RunID, w.opts.ErrorRetryDelay)
	}
	if result.Terminal {
		return w.queue.Ack(ctx, reserved.RunID)
	}
	delay := result.RequeueAfter
	if delay <= 0 {
		delay = w.opts.DefaultRequeueDelay
	}
	return w.queue.RequeueAfter(ctx, reserved.RunID, delay)
}

// process 处理单个任务：加载载荷 → 执行 → 结算。所有结算动作幂等。
func (w *CreativeRunWorker) process(ctx context.Context, runID string) (CreativeProcessResult, error) {
	run, err := w.repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		if errors.Is(err, ErrCreativeRunNotFound) {
			// 任务已被删除：直接出队。
			return CreativeProcessResult{Terminal: true}, nil
		}
		return CreativeProcessResult{}, err
	}
	if IsTerminalCreativeRunStatus(run.Status) {
		return CreativeProcessResult{Terminal: true}, nil
	}

	payload, err := w.loadPayload(ctx, runID)
	if err != nil {
		// 载荷或输入已过期（TTL）：provider 未执行，按 result_lost 处理并释放预占。
		if markErr := w.service.MarkResultLost(ctx, runID, false); markErr != nil {
			logger.L().Warn("creative.worker_mark_result_lost_failed",
				zap.String("run_id", runID),
				zap.Error(markErr),
			)
		}
		return CreativeProcessResult{Terminal: true}, nil
	}

	// 幂等推进 running；任务已处于终态时 MarkRunning 无副作用。
	if err := w.service.MarkRunning(ctx, runID, 0); err != nil {
		return CreativeProcessResult{}, err
	}
	// 执行前再次检查：任务已进入 cancelled 则不再调用上游。
	current, err := w.repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return CreativeProcessResult{}, err
	}
	if current.Status == CreativeRunStatusCancelled || IsTerminalCreativeRunStatus(current.Status) {
		if err := w.service.CancelRunByWorker(ctx, runID); err != nil {
			return CreativeProcessResult{}, err
		}
		return CreativeProcessResult{Terminal: true}, nil
	}

	result, err := w.executor.Execute(ctx, *current, *payload)
	if err != nil {
		return w.handleExecuteError(ctx, runID, err)
	}
	results := make([]CreativeOutputResult, 0, len(result.Outputs))
	for _, output := range result.Outputs {
		results = append(results, CreativeOutputResult{
			Index:   output.Index,
			Success: true,
			Bytes:   output.Bytes,
			Mime:    output.Mime,
		})
	}
	if _, err := w.service.SucceedRun(ctx, runID, result.AccountID, results); err != nil {
		// 结算失败（如计费暂时不可用）：按错误重试预算重排。
		return w.retryOrGiveUp(ctx, runID, "SETTLEMENT_FAILED", err)
	}
	return CreativeProcessResult{Terminal: true}, nil
}

// loadPayload 从临时存储加载任务载荷与输入字节；缺失即视为结果丢失。
func (w *CreativeRunWorker) loadPayload(ctx context.Context, runID string) (*CreativeRunPayload, error) {
	if w.store == nil {
		return nil, errors.New("creative transient store is not configured")
	}
	payload, err := w.store.LoadPayload(ctx, runID)
	if err != nil {
		return nil, err
	}
	if payload.SourceCount > 0 {
		inputs, err := w.store.LoadInputs(ctx, runID, payload.SourceCount)
		if err != nil {
			return nil, err
		}
		payload.Sources = make([]CreativeInputImage, 0, len(inputs))
		// 载荷中没有逐图 MIME，输入键按字节保存；执行器按魔数嗅探。
		for _, data := range inputs {
			payload.Sources = append(payload.Sources, CreativeInputImage{Bytes: data, Mime: sniffCreativeImageMime(data)})
		}
	}
	if payload.HasMask {
		mask, err := w.store.LoadMask(ctx, runID)
		if err != nil {
			return nil, err
		}
		payload.Mask = &CreativeInputImage{Bytes: mask, Mime: sniffCreativeImageMime(mask)}
	}
	return payload, nil
}

// handleExecuteError 处理执行错误：可重试且未达上限 → 递增 attempt 并重排；否则 FailRun 出队。
func (w *CreativeRunWorker) handleExecuteError(ctx context.Context, runID string, execErr error) (CreativeProcessResult, error) {
	if w.executor.IsRetryable(execErr) {
		attempts, err := w.repo.IncrementCreativeRunAttempt(ctx, runID)
		if err != nil {
			return CreativeProcessResult{}, err
		}
		if attempts < w.opts.MaxAttempts {
			return CreativeProcessResult{RequeueAfter: w.opts.ErrorRetryDelay}, nil
		}
		logger.L().Warn("creative.worker_attempts_exhausted",
			zap.String("run_id", runID),
			zap.Int("attempts", attempts),
			zap.Error(execErr),
		)
	}
	code, message := creativeExecuteErrorParts(execErr)
	if err := w.service.FailRun(ctx, runID, code, message); err != nil {
		return CreativeProcessResult{}, err
	}
	return CreativeProcessResult{Terminal: true}, nil
}

// retryOrGiveUp 结算类错误的重试路径：按错误重试预算重排，超限后放弃（保留当前状态出队）。
func (w *CreativeRunWorker) retryOrGiveUp(ctx context.Context, runID, code string, cause error) (CreativeProcessResult, error) {
	attempts, err := w.repo.IncrementCreativeRunAttempt(ctx, runID)
	if err != nil {
		return CreativeProcessResult{}, err
	}
	if attempts < w.opts.MaxAttempts*2 {
		return CreativeProcessResult{RequeueAfter: w.opts.ErrorRetryDelay}, nil
	}
	logger.L().Warn("creative.worker_settlement_retry_exhausted",
		zap.String("run_id", runID),
		zap.String("code", code),
		zap.Int("attempts", attempts),
		zap.Error(cause),
	)
	return CreativeProcessResult{Terminal: true}, nil
}

// creativeExecuteErrorParts 把执行错误映射为落库的错误码与消息。
func creativeExecuteErrorParts(err error) (string, string) {
	var upstreamErr *CreativeUpstreamError
	if errors.As(err, &upstreamErr) {
		code := "PROVIDER_FAILED"
		if upstreamErr.StatusCode > 0 {
			code = "UPSTREAM_STATUS_" + itoaPositive(upstreamErr.StatusCode)
		}
		return code, upstreamErr.Message
	}
	return "PROVIDER_FAILED", sanitizeCreativeMessage(err.Error())
}

func itoaPositive(v int) string {
	if v <= 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

// runJobHeartbeat 处理期间持续心跳：刷新 active 时间戳并续期锁。
func (w *CreativeRunWorker) runJobHeartbeat(ctx context.Context, runID string, lock CreativeRunJobLock, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.heartbeatInterval())
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.queue.Heartbeat(ctx, runID); err != nil && ctx.Err() == nil {
				logger.L().Warn("creative.worker_heartbeat_failed",
					zap.String("run_id", runID),
					zap.Error(err),
				)
			}
			if refresher, ok := lock.(CreativeRunJobLockRefresher); ok {
				if err := refresher.Refresh(ctx, w.opts.JobLockTTL); err != nil && ctx.Err() == nil {
					logger.L().Warn("creative.worker_lock_refresh_failed",
						zap.String("run_id", runID),
						zap.Error(err),
					)
				}
			}
		}
	}
}

func (w *CreativeRunWorker) heartbeatInterval() time.Duration {
	interval := w.opts.JobLockTTL
	if w.opts.StaleActiveAfter < interval {
		interval = w.opts.StaleActiveAfter
	}
	interval /= 3
	if interval < time.Second {
		interval = time.Second
	}
	return interval
}

// MoveDueDelayedOnce 把到期的 delayed 任务搬回 ready。
func (w *CreativeRunWorker) MoveDueDelayedOnce(ctx context.Context) (int, error) {
	if w == nil || w.queue == nil {
		return 0, nil
	}
	return w.queue.MoveDueDelayedToReady(ctx, w.opts.DelayedMoveLimit)
}

// RunDelayedMover 是 delayed mover 主循环。
func (w *CreativeRunWorker) RunDelayedMover(ctx context.Context) {
	if w == nil {
		return
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		moved, _ := w.MoveDueDelayedOnce(ctx)
		if moved > 0 {
			continue
		}
		sleepOrDone(ctx, w.opts.DelayedPollInterval)
	}
}

// RecoverStaleActiveOnce 把超时未心跳的 active 任务重投回 ready（worker 重启恢复）。
func (w *CreativeRunWorker) RecoverStaleActiveOnce(ctx context.Context) (int, error) {
	if w == nil || w.queue == nil {
		return 0, nil
	}
	return w.queue.RecoverStaleActive(ctx, w.opts.StaleActiveAfter, w.opts.RecoverLimit)
}

// RunStaleActiveRecovery 是 stale active 恢复主循环。
func (w *CreativeRunWorker) RunStaleActiveRecovery(ctx context.Context) {
	if w == nil {
		return
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		_, _ = w.RecoverStaleActiveOnce(ctx)
		sleepOrDone(ctx, w.opts.RecoveryInterval)
	}
}
