package threading

import (
	"context"
	"fmt"
	"sync"
)

var (
	maxGoroutines = 100 // 默认最大并发数
	semMu         sync.Mutex
	sem           = make(chan struct{}, maxGoroutines)
)

// SetMaxGoroutines 动态设置最大并发 goroutine 数
func SetMaxGoroutines(n int) {
	if n <= 0 {
		return
	}
	semMu.Lock()
	defer semMu.Unlock()
	if n == cap(sem) {
		return
	}
	oldSem := sem
	sem = make(chan struct{}, n)
	// 尽量保留原有已占用的 goroutine 数
	for i := 0; i < len(oldSem) && i < n; i++ {
		sem <- struct{}{}
	}
	maxGoroutines = n
}

// GoSafe 启动受控 goroutine，支持 context、panic recovery，返回详细 error
func GoSafe(ctx context.Context, fn func(ctx context.Context) error) error {
	// 先尝试获取信号量
	select {
	case sem <- struct{}{}:
		// 成功获取
	case <-ctx.Done():
		return fmt.Errorf("GoSafe: context canceled before acquiring slot: %w", ctx.Err())
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			<-sem // 归还信号量
			if r := recover(); r != nil {
				done <- fmt.Errorf("GoSafe: panic recovered: %v", r)
			}
		}()
		// 执行用户函数
		err := fn(ctx)
		done <- err
	}()
	// 等待 goroutine 结束或 context 取消
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("GoSafe: function error: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("GoSafe: context canceled while waiting: %w", ctx.Err())
	}
}
