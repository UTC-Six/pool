package worker_pool

import (
	"context"
	"time"
)

// TaskPriority 定义任务优先级，数值越大优先级越高
// Task 表示一个池中要执行的任务，支持优先级、超时、recovery、日志、标签、钩子等扩展
// 字段说明：
// - Index: 用于优先队列（heap）定位任务位置，支持高效的优先级调度
// - LogFn: 任务级日志函数，记录任务执行细节
// - Tag: 任务标签，便于日志、监控、调试区分任务类型
// - Before/After: 任务前后钩子，支持埋点、监控等扩展
// - 其余字段见主流程注释
// ctx: 推荐作为所有并发/超时/取消相关函数的第一个参数，便于统一管理生命周期
type Task struct {
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

// TaskResult 用于封装任务执行结果
type TaskResult struct {
	Result interface{}
	Err    error
}

// taskPriorityQueue 实现 heap.Interface，用于任务优先级调度
type taskPriorityQueue []*Task

func (pq taskPriorityQueue) Len() int           { return len(pq) }
func (pq taskPriorityQueue) Less(i, j int) bool { return pq[i].Priority > pq[j].Priority }
func (pq taskPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}
func (pq *taskPriorityQueue) Push(x interface{}) {
	t := x.(*Task)
	t.Index = len(*pq)
	*pq = append(*pq, t)
}
func (pq *taskPriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	t := old[n-1]
	t.Index = -1
	*pq = old[0 : n-1]
	return t
}
