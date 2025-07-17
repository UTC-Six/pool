package worker_pool

// ErrPoolClosed is returned when submitting a task to a closed pool.
var ErrPoolClosed = &PoolError{"pool closed"}

// PoolError represents an error specific to the worker pool.
type PoolError struct {
	msg string
}

// Error implements the error interface for PoolError.
func (e *PoolError) Error() string { return e.msg }
