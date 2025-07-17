# pool 协程池

## 项目简介

`pool` 是一个高性能、可扩展的 Go 协程池，支持任务优先级、自动扩容、任务超时、父 context 复用、panic recovery、统计信息等特性，适用于高并发场景下对 goroutine 管理有严格要求的业务。

---

## 主要特性
- **最大/最小 worker 数可控**，自动扩容
- **任务优先级调度**，高优先级任务优先执行
- **任务超时自动取消**，支持 context 继承
- **panic recovery**，防止异常导致池崩溃
- **协程安全**，多 goroutine 并发安全
- **支持带返回值任务**
- **实时统计信息**，活跃 worker、排队任务、已完成任务数
- **优雅关闭**，等待所有任务完成

---

## 安装与引入

假设你的 pool 包托管在 `github.com/yourorg/pool`，其它项目可这样引入：

```sh
go get github.com/yourorg/pool
```

在代码中 import：

```go
import "github.com/yourorg/pool"
```

---

## 其它项目中最佳实践示例

### main.go 示例
```go
package main

import (
    "context"
    "fmt"
    "time"

  "github.com/UTC-Six/pool/worker_pool"
)

func main() {
    // 创建协程池
    p := worker_pool.NewPool(2, worker_pool.WithMaxWorkers(10))
    defer p.Shutdown()

    // 提交普通任务
    err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
        fmt.Println("hello from pool")
        return nil, nil
    })
    if err != nil {
        panic(err)
    }

    // 提交带返回值任务
    resultCh, _ := p.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
        return "result from pool", nil
    }, worker_pool.WithPriority(worker_pool.PriorityHigh))
    res := <-resultCh
    fmt.Println("result:", res.Result, "err:", res.Err)

    // 获取统计信息
    stats := p.Stats()
    fmt.Printf("活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}
```

### go.mod 示例
```
module yourapp

go 1.20

require github.com/yourorg/pool latest
```

---

## 结构与主要接口说明

### Pool 结构体

```
type Pool struct {
    // ...
}
```
- `NewPool(minWorkers int, opts ...PoolOption) *Pool`  
  创建协程池，指定最小和最大 worker 数。
- `Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error`  
  提交任务，支持优先级、超时、recovery。
- `SubmitWithResult(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) (<-chan TaskResult, error)`  
  提交带返回值任务。
- `Stats() PoolStats`  
  获取池的统计信息。
- `Shutdown()`  
  关闭池，等待所有任务完成。

### Task 结构体

```
type Task struct {
    Ctx        context.Context
    Priority   int
    Timeout    time.Duration
    TaskFunc   func(ctx context.Context) (interface{}, error)
    Recovery   func(interface{})
    ResultChan chan TaskResult
    Index      int
    LogFn      func(format string, args ...interface{})
    Tag        string
    Before     func()
    After      func()
}
```

### PoolStats 结构体

```
type PoolStats struct {
    ActiveWorkers int // 当前活跃 worker 数
    QueuedTasks   int // 排队任务数
    Completed     int // 已完成任务数
}
```

### 错误
- `ErrPoolClosed`：池已关闭时提交任务会返回该错误

---

## 典型用法示例

### 1. 创建协程池
```go
p := worker_pool.NewPool(2, worker_pool.WithMaxWorkers(10)) // 最小2，最大10个worker
```

### 2. 提交普通任务
```go
err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    // 你的任务逻辑
    fmt.Println("hello pool")
    return nil, nil
})
```

### 3. 提交带超时的任务
```go
err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    select {
    case <-time.After(3 * time.Second):
        fmt.Println("done")
    case <-ctx.Done():
        fmt.Println("timeout or cancelled")
    }
    return nil, nil
}, worker_pool.WithTimeout(2*time.Second))
```

### 4. 提交高优先级任务
```go
_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    fmt.Println("high priority task")
    return nil, nil
}, worker_pool.WithPriority(worker_pool.PriorityHigh))
```

### 5. 提交带返回值的任务
```go
resultCh, _ := p.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
    return 42, nil
})
res := <-resultCh
fmt.Println(res.Result, res.Err)
```

### 6. 提交带 recovery 的任务
```go
_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
    panic("something wrong")
}, worker_pool.WithRecovery(func(r interface{}) {
    fmt.Println("recovered from:", r)
}))
```

### 7. 获取统计信息
```go
stats := p.Stats()
fmt.Printf("活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
```

### 8. 优雅关闭池
```go
p.Shutdown() // 等待所有任务完成
```

---

## threading.GoSafe 用法示例

### 1. 设置最大并发 goroutine 数
```go
import "yourmodule/threading"

threading.SetMaxGoroutines(10) // 最多允许10个 goroutine 并发
```

### 2. 启动受控 goroutine，支持 context、错误返回
```go
import (
    "context"
    "fmt"
    "time"
    "yourmodule/threading"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    err := threading.GoSafe(ctx, func(ctx context.Context) error {
        select {
        case <-time.After(3 * time.Second):
            fmt.Println("done")
        case <-ctx.Done():
            fmt.Println("timeout or cancelled")
            return ctx.Err()
        }
        return nil
    })
    if err != nil {
        fmt.Println("GoSafe error:", err)
    }
}
```

### 3. panic 自动捕获
```go
err := threading.GoSafe(context.Background(), func(ctx context.Context) error {
    panic("something wrong")
})
if err != nil {
    fmt.Println("panic recovered:", err)
}
```

---

## 注意事项
- 池关闭后再提交任务会返回 `ErrPoolClosed`
- 任务函数内如需感知超时/取消，请监听 `ctx.Done()`
- 自动扩容策略可根据业务需求调整
- 任务优先级为整数，数值越大优先级越高

---

## 贡献与反馈
如有建议、bug 或需求，欢迎 issue 或 PR。 