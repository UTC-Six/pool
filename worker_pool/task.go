package worker_pool

import (
	"context"
	"time"
)

// Task 表示一个池中要执行的任务，支持优先级、超时、recovery、日志、标签、钩子等扩展
type Task struct {
	Ctx        context.Context // 新增：保存业务层传入的 ctx
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

// TaskOption 用于配置 Task 的可选参数
type TaskOption func(*Task)

// WithTimeout 设置任务超时时间
func WithTimeout(timeout time.Duration) TaskOption {
	return func(t *Task) {
		t.Timeout = timeout
	}
}

// WithPriority 设置任务优先级
func WithPriority(priority int) TaskOption {
	return func(t *Task) {
		t.Priority = priority
	}
}

// WithRecovery 设置任务的 panic 恢复处理
func WithRecovery(recovery func(interface{})) TaskOption {
	return func(t *Task) {
		t.Recovery = recovery
	}
}

// WithLog 设置任务的日志函数
func WithLog(logFn func(format string, args ...interface{})) TaskOption {
	return func(t *Task) {
		t.LogFn = logFn
	}
}

// WithTag 设置任务标签
func WithTag(tag string) TaskOption {
	return func(t *Task) {
		t.Tag = tag
	}
}

// WithBefore 设置任务前置钩子
func WithBefore(before func()) TaskOption {
	return func(t *Task) {
		t.Before = before
	}
}

// WithAfter 设置任务后置钩子
func WithAfter(after func()) TaskOption {
	return func(t *Task) {
		t.After = after
	}
}
