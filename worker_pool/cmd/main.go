package main

import (
	"context"
	"fmt"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)

func main() {
	// 创建协程池
	p := worker_pool.NewPool(2, worker_pool.WithMaxWorkers(5))
	defer p.Shutdown()

	// 提交普通任务
	err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool] hello from pool")
		return nil, nil
	})
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带返回值任务
	resultCh, _ := p.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "result from pool", nil
	}, worker_pool.WithPriority(worker_pool.PriorityHigh))
	res := <-resultCh
	fmt.Println("[worker_pool] result:", res.Result, "err:", res.Err)

	// 提交带超时的任务
	err = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(2 * time.Second):
			fmt.Println("[worker_pool] done")
		case <-ctx.Done():
			fmt.Println("[worker_pool] timeout or cancelled")
		}
		return nil, nil
	}, worker_pool.WithTimeout(1*time.Second))
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带 recovery 的任务
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		panic("something wrong")
	}, worker_pool.WithRecovery(func(r interface{}) {
		fmt.Println("[worker_pool] recovered from:", r)
	}))

	// 批量提交任务，带标签和日志
	// 每个任务可单独设置 WithTag（任务标签）和 WithLog（任务日志），便于区分和追踪批量任务执行情况。
	tasks := []worker_pool.BatchTaskOpt{
		{
			TaskFunc: func(ctx context.Context) (interface{}, error) {
				fmt.Println("[worker_pool] batch task 1 running")
				return "A", nil
			},
			Options: []worker_pool.TaskOption{worker_pool.WithTag("A"), worker_pool.WithLog(func(format string, args ...interface{}) { fmt.Printf("[TASK] "+format+"\n", args...) })},
		},
		{
			TaskFunc: func(ctx context.Context) (interface{}, error) {
				fmt.Println("[worker_pool] batch task 2 running")
				return "B", nil
			},
			Options: []worker_pool.TaskOption{worker_pool.WithTag("B"), worker_pool.WithLog(func(format string, args ...interface{}) { fmt.Printf("[TASK] "+format+"\n", args...) })},
		},
	}
	_ = p.BatchSubmit(context.Background(), tasks)

	// 任务前后钩子
	// WithBefore/WithAfter 可在任务执行前后自动执行自定义逻辑，常用于埋点、监控等场景。
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool] task with hooks running")
		return nil, nil
	}, worker_pool.WithBefore(func() { fmt.Println("[worker_pool] before hook") }), worker_pool.WithAfter(func() { fmt.Println("[worker_pool] after hook") }))

	// 动态调整
	// 运行时动态调整池的 min/max worker，适合弹性伸缩场景。
	p.SetMinWorkers(3)
	p.SetMaxWorkers(6)
	fmt.Printf("[worker_pool] 动态调整后: min=%d, max=%d\n", p.Stats().ActiveWorkers, p.Stats().QueuedTasks)

	// 获取统计信息
	stats := p.Stats()
	fmt.Printf("[worker_pool] 活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}
