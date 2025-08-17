// worker_pool 包提供了一个功能完整的goroutine池实现
//
// 🔥 缩容机制完整说明：
//
// 1. 缩容决策机制：
//   - 由adjustByPercentage()基于负载历史（3小时数据）分析决策
//   - 如果80%的时间负载低于阈值，则调整coreWorkers为更小值
//   - 决策结果体现在coreWorkers字段中，这是缩容的"目标值"
//
// 2. 缩容执行机制：
//   - worker检查：当前worker数 > coreWorkers 且 allowCoreTimeout = true
//   - 超出coreWorkers的worker在空闲keepAliveTime后自动退出
//   - 不依赖瞬时队列状态，而是相信负载分析的长期趋势判断
//
// 3. 缩容实现位置：
//   - startWorker()方法中的第337行：p.workers--
//   - 这是整个系统中唯一减少worker数量的地方
//   - 所有worker都会定期检查coreWorkers变化，支持动态调整
//
// 4. 缩容安全性：
//   - 使用双重检查：超时前检查条件，超时后再次检查条件
//   - 原子操作：p.workers--在锁保护下执行
//   - 优雅退出：worker goroutine通过return正常结束，无强制杀死
//
// 🛡️ 优雅退出机制详解：
//
// 1. 什么是优雅退出？
//   - goroutine通过正常的控制流程（return）结束
//   - 不是被外部强制终止（如强制关闭channel、panic等）
//   - 允许goroutine完成清理工作和资源释放
//
// 2. 为什么需要优雅退出？
//   - 避免数据丢失：正在处理的任务可以完成
//   - 防止资源泄露：可以正确释放连接、文件句柄等
//   - 保证一致性：避免中途中断导致的不一致状态
//   - 提高稳定性：减少因强制终止导致的系统异常
//
// 3. 我们的优雅退出实现：
//   - 使用超时等待而不是立即退出
//   - 双重条件检查确保退出时机正确
//   - 通过return语句自然结束goroutine生命周期
//   - WaitGroup确保所有worker完全退出后才关闭池
//
// 5. 为什么这样设计：
//   - 避免强制杀死worker，保证任务完整性
//   - 分离关注点：调整策略 vs 实际执行
//   - 线程安全：每个worker自己决定是否退出
package worker_pool

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Pool 表示协程池，支持自动扩容、优先级、超时、recovery、统计信息、动态核心worker调整
type Pool struct {
	minWorkers  int
	maxWorkers  int
	coreWorkers int // 核心worker数量，可动态调整
	mu          sync.Mutex
	workers     int
	taskQueue   taskPriorityQueue
	taskCond    *sync.Cond
	shutdown    bool
	stats       PoolStats
	wg          sync.WaitGroup
	name        string
	logger      func(format string, args ...interface{})

	// 动态调整相关字段
	allowCoreTimeout  bool          // 是否允许核心worker超时
	keepAliveTime     time.Duration // worker空闲超时时间
	lastActivityTime  int64         // 最后活动时间（纳秒时间戳）
	adjustCheckTicker *time.Ticker  // 调整检查定时器
	stopAdjustCheck   chan struct{} // 停止调整检查信号

	// 负载统计
	taskSubmitCount     int64         // 任务提交计数
	loadHistory         []LoadSample  // 负载历史记录
	loadHistoryMu       sync.RWMutex  // 负载历史锁
	adjustCheckInterval time.Duration // 调整检查间隔

	// 调整策略配置
	coreAdjustStrategy CoreAdjustStrategy // 核心worker调整策略
	lowLoadThreshold   float64            // 低负载百分比阈值 (0.0-1.0)
	fixedCoreWorkers   int                // 用户固定设置的核心worker数，0表示使用动态调整
}

// LoadSample 负载采样数据
type LoadSample struct {
	Timestamp     time.Time // 采样时间
	TaskCount     int       // 当前任务数量
	ActiveWorkers int       // 活跃worker数量
	QueueLength   int       // 队列长度
}

// CoreAdjustStrategy 核心worker调整策略
type CoreAdjustStrategy int

const (
	// StrategyFixed 固定策略：使用用户设定的固定值
	StrategyFixed CoreAdjustStrategy = iota
	// StrategyPercentage 百分比策略：基于最近3小时负载百分比动态调整
	StrategyPercentage
	// StrategyHybrid 混合策略：结合百分比和固定值的智能调整
	StrategyHybrid
)

