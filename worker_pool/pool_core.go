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
	// 初始化条件变量（Condition Variable），用于高效的任务等待与唤醒：
	// - p.taskCond = sync.NewCond(&p.mu) 创建一个条件变量，绑定池的主锁。
	// - 条件变量允许 worker 在任务队列为空时休眠，避免忙等浪费 CPU。
	// - 有新任务到来时，可用 Signal/Broadcast 唤醒等待的 worker。
	// - 并发原理：
	//   * Wait()：worker 在无任务时调用，自动释放锁并进入等待队列，直到被唤醒。
	//   * Signal()：唤醒一个等待的 worker，适合有新任务到来时。
	//   * Broadcast()：唤醒所有等待的 worker，适合池关闭等全局事件。
	//   * 所有操作都需在持有互斥锁下调用，保证并发安全。
	// - 典型场景：池/队列/生产者-消费者模型。
	// - 代码演示：
	//   p.mu.Lock()
	//   for len(p.taskQueue) == 0 && !p.shutdown {
	//       p.taskCond.Wait() // worker 休眠等待任务
	//   }
	//   ... // 处理任务
	//   p.mu.Unlock()
	//   // 新任务到来时：
	//   p.mu.Lock()
	//   heap.Push(&p.taskQueue, task)
	//   p.taskCond.Signal() // 唤醒一个 worker
	//   p.mu.Unlock()
	// - 最佳实践：涉及并发/超时/取消的函数，ctx 应作为第一个参数传递，便于统一管理生命周期。
	p.taskCond = sync.NewCond(&p.mu)
	// 初始化任务队列为堆结构，支持高效的优先级调度：
	// - heap.Init(&p.taskQueue) 将 taskQueue 初始化为优先队列（堆），可用 heap.Push/Pop 保证每次取出/插入都是最高优先级。
	// - 堆结构插入/取出都是 O(log n)，适合频繁调度和优先级任务。
	// - 普通队列/数组/链表/map 无法高效支持优先级调度。
	// - 典型场景：任务池、调度器、定时器等。
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
			// 细粒度注释：
			// worker主循环，若任务队列为空且未关闭，则进入条件变量等待：
			// - Wait()会自动释放锁并将当前goroutine挂起，直到被Signal/Broadcast唤醒。
			// - 这样可避免忙等，提升并发效率。
			// - 唤醒后会重新获得锁，继续检查条件。
			for len(p.taskQueue) == 0 && !p.shutdown {
				p.taskCond.Wait() // 细粒度注释：高效等待，有新任务/关闭时被唤醒
			}
			// 细粒度注释：
			// 如果池已关闭且队列为空，worker安全退出。
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
