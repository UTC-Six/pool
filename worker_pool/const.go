package worker_pool

import "time"

const (
	// Task 优先级
	PriorityLow    = 1
	PriorityNormal = 5
	PriorityHigh   = 10

	// 默认任务超时时间
	DefaultTaskTimeout = 3 * time.Second // 默认3秒，业务可覆盖
)
