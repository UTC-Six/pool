package worker_pool

import (
	"context"
	"testing"
	"time"
)

// 示例1：普通任务提交
func TestPool_Submit(t *testing.T) {
	p := NewPool(2, WithMaxWorkers(5))
	defer p.Shutdown()

	err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("hello pool")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// 示例2：带超时的任务
func TestPool_Timeout(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(3))
	defer p.Shutdown()

	err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(2 * time.Second):
			return "done", nil
		case <-ctx.Done():
			t.Logf("err=%v", ctx.Err())
			return nil, ctx.Err()
		}
	}, WithTimeout(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
}

// 示例3：高优先级任务
func TestPool_Priority(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	defer p.Shutdown()

	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("low priority")
		return nil, nil
	}, WithPriority(PriorityLow))
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("high priority")
		return nil, nil
	}, WithPriority(PriorityHigh))
	// 高优先级任务会先执行
	// 可通过日志顺序观察
}

// 示例4：带返回值的任务
func TestPool_WithResult(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	defer p.Shutdown()

	ch, err := p.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "123", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	res := <-ch
	if res.Result != "123" || res.Err != nil {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// 示例5：带 recovery 的任务
func TestPool_Recovery(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	defer p.Shutdown()

	recovered := false
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		panic("panic in task")
	}, WithRecovery(func(r interface{}) {
		recovered = true
		t.Logf("recovered: %v", r)
	}))
	time.Sleep(100 * time.Millisecond)
	if !recovered {
		t.Error("recovery not triggered")
	}
}

// 示例6：获取统计信息
func TestPool_Stats(t *testing.T) {
	p := NewPool(2, WithMaxWorkers(4))
	defer p.Shutdown()

	for i := 0; i < 5; i++ {
		_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, nil
		})
	}
	time.Sleep(20 * time.Millisecond)
	stats := p.Stats()
	t.Logf("活跃: %d, 排队: %d, 完成: %d", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}

// 示例7：优雅关闭池
func TestPool_Shutdown(t *testing.T) {
	p := NewPool(2, WithMaxWorkers(4))
	for i := 0; i < 3; i++ {
		_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(30 * time.Millisecond)
			return nil, nil
		})
	}
	p.Shutdown() // 等待所有任务完成
	t.Log("pool shutdown gracefully")
}

// 示例8：动态调整最小/最大worker
func TestPool_SetMinMaxWorkers(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	defer p.Shutdown()

	p.SetMinWorkers(3)
	if p.minWorkers != 3 {
		t.Errorf("expected minWorkers=3, got %d", p.minWorkers)
	}
	p.SetMaxWorkers(5)
	if p.maxWorkers != 5 {
		t.Errorf("expected maxWorkers=5, got %d", p.maxWorkers)
	}
}

// 示例9：池重启
func TestPool_Restart(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("before shutdown")
		return nil, nil
	})
	p.Shutdown()
	p.Restart()
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("after restart")
		return nil, nil
	})
	p.Shutdown()
}

// 示例10：批量任务带标签和日志
// 推荐用 Option 风格批量提交任务：循环调用 Submit/SubmitWithResult
func TestPool_BatchWithTagLog(t *testing.T) {
	p := NewPool(2, WithMaxWorkers(4), WithName("batch-logger"), WithLogger(func(format string, args ...interface{}) {
		t.Logf("[POOL] "+format, args...)
	}))
	defer p.Shutdown()

	tasks := []struct {
		TaskFunc func(ctx context.Context) (interface{}, error)
		Tag      string
	}{
		{func(ctx context.Context) (interface{}, error) { t.Log("task 1 running"); return "A", nil }, "A"},
		{func(ctx context.Context) (interface{}, error) { t.Log("task 2 running"); return "B", nil }, "B"},
	}
	for _, task := range tasks {
		_ = p.Submit(context.Background(), task.TaskFunc, WithTag(task.Tag), WithLog(func(format string, args ...interface{}) { t.Logf("[TASK] "+format, args...) }))
	}
	time.Sleep(100 * time.Millisecond)
}

// 示例11：任务前后钩子
// 演示 WithBefore/WithAfter 可在任务执行前后自动执行自定义逻辑，常用于埋点、监控等场景。
func TestPool_TaskHooks(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	defer p.Shutdown()

	beforeCalled, afterCalled := false, false
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		t.Log("main task running")
		return nil, nil
	}, WithBefore(func() { beforeCalled = true; t.Log("before hook") }), WithAfter(func() { afterCalled = true; t.Log("after hook") }))
	time.Sleep(50 * time.Millisecond)
	if !beforeCalled || !afterCalled {
		t.Errorf("hooks not called: before=%v, after=%v", beforeCalled, afterCalled)
	}
}

// 示例12：动态调整最小/最大worker
// 演示运行时动态调整池的 min/max worker，适合弹性伸缩场景。
func TestPool_DynamicResize(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2), WithName("dyn"))
	defer p.Shutdown()

	p.SetMinWorkers(3) // 动态扩容
	if p.minWorkers != 3 {
		t.Errorf("expected minWorkers=3, got %d", p.minWorkers)
	}
	p.SetMaxWorkers(5) // 动态调整最大
	if p.maxWorkers != 5 {
		t.Errorf("expected maxWorkers=5, got %d", p.maxWorkers)
	}
}

// 边界条件测试：池关闭后提交任务
func TestPool_SubmitAfterShutdown(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(2))
	p.Shutdown()
	err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, nil
	})
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

// Table-driven 测试：不同优先级任务执行顺序
func TestPool_PriorityOrder(t *testing.T) {
	p := NewPool(1, WithMaxWorkers(1))
	defer p.Shutdown()

	tasks := []struct {
		priority int
		msg      string
	}{
		{PriorityLow, "low"},
		{PriorityHigh, "high"},
		{PriorityNormal, "normal"},
	}

	results := make(chan string, len(tasks))
	for _, tc := range tasks {
		tc := tc
		_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			results <- tc.msg
			return nil, nil
		}, WithPriority(tc.priority))
	}

	time.Sleep(100 * time.Millisecond)
	close(results)
	var got []string
	for r := range results {
		got = append(got, r)
	}
	// 由于优先级，high 应先于 normal，再于 low
	// 这里只能保证 high 在最前，low 在最后
	if got[0] != "high" || got[len(got)-1] != "low" {
		t.Errorf("priority order not as expected: %v", got)
	}
}