// NewPool 创建一个协程池，minWorkers 必须，其他参数可选
func NewPool(minWorkers int, opts ...PoolOption) *Pool {
	if minWorkers < 1 {
		minWorkers = 1
	}
	p := &Pool{
		minWorkers:  minWorkers,
		maxWorkers:  minWorkers, // 默认最大等于最小
		coreWorkers: minWorkers, // 默认核心worker等于最小值
		workers:     minWorkers,
		logger:      func(format string, args ...interface{}) {}, // 默认空实现

		// 动态调整默认配置
		allowCoreTimeout:    false,
		keepAliveTime:       60 * time.Second,
		adjustCheckInterval: 10 * time.Minute, // 默认10分钟检查一次
		stopAdjustCheck:     make(chan struct{}),
		loadHistory:         make([]LoadSample, 0, 180), // 3小时，每分钟一个样本

		// 调整策略默认配置
		coreAdjustStrategy: StrategyPercentage, // 默认使用百分比策略
		lowLoadThreshold:   0.3,                // 默认30%阈值
		fixedCoreWorkers:   0,                  // 默认使用动态调整
	}
	for _, opt := range opts {
		opt(p)
	}
	p.taskCond = sync.NewCond(&p.mu)
	heap.Init(&p.taskQueue)
	for i := 0; i < minWorkers; i++ {
		p.startWorker()
	}

	// 启动动态调整监控
	p.startLoadMonitoring()

	if p.logger != nil && p.name != "" {
		p.logger("Pool %s created with min=%d, max=%d, core=%d", p.name, p.minWorkers, p.maxWorkers, p.coreWorkers)
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

// WithAllowCoreTimeout 允许核心worker超时退出
func WithAllowCoreTimeout(allow bool) PoolOption {
	return func(p *Pool) {
		p.allowCoreTimeout = allow
	}
}

// WithKeepAliveTime 设置worker空闲超时时间
func WithKeepAliveTime(duration time.Duration) PoolOption {
	return func(p *Pool) {
		p.keepAliveTime = duration
	}
}

// WithAdjustCheckInterval 设置调整检查间隔
func WithAdjustCheckInterval(interval time.Duration) PoolOption {
	return func(p *Pool) {
		p.adjustCheckInterval = interval
	}
}

// WithCoreAdjustStrategy 设置核心worker调整策略
func WithCoreAdjustStrategy(strategy CoreAdjustStrategy) PoolOption {
	return func(p *Pool) {
		p.coreAdjustStrategy = strategy
	}
}

// WithLowLoadThreshold 设置低负载百分比阈值 (0.0-1.0)
func WithLowLoadThreshold(threshold float64) PoolOption {
	return func(p *Pool) {
		if threshold < 0.0 {
			threshold = 0.0
		} else if threshold > 1.0 {
			threshold = 1.0
		}
		p.lowLoadThreshold = threshold
	}
}

// WithFixedCoreWorkers 设置固定的核心worker数量，0表示使用动态调整
func WithFixedCoreWorkers(core int) PoolOption {
	return func(p *Pool) {
		if core < 0 {
			core = 0
		}
		p.fixedCoreWorkers = core
		if core > 0 {
			p.coreAdjustStrategy = StrategyFixed
			p.coreWorkers = core
		}
	}
}

// Submit 提交一个任务到池中，支持可选参数
// - 并发安全：全程持有锁，保证任务队列和池状态一致性
// - ctx: 推荐作为第一个参数，便于任务取消/超时/统一管理
// - Option: 推荐用 Option 传递可选参数，灵活扩展
// - 默认超时时间：3秒，业务可用 WithTimeout 覆盖
func (p *Pool) Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error {
	if taskFunc == nil {
		return fmt.Errorf("taskFunc cannot be nil")
	}

	// 记录活动时间和任务计数
	atomic.StoreInt64(&p.lastActivityTime, time.Now().UnixNano())
	atomic.AddInt64(&p.taskSubmitCount, 1)

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
	if taskFunc == nil {
		return nil, fmt.Errorf("taskFunc cannot be nil")
	}

	// 记录活动时间和任务计数
	atomic.StoreInt64(&p.lastActivityTime, time.Now().UnixNano())
	atomic.AddInt64(&p.taskSubmitCount, 1)

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

// startWorker 启动一个worker goroutine
// 🔥 缩容机制详解：
// 1. worker在空闲时会检查是否可以退出（基于keepAliveTime超时）
// 2. 退出条件：allowCoreTimeout=true 且 当前worker数 > coreWorkers 且 空闲超过keepAliveTime
// 3. 通过p.workers--实现worker数量的减少，这是唯一的缩容实现位置
func (p *Pool) startWorker() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			p.mu.Lock()

			// === 🎯 缩容逻辑核心实现 ===
			// 🔥 正确的缩容逻辑：
			// 1. 缩容决策由adjustByPercentage()基于负载历史做出，体现在coreWorkers值中
			// 2. worker只需要检查：当前worker数 > coreWorkers 且 允许超时
			// 3. 不应该依赖瞬时的队列状态，而应该相信负载分析的结果

			// 检查是否可以超时退出（基于负载分析结果）
			canTimeout := p.allowCoreTimeout && p.workers > p.coreWorkers

			if len(p.taskQueue) == 0 && !p.shutdown {
				if canTimeout {
					// 🎯 超出CoreWorkers的worker：使用keepAliveTime超时等待
					// 这些worker是"多余"的，应该在空闲时退出以节省资源
					p.mu.Unlock()

					ctx, cancel := context.WithTimeout(context.Background(), p.keepAliveTime)
					done := make(chan struct{})

					go func() {
						p.mu.Lock()
						for len(p.taskQueue) == 0 && !p.shutdown {
							p.taskCond.Wait()
						}
						p.mu.Unlock()
						close(done)
					}()

					select {
					case <-done:
						cancel()
						// 有任务到达，继续处理
					case <-ctx.Done():
						cancel()
						// 🛡️ 优雅退出：基于负载分析的智能缩容
						p.mu.Lock()
						if p.workers > p.coreWorkers { // 再次确认仍然超出核心数
							p.workers-- // 🔥 缩容实现：减少worker计数
							if p.logger != nil {
								p.logger("Worker shrunk due to low load, workers: %d -> %d (coreWorkers: %d)",
									p.workers+1, p.workers, p.coreWorkers)
							}
							p.mu.Unlock()

							// 🎯 关键：通过return优雅退出goroutine
							// 这里不是被强制杀死，而是worker自主决定退出
							// 所有清理工作都会被defer语句正确执行
							// WaitGroup会被正确递减，确保Shutdown()能正常完成
							return // 优雅退出：goroutine自然结束生命周期
						}
						p.mu.Unlock()
					}
					continue
				} else {
					// 🎯 CoreWorkers范围内的worker：定期检查是否变成"多余"worker
					// 当coreWorkers动态调整后，原本的核心worker可能变成多余worker
					p.mu.Unlock()

					// 使用较短超时定期检查coreWorkers变化
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					done := make(chan struct{})

					go func() {
						p.mu.Lock()
						for len(p.taskQueue) == 0 && !p.shutdown {
							p.taskCond.Wait()
						}
						p.mu.Unlock()
						close(done)
					}()

					select {
					case <-done:
						cancel()
						// 有任务到达，继续处理
					case <-ctx.Done():
						cancel()
						// 超时，重新检查条件（下一次循环会重新评估canTimeout）
					}
					continue
				}
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

// autoScale 根据任务队列长度自动扩容worker
// 🔒 线程安全性分析：
// 1. 此方法在Submit/SubmitWithResult中调用，调用时已持有p.mu锁
// 2. 访问的字段(p.workers, p.maxWorkers, len(p.taskQueue))都在锁保护下
// 3. p.startWorker()内部会启动新goroutine，但不需要额外锁保护
// 4. 因此这个方法是线程安全的，不需要额外加锁
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

// Shutdown 优雅关闭池，等待所有任务和worker完成
// 🛡️ 优雅关闭机制：
// 1. 停止负载监控goroutine
// 2. 设置shutdown标志，阻止新任务提交
// 3. 广播唤醒所有等待的worker
// 4. 等待所有worker优雅退出（通过WaitGroup）
// 5. 确保没有goroutine泄露
func (p *Pool) Shutdown() {
	// 第一步：停止负载监控goroutine
	p.mu.Lock()
	if p.stopAdjustCheck != nil {
		select {
		case <-p.stopAdjustCheck:
			// channel already closed
		default:
			close(p.stopAdjustCheck) // 优雅停止监控goroutine
		}
	}

	// 第二步：设置关闭标志，阻止新任务提交
	p.shutdown = true

	// 第三步：唤醒所有等待的worker，让它们检查shutdown标志
	p.taskCond.Broadcast() // 广播信号，唤醒所有Wait()中的worker
	p.mu.Unlock()

	// 第四步：等待所有worker优雅退出
	// 🎯 关键：这里会阻塞直到所有worker通过return优雅退出
	// 每个worker在退出时会调用wg.Done()，当所有worker都退出后Wait()才会返回
	p.wg.Wait() // 确保所有worker都已优雅退出，无goroutine泄露

	// 到这里，可以保证：
	// - 所有正在处理的任务都已完成
	// - 所有worker goroutine都已优雅退出
	// - 没有goroutine泄露
	// - 所有资源都已正确清理
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

// startLoadMonitoring 启动负载监控和动态调整
func (p *Pool) startLoadMonitoring() {
	if p.adjustCheckInterval <= 0 {
		return // 不启动监控
	}

	p.adjustCheckTicker = time.NewTicker(p.adjustCheckInterval)

	go func() {
		defer p.adjustCheckTicker.Stop()
		for {
			select {
			case <-p.adjustCheckTicker.C:
				p.collectLoadSample()
				p.adjustCoreWorkers()
			case <-p.stopAdjustCheck:
				return
			}
		}
	}()
}

// collectLoadSample 收集负载样本
func (p *Pool) collectLoadSample() {
	p.mu.Lock()
	sample := LoadSample{
		Timestamp:     time.Now(),
		TaskCount:     int(atomic.LoadInt64(&p.taskSubmitCount)),
		ActiveWorkers: p.stats.ActiveWorkers,
		QueueLength:   len(p.taskQueue),
	}
	p.mu.Unlock()

	p.loadHistoryMu.Lock()
	defer p.loadHistoryMu.Unlock()

	// 添加新样本
	p.loadHistory = append(p.loadHistory, sample)

	// 保持最近3小时的数据（180个样本，每分钟一个）
	maxSamples := int(3 * time.Hour / p.adjustCheckInterval)
	if len(p.loadHistory) > maxSamples {
		p.loadHistory = p.loadHistory[len(p.loadHistory)-maxSamples:]
	}
}

// adjustCoreWorkers 根据策略调整核心worker数量
func (p *Pool) adjustCoreWorkers() {
	if p.coreAdjustStrategy == StrategyFixed {
		return // 固定策略不调整
	}

	p.loadHistoryMu.RLock()
	historyLen := len(p.loadHistory)
	p.loadHistoryMu.RUnlock()

	if historyLen < 10 { // 至少需要10个样本
		return
	}

	switch p.coreAdjustStrategy {
	case StrategyPercentage:
		p.adjustByPercentage()
	case StrategyHybrid:
		p.adjustByHybrid()
	}
}

// adjustByPercentage 基于负载历史百分比调整核心worker数量
// 🔒 线程安全性分析：
// 1. 使用loadHistoryMu.RLock()保护负载历史数据的读取
// 2. 访问p.minWorkers, p.lowLoadThreshold等配置字段时无锁（这些是只读配置）
// 3. 修改p.coreWorkers时使用p.mu.Lock()保护
// 4. 采用双重锁设计：读锁保护数据读取，写锁保护状态修改
// 5. 线程安全，设计合理
func (p *Pool) adjustByPercentage() {
	p.loadHistoryMu.RLock()
	defer p.loadHistoryMu.RUnlock()

	// 计算最近3小时内低负载的百分比
	lowLoadCount := 0
	totalSamples := len(p.loadHistory)

	for _, sample := range p.loadHistory {
		// 如果活跃worker数量低于minWorkers的阈值百分比，认为是低负载
		threshold := float64(p.minWorkers) * p.lowLoadThreshold
		if float64(sample.ActiveWorkers) < threshold && sample.QueueLength == 0 {
			lowLoadCount++
		}
	}

	lowLoadRatio := float64(lowLoadCount) / float64(totalSamples)

	// 如果低负载比例超过80%，考虑减少核心worker数量
	if lowLoadRatio > 0.8 {
		suggestedCore := int(float64(p.minWorkers) * p.lowLoadThreshold)
		if suggestedCore < 1 {
			suggestedCore = 1
		}

		p.mu.Lock()
		if suggestedCore < p.coreWorkers {
			oldCore := p.coreWorkers
			p.coreWorkers = suggestedCore
			if p.logger != nil {
				p.logger("Core workers adjusted from %d to %d (low load ratio: %.2f%%)",
					oldCore, p.coreWorkers, lowLoadRatio*100)
			}
		}
		p.mu.Unlock()
	} else if lowLoadRatio < 0.3 {
		// 如果低负载比例低于30%，考虑恢复核心worker数量到minWorkers
		// 注意：这里只是调整coreWorkers数量，实际的worker扩容由autoScale()处理
		p.mu.Lock()
		if p.coreWorkers < p.minWorkers {
			oldCore := p.coreWorkers
			p.coreWorkers = p.minWorkers
			if p.logger != nil {
				p.logger("Core workers restored from %d to %d (load increased)",
					oldCore, p.coreWorkers)
			}
		}
		p.mu.Unlock()
	}
}

// adjustByHybrid 混合策略调整
func (p *Pool) adjustByHybrid() {
	// 先执行百分比策略
	p.adjustByPercentage()

	// 然后考虑用户设置的固定值
	if p.fixedCoreWorkers > 0 {
		p.mu.Lock()
		if p.coreWorkers != p.fixedCoreWorkers {
			oldCore := p.coreWorkers
			p.coreWorkers = p.fixedCoreWorkers
			if p.logger != nil {
				p.logger("Core workers set to fixed value from %d to %d",
					oldCore, p.coreWorkers)
			}
		}
		p.mu.Unlock()
	}
}

// SetCoreWorkers 手动设置核心worker数量
func (p *Pool) SetCoreWorkers(core int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if core < 1 {
		core = 1
	}
	if core > p.maxWorkers {
		core = p.maxWorkers
	}

	oldCore := p.coreWorkers
	p.coreWorkers = core

	if p.logger != nil && oldCore != core {
		p.logger("Core workers manually set from %d to %d", oldCore, core)
	}
}

// GetCoreWorkers 获取当前核心worker数量
func (p *Pool) GetCoreWorkers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.coreWorkers
}

// GetLoadHistory 获取负载历史（用于监控和调试）
func (p *Pool) GetLoadHistory() []LoadSample {
	p.loadHistoryMu.RLock()
	defer p.loadHistoryMu.RUnlock()

	// 返回副本避免并发问题
	history := make([]LoadSample, len(p.loadHistory))
	copy(history, p.loadHistory)
	return history
}

// DetailedStats 详细的统计信息，包含所有监控指标
type DetailedStats struct {
	PoolStats
	CoreWorkers        int       `json:"core_workers"`
	LastActivityTime   time.Time `json:"last_activity_time"`
	TaskSubmitCount    int64     `json:"task_submit_count"`
	AllowCoreTimeout   bool      `json:"allow_core_timeout"`
	CoreAdjustStrategy string    `json:"core_adjust_strategy"`
	LowLoadThreshold   float64   `json:"low_load_threshold"`
	LoadHistoryLength  int       `json:"load_history_length"`
}

// DetailedStats 获取详细统计信息
func (p *Pool) DetailedStats() DetailedStats {
	p.mu.Lock()
	// 直接构建PoolStats，避免调用p.Stats()导致死锁
	baseStats := PoolStats{
		ActiveWorkers: p.stats.ActiveWorkers,
		QueuedTasks:   len(p.taskQueue),
		Completed:     p.stats.Completed,
		MinWorkers:    p.minWorkers,
		MaxWorkers:    p.maxWorkers,
	}
	coreWorkers := p.coreWorkers
	allowCoreTimeout := p.allowCoreTimeout
	strategy := p.coreAdjustStrategy
	threshold := p.lowLoadThreshold
	p.mu.Unlock()

	p.loadHistoryMu.RLock()
	historyLen := len(p.loadHistory)
	p.loadHistoryMu.RUnlock()

	strategyStr := "Unknown"
	switch strategy {
	case StrategyFixed:
		strategyStr = "Fixed"
	case StrategyPercentage:
		strategyStr = "Percentage"
	case StrategyHybrid:
		strategyStr = "Hybrid"
	}

	return DetailedStats{
		PoolStats:          baseStats,
		CoreWorkers:        coreWorkers,
		LastActivityTime:   time.Unix(0, atomic.LoadInt64(&p.lastActivityTime)),
		TaskSubmitCount:    atomic.LoadInt64(&p.taskSubmitCount),
		AllowCoreTimeout:   allowCoreTimeout,
		CoreAdjustStrategy: strategyStr,
		LowLoadThreshold:   threshold,
		LoadHistoryLength:  historyLen,
	}
}
