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
// - 并发安全：需在持有锁下调用
// - 设计动机：根据负载弹性扩容，提升资源利用率
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
// - 并发安全：持有锁，保证统计数据一致性
// - 设计动机：便于监控池运行状态
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStats{
		ActiveWorkers: p.stats.ActiveWorkers,
		QueuedTasks:   len(p.taskQueue),
		Completed:     p.stats.Completed,
	}
}

// Shutdown 关闭池，等待所有任务完成
// - 并发安全：加锁+条件变量广播，确保所有 worker 安全退出
// - 典型场景：服务关闭、资源释放，保证无任务丢失
/*
	- 该方法用于优雅关闭协程池，确保：
	- 不再接受新任务。
	- 等待所有已提交的任务执行完毕。
	- 所有 worker goroutine 都安全退出。
	- 适用于服务关闭、资源释放等场景，保证不会有任务丢失或 goroutine 泄漏。
*/
func (p *Pool) Shutdown() {
	// 加锁，保证对协程池状态的修改是线程安全的，防止并发冲突
	p.mu.Lock()
	// 设置池的 shutdown 标志为 true，表示协程池即将关闭，不再接受新的任务提交。
	p.shutdown = true
	// 唤醒所有正在等待任务的 worker goroutine。
	// 由于有些 worker 可能正处于等待任务的状态（p.taskCond.Wait()），需要通过广播通知它们池已关闭，让它们有机会检查 shutdown 状态并退出。
	p.taskCond.Broadcast()
	// 解锁，允许其他 goroutine 访问池的状态。
	p.mu.Unlock()
	// 等待所有 worker goroutine 退出。
	// p.wg 是一个 sync.WaitGroup，每启动一个 worker 时会 Add(1)，worker 退出时会 Done()。这里的 Wait() 会阻塞，直到所有 worker 都退出，确保所有任务都已处理完毕。
	p.wg.Wait()
}

/*
	p.taskCond.Broadcast() 的作用是唤醒所有正在等待条件变量的 goroutine，在这里就是唤醒所有正在等待任务的 worker。下面详细解释其效果和原理：
	1. 条件变量的背景
	p.taskCond 是一个条件变量（*sync.Cond），它和 p.mu 互斥锁配合使用。
	在 worker 的主循环中，有如下代码：
	Apply to pool_core.go
	}
	这表示：如果任务队列为空且没有关闭，worker 就会阻塞在 Wait()，等待被唤醒。
	2. Broadcast 的作用
	Broadcast() 会唤醒所有因为 Wait() 而阻塞的 goroutine（worker）。
	这样做的目的是：当有重要状态变化（如池关闭）时，所有等待的 worker 都能及时响应。
	3. 在 Shutdown 中的效果
	当 Shutdown() 被调用时，p.shutdown 被设为 true，然后 Broadcast() 唤醒所有等待的 worker。
	被唤醒的 worker 会重新获得锁，然后检查循环条件：
	Apply to pool_core.go
	}
	由于此时 p.shutdown == true，worker 会跳出循环，继续往下执行，最终退出 goroutine。
	4. 为什么不用 Signal？
	Signal() 只会唤醒一个等待的 goroutine。
	但在关闭池时，可能有多个 worker 都在等待任务，必须全部唤醒，让它们都能检查到池已关闭，及时退出。
	所以用 Broadcast()，确保所有等待的 worker 都能收到通知。
	5. 总结
	p.taskCond.Broadcast() 的效果是：让所有等待任务的 worker 都被唤醒，及时感知到池已关闭，安全退出。
	这是实现优雅关闭协程池的关键步骤。
*/

// SetMinWorkers 动态设置最小 worker 数
func (p *Pool) SetMinWorkers(min int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if min < 1 {
		min = 1
	}
	if min > p.maxWorkers {
		min = p.maxWorkers
	}
	p.minWorkers = min
	// 增加 worker
	for p.workers < p.minWorkers {
		p.workers++
		p.startWorker()
	}
	// 不主动减少 worker，worker 会在空闲时自然退出
}

// SetMaxWorkers 动态设置最大 worker 数
func (p *Pool) SetMaxWorkers(max int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if max < p.minWorkers {
		max = p.minWorkers
	}
	p.maxWorkers = max
	// 如果当前 worker 超过 maxWorkers，worker 会在空闲时自然退出
}

// Restart 重新启动池（关闭后重启）
func (p *Pool) Restart() {
	p.mu.Lock()
	if !p.shutdown {
		p.mu.Unlock()
		return // 只有关闭后才能重启
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
