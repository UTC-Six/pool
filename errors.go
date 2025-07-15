package pool

// ErrPoolClosed 表示池已关闭
var ErrPoolClosed = &PoolError{"pool closed"}

type PoolError struct {
	msg string
}

func (e *PoolError) Error() string { return e.msg }
