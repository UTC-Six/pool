package worker_pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBasicFunctionality 基础功能测试
func TestBasicFunctionality(t *testing.T) {
	t.Run("Pool Creation and Shutdown", func(t *testing.T) {
		pool := NewPool(5, WithMaxWorkers(10), WithName("test-pool"))
		if pool == nil {
			t.Fatal("Failed to create pool")
		}

		stats := pool.EnhancedStats()
		if stats.MinWorkers != 5 {
			t.Errorf("Expected MinWorkers=5, got %d", stats.MinWorkers)
		}
		if stats.MaxWorkers != 10 {
			t.Errorf("Expected MaxWorkers=10, got %d", stats.MaxWorkers)
		}
		if stats.CoreWorkers != 5 {
			t.Errorf("Expected CoreWorkers=5, got %d", stats.CoreWorkers)
		}

		pool.Shutdown()
	})

	t.Run("Task Submission and Execution", func(t *testing.T) {
		pool := NewPool(3, WithMaxWorkers(5))
		defer pool.Shutdown()

		var counter int64
		var wg sync.WaitGroup

		// 提交10个任务
		for i := 0; i < 10; i++ {
			wg.Add(1)
			err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				atomic.AddInt64(&counter, 1)
				wg.Done()
				return nil, nil
			})
			if err != nil {
				t.Errorf("Failed to submit task: %v", err)
			}
		}

		wg.Wait()

		if atomic.LoadInt64(&counter) != 10 {
			t.Errorf("Expected 10 tasks executed, got %d", counter)
		}

		stats := pool.EnhancedStats()
		if stats.TaskSubmitCount != 10 {
			t.Errorf("Expected TaskSubmitCount=10, got %d", stats.TaskSubmitCount)
		}
	})
}

// TestSubmitWithResult 测试带返回值的任务提交
func TestSubmitWithResult(t *testing.T) {
	pool := NewPool(2, WithMaxWorkers(4))
	defer pool.Shutdown()

	// 测试正常返回值
	resultCh, err := pool.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "test-result", nil
	})
	if err != nil {
		t.Fatalf("Failed to submit task with result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.Result != "test-result" {
			t.Errorf("Expected result='test-result', got %v", result.Result)
		}
		if result.Err != nil {
			t.Errorf("Expected no error, got %v", result.Err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Task execution timeout")
	}

	// 测试错误返回
	resultCh, err = pool.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("test error")
	})
	if err != nil {
		t.Fatalf("Failed to submit task with result: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.Err == nil {
			t.Error("Expected error, got nil")
		}
		if result.Err.Error() != "test error" {
			t.Errorf("Expected error='test error', got %v", result.Err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Task execution timeout")
	}
}

// TestConcurrentSubmission 并发提交测试
func TestConcurrentSubmission(t *testing.T) {
	pool := NewPool(5, WithMaxWorkers(20))
	defer pool.Shutdown()

	var counter int64
	var wg sync.WaitGroup
	numGoroutines := 10
	tasksPerGoroutine := 100

	// 启动多个goroutine并发提交任务
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
					atomic.AddInt64(&counter, 1)
					time.Sleep(1 * time.Millisecond) // 模拟工作
					return nil, nil
				})
				if err != nil {
					t.Errorf("Failed to submit task: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(2 * time.Second)

	expectedTasks := int64(numGoroutines * tasksPerGoroutine)
	if atomic.LoadInt64(&counter) != expectedTasks {
		t.Errorf("Expected %d tasks executed, got %d", expectedTasks, counter)
	}

	stats := pool.EnhancedStats()
	if stats.TaskSubmitCount != expectedTasks {
		t.Errorf("Expected TaskSubmitCount=%d, got %d", expectedTasks, stats.TaskSubmitCount)
	}
}

// TestContextCancellation 上下文取消测试
func TestContextCancellation(t *testing.T) {
	pool := NewPool(2, WithMaxWorkers(4))
	defer pool.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var taskStarted, taskCompleted int64

	err := pool.Submit(ctx, func(ctx context.Context) (interface{}, error) {
		atomic.AddInt64(&taskStarted, 1)
		select {
		case <-time.After(200 * time.Millisecond): // 任务需要200ms，但context只给100ms
			atomic.AddInt64(&taskCompleted, 1)
			return "completed", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	if err != nil {
		t.Errorf("Failed to submit task: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if atomic.LoadInt64(&taskStarted) != 1 {
		t.Errorf("Expected 1 task started, got %d", taskStarted)
	}

	// 任务应该被context取消，而不是完成
	if atomic.LoadInt64(&taskCompleted) != 0 {
		t.Errorf("Expected 0 tasks completed (should be cancelled), got %d", taskCompleted)
	}
}

// TestPoolShutdown 池关闭测试
func TestPoolShutdown(t *testing.T) {
	pool := NewPool(3, WithMaxWorkers(5))

	var tasksCompleted int64

	// 提交一些任务
	for i := 0; i < 5; i++ {
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt64(&tasksCompleted, 1)
			return nil, nil
		})
	}

	// 关闭池
	pool.Shutdown()

	// 关闭后不应该能提交新任务
	err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, nil
	})

	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed after shutdown, got %v", err)
	}

	// 所有任务都应该完成
	if atomic.LoadInt64(&tasksCompleted) != 5 {
		t.Errorf("Expected 5 tasks completed, got %d", tasksCompleted)
	}
}

