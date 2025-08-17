// Package threading 提供受控的goroutine启动工具
// 🎯 核心设计理念：
// 1. 轻量级：按需创建goroutine，无预分配开销
// 2. 受控制：通过信号量限制并发数，避免goroutine爆炸
// 3. 安全性：内置panic恢复，单个任务失败不影响全局
// 4. 灵活性：丰富的配置选项，支持超时、钩子、日志等
//
// 🔄 与worker_pool的区别：
// - worker_pool: 预分配worker池，适合持续高频任务
// - threading: 按需创建，适合偶发性或一次性任务
package threading

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// === 全局并发控制 ===
// 使用信号量模式控制全局goroutine数量，这是Go中控制并发的经典模式
var (
	maxGoroutines = DefaultMaxGoroutines               // 默认最大并发数
	semMu         sync.Mutex                           // 保护信号量重建的互斥锁
	sem           = make(chan struct{}, maxGoroutines) // 信号量：channel容量=最大并发数
)

// SetMaxGoroutines 动态设置全局最大并发goroutine数
// 🔧 使用场景：
// - 系统启动时根据CPU核心数设置合理并发数
// - 运行时根据系统负载动态调整并发限制
// - 不同环境（开发/测试/生产）使用不同并发配置
//
// 💡 设计亮点：
// - 线程安全的动态调整，运行时无需重启
// - 智能保留现有goroutine占用，避免突然中断
// - 无效参数直接忽略，不会破坏现有状态
func SetMaxGoroutines(n int) {
	// 参数校验：负数和零值无意义
	if n <= 0 {
		return
	}

	// === 🔒 临界区：重建信号量 ===
	semMu.Lock()
	defer semMu.Unlock()

	// 如果新值与当前容量相同，无需操作
	if n == cap(sem) {
		return
	}

	// 保存旧信号量，创建新信号量
	oldSem := sem
	sem = make(chan struct{}, n)

	// 🧠 智能迁移：尽量保留原有的goroutine占用
	// 这样可以避免正在运行的goroutine被突然中断
	for i := 0; i < len(oldSem) && i < n; i++ {
		sem <- struct{}{} // 将占用状态迁移到新信号量
	}

	maxGoroutines = n
}

// goSafeConfig 用于 GoSafe 的可选参数配置
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
	timeout       time.Duration // 新增：可选超时
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

// WithTimeout 设置本次 GoSafe 的超时时间（仅 GoSafe 有效，GoSafeCtx 请直接传 context）
func WithTimeout(timeout time.Duration) GoSafeOption {
	return func(cfg *goSafeConfig) {
		cfg.timeout = timeout
	}
}

// GoSafe 启动受控 goroutine（无 ctx 版本），支持 WithTimeout
func GoSafe(fn func() error, opts ...GoSafeOption) error {
	cfg := goSafeConfig{
		maxGoroutines: maxGoroutines,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	var ctx context.Context = context.Background()
	var cancel context.CancelFunc
	if cfg.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}
	return GoSafeCtx(ctx, func(_ context.Context) error { return fn() }, opts...)
}

// GoSafeCtx 启动受控 goroutine
func GoSafeCtx(ctx context.Context, fn func(ctx context.Context) error, opts ...GoSafeOption) error {
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
