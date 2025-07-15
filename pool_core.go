package pool

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
}

// NewPool 创建一个协程池，指定最小和最大 worker 数
func NewPool(minWorkers, maxWorkers int) *Pool {
	if minWorkers < 1 {
		minWorkers = 1
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers
	}
	p := &Pool{
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		workers:    minWorkers,
	}
	p.taskCond = sync.NewCond(&p.mu)
	heap.Init(&p.taskQueue)
	for i := 0; i < minWorkers; i++ {
		p.startWorker()
	}
	return p
}

// Submit 提交一个任务到池中，支持优先级、超时、recovery
func (p *Pool) Submit(ctx context.Context, priority int, timeout time.Duration, taskFunc func(ctx context.Context) (interface{}, error), recovery func(interface{})) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return ErrPoolClosed
	}
	task := &Task{
		ctx:        ctx,
		priority:   priority,
		timeout:    timeout,
		taskFunc:   taskFunc,
		recovery:   recovery,
		resultChan: nil,
	}
	heap.Push(&p.taskQueue, task)
	p.taskCond.Signal()
	p.autoScale()
	return nil
}

// SubmitWithResult 提交带返回值的任务，返回结果 channel
func (p *Pool) SubmitWithResult(ctx context.Context, priority int, timeout time.Duration, taskFunc func(ctx context.Context) (interface{}, error), recovery func(interface{})) (<-chan taskResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shutdown {
		return nil, ErrPoolClosed
	}
	resultChan := make(chan taskResult, 1)
	task := &Task{
		ctx:        ctx,
		priority:   priority,
		timeout:    timeout,
		taskFunc:   taskFunc,
		recovery:   recovery,
		resultChan: resultChan,
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

			// 处理任务
			p.handleTask(task)

			p.mu.Lock()
			p.stats.ActiveWorkers--
			p.stats.Completed++
			p.stats.QueuedTasks = len(p.taskQueue)
			p.mu.Unlock()
		}
	}()
}

// handleTask 处理单个任务，支持超时、recovery
func (p *Pool) handleTask(task *Task) {
	ctx := task.ctx
	if task.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.timeout)
		defer cancel()
	}
	defer func() {
		if r := recover(); r != nil && task.recovery != nil {
			task.recovery(r)
		}
	}()
	result, err := task.taskFunc(ctx)
	if task.resultChan != nil {
		task.resultChan <- taskResult{result: result, err: err}
		close(task.resultChan)
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
func (p *Pool) Shutdown() {
	p.mu.Lock()
	p.shutdown = true
	p.taskCond.Broadcast()
	p.mu.Unlock()
	p.wg.Wait()
}

// 预留接口实现与扩展方法（如 SetMinWorkers、SetMaxWorkers、Restart、BatchSubmit 等）
