package pool

import (
	"context"
	"testing"
	"time"
)

// 示例1：普通任务提交
func TestPool_Submit(t *testing.T) {
	p := NewPool(2, 5)
	defer p.Shutdown()

	err := p.Submit(context.Background(), PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
		t.Log("hello pool")
		return nil, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// 示例2：带超时的任务
func TestPool_Timeout(t *testing.T) {
	p := NewPool(1, 3)
	defer p.Shutdown()

	err := p.Submit(context.Background(), PriorityNormal, 500*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(2 * time.Second):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// 示例3：高优先级任务
func TestPool_Priority(t *testing.T) {
	p := NewPool(1, 2)
	defer p.Shutdown()

	_ = p.Submit(context.Background(), PriorityLow, 0, func(ctx context.Context) (interface{}, error) {
		t.Log("low priority")
		return nil, nil
	}, nil)
	_ = p.Submit(context.Background(), PriorityHigh, 0, func(ctx context.Context) (interface{}, error) {
		t.Log("high priority")
		return nil, nil
	}, nil)
	// 高优先级任务会先执行
	// 可通过日志顺序观察
}

// 示例4：带返回值的任务
func TestPool_WithResult(t *testing.T) {
	p := NewPool(1, 2)
	defer p.Shutdown()

	ch, err := p.SubmitWithResult(context.Background(), PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
		return 123, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := <-ch
	if res.result != 123 || res.err != nil {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// 示例5：带 recovery 的任务
func TestPool_Recovery(t *testing.T) {
	p := NewPool(1, 2)
	defer p.Shutdown()

	recovered := false
	_ = p.Submit(context.Background(), PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
		panic("panic in task")
	}, func(r interface{}) {
		recovered = true
		t.Logf("recovered: %v", r)
	})
	time.Sleep(100 * time.Millisecond)
	if !recovered {
		t.Error("recovery not triggered")
	}
}

// 示例6：获取统计信息
func TestPool_Stats(t *testing.T) {
	p := NewPool(2, 4)
	defer p.Shutdown()

	for i := 0; i < 5; i++ {
		_ = p.Submit(context.Background(), PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, nil
		}, nil)
	}
	time.Sleep(20 * time.Millisecond)
	stats := p.Stats()
	t.Logf("活跃: %d, 排队: %d, 完成: %d", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)
}

// 示例7：优雅关闭池
func TestPool_Shutdown(t *testing.T) {
	p := NewPool(2, 4)
	for i := 0; i < 3; i++ {
		_ = p.Submit(context.Background(), PriorityNormal, 0, func(ctx context.Context) (interface{}, error) {
			time.Sleep(30 * time.Millisecond)
			return nil, nil
		}, nil)
	}
	p.Shutdown() // 等待所有任务完成
	t.Log("pool shutdown gracefully")
}
