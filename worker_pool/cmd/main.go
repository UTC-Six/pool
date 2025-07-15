package main

import (
	"context"
	"fmt"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)

func main() {
	// 创建协程池
	p := worker_pool.NewPool(2, 5)
	defer p.Shutdown()

	// 提交普通任务
	err := p.Submit(context.Background(), worker_pool.PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool] hello from pool")
		return nil, nil
	}, nil)
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带返回值任务
	resultCh, _ := p.SubmitWithResult(context.Background(), worker_pool.PriorityHigh, 0, func(ctx context.Context) (interface{}, error) {
		return "result from pool", nil
	}, nil)
	res := <-resultCh
	fmt.Println("[worker_pool] result:", res.Result, "err:", res.Err)

	// 提交带超时的任务
	err = p.Submit(context.Background(), worker_pool.PriorityNormal, 1*time.Second, func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(2 * time.Second):
			fmt.Println("[worker_pool] done")
		case <-ctx.Done():
			fmt.Println("[worker_pool] timeout or cancelled")
		}
		return nil, nil
	}, nil)
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带 recovery 的任务
	_ = p.Submit(context.Background(), worker_pool.PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
		panic("something wrong")
	}, func(r interface{}) {
		fmt.Println("[worker_pool] recovered from:", r)
	})

	// 获取统计信息
	stats := p.Stats()
	fmt.Printf("[worker_pool] 活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}
