package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/UTC-Six/pool/threading"
)

// ContextProcessor 处理context的函数类型
type ContextProcessor func(ctx context.Context) context.Context

// MonitorOption 监控器选项
type MonitorOption func(*PerformanceMonitor)

// WithLogger 设置日志函数选项
func WithLogger(logger func(ctx context.Context, format string, args ...interface{})) MonitorOption {
	return func(pm *PerformanceMonitor) {
		pm.logger = logger
	}
}

// WithCtx 设置context处理函数选项
func WithCtx(processor ContextProcessor) MonitorOption {
	return func(pm *PerformanceMonitor) {
		pm.contextProcessor = processor
	}
}

// Monitor 监控器实例
var defaultMonitor *PerformanceMonitor

// PerformanceMonitor 性能监控器
type PerformanceMonitor struct {
	logger           func(ctx context.Context, format string, args ...interface{})
	contextProcessor ContextProcessor
}

// NewPerformanceMonitor 创建性能监控器
func NewPerformanceMonitor(opts ...MonitorOption) *PerformanceMonitor {
	pm := &PerformanceMonitor{
		logger:           defaultLogger,           // 使用默认logger
		contextProcessor: defaultContextProcessor, // 使用默认的context处理器
	}

	// 应用选项
	for _, opt := range opts {
		opt(pm)
	}

	return pm
}

// defaultLogger 默认日志函数
func defaultLogger(ctx context.Context, format string, args ...interface{}) {
	// 默认使用标准log，可以通过WithLogger覆盖
	fmt.Printf(format+"\n", args...)
}

// defaultContextProcessor 默认的context处理器
func defaultContextProcessor(ctx context.Context) context.Context {
	// 如果没有设置WithCtx，直接返回Background
	return context.Background()
}

// Monitor 监控执行时间（最优实现）
func Monitor(ctx context.Context, startTime time.Time, name string, logger func(ctx context.Context, format string, args ...interface{})) {
	// 确保defaultMonitor已初始化
	if defaultMonitor == nil {
		defaultMonitor = NewPerformanceMonitor()
	}

	// 如果没有提供logger，使用默认的
	if logger == nil {
		logger = defaultMonitor.logger
	}

	// 计算耗时
	duration := time.Since(startTime)

	// 使用配置的context处理器创建新的context
	asyncCtx := defaultMonitor.contextProcessor(ctx)

	// 使用 threading.GoSafe 异步记录完成日志（最优选择）
	threading.GoSafe(func() error {
		// 使用 logz 格式的日志，traceID 会自动从 asyncCtx 中提取
		logger(asyncCtx, "[Performance] Name=%s, Duration=%v, Status=completed", name, duration)
		return nil
	}, threading.WithTag(name),
		threading.WithLog(func(format string, args ...interface{}) {
			// 如果 threading 内部需要日志，也使用我们的 logger
			logger(asyncCtx, format, args...)
		}),
		threading.WithRecovery(func(r interface{}) {
			logger(asyncCtx, "[Performance] Name=%s, Duration=%v, Status=logError, Error=%v", name, duration, r)
		}))
}
