# Monitor - 高性能异步监控包

## 概述

Monitor 是一个专为 Go 应用设计的轻量级、高性能异步监控包。它使用 `threading.GoSafe` 作为唯一的异步处理方案，为函数和 API 执行提供非阻塞的性能监控，并完美集成 `logz` 格式的日志系统。

## 核心特性

### 🚀 **极简设计**
- **单一方法**：只提供一个 `Monitor` 方法，简洁高效
- **灵活使用**：用户可以根据需要自定义 name 参数
- **零学习成本**：API 简单直观，易于使用

### ⚡ **零性能影响**
- **异步处理**：监控逻辑完全异步，不影响原函数执行时间
- **即时计算**：在主线程计算耗时，异步线程只负责日志记录
- **最小开销**：使用最轻量级的异步处理方式

### 🔗 **完美集成 logz 日志系统**
- **自动 traceID 提取**：使用 `logz.Infof(ctx, fmt, params...)` 格式
- **Context 安全传递**：创建新的 context 确保异步执行时 traceID 仍然有效
- **统一日志格式**：与现有 logz 系统完全兼容

### 🛡️ **安全可靠**
- **Panic 恢复**：监控逻辑的 panic 不会影响主流程
- **错误隔离**：异步处理确保监控错误不影响业务逻辑
- **资源管理**：自动管理 goroutine 生命周期

### 🔧 **Optional 配置方式**
- **无需 init 函数**：支持默认配置，开箱即用
- **WithLogger 选项**：如果没有设置，使用默认 logger
- **WithCtx 选项**：如果没有设置，直接返回 `context.Background()`
- **组合使用**：支持同时使用多个选项

## 为什么选择 threading.GoSafe？

### 1. **轻量级设计**
```go
// threading.GoSafe 专门为轻量级异步任务设计
threading.GoSafe(func() error {
    // 简单的日志记录任务
    logz.Infof(asyncCtx, "[Performance] Name=%s, Duration=%v", name, duration)
    return nil
})
```

### 2. **内置并发控制**
- 自动限制并发 goroutine 数量
- 防止资源耗尽和系统过载
- 适合高频轻量级任务

### 3. **简单可靠**
- 实现简单，逻辑清晰
- 不容易出错，维护成本低
- 性能稳定，可预测

### 4. **对比 worker_pool**
| 特性 | threading.GoSafe | worker_pool |
|------|------------------|-------------|
| 适用场景 | 轻量级任务 | 重量级任务 |
| 资源开销 | 极低 | 中等 |
| 复杂度 | 简单 | 复杂 |
| 功能特性 | 基础 | 丰富（队列、优先级等） |
| 性能 | 优秀 | 良好 |

**结论**：对于日志记录这种轻量级任务，`threading.GoSafe` 是最优选择。

## API 设计

### 核心 API

```go
// 监控执行时间
func Monitor(ctx context.Context, startTime time.Time, name string, logger func(ctx context.Context, format string, args ...interface{}))

// 创建监控器（支持选项）
func NewPerformanceMonitor(opts ...MonitorOption) *PerformanceMonitor

// 选项类型
type MonitorOption func(*PerformanceMonitor)

// WithLogger 设置日志函数选项
func WithLogger(logger func(ctx context.Context, format string, args ...interface{})) MonitorOption

// WithCtx 设置context处理函数选项
func WithCtx(processor ContextProcessor) MonitorOption

// ContextProcessor 类型定义
type ContextProcessor func(ctx context.Context) context.Context
```

### 优雅的使用方式

```go
// 监控函数执行时间
func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 你的业务逻辑
    return nil
}

// 监控API接口执行时间
func apiHandler(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "GET /api/users", nil)
    
    // API处理逻辑
    return nil
}

// 自定义监控名称
func complexFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "ComplexBusinessLogic", nil)
    
    // 复杂业务逻辑
    return nil
}
```

## 使用示例

### 基本用法（使用默认配置）

```go
package main

import (
    "context"
    "time"
    "github.com/UTC-Six/pool/monitor"
)

func businessFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "businessFunction", nil)
    
    // 业务逻辑
    time.Sleep(100 * time.Millisecond)
    return nil
}

func main() {
    ctx := context.Background()
    businessFunction(ctx)
}
```

### 使用 WithLogger 选项

```go
// 设置 logz 格式的日志函数
monitor := NewPerformanceMonitor(
    WithLogger(func(ctx context.Context, format string, args ...interface{}) {
        logz.Infof(ctx, format, args...)
    }),
)

func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 业务逻辑
    return nil
}
```

### 使用 WithCtx 选项

```go
// 自定义的context处理器
func customContextProcessor(ctx context.Context) context.Context {
    // 如果ctx为空，返回Background
    if ctx == nil {
        return context.Background()
    }

    // 创建一个新的 context
    asyncCtx := context.Background()
    
    // 复制原 ctx 中的关键信息到新的 ctx
    if traceValue := ctx.Value("trace-id"); traceValue != nil {
        asyncCtx = context.WithValue(asyncCtx, "trace-id", traceValue)
    }
    if traceValue := ctx.Value("uber-trace-id"); traceValue != nil {
        asyncCtx = context.WithValue(asyncCtx, "uber-trace-id", traceValue)
    }
    if traceValue := ctx.Value("X-B3-TraceId"); traceValue != nil {
        asyncCtx = context.WithValue(asyncCtx, "X-B3-TraceId", traceValue)
    }
    
    // 如果没有找到任何traceID，返回Background
    if asyncCtx.Value("trace-id") == nil && 
       asyncCtx.Value("uber-trace-id") == nil && 
       asyncCtx.Value("X-B3-TraceId") == nil {
        return context.Background()
    }
    
    return asyncCtx
}

// 创建监控器
monitor := NewPerformanceMonitor(
    WithCtx(customContextProcessor),
)
```

