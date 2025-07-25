# 一、 worker_pool 任务优先级调度原理详解

`worker_pool` 能够保证任务优先级调度，核心在于**任务队列采用了优先级队列（堆）结构**，每次 worker 取任务时，都会优先取出优先级最高的任务。下面详细解释其实现原理：

---

## 1. 任务队列采用优先级堆

在 `worker_pool` 的实现中，任务队列 `taskQueue` 实际上是一个**大顶堆**（priority queue），底层用 Go 的 `container/heap` 实现：

```go
type taskPriorityQueue []*Task

func (pq taskPriorityQueue) Less(i, j int) bool {
    return pq[i].Priority > pq[j].Priority // 数值越大优先级越高
}
```
- 这里 `Less` 方法决定了堆的顺序，**优先级数值越大，任务越靠前**。

---

## 2. 提交任务时入堆

每次调用 `Submit` 或 `SubmitWithResult`，任务会被 push 到堆中：

```go
heap.Push(&p.taskQueue, task)
```
- 任务的 `Priority` 字段由 `WithPriority` Option 设置，默认是普通优先级。

---

## 3. worker 取任务时出堆

每个 worker 在主循环中取任务时，都是从堆顶 pop：

```go
task := heap.Pop(&p.taskQueue).(*Task)
```
- 堆顶永远是当前队列中**优先级最高的任务**。

---

## 4. 线程安全保证

- 所有对 `taskQueue` 的操作都在加锁（`p.mu.Lock()`）的保护下进行，保证并发安全。
- 多个 worker 并发取任务时，始终只有一个能成功 pop 堆顶任务。

---

## 5. 任务优先级的实际效果

- **高优先级任务**：即使后提交，也会被优先执行。
- **低优先级任务**：只有在高优先级任务都被处理完后才会被执行。

---

## 6. 相同优先级任务调试顺序是不确定的，不是 FIFO，如果想要实现FIFO的实际效果，请改造如下方法
- 你可以在 Task 结构体中增加一个自增的 Index 字段（或时间戳），并在 Less 方法中做二次比较
```golang
type Task struct {
    Priority int
    Index    int // 或 int64 时间戳
    // ...
}

func (pq taskPriorityQueue) Less(i, j int) bool {
    if pq[i].Priority == pq[j].Priority {
        return pq[i].Index < pq[j].Index // Index 小的先出队，实现 FIFO
    }
    return pq[i].Priority > pq[j].Priority
}
```
- 这样就能保证同优先级时，先 push 的任务会先被调度。

---

## 总结

worker_pool 的优先级调度依赖于：
- 任务队列为大顶堆（priority queue）
- worker 只从堆顶取任务
- 线程安全的堆操作

**只要任务优先级设置正确，worker_pool 就能保证高优先级任务总是优先被调度执行。**

# 二、每个任务可独立设置超时，防止“僵尸”任务拖垮系统

## 1 独立超时机制说明
worker_pool 支持为每个任务单独设置超时时间，具体机制如下：

- 在提交任务时，可以通过 `WithTimeout(duration)` Option 为该任务指定超时时间。
- 在 worker 执行任务时，`handleTask` 方法会判断 `task.Timeout` 是否大于 0，如果是，则用 `context.WithTimeout(ctx, task.Timeout)` 包裹原有 context，生成一个带超时控制的新 context。
- 任务函数内部只需监听 `ctx.Done()`，即可感知超时或取消。
- 一旦超时，context 会自动取消，任务函数会收到 `ctx.Err() == context.DeadlineExceeded`，可以及时退出，防止任务长时间阻塞或“僵尸”化。
- 这样即使有任务卡死，也不会影响整个池的健康，提升系统健壮性。

## 2 关键代码片段
```go
func (p *Pool) handleTask(ctx context.Context, task *Task) {
    if ctx == nil {
        ctx = context.Background()
    }
    if task.Timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, task.Timeout)
        defer cancel()
    }
    // ... 执行任务 ...
    result, err := task.TaskFunc(ctx)
    // ...
}
```

## 3 实际效果
- 每个任务都能独立设置超时，互不影响。
- 防止单个任务长时间阻塞，提升系统整体可用性。
- 业务只需在任务函数内监听 `ctx.Done()`，即可优雅处理超时和取消。

# 三、实时获取活跃 worker、排队任务、已完成任务数，便于监控与调优

## 1 实时获取池状态
worker_pool 提供了 `Stats()` 方法，可以实时获取当前池的活跃 worker 数、排队任务数、已完成任务数等关键指标，便于监控和分析。

**示例代码：**
```go
stats := p.Stats()
fmt.Printf("活跃worker: %d, 排队任务: %d, 已完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
```
- `ActiveWorkers`：当前正在执行任务的 worker 数。
- `QueuedTasks`：当前等待队列中的任务数。
- `Completed`：已完成的任务总数。

## 2 监控与调优建议
- **监控活跃 worker 数**：
  - 如果活跃 worker 长期接近 `MaxWorkers`，说明池已满载，可考虑提升 `MaxWorkers` 以增强并发能力。
  - 如果活跃 worker 明显小于 `MinWorkers`，可适当降低 `MinWorkers`，节省资源。
