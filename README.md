# pool —— 高性能 Go 协程池与并发控制工具集

## 项目简介

**pool** 是一套专为高并发场景设计的 Go 协程池与并发控制工具集，包含两个核心包：

- **worker_pool**：功能强大的协程池，支持任务优先级、自动扩容、任务超时、panic recovery、实时统计等特性，适用于需要精细化 goroutine 管理的复杂业务场景。
- **threading**：极简高效的 goroutine 并发控制工具，支持最大并发数限制、超时、panic 自动捕获、钩子、标签、日志等，适合轻量级并发任务的受控启动。

本项目致力于为 Go 开发者提供**安全、灵活、易用**的并发基础设施，助力你在高并发、资源敏感、任务复杂的业务中游刃有余。

---

## 适用场景

### worker_pool —— 专业协程池

适用于：
- **高并发任务调度**：如批量数据处理、爬虫、消息消费、批量 API 调用等。
- **任务优先级调度**：需要区分高低优先级任务，保证关键任务优先执行。
- **任务超时与取消**：每个任务可独立设置超时，防止“僵尸”任务拖垮系统。
- **资源受限环境**：如服务端、微服务、云函数等，需严格控制 goroutine 数量。
- **任务执行统计与监控**：实时获取活跃 worker、排队任务、已完成任务数，便于监控与调优。
- **优雅关闭与重启**：支持池的优雅关闭与重启，适合服务热升级、平滑重启场景。

**特色功能**：
- 自动扩容与弹性收缩
- 任务优先级队列
- 任务级超时与 panic recovery
- 任务前后钩子、日志、标签
- 实时池状态统计
- 线程安全，接口简洁

### threading —— 轻量级并发控制

适用于：
- **简单并发任务受控启动**：如批量异步请求、并发 I/O、定时任务等。
- **全局/临时最大并发数限制**：防止 goroutine 爆炸，保护下游服务。
- **需要超时、取消、panic 自动捕获的场景**：如网络请求、外部服务调用等。
- **需要任务级别日志、标签、钩子的场景**：便于追踪和监控每个 goroutine 的生命周期。

**特色功能**：
- 一行代码限制最大并发 goroutine 数
- 支持全局和临时并发上限
- 支持 WithTimeout、WithRecovery、WithTag、WithLog、WithBefore/After 等丰富 Option
- 支持 context 透传，灵活控制生命周期
- panic 自动捕获，防止服务崩溃
- 适合轻量、无池化需求的并发场景

---

## 推荐用法

### worker_pool 示例

```go
p := worker_pool.NewPool(2, worker_pool.WithMaxWorkers(10))
defer p.Shutdown()

// 提交带优先级和超时的任务
_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    // 任务逻辑
    return nil, nil
}, worker_pool.WithPriority(worker_pool.PriorityHigh), worker_pool.WithTimeout(2*time.Second))

// 获取池状态
stats := p.Stats()
fmt.Printf("活跃: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
```

### threading 示例

```go
threading.SetMaxGoroutines(5) // 全局最大并发5

// 无 ctx 版本，支持 WithTimeout
err := threading.GoSafe(func() error {
    // 任务逻辑
    return nil
}, threading.WithTimeout(1*time.Second), threading.WithTag("job1"))

// 带 ctx 版本，支持外部取消
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
err = threading.GoSafeCtx(ctx, func(ctx context.Context) error {
    // 任务逻辑
    return nil
}, threading.WithLog(func(format string, args ...interface{}) { fmt.Printf(format, args...) }))
```

---

## 为什么选择 pool？

- **极致性能**：底层实现高效，最大化资源利用率。
- **灵活易用**：Option 风格配置，API 简洁，易于集成。
- **安全可靠**：panic 自动捕获，任务超时、取消、优雅关闭，保障服务稳定。
- **可观测性强**：实时统计、任务日志、标签、钩子，便于监控和调优。
- **适用广泛**：既能满足高阶池化需求，也能覆盖轻量级并发场景。

---

## 立即体验

欢迎 star、fork、issue、PR！  
让 pool 成为你 Go 并发开发的得力助手！

GitHub 地址：https://github.com/UTC-Six/pool

---

如需定制化支持或有建议，欢迎联系作者或提交 issue！ 