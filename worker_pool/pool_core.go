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
	// 停止负载监控
	p.mu.Lock()
	if p.stopAdjustCheck != nil {
		select {
		case <-p.stopAdjustCheck:
			// channel already closed
		default:
			close(p.stopAdjustCheck)
		}
	}
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

// adjustByPercentage 基于百分比策略调整
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
		// 如果低负载比例低于30%，考虑恢复核心worker数量
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

// EnhancedStats 增强的统计信息
type EnhancedStats struct {
	PoolStats
	CoreWorkers        int       `json:"core_workers"`
	LastActivityTime   time.Time `json:"last_activity_time"`
	TaskSubmitCount    int64     `json:"task_submit_count"`
	AllowCoreTimeout   bool      `json:"allow_core_timeout"`
	CoreAdjustStrategy string    `json:"core_adjust_strategy"`
	LowLoadThreshold   float64   `json:"low_load_threshold"`
	LoadHistoryLength  int       `json:"load_history_length"`
}

// EnhancedStats 获取增强统计信息
func (p *Pool) EnhancedStats() EnhancedStats {
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

	return EnhancedStats{
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
