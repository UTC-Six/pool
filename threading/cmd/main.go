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
		})
		if err != nil {
			fmt.Printf("[threading] task %d GoSafe error: %v\n", idx, err)
		}
	}

	// panic 自动捕获
	err := threading.GoSafe(context.Background(), func(ctx context.Context) error {
		panic("something wrong")
	})
	if err != nil {
		fmt.Println("[threading] panic recovered:", err)
	}
}
