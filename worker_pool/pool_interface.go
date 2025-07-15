package worker_pool

import (
	"context"
	"time"
)

// Pooler 定义协程池的主要接口，便于扩展和 mock
type Pooler interface {
	Submit(ctx context.Context, priority int, timeout time.Duration, taskFunc func(ctx context.Context) (interface{}, error), recovery func(interface{})) error
	SubmitWithResult(ctx context.Context, priority int, timeout time.Duration, taskFunc func(ctx context.Context) (interface{}, error), recovery func(interface{})) (<-chan TaskResult, error)
	Stats() PoolStats
	Shutdown()
	SetMinWorkers(n int)
	SetMaxWorkers(n int)
	Restart() error
	BatchSubmit(ctx context.Context, tasks []Task) []error
}
