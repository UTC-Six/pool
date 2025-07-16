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

	// 批量任务示例
	// 每个 goroutine 可单独设置 WithTag（任务标签）和 WithLog（任务日志），便于区分和追踪批量任务执行情况。
	for i := 0; i < 3; i++ {
		idx := i
		_ = threading.GoSafe(ctx, func(ctx context.Context) error {
			fmt.Printf("[threading] batch task %d running\n", idx)
			return nil
		}, threading.WithTag(fmt.Sprintf("batch-%d", idx)), threading.WithLog(func(format string, args ...interface{}) { fmt.Printf("[BATCH-TASK] "+format+"\n", args...) }))
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
