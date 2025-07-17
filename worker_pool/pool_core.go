package worker_pool

import (
	"container/heap"
	"context"
	"sync"
)

// Pool 表示协程池，支持自动扩容、优先级、超时、recovery、统计信息
type Pool struct {
	minWorkers int
	maxWorkers int
	mu         sync.Mutex
	workers    int
	taskQueue  taskPriorityQueue
	taskCond   *sync.Cond
	shutdown   bool
	stats      PoolStats
	wg         sync.WaitGroup
	completed  int
	name       string
	logger     func(format string, args ...interface{})
}

// NewPool 创建一个协程池，minWorkers 必须，其他参数可选
func NewPool(minWorkers int, opts ...PoolOption) *Pool {
	if minWorkers < 1 {
		minWorkers = 1
	}
	p := &Pool{
		minWorkers: minWorkers,
		maxWorkers: minWorkers, // 默认最大等于最小
		workers:    minWorkers,
		logger:     func(format string, args ...interface{}) {}, // 默认空实现
	}
	for _, opt := range opts {
		opt(p)
	}
	p.taskCond = sync.NewCond(&p.mu)
	heap.Init(&p.taskQueue)
	for i := 0; i < minWorkers; i++ {
		p.startWorker()
	}
	if p.logger != nil && p.name != "" {
		p.logger("Pool %s created with min=%d, max=%d", p.name, p.minWorkers, p.maxWorkers)
	}
	return p
}

// PoolOption 用于配置 Pool 的可选参数
type PoolOption func(*Pool)

// WithMaxWorkers 设置最大 worker 数
func WithMaxWorkers(max int) PoolOption {
	return func(p *Pool) {
		if max < p.minWorkers {
			max = p.minWorkers
		}
		p.maxWorkers = max
	}
}

// WithName 设置池的名字
func WithName(name string) PoolOption {
	return func(p *Pool) {
		p.name = name
	}
}

// WithLogger 设置池的日志函数
func WithLogger(logger func(format string, args ...interface{})) PoolOption {
	return func(p *Pool) {
		p.logger = logger
	}
}

// Submit 提交一个任务到池中，支持可选参数
// - 并发安全：全程持有锁，保证任务队列和池状态一致性
// - ctx: 推荐作为第一个参数，便于任务取消/超时/统一管理
// - Option: 推荐用 Option 传递可选参数，灵活扩展
// - 默认超时时间：3秒，业务可用 WithTimeout 覆盖
func (p *Pool) Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return ErrPoolClosed
	}
	task := &Task{
		Priority: PriorityNormal,     // 默认
		Timeout:  DefaultTaskTimeout, // 默认超时3秒，业务可覆盖
		TaskFunc: taskFunc,
		Ctx:      ctx, // 新增
	}
	for _, opt := range opts {
		opt(task)
	}
	heap.Push(&p.taskQueue, task)
	p.taskCond.Signal()
	p.autoScale()
	return nil
}

// SubmitWithResult 提交带返回值的任务，支持可选参数
// - 并发安全：全程持有锁，保证任务队列和池状态一致性
// - ctx: 推荐作为第一个参数，便于任务取消/超时/统一管理
// - Option: 推荐用 Option 传递可选参数，灵活扩展
// - 默认超时时间：3秒，业务可用 WithTimeout 覆盖
func (p *Pool) SubmitWithResult(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) (<-chan TaskResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return nil, ErrPoolClosed
	}
	resultChan := make(chan TaskResult, 1)
	task := &Task{
		Priority:   PriorityNormal,
		Timeout:    DefaultTaskTimeout, // 默认超时3秒，业务可覆盖
		TaskFunc:   taskFunc,
		ResultChan: resultChan,
		Ctx:        ctx, // 新增
	}
	for _, opt := range opts {
		opt(task)
	}
	heap.Push(&p.taskQueue, task)
	p.taskCond.Signal()
	p.autoScale()
	return resultChan, nil
}

