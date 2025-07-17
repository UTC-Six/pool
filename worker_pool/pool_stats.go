package worker_pool

// PoolStats 用于统计池的运行状态
type PoolStats struct {
	ActiveWorkers int // 当前活跃 worker 数
	QueuedTasks   int // 排队任务数
	Completed     int // 已完成任务数
	MinWorkers    int // 最小 worker 数
	MaxWorkers    int // 最大 worker 数
}
