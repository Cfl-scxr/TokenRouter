package service

import (
	"context"
	"sync"

	"github.com/TokenFlux/TokenRouter/internal/config"
)

// CreativeWorkerRuntime 管理创作台 worker 的生命周期：
// cfg.Creative.QueueEnabled 时启动 worker / delayed mover / stale recovery 三个 goroutine。
type CreativeWorkerRuntime struct {
	worker *CreativeRunWorker
	cfg    *config.Config

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewCreativeWorkerRuntime 创建创作台 worker runtime（不自动启动）。
func NewCreativeWorkerRuntime(worker *CreativeRunWorker, cfg *config.Config) *CreativeWorkerRuntime {
	return &CreativeWorkerRuntime{worker: worker, cfg: cfg}
}

// ProvideCreativeWorkerRuntime 组装创作台 worker 并启动 runtime。
func ProvideCreativeWorkerRuntime(
	repo CreativeRunRepository,
	store CreativeTransientStore,
	queue CreativeRunQueue,
	executor CreativeRunExecutor,
	creativeService *CreativePublicService,
	cfg *config.Config,
) *CreativeWorkerRuntime {
	worker := NewCreativeRunWorker(queue, repo, store, executor, creativeService, NewCreativeWorkerOptionsFromConfig(cfg))
	runtime := NewCreativeWorkerRuntime(worker, cfg)
	runtime.Start()
	return runtime
}

// Start 启动三个后台 goroutine；重复调用幂等。
func (r *CreativeWorkerRuntime) Start() {
	if r == nil || r.worker == nil || r.cfg == nil || !r.cfg.Creative.QueueEnabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.cancel = cancel
	r.done = done

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		r.worker.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		r.worker.RunDelayedMover(ctx)
	}()
	go func() {
		defer wg.Done()
		r.worker.RunStaleActiveRecovery(ctx)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
}

// Stop 停止 runtime；重复调用幂等。
func (r *CreativeWorkerRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Running 报告 runtime 是否正在运行。
func (r *CreativeWorkerRuntime) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
}