### 组合使用多个选项

```go
// 同时使用 WithLogger 和 WithCtx
monitor := NewPerformanceMonitor(
    WithLogger(func(ctx context.Context, format string, args ...interface{}) {
        logz.Infof(ctx, format, args...)
    }),
    WithCtx(customContextProcessor),
)

func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 业务逻辑
    return nil
}
```

### 链路追踪集成

```go
// 支持多种链路追踪系统
ctx := context.WithValue(context.Background(), "trace-id", "trace-12345")
// 或者
ctx := context.WithValue(context.Background(), "uber-trace-id", "1-5759e988-bd862e3fe1be46a994272793")
// 或者
ctx := context.WithValue(context.Background(), "X-B3-TraceId", "bd862e3fe1be46a994272793")

func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 业务逻辑
    return nil
}
```

## 日志输出格式

### 监控日志
```
[TraceID=trace-12345] [Performance] Name=myFunction, Duration=100.5ms, Status=completed
```

### 错误日志
```
[TraceID=trace-12345] [Performance] Name=myFunction, Duration=100.5ms, Status=logError, Error=panic occurred
```

## Context 安全传递机制

### 问题背景
当使用异步处理时，原始的 context 可能在异步 goroutine 执行时已经失效，导致 traceID 等信息丢失。

### 解决方案
```go
// 使用配置的context处理器创建新的context
asyncCtx := defaultMonitor.contextProcessor(ctx)

// 在异步 goroutine 中使用 asyncCtx
threading.GoSafe(func() error {
    logz.Infof(asyncCtx, "[Performance] Name=%s, Duration=%v", name, duration)
    return nil
})
```

### 优势
1. **保证 traceID 可用**：异步执行时 traceID 仍然有效
2. **避免 context 失效**：不依赖可能已失效的原始 context
3. **完整信息传递**：复制所有关键的链路追踪信息
4. **统一配置**：公司内部可以统一配置一次，所有服务复用
5. **兜底处理**：空 context 或无 traceID 时返回 `context.Background()`

## 配置

### 默认配置（开箱即用）

```go
// 无需任何配置，直接使用
func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 业务逻辑
    return nil
}
```

### 自定义配置

```go
// 创建自定义监控器
monitor := NewPerformanceMonitor(
    WithLogger(func(ctx context.Context, format string, args ...interface{}) {
        logz.Infof(ctx, format, args...)
    }),
    WithCtx(customContextProcessor),
)
```

## 性能优势

### 1. **零性能影响**
- 监控逻辑完全异步执行
- 主线程只计算耗时，开销极小
- 不影响原函数执行时间

### 2. **高效异步处理**
- 使用 `threading.GoSafe` 轻量级异步处理
- 内置并发控制，防止资源耗尽
- 适合高频调用场景

### 3. **内存友好**
- 无复杂的数据结构
- 无队列管理开销
- 最小化内存占用

## 最佳实践

### 1. **统一使用 defer 模式**
```go
func myFunction(ctx context.Context) error {
    start := time.Now()
    defer monitor.Monitor(ctx, start, "myFunction", nil)
    
    // 业务逻辑
    return nil
}
```

### 2. **使用 WithLogger 集成 logz**
```go
monitor := NewPerformanceMonitor(
    WithLogger(func(ctx context.Context, format string, args ...interface{}) {
        logz.Infof(ctx, format, args...)
    }),
)
```

### 3. **使用 WithCtx 自定义 Context 处理**
```go
monitor := NewPerformanceMonitor(
    WithCtx(customContextProcessor),
)
```

### 4. **利用链路追踪**
```go
// 确保 context 中包含 traceID
ctx := context.WithValue(ctx, "trace-id", traceID)
```

### 5. **合理命名**
```go
// 使用有意义的名称
defer monitor.Monitor(ctx, start, "UserAuthentication", nil)
defer monitor.Monitor(ctx, start, "DatabaseQuery", nil)
defer monitor.Monitor(ctx, start, "GET /api/users", nil)
```

### 6. **避免过度监控**
```go
// 只监控重要的函数和API
// 避免监控过于频繁的简单函数
```

## 设计优势总结

### 1. **极简设计**
- 只有一个 Monitor 方法，简洁高效
- 用户可以根据需要自定义 name 参数
- 零学习成本，易于使用

### 2. **Optional 配置**
- 无需 init 函数，支持默认配置
- 使用 WithLogger 和 WithCtx 的 optional 方式
- 支持组合使用多个选项

### 3. **简单可靠**
- 代码简洁，逻辑清晰
- 不容易出错，维护成本低
- 性能稳定，可预测

### 4. **高性能**
- 轻量级异步处理，零性能影响
- 内置并发控制，防止资源耗尽
- 适合高频调用场景

### 5. **完美集成**
- 与 logz 系统无缝集成
- 支持自动 traceID 提取
- Context 安全传递

### 6. **可配置性**
- 支持自定义 ContextProcessor
- 公司内部可以统一配置一次
- 兜底处理，安全可靠

### 7. **易维护**
- 单一实现方案，维护成本低
- 无复杂的fallback逻辑
- 清晰的API设计

这个设计避免了过度工程化，专注于核心需求，是最优的实现选择。 