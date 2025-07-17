package worker_pool

import (
	"context"
)

// Pooler 定义协程池的主要接口，便于扩展和 mock
type Pooler interface {
	Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error
	SubmitWithResult(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) (<-chan TaskResult, error)
	Stats() PoolStats
	Shutdown()
	SetMinWorkers(n int)
	SetMaxWorkers(n int)
	Restart() error
}