- **监控排队任务数**：
  - 排队任务数持续较高，说明任务处理能力不足，可适当增加 worker 数或优化任务执行逻辑。
  - 排队任务数长期为 0，说明资源充足，可适当减少 worker 数，降低资源消耗。
- **监控已完成任务数**：
  - 结合业务量，分析任务处理速率，发现异常波动及时排查。
- **动态调整池参数**：
  - 可通过 `SetMinWorkers`、`SetMaxWorkers` 方法动态调整池的最小/最大 worker 数，实现弹性伸缩。

**调优建议总结：**
- 定期采集和分析池的统计数据，结合业务高峰和低谷动态调整参数。
- 监控异常（如排队任务激增、worker 长期满载）时及时告警和扩容。
- 结合任务超时机制，防止单个任务拖垮整体。

---

通过实时监控和灵活调优，能够充分发挥 worker_pool 的高并发和高可用优势，保障系统稳定运行。

# 四、如何支持池的优雅关闭与重启，适合服务热升级、平滑重启场景？

## 1 优雅关闭池（Shutdown）
worker_pool 提供了 `Shutdown()` 方法，可以实现池的优雅关闭：
- 调用 `Shutdown()` 后，池会停止接收新任务，但会等待所有已提交的任务执行完成后再退出。
- 适用于服务下线、热升级等需要平滑停止任务处理的场景，确保任务不丢失、不中断。

**示例代码：**
```go
p := NewPool(2, WithMaxWorkers(4))
// ... 提交任务 ...
p.Shutdown() // 等待所有任务完成后关闭
```

## 2 池的重启（Restart）
worker_pool 支持通过 `Restart()` 方法在关闭后重新启动池：
- 调用 `Shutdown()` 完成优雅关闭后，可通过 `Restart()` 重新初始化池，恢复任务处理能力。
- 适合服务热升级、配置变更、平滑重启等场景，无需重建对象即可恢复服务。

**示例代码：**
```go
p := NewPool(2, WithMaxWorkers(4))
// ... 提交任务 ...
p.Shutdown() // 优雅关闭
// ... 进行升级或配置变更 ...
p.Restart()  // 重新启动池
// 可继续提交新任务
```

## 3 典型应用场景
- **服务热升级**：先调用 `Shutdown()` 等待任务处理完毕，升级后用 `Restart()` 恢复服务，无需丢弃任务或重启进程。
- **平滑重启**：在不中断业务的前提下，动态关闭和重启池，保障任务安全和服务连续性。

---

通过优雅关闭与重启机制，worker_pool 能够很好地支持高可用服务的热升级与平滑重启需求。

> **注意：**
>
> worker_pool 的 `Shutdown()` 和 `Restart()` 仅作用于池本身，不会影响服务进程的生命周期。如果池是作为依赖注入到服务中的，服务本身不会因为池的重启而中断或重启。
>
> 这意味着：
> - 池的 shutdown/restart 适合池级别的维护、参数调整、任务暂停恢复等场景。
> - 若需实现真正的“服务热升级”或“平滑重启”，还需结合服务的流量切换、实例替换等机制。

# 五、panic 自动捕获与恢复机制详解

## 1 自动捕获 panic 的原理
worker_pool 在执行每个任务时，内部通过 defer + recover 机制自动捕获任务函数中的 panic，防止因单个任务异常导致整个 worker 或池崩溃。

核心实现片段：
```go
defer func() {
    if r := recover(); r != nil && task.Recovery != nil {
        task.Recovery(r)
    }
    // ... 其他清理逻辑 ...
}()
result, err := task.TaskFunc(ctx)
```
- 每个任务执行前注册 defer，捕获 panic。
- 如果任务发生 panic，recover 会捕获异常，并调用用户自定义的 Recovery 回调（如果有）。
- 这样即使任务 panic，worker 线程也不会退出，池整体依然健壮。

## 2 自定义 panic 恢复逻辑
用户可通过 `WithRecovery` Option 为每个任务单独指定 panic 恢复回调：

**示例代码：**
```go
p := NewPool(1, WithMaxWorkers(2))
_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    panic("panic in task")
}, WithRecovery(func(r interface{}) {
    fmt.Printf("recovered: %v\n", r)
}))
```
- 当任务 panic 时，`WithRecovery` 指定的回调会被调用，可用于日志记录、报警、统计等。
- 如果未指定 Recovery 回调，panic 只会被 recover 捕获，不会向外传播。

## 3 实际意义与优势
- **防止池崩溃**：即使任务代码有 bug 或第三方库异常，也不会影响整个池的稳定性。
- **灵活处理异常**：可为不同任务定制不同的异常处理逻辑，便于定位和追踪问题。
- **提升健壮性**：结合日志、报警等手段，能及时发现和响应异常，保障服务高可用。

## 4 典型应用场景
- 任务中调用第三方库、外部接口等不可靠代码时，防止因 panic 影响主流程。
- 需要对异常进行埋点、报警、统计分析时。

---

通过自动 panic 捕获与可定制恢复机制，worker_pool 能够极大提升并发任务调度的健壮性和可维护性。
