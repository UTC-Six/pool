package worker_pool

import (
	"context"
)

// Pooler 定义协程池的主要接口，便于扩展和 mock
// 推荐使用 Option 风格传递可选参数，接口更简洁灵活。
type Pooler interface {
	Submit(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) error
	SubmitWithResult(ctx context.Context, taskFunc func(ctx context.Context) (interface{}, error), opts ...TaskOption) (<-chan TaskResult, error)
	Stats() PoolStats
	Shutdown()
	// SetMinWorkers/SetMaxWorkers 说明：
	// - 推荐用 Option（WithMinWorkers/WithMaxWorkers）在创建池时设定 min/max worker，接口更简洁。
	// - 若需运行时动态调整（如弹性伸缩），可保留 SetMinWorkers/SetMaxWorkers。
	// - 一般业务可移除，仅用 Option；需动态调整则保留。
	SetMinWorkers(n int)
	SetMaxWorkers(n int)
	Restart() error
}
