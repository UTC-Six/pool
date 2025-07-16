# Worker Pool 使用说明

## 推荐用法

### 单任务提交
```go
p := NewPool(2, WithMaxWorkers(4))
_ = p.Submit(ctx, func(ctx context.Context) (interface{}, error) { ... }, WithTag("tag"), WithLog(...))
```

### 批量任务提交
```go
for _, task := range tasks {
    _ = p.Submit(ctx, task.Func, WithTag(task.Tag), WithLog(...))
}
```

### 说明
- 所有任务参数推荐用 Option 方式传递，灵活扩展。
- 不再推荐 BatchSubmit/BatchSubmitWithResult，直接循环调用 Submit/SubmitWithResult 即可。
- 详见 example_test.go。 