// TestCoreWorkersAdjustment 核心worker调整测试
func TestCoreWorkersAdjustment(t *testing.T) {
	pool := NewPool(10, WithMaxWorkers(20), WithFixedCoreWorkers(5))
	defer pool.Shutdown()

	// 测试固定策略
	stats := pool.EnhancedStats()
	if stats.CoreWorkers != 5 {
		t.Errorf("Expected CoreWorkers=5, got %d", stats.CoreWorkers)
	}
	if stats.CoreAdjustStrategy != "Fixed" {
		t.Errorf("Expected strategy=Fixed, got %s", stats.CoreAdjustStrategy)
	}

	// 测试手动调整
	pool.SetCoreWorkers(8)
	if pool.GetCoreWorkers() != 8 {
		t.Errorf("Expected CoreWorkers=8 after manual set, got %d", pool.GetCoreWorkers())
	}

	// 测试边界值
	pool.SetCoreWorkers(0) // 应该被调整为1
	if pool.GetCoreWorkers() != 1 {
		t.Errorf("Expected CoreWorkers=1 (minimum), got %d", pool.GetCoreWorkers())
	}

	pool.SetCoreWorkers(25) // 应该被调整为maxWorkers(20)
	if pool.GetCoreWorkers() != 20 {
		t.Errorf("Expected CoreWorkers=20 (maximum), got %d", pool.GetCoreWorkers())
	}
}

// TestLoadMonitoring 负载监控测试
func TestLoadMonitoring(t *testing.T) {
	pool := NewPool(5,
		WithMaxWorkers(10),
		WithAdjustCheckInterval(100*time.Millisecond), // 快速检查用于测试
		WithCoreAdjustStrategy(StrategyPercentage),
	)
	defer pool.Shutdown()

	// 等待几个监控周期
	time.Sleep(500 * time.Millisecond)

	stats := pool.EnhancedStats()
	if stats.LoadHistoryLength == 0 {
		t.Error("Expected load history to be collected")
	}

	history := pool.GetLoadHistory()
	if len(history) == 0 {
		t.Error("Expected non-empty load history")
	}

	// 验证历史记录的时间戳是递增的
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Error("Load history timestamps should be in ascending order")
		}
	}
}

// TestTaskOptions 任务选项测试
func TestTaskOptions(t *testing.T) {
	pool := NewPool(2, WithMaxWorkers(4))
	defer pool.Shutdown()

	var beforeCalled, afterCalled bool
	var recoveredPanic interface{}

	// 测试各种选项
	err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		panic("test panic")
	},
		WithBefore(func() { beforeCalled = true }),
		WithAfter(func() { afterCalled = true }),
		WithRecovery(func(r interface{}) { recoveredPanic = r }),
		WithTag("test-task"),
		WithTimeout(1*time.Second),
	)

	if err != nil {
		t.Errorf("Failed to submit task with options: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !beforeCalled {
		t.Error("Before hook was not called")
	}
	if !afterCalled {
		t.Error("After hook was not called")
	}
	if recoveredPanic != "test panic" {
		t.Errorf("Expected panic 'test panic', got %v", recoveredPanic)
	}
}

// TestHighLoad 高负载测试
func TestHighLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	pool := NewPool(10, WithMaxWorkers(50))
	defer pool.Shutdown()

	numTasks := 1000
	var completed int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			defer wg.Done()
			// 模拟一些工作
			time.Sleep(time.Microsecond * 100)
			atomic.AddInt64(&completed, 1)
			return nil, nil
		})
		if err != nil {
			t.Errorf("Failed to submit task %d: %v", i, err)
		}
	}

	wg.Wait()
	duration := time.Since(start)

	if atomic.LoadInt64(&completed) != int64(numTasks) {
		t.Errorf("Expected %d tasks completed, got %d", numTasks, completed)
	}

	stats := pool.EnhancedStats()
	t.Logf("High load test completed: %d tasks in %v", numTasks, duration)
	t.Logf("Final stats: Active=%d, Completed=%d, Total=%d",
		stats.ActiveWorkers, stats.Completed, stats.TaskSubmitCount)

	// 验证性能指标
	tasksPerSecond := float64(numTasks) / duration.Seconds()
	if tasksPerSecond < 1000 { // 期望至少1000 TPS
		t.Errorf("Performance too low: %.2f TPS, expected > 1000 TPS", tasksPerSecond)
	}

	t.Logf("Performance: %.2f TPS", tasksPerSecond)
}