// startWorker 启动一个 worker goroutine
func (p *Pool) startWorker() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			p.mu.Lock()
			for len(p.taskQueue) == 0 && !p.shutdown {
				p.taskCond.Wait()
			}
			if p.shutdown && len(p.taskQueue) == 0 {
				p.mu.Unlock()
				return
			}
			task := heap.Pop(&p.taskQueue).(*Task)
			p.stats.ActiveWorkers++
			p.stats.QueuedTasks = len(p.taskQueue)
			p.mu.Unlock()

			// 处理任务，ctx 由 Task.Ctx 传递
			p.handleTask(task.Ctx, task)

			p.mu.Lock()
			p.stats.ActiveWorkers--
			p.stats.Completed++
			p.stats.QueuedTasks = len(p.taskQueue)
			p.mu.Unlock()
		}
	}()
}

// handleTask 处理单个任务，ctx 作为第一个参数
func (p *Pool) handleTask(ctx context.Context, task *Task) {
	if ctx == nil {
		ctx = context.Background()
	}
	if task.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()
	}
	if task.Before != nil {
		task.Before()
	}
	if task.LogFn != nil {
		task.LogFn("[Task] start tag=%s", task.Tag)
	}
	defer func() {
		if r := recover(); r != nil && task.Recovery != nil {
			task.Recovery(r)
		}
		if task.After != nil {
			task.After()
		}
		if task.LogFn != nil {
			task.LogFn("[Task] end tag=%s", task.Tag)
		}
	}()
	result, err := task.TaskFunc(ctx)
	if task.ResultChan != nil {
		task.ResultChan <- TaskResult{Result: result, Err: err}
		close(task.ResultChan)
	}
}

// autoScale 根据任务队列长度自动扩容 worker
func (p *Pool) autoScale() {
	if p.workers >= p.maxWorkers {
		return
	}
	if len(p.taskQueue) > p.workers {
		p.workers++
		p.startWorker()
	}
}

// Stats 返回当前池的统计信息
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStats{
		ActiveWorkers: p.stats.ActiveWorkers,
		QueuedTasks:   len(p.taskQueue),
		Completed:     p.stats.Completed,
		MinWorkers:    p.minWorkers,
		MaxWorkers:    p.maxWorkers,
	}
}

// Shutdown 关闭池，等待所有任务完成
func (p *Pool) Shutdown() {
	p.mu.Lock()
	p.shutdown = true
	p.taskCond.Broadcast()
	p.mu.Unlock()
	p.wg.Wait()
}

// SetMinWorkers 动态设置最小 worker 数
func (p *Pool) SetMinWorkers(min int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if min < 1 {
		min = 1
	}
	if min > p.maxWorkers {
		// 自动扩容 maxWorkers 以适配更大的 minWorkers
		p.maxWorkers = min
	}
	p.minWorkers = min
	// 增加 worker
	for p.workers < p.minWorkers {
		p.workers++
		p.startWorker()
	}
}

// SetMaxWorkers 动态设置最大 worker 数
func (p *Pool) SetMaxWorkers(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if max < p.minWorkers {
		// 自动收缩 minWorkers 以适配更小的 maxWorkers
		p.minWorkers = max
	}
	p.maxWorkers = max
	// 如果当前 worker 超过 maxWorkers，worker 会在空闲时自然退出
}

// Restart 重新启动池（关闭后重启）
func (p *Pool) Restart() {
	p.mu.Lock()
	if !p.shutdown {
		p.mu.Unlock()
		return
	}
	p.shutdown = false
	p.stats = PoolStats{}
	p.workers = p.minWorkers
	heap.Init(&p.taskQueue)
	p.taskCond = sync.NewCond(&p.mu)
	for i := 0; i < p.minWorkers; i++ {
		p.startWorker()
	}
	p.mu.Unlock()
}
