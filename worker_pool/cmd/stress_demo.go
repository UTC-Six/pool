package main

// 所有导入已注释，如需运行请取消注释main函数和相关导入
/*
import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)
*/

// 如需运行压力测试，请取消注释下面的main函数
/*
func main() {
	fmt.Println("🚀 Worker Pool 生产环境压力测试")
	fmt.Printf("系统信息: CPU核心数=%d, GOMAXPROCS=%d\n", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	// 运行所有测试
	runBasicStressTest()
	runHighConcurrencyTest()
	runLongRunningTest()
	runResourceLeakTest()
	runCoreWorkersAdjustmentTest()
	runErrorHandlingTest()
	runContextCancellationTest()

	fmt.Println("✅ 所有压力测试完成！")
}

// 基础压力测试
func runBasicStressTest() {
	fmt.Println("\n--- 基础压力测试 ---")

	pool := worker_pool.NewPool(20,
		worker_pool.WithMaxWorkers(100),
		worker_pool.WithName("stress-test-pool"),
		worker_pool.WithLogger(log.Printf),
	)
	defer pool.Shutdown()

	numTasks := 10000
	var completed int64
	var errors int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			defer wg.Done()

			// 模拟不同类型的工作负载
			workType := atomic.AddInt64(&completed, 1) % 3
			switch workType {
			case 0:
				// CPU密集型
				sum := 0
				for j := 0; j < 10000; j++ {
					sum += j
				}
				return sum, nil
			case 1:
				// IO密集型（模拟）
				time.Sleep(1 * time.Millisecond)
				return "io-result", nil
			case 2:
				// 混合型
				time.Sleep(500 * time.Microsecond)
				sum := 0
				for j := 0; j < 1000; j++ {
					sum += j
				}
				return sum, nil
			}
			return nil, nil
		})

		if err != nil {
			atomic.AddInt64(&errors, 1)
		}
	}

	wg.Wait()
	duration := time.Since(start)

	stats := pool.EnhancedStats()
	tps := float64(numTasks) / duration.Seconds()

	fmt.Printf("✅ 基础压力测试完成:\n")
	fmt.Printf("   - 任务数: %d\n", numTasks)
	fmt.Printf("   - 完成数: %d\n", completed)
	fmt.Printf("   - 错误数: %d\n", errors)
	fmt.Printf("   - 用时: %v\n", duration)
	fmt.Printf("   - TPS: %.2f\n", tps)
	fmt.Printf("   - 最终统计: Active=%d, Completed=%d, Total=%d\n",
		stats.ActiveWorkers, stats.Completed, stats.TaskSubmitCount)

	if errors > 0 {
		fmt.Printf("⚠️  发现 %d 个错误\n", errors)
	}
	if tps < 500 {
		fmt.Printf("⚠️  性能较低: %.2f TPS (期望 > 500)\n", tps)
	}
}

// 高并发测试
func runHighConcurrencyTest() {
	fmt.Println("\n--- 高并发测试 ---")

	pool := worker_pool.NewPool(10,
		worker_pool.WithMaxWorkers(200),
		worker_pool.WithName("concurrency-test-pool"),
	)
	defer pool.Shutdown()

	numGoroutines := 100
	tasksPerGoroutine := 100
	var totalCompleted int64
	var wg sync.WaitGroup

	start := time.Now()

	// 启动多个goroutine并发提交任务
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			var localWg sync.WaitGroup
			for j := 0; j < tasksPerGoroutine; j++ {
				localWg.Add(1)
				err := pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
					defer localWg.Done()
					atomic.AddInt64(&totalCompleted, 1)
					// 随机工作负载
					if goroutineID%2 == 0 {
						time.Sleep(100 * time.Microsecond)
					} else {
						sum := 0
						for k := 0; k < 1000; k++ {
							sum += k
						}
					}
					return nil, nil
				})

				if err != nil {
					fmt.Printf("❌ Goroutine %d 提交任务失败: %v\n", goroutineID, err)
					localWg.Done() // 如果提交失败，手动调用Done
				}
			}
			localWg.Wait() // 等待该goroutine的所有任务完成
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	expectedTasks := int64(numGoroutines * tasksPerGoroutine)
	stats := pool.EnhancedStats()

	fmt.Printf("✅ 高并发测试完成:\n")
	fmt.Printf("   - 并发数: %d goroutines\n", numGoroutines)
	fmt.Printf("   - 每个goroutine任务数: %d\n", tasksPerGoroutine)
	fmt.Printf("   - 预期任务数: %d\n", expectedTasks)
	fmt.Printf("   - 实际完成数: %d\n", totalCompleted)
	fmt.Printf("   - 用时: %v\n", duration)
	fmt.Printf("   - TPS: %.2f\n", float64(expectedTasks)/duration.Seconds())
	fmt.Printf("   - Worker扩容情况: Min=%d, Max=%d, 当前Active=%d\n",
		stats.MinWorkers, stats.MaxWorkers, stats.ActiveWorkers)

	if totalCompleted != expectedTasks {
		fmt.Printf("❌ 任务完成数不匹配: 期望 %d, 实际 %d\n", expectedTasks, totalCompleted)
	}
}

// 长时间运行测试
func runLongRunningTest() {
	fmt.Println("\n--- 长时间运行测试 ---")

	pool := worker_pool.NewPool(5,
		worker_pool.WithMaxWorkers(20),
		worker_pool.WithName("long-running-test"),
		worker_pool.WithAdjustCheckInterval(1*time.Second), // 快速调整用于测试
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage),
		worker_pool.WithLowLoadThreshold(0.3),
	)
	defer pool.Shutdown()

	var taskCounter int64
	stopCh := make(chan struct{})

	// 启动任务提交器
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
					atomic.AddInt64(&taskCounter, 1)
					time.Sleep(5 * time.Millisecond)
					return nil, nil
				})
			case <-stopCh:
				return
			}
		}
	}()

	// 启动统计监控
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := pool.EnhancedStats()
				fmt.Printf("   [监控] Tasks=%d, Active=%d, Core=%d, Queue=%d, History=%d\n",
					atomic.LoadInt64(&taskCounter), stats.ActiveWorkers, stats.CoreWorkers,
					stats.QueuedTasks, stats.LoadHistoryLength)
			case <-stopCh:
				return
			}
		}
	}()

	// 运行10秒
	fmt.Println("   运行10秒长时间测试...")
	time.Sleep(10 * time.Second)
	close(stopCh)

	// 等待剩余任务完成
	time.Sleep(1 * time.Second)

	finalStats := pool.EnhancedStats()
	fmt.Printf("✅ 长时间运行测试完成:\n")
	fmt.Printf("   - 总任务数: %d\n", atomic.LoadInt64(&taskCounter))
	fmt.Printf("   - 最终统计: Active=%d, Core=%d, Completed=%d\n",
		finalStats.ActiveWorkers, finalStats.CoreWorkers, finalStats.Completed)
	fmt.Printf("   - 负载历史长度: %d\n", finalStats.LoadHistoryLength)
}

// 资源泄露测试
func runResourceLeakTest() {
	fmt.Println("\n--- 资源泄露测试 ---")

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	initialGoroutines := runtime.NumGoroutine()

	// 创建和销毁多个池
	for i := 0; i < 50; i++ {
		pool := worker_pool.NewPool(3,
			worker_pool.WithMaxWorkers(10),
			worker_pool.WithAdjustCheckInterval(100*time.Millisecond),
		)

		// 提交一些任务
		for j := 0; j < 20; j++ {
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(1 * time.Millisecond)
				return nil, nil
			})
		}

		time.Sleep(50 * time.Millisecond)
		pool.Shutdown()
	}

	// 等待资源清理
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	runtime.ReadMemStats(&m2)

	finalGoroutines := runtime.NumGoroutine()
	var memoryIncrease int64
	if m2.Alloc >= m1.Alloc {
		memoryIncrease = int64(m2.Alloc - m1.Alloc)
	} else {
		memoryIncrease = -int64(m1.Alloc - m2.Alloc) // 内存实际减少了
	}
	goroutineIncrease := finalGoroutines - initialGoroutines

	fmt.Printf("✅ 资源泄露测试完成:\n")
	fmt.Printf("   - 初始Goroutines: %d\n", initialGoroutines)
	fmt.Printf("   - 最终Goroutines: %d\n", finalGoroutines)
	fmt.Printf("   - Goroutine增长: %d\n", goroutineIncrease)
	fmt.Printf("   - 内存变化: %d bytes (%.2f KB)\n", memoryIncrease, float64(memoryIncrease)/1024)

	if goroutineIncrease > 10 {
		fmt.Printf("⚠️  可能存在Goroutine泄露: 增长了 %d 个\n", goroutineIncrease)
	}
	if memoryIncrease > 5*1024*1024 { // 5MB
		fmt.Printf("⚠️  可能存在内存泄露: 增长了 %.2f MB\n", float64(memoryIncrease)/(1024*1024))
	}
}

// CoreWorkers调整测试
func runCoreWorkersAdjustmentTest() {
	fmt.Println("\n--- CoreWorkers调整测试 ---")

	// 测试不同策略
	strategies := []struct {
		name     string
		strategy worker_pool.CoreAdjustStrategy
		setup    func(*worker_pool.Pool)
	}{
		{
			name:     "固定策略",
			strategy: worker_pool.StrategyFixed,
			setup: func(p *worker_pool.Pool) {
				// 固定策略通过WithFixedCoreWorkers设置
			},
		},
		{
			name:     "百分比策略",
			strategy: worker_pool.StrategyPercentage,
			setup:    func(p *worker_pool.Pool) {},
		},
		{
			name:     "混合策略",
			strategy: worker_pool.StrategyHybrid,
			setup:    func(p *worker_pool.Pool) {},
		},
	}

	for _, s := range strategies {
		fmt.Printf("   测试 %s:\n", s.name)

		var pool *worker_pool.Pool
		if s.name == "固定策略" {
			pool = worker_pool.NewPool(10,
				worker_pool.WithMaxWorkers(20),
				worker_pool.WithFixedCoreWorkers(5),
				worker_pool.WithName(s.name),
			)
		} else {
			pool = worker_pool.NewPool(10,
				worker_pool.WithMaxWorkers(20),
				worker_pool.WithCoreAdjustStrategy(s.strategy),
				worker_pool.WithLowLoadThreshold(0.3),
				worker_pool.WithAdjustCheckInterval(500*time.Millisecond),
				worker_pool.WithName(s.name),
			)
		}

		s.setup(pool)

		initialStats := pool.EnhancedStats()
		fmt.Printf("     初始状态: Core=%d, Strategy=%s\n",
			initialStats.CoreWorkers, initialStats.CoreAdjustStrategy)

		// 手动调整测试
		pool.SetCoreWorkers(8)
		afterManualStats := pool.EnhancedStats()
		fmt.Printf("     手动调整后: Core=%d\n", afterManualStats.CoreWorkers)

		// 提交一些任务测试自动调整
		for i := 0; i < 50; i++ {
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			})
		}

		// 等待任务完成和可能的自动调整
		time.Sleep(2 * time.Second)

		finalStats := pool.EnhancedStats()
		fmt.Printf("     最终状态: Core=%d, Active=%d, Completed=%d, History=%d\n",
			finalStats.CoreWorkers, finalStats.ActiveWorkers,
			finalStats.Completed, finalStats.LoadHistoryLength)

		pool.Shutdown()
	}
}

// 错误处理测试
func runErrorHandlingTest() {
	fmt.Println("\n--- 错误处理测试 ---")

	pool := worker_pool.NewPool(3,
		worker_pool.WithMaxWorkers(5),
		worker_pool.WithName("error-test-pool"),
	)
	defer pool.Shutdown()

	var panicCount, errorCount, successCount int64
	var wg sync.WaitGroup

	// 提交各种类型的任务
	for i := 0; i < 100; i++ {
		wg.Add(1)

		var taskType int = i % 4

		switch taskType {
		case 0: // 正常任务
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				atomic.AddInt64(&successCount, 1)
				return "success", nil
			})
		case 1: // 返回错误的任务
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				atomic.AddInt64(&errorCount, 1)
				return nil, fmt.Errorf("task error %d", i)
			})
		case 2: // Panic任务（带恢复）
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				panic(fmt.Sprintf("task panic %d", i))
			}, worker_pool.WithRecovery(func(r interface{}) {
				atomic.AddInt64(&panicCount, 1)
			}))
		case 3: // 带超时的任务
			pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				select {
				case <-time.After(50 * time.Millisecond):
					atomic.AddInt64(&successCount, 1)
					return "timeout-success", nil
				case <-ctx.Done():
					atomic.AddInt64(&errorCount, 1)
					return nil, ctx.Err()
				}
			}, worker_pool.WithTimeout(100*time.Millisecond))
		}
	}

	wg.Wait()

	fmt.Printf("✅ 错误处理测试完成:\n")
	fmt.Printf("   - 成功任务: %d\n", successCount)
	fmt.Printf("   - 错误任务: %d\n", errorCount)
	fmt.Printf("   - Panic任务: %d\n", panicCount)
	fmt.Printf("   - 总计: %d\n", successCount+errorCount+panicCount)

	if successCount+errorCount+panicCount != 100 {
		fmt.Printf("❌ 任务数量不匹配\n")
	}
}

// Context取消测试
func runContextCancellationTest() {
	fmt.Println("\n--- Context取消测试 ---")

	pool := worker_pool.NewPool(5,
		worker_pool.WithMaxWorkers(10),
		worker_pool.WithName("context-test-pool"),
	)
	defer pool.Shutdown()

	var cancelledCount, completedCount int64
	var wg sync.WaitGroup

	// 测试不同的取消场景
	for i := 0; i < 50; i++ {
		wg.Add(1)

		if i%2 == 0 {
			// 快速取消的context
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			pool.Submit(ctx, func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				select {
				case <-time.After(50 * time.Millisecond): // 任务需要50ms
					atomic.AddInt64(&completedCount, 1)
					return "completed", nil
				case <-ctx.Done():
					atomic.AddInt64(&cancelledCount, 1)
					return nil, ctx.Err()
				}
			})
			cancel() // 立即取消
		} else {
			// 正常完成的任务
			ctx := context.Background()
			pool.Submit(ctx, func(ctx context.Context) (interface{}, error) {
				defer wg.Done()
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt64(&completedCount, 1)
				return "completed", nil
			})
		}
	}

	wg.Wait()

	fmt.Printf("✅ Context取消测试完成:\n")
	fmt.Printf("   - 完成任务: %d\n", completedCount)
	fmt.Printf("   - 取消任务: %d\n", cancelledCount)
	fmt.Printf("   - 总计: %d\n", completedCount+cancelledCount)

	if completedCount+cancelledCount != 50 {
		fmt.Printf("❌ 任务数量不匹配\n")
	}

	// 验证取消的任务数量合理
	if cancelledCount == 0 {
		fmt.Printf("⚠️  没有任务被取消，可能context取消机制有问题\n")
	}
}
*/
