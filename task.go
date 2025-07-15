package pool

import (
	"context"
	"time"
)

// TaskPriority 定义任务优先级，数值越大优先级越高
const (
	PriorityLow    = 1
	PriorityNormal = 5
	PriorityHigh   = 10
)

// Task 表示一个池中要执行的任务
// 支持 context、优先级、超时、recovery
type Task struct {
	ctx        context.Context
	priority   int
	timeout    time.Duration
	taskFunc   func(ctx context.Context) (interface{}, error)
	recovery   func(interface{})
	resultChan chan taskResult
	index      int
}

// taskResult 用于封装任务执行结果
type taskResult struct {
	result interface{}
	err    error
}

// taskPriorityQueue 实现 heap.Interface，用于任务优先级调度
type taskPriorityQueue []*Task

func (pq taskPriorityQueue) Len() int           { return len(pq) }
func (pq taskPriorityQueue) Less(i, j int) bool { return pq[i].priority > pq[j].priority }
func (pq taskPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *taskPriorityQueue) Push(x interface{}) {
	t := x.(*Task)
	t.index = len(*pq)
	*pq = append(*pq, t)
}
func (pq *taskPriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	t := old[n-1]
	t.index = -1
	*pq = old[0 : n-1]
	return t
}
