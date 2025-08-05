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

	// GoSafe（无 ctx 版本，支持 WithTimeout）
	err := threading.GoSafe(func() error {
		fmt.Println("[threading] GoSafe 无 ctx 任务开始")
		time.Sleep(1 * time.Second)
		fmt.Println("[threading] GoSafe 无 ctx 任务结束")
		return nil
	}, threading.WithTimeout(1500*time.Millisecond))
	if err != nil {
		fmt.Println("[threading] GoSafe error:", err)
	}

	// GoSafeCtx（带 ctx 版本，支持外部 context 控制）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = threading.GoSafeCtx(ctx, func(ctx context.Context) error {
		fmt.Println("[threading] GoSafeCtx 任务开始")
		select {
		case <-time.After(3 * time.Second):
			fmt.Println("[threading] GoSafeCtx 正常完成")
		case <-ctx.Done():
			fmt.Println("[threading] GoSafeCtx 超时/取消:", ctx.Err())
			return ctx.Err()
		}
		return nil
	})
	if err != nil {
		fmt.Println("[threading] GoSafeCtx error:", err)
	}

	// 其它 Option 用法同理

	// GoSafe 使用 WithRecovery，自动捕获 panic 并自定义处理
	err = threading.GoSafe(func() error {
		fmt.Println("[threading] GoSafe panic 任务开始")
		panic("something went wrong!")
	}, threading.WithRecovery(func(r interface{}) {
		fmt.Printf("[threading] panic recovered: %v\n", r)
	}))
	if err != nil {
		fmt.Println("[threading] GoSafe panic error:", err)
	}
}
