package worker_pool

import (
	"container/heap"
	"context"
	"sync"
	"time"
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

// PoolOption 用于配置 Pool 的可选参数
// TaskOption 用于配置 Task 的可选参数
type PoolOption func(*Pool)
type TaskOption func(*Task)

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
// 说明：池级日志，仅在创建 Pool 时设置一次，记录池的全局事件（如创建、扩容、关闭等）。
// 用法示例：
//
//	p := NewPool(2, WithMaxWorkers(4), WithName("my-pool"), WithLogger(func(format string, args ...interface{}) {
//	    fmt.Printf("[POOL-LOG] "+format+"\n", args...)
//	}))
//
// 输出示例：
//
//	[POOL-LOG] Pool my-pool created with min=2, max=4
func WithLogger(logger func(format string, args ...interface{})) PoolOption {
	return func(p *Pool) {
		p.logger = logger
	}
}

// WithTimeout 设置任务超时时间
func WithTimeout(timeout time.Duration) TaskOption {
	return func(t *Task) {
		t.Timeout = timeout
	}
}

// WithPriority 设置任务优先级
func WithPriority(priority int) TaskOption {
	return func(t *Task) {
		t.Priority = priority
	}
}

// WithRecovery 设置任务的 panic 恢复处理
func WithRecovery(recovery func(interface{})) TaskOption {
	return func(t *Task) {
		t.Recovery = recovery
	}
}

// WithLog 设置任务的日志函数
// 说明：任务级日志，每次提交任务时单独设置，记录该任务的执行细节（如开始、结束、异常等）。
// 用法示例：
//
//	_ = p.Submit(ctx, func(ctx context.Context) (interface{}, error) { ... },
//	    WithLog(func(format string, args ...interface{}) {
//	        fmt.Printf("[TASK-LOG] "+format+"\n", args...)
//	    }),
//	    WithTag("sync-job"))
//
// 输出示例：
//
//	[TASK-LOG] [Task] start tag=sync-job
//	[TASK-LOG] [Task] end tag=sync-job
//	[TASK-LOG] [Task] panic recovered: panic info, tag=sync-job
func WithLog(logFn func(format string, args ...interface{})) TaskOption {
	return func(t *Task) {
		t.LogFn = logFn
	}
}

// WithTag 设置任务标签
// 说明：为单个任务打上自定义标签，便于日志、监控、调试时区分不同任务。常与 WithLog 配合使用。
// 用法示例：
//
//	_ = p.Submit(ctx, func(ctx context.Context) (interface{}, error) { ... },
//	    WithTag("order-sync"),
//	    WithLog(func(format string, args ...interface{}) {
//	        fmt.Printf("[TASK][order-sync] "+format+"\n", args...)
//	    }))
//
// 输出示例：
//
//	[TASK][order-sync] [Task] start tag=order-sync
//	[TASK][order-sync] [Task] end tag=order-sync
func WithTag(tag string) TaskOption {
	return func(t *Task) {
		t.Tag = tag
	}
}

// WithBefore 设置任务前置钩子
func WithBefore(before func()) TaskOption {
	return func(t *Task) {
		t.Before = before
	}
}

// WithAfter 设置任务后置钩子
func WithAfter(after func()) TaskOption {
	return func(t *Task) {
		t.After = after
	}
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

// Submit 提交一个任务到池中，支持可选参数
func (p *Pool) Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return ErrPoolClosed
	}
	task := &Task{
		Priority: PriorityNormal, // 默认
		Timeout:  0,              // 默认
		TaskFunc: taskFunc,
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
func (p *Pool) SubmitWithResult(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) (<-chan TaskResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return nil, ErrPoolClosed
	}
	resultChan := make(chan TaskResult, 1)
	task := &Task{
		Priority:   PriorityNormal,
		Timeout:    0,
		TaskFunc:   taskFunc,
		ResultChan: resultChan,
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

			// 处理任务，ctx 由 Submit/SubmitWithResult 传递
			p.handleTask(task, nil)

			p.mu.Lock()
			p.stats.ActiveWorkers--
			p.stats.Completed++
			p.stats.QueuedTasks = len(p.taskQueue)
			p.mu.Unlock()
		}
	}()
}

// handleTask 处理单个任务，支持超时、recovery
// 新增 ctx 参数，外部传递
func (p *Pool) handleTask(task *Task, parentCtx context.Context) {
	ctx := parentCtx
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
	}
}

// Shutdown 关闭池，等待所有任务完成
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

// 预留接口实现与扩展方法（如 SetMinWorkers、SetMaxWorkers、Restart、BatchSubmit 等）
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

// Pool 实现 Pooler 接口（已是 Option 风格，无需变更）。
