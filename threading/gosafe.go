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

// GoSafeOption 和 goSafeConfig 用于可选参数配置
type GoSafeOption func(*goSafeConfig)
type goSafeConfig struct {
	maxGoroutines int
	recovery      func(interface{})
	logFn         func(format string, args ...interface{})
	tag           string
	before        func()
	after         func()
	name          string
	logger        func(format string, args ...interface{})
}

// WithMaxGoroutines 临时设置本次 GoSafe 的最大并发 goroutine 数
func WithMaxGoroutines(n int) GoSafeOption {
	return func(cfg *goSafeConfig) {
		if n > 0 {
			cfg.maxGoroutines = n
		}
	}
}

// WithRecovery 设置 panic 恢复处理
func WithRecovery(recovery func(interface{})) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.recovery = recovery
	}
}

func WithLog(logFn func(format string, args ...interface{})) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.logFn = logFn
	}
}
func WithTag(tag string) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.tag = tag
	}
}
func WithBefore(before func()) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.before = before
	}
}
func WithAfter(after func()) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.after = after
	}
}
func WithName(name string) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.name = name
	}
}
func WithLogger(logger func(format string, args ...interface{})) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.logger = logger
	}
}

// GoSafe 启动受控 goroutine，支持 context、panic recovery，返回详细 error
func GoSafe(ctx context.Context, fn func(ctx context.Context) error, opts ...GoSafeOption) error {
	cfg := goSafeConfig{
		maxGoroutines: maxGoroutines,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.logger != nil && cfg.name != "" {
		cfg.logger("GoSafe %s created with maxGoroutines=%d", cfg.name, cfg.maxGoroutines)
	}

	// 临时并发控制（如 Option 覆盖）
	var semLocal chan struct{}
	var useLocalSem bool
	if cfg.maxGoroutines != maxGoroutines {
		semLocal = make(chan struct{}, cfg.maxGoroutines)
		useLocalSem = true
	}

	acquire := func() bool {
		if useLocalSem {
			select {
			case semLocal <- struct{}{}:
				return true
			case <-ctx.Done():
				return false
			}
		} else {
			select {
			case sem <- struct{}{}:
				return true
			case <-ctx.Done():
				return false
			}
		}
	}
	release := func() {
		if useLocalSem {
			<-semLocal
		} else {
			<-sem
		}
	}

	if !acquire() {
		return fmt.Errorf("GoSafe: context canceled before acquiring slot: %w", ctx.Err())
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			release()
			if r := recover(); r != nil {
				if cfg.recovery != nil {
					cfg.recovery(r)
				}
				if cfg.logFn != nil {
					cfg.logFn("[GoSafe] panic recovered: %v, tag=%s", r, cfg.tag)
				}
				if cfg.after != nil {
					cfg.after()
				}
				done <- fmt.Errorf("GoSafe: panic recovered: %v", r)
			}
		}()
		if cfg.before != nil {
			cfg.before()
		}
		if cfg.logFn != nil {
			cfg.logFn("[GoSafe] start tag=%s", cfg.tag)
		}
		// 执行用户函数
		err := fn(ctx)
		if cfg.after != nil {
			cfg.after()
		}
		if cfg.logFn != nil {
			cfg.logFn("[GoSafe] end tag=%s", cfg.tag)
		}
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