// TestMemoryLeak 内存泄露测试
func TestMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 创建和销毁多个池
	for i := 0; i < 100; i++ {
		pool := NewPool(5, WithMaxWorkers(10))

		// 提交一些任务
		for j := 0; j < 10; j++ {
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(time.Millisecond)
				return nil, nil
			})
		}

		time.Sleep(10 * time.Millisecond)
		pool.Shutdown()
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	memoryIncrease := m2.Alloc - m1.Alloc
	t.Logf("Memory increase: %d bytes", memoryIncrease)

	// 内存增长不应该太大（允许一些合理的增长）
	if memoryIncrease > 10*1024*1024 { // 10MB
		t.Errorf("Potential memory leak detected: %d bytes increase", memoryIncrease)
	}
}

// TestEdgeCases 边界情况测试
func TestEdgeCases(t *testing.T) {
	t.Run("Zero MinWorkers", func(t *testing.T) {
		pool := NewPool(0) // 应该被调整为1
		defer pool.Shutdown()

		stats := pool.EnhancedStats()
		if stats.MinWorkers != 1 {
			t.Errorf("Expected MinWorkers=1, got %d", stats.MinWorkers)
		}
	})

	t.Run("Negative MinWorkers", func(t *testing.T) {
		pool := NewPool(-5) // 应该被调整为1
		defer pool.Shutdown()

		stats := pool.EnhancedStats()
		if stats.MinWorkers != 1 {
			t.Errorf("Expected MinWorkers=1, got %d", stats.MinWorkers)
		}
	})

	t.Run("MaxWorkers Less Than MinWorkers", func(t *testing.T) {
		pool := NewPool(10, WithMaxWorkers(5)) // MaxWorkers应该被调整为10
		defer pool.Shutdown()

		stats := pool.EnhancedStats()
		if stats.MaxWorkers != 10 {
			t.Errorf("Expected MaxWorkers=10, got %d", stats.MaxWorkers)
		}
	})

	t.Run("Nil Task Function", func(t *testing.T) {
		pool := NewPool(2)
		defer pool.Shutdown()

		// 应该返回错误而不是panic
		err := pool.Submit(context.Background(), nil)
		if err == nil {
			t.Error("Expected error when submitting nil task function")
		}
		if err.Error() != "taskFunc cannot be nil" {
			t.Errorf("Expected 'taskFunc cannot be nil', got %v", err)
		}

		// 测试SubmitWithResult
		_, err = pool.SubmitWithResult(context.Background(), nil)
		if err == nil {
			t.Error("Expected error when submitting nil task function to SubmitWithResult")
		}
	})
}

// TestRaceConditions 竞态条件测试
func TestRaceConditions(t *testing.T) {
	pool := NewPool(5, WithMaxWorkers(20))
	defer pool.Shutdown()

	var wg sync.WaitGroup
	numGoroutines := 50

	// 并发执行多种操作
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			switch id % 4 {
			case 0:
				// 提交任务
				pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
					time.Sleep(time.Millisecond)
					return nil, nil
				})
			case 1:
				// 获取统计信息
				pool.EnhancedStats()
			case 2:
				// 调整CoreWorkers
				pool.SetCoreWorkers(5 + (id % 10))
			case 3:
				// 获取负载历史
				pool.GetLoadHistory()
			}
		}(i)
	}

	wg.Wait()

	// 如果没有panic，说明没有明显的竞态条件
	t.Log("Race condition test completed without panic")
}

// BenchmarkTaskSubmission 任务提交性能基准测试
func BenchmarkTaskSubmission(b *testing.B) {
	pool := NewPool(10, WithMaxWorkers(50))
	defer pool.Shutdown()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				// 空任务，测试提交开销
				return nil, nil
			})
		}
	})
}

// BenchmarkTaskExecution 任务执行性能基准测试
func BenchmarkTaskExecution(b *testing.B) {
	pool := NewPool(runtime.NumCPU(), WithMaxWorkers(runtime.NumCPU()*2))
	defer pool.Shutdown()

	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			defer wg.Done()
			// 模拟CPU密集型工作
			sum := 0
			for j := 0; j < 1000; j++ {
				sum += j
			}
			return sum, nil
		})
	}
	wg.Wait()
}
