package main

import (
	"context"
	"fmt"
	"time"

	"github.com/UTC-Six/pool/threading"
)

func main() {
	// 设置最大并发 goroutine 数
	threading.SetMaxGoroutines(3)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recoveryFn := func(r interface{}) {
		fmt.Println("[threading] recovered in custom handler:", r)
	}

	for i := 0; i < 5; i++ {
		idx := i
		err := threading.GoSafe(ctx, func(ctx context.Context) error {
			fmt.Printf("[threading] task %d start\n", idx)
			select {
			case <-time.After(time.Duration(idx+1) * 500 * time.Millisecond):
				fmt.Printf("[threading] task %d done\n", idx)
				return nil
			case <-ctx.Done():
				fmt.Printf("[threading] task %d canceled: %v\n", idx, ctx.Err())
				return ctx.Err()
			}
		}, threading.WithRecovery(recoveryFn))
		if err != nil {
			fmt.Printf("[threading] task %d GoSafe error: %v\n", idx, err)
		}
	}

	// 推荐用 Option 风格批量提交 goroutine、设置钩子、日志、标签等参数，详见下方示例。
	// - ctx: 推荐作为第一个参数，便于统一管理生命周期、超时、取消
	// - Option: 推荐用 Option 传递可选参数，灵活扩展
	// - before/after: 支持 goroutine 执行前后自动扩展逻辑
	// - logFn/tag: 支持每个 goroutine 独立日志和标签，便于追踪
	// - name/logger: 支持全局命名和日志，便于分组和全局事件追踪
	tasks := []struct {
		TaskFunc func(ctx context.Context) error
		Tag      string
	}{
		{func(ctx context.Context) error { fmt.Println("[threading] batch task 1 running"); return nil }, "batch-1"},
		{func(ctx context.Context) error { fmt.Println("[threading] batch task 2 running"); return nil }, "batch-2"},
	}
	for _, task := range tasks {
		_ = threading.GoSafe(ctx, task.TaskFunc, threading.WithTag(task.Tag), threading.WithLog(func(format string, args ...interface{}) { fmt.Printf("[BATCH-TASK] "+format+"\n", args...) }))
	}

	// 任务前后钩子
	// WithBefore/WithAfter 可在 goroutine 执行前后自动执行自定义逻辑，常用于埋点、监控等场景。
	_ = threading.GoSafe(ctx, func(ctx context.Context) error {
		fmt.Println("[threading] task with hooks running")
		return nil
	}, threading.WithBefore(func() { fmt.Println("[threading] before hook") }), threading.WithAfter(func() { fmt.Println("[threading] after hook") }))

	// 日志与命名
	// WithName/WithLogger 可为 goroutine 命名并记录全局日志，WithLog 记录单个任务日志。
	_ = threading.GoSafe(ctx, func(ctx context.Context) error {
		fmt.Println("[threading] named task running")
		return nil
	}, threading.WithName("special-task"), threading.WithLogger(func(format string, args ...interface{}) { fmt.Printf("[LOGGER] "+format+"\n", args...) }))

	// panic 自动捕获 + 临时并发限制
	err := threading.GoSafe(context.Background(), func(ctx context.Context) error {
		panic("something wrong")
	}, threading.WithMaxGoroutines(2), threading.WithRecovery(recoveryFn))
	if err != nil {
		fmt.Println("[threading] panic recovered:", err)
	}
}
