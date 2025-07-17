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
		fmt.Println("[worker_pool.Submit] hello from pool")
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
	fmt.Println("[worker_pool.SubmitWithResult] result:", res.Result, "err:", res.Err)

	// 提交带超时的任务
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = p.Submit(ctx, func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(2 * time.Second):
			fmt.Println("[worker_pool] done")
		case <-ctx.Done():
			fmt.Println("[worker_pool] ctx cancelled 场景")
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

	// 批量提交任务，推荐用 Option 风格循环调用 Submit
	tasks := []struct {
		TaskFunc func(ctx context.Context) (interface{}, error)
		Tag      string
	}{
		{func(ctx context.Context) (interface{}, error) {
			fmt.Println("[worker_pool] batch task 1 running")
			return "A", nil
		}, "A"},
		{func(ctx context.Context) (interface{}, error) {
			fmt.Println("[worker_pool] batch task 2 running")
			return "B", nil
		}, "B"},
	}
	for _, task := range tasks {
		_ = p.Submit(context.Background(), task.TaskFunc, worker_pool.WithTag(task.Tag), worker_pool.WithLog(func(format string, args ...interface{}) { fmt.Printf("[TASK] "+format+"\n", args...) }))
	}

	// 任务前后钩子
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool] task with hooks running")
		return nil, nil
	}, worker_pool.WithBefore(func() { fmt.Println("[worker_pool] before hook") }), worker_pool.WithAfter(func() { fmt.Println("[worker_pool] after hook") }))

	// 动态调整
	p.SetMinWorkers(3)
	p.SetMaxWorkers(6)
	stats := p.Stats()
	fmt.Printf("[worker_pool] 动态调整后: min=%d, max=%d, active=%d, queued=%d\n", stats.MinWorkers, stats.MaxWorkers, stats.ActiveWorkers, stats.QueuedTasks)

	// 获取统计信息
	stats = p.Stats()
	fmt.Printf("[worker_pool] 活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}
