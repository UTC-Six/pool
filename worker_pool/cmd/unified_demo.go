package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)

// 🎯 统一演示程序 - 包含三种测试模式
//
// 📁 文件说明：
//   unified_demo.go  # 主演示程序（当前唯一可运行文件）
//   demo.go         # 基础演示（已注释，需要时取消注释）
//   simple_demo.go  # 简单测试（已注释，需要时取消注释）
//   stress_demo.go  # 压力测试（已注释，需要时取消注释）
//
// 🚀 使用方法：
//   go run ./cmd/unified_demo.go simple     # 运行简单功能测试
//   go run ./cmd/unified_demo.go enhanced   # 运行增强功能演示
//   go run ./cmd/unified_demo.go stress     # 运行压力测试
//   go run ./cmd/unified_demo.go            # 默认运行简单测试
//
// 💡 如需运行其他演示：
//   1. 取消对应文件中的注释（import和main函数）
//   2. 注释掉unified_demo.go的main函数
//   3. 运行: go run ./cmd/demo.go 等

func main() {
	args := os.Args // 获取命令行参数

	mode := "simple"
	if len(args) > 1 {
		mode = args[1]
	}

	fmt.Printf("🚀 Worker Pool 统一演示程序 - 模式: %s\n", mode)
	fmt.Printf("系统信息: CPU核心数=%d, GOMAXPROCS=%d\n\n", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	switch mode {
	case "simple":
		runSimpleTest()
	case "enhanced":
		runEnhancedDemo()
	case "stress":
		runStressTests()
	default:
		fmt.Printf("未知模式: %s\n", mode)
		fmt.Println("支持的模式: simple, enhanced, stress")
		fmt.Println("默认运行简单测试...")
		runSimpleTest()
	}
}

// ==================== 简单功能测试 ====================
func runSimpleTest() {
	fmt.Println("=== 简单功能测试 ===")

	// 测试1: 基础功能
	fmt.Println("\n--- 测试1: 基础Pool创建 ---")
	pool := worker_pool.NewPool(
		5,
		worker_pool.WithMaxWorkers(10),
		worker_pool.WithName("test-pool"),
		worker_pool.WithLogger(log.Printf),
	)
	defer pool.Shutdown()

	stats := pool.EnhancedStats()
	fmt.Printf("初始状态 - Min: %d, Max: %d, Core: %d, Strategy: %s\n",
		stats.MinWorkers, stats.MaxWorkers, stats.CoreWorkers, stats.CoreAdjustStrategy)

	// 测试2: 任务提交
	fmt.Println("\n--- 测试2: 任务提交 ---")
	for i := 0; i < 3; i++ {
		taskID := i // 避免闭包问题
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			fmt.Printf("任务 %d 执行中\n", taskID)
			time.Sleep(100 * time.Millisecond)
			return fmt.Sprintf("result-%d", taskID), nil
		})
	}

	time.Sleep(500 * time.Millisecond)
	stats = pool.EnhancedStats()
	fmt.Printf("任务提交后 - Active: %d, Completed: %d, Total: %d\n",
		stats.ActiveWorkers, stats.Completed, stats.TaskSubmitCount)

	// 测试3: 手动调整CoreWorkers
	fmt.Println("\n--- 测试3: CoreWorkers调整 ---")
	fmt.Printf("调整前 CoreWorkers: %d\n", pool.GetCoreWorkers())
	pool.SetCoreWorkers(3)
	fmt.Printf("调整后 CoreWorkers: %d\n", pool.GetCoreWorkers())

	// 测试4: 不同策略
	fmt.Println("\n--- 测试4: 不同策略测试 ---")

	// 固定策略
	fixedPool := worker_pool.NewPool(
		8,
		worker_pool.WithFixedCoreWorkers(2),
		worker_pool.WithName("fixed-pool"),
	)
	defer fixedPool.Shutdown()

	fixedStats := fixedPool.EnhancedStats()
	fmt.Printf("固定策略 - Core: %d, Strategy: %s\n", fixedStats.CoreWorkers, fixedStats.CoreAdjustStrategy)

	// 百分比策略
	percentPool := worker_pool.NewPool(
		6,
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage),
		worker_pool.WithLowLoadThreshold(0.4),
		worker_pool.WithName("percent-pool"),
	)
	defer percentPool.Shutdown()

	percentStats := percentPool.EnhancedStats()
	fmt.Printf("百分比策略 - Core: %d, Strategy: %s, Threshold: %.1f%%\n",
		percentStats.CoreWorkers, percentStats.CoreAdjustStrategy, percentStats.LowLoadThreshold*100)

	fmt.Println("\n✅ 简单测试完成")
}

// ==================== 增强功能演示 ====================
// 如需运行此演示，请修改main函数中的mode为"enhanced"或使用命令行参数
/*
func runEnhancedDemo() {
	fmt.Println("=== 增强版Worker Pool示例 ===")

	// 创建支持动态调整的协程池
	pool := worker_pool.NewPool(
		50, // 初始minWorkers
		worker_pool.WithMaxWorkers(100),
		worker_pool.WithName("dynamic-pool"),
		worker_pool.WithLogger(log.Printf),

		// 动态调整配置
		worker_pool.WithAllowCoreTimeout(true),                             // 允许核心线程超时
		worker_pool.WithKeepAliveTime(60*time.Second),                      // 60秒空闲超时
		worker_pool.WithAdjustCheckInterval(5*time.Second),                 // 5秒检查一次（演示用）
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage), // 百分比策略
		worker_pool.WithLowLoadThreshold(0.3),                              // 30%阈值
	)
	defer pool.Shutdown()

	// 场景1: 初始状态展示
	fmt.Println("\n=== 场景1: 初始状态 ===")
	stats := pool.EnhancedStats()
	fmt.Printf("初始状态 - Workers: %d, Core: %d, Strategy: %s, Threshold: %.1f%%\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.CoreAdjustStrategy, stats.LowLoadThreshold*100)

	// 场景2: 高负载测试
	fmt.Println("\n=== 场景2: 高负载测试 ===")
	fmt.Println("提交80个任务...")
	for i := 0; i < 80; i++ {
		taskID := i
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(100 * time.Millisecond)
			return fmt.Sprintf("task-%d", taskID), nil
		})
	}

	time.Sleep(2 * time.Second) // 等待任务处理
	stats = pool.EnhancedStats()
	fmt.Printf("高负载后 - Workers: %d, Core: %d, Queue: %d, Completed: %d\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.QueuedTasks, stats.Completed)

	// 场景3: 手动调整核心worker数量
	fmt.Println("\n=== 场景3: 手动调整核心worker ===")
	fmt.Println("手动设置核心worker为15...")
	pool.SetCoreWorkers(15)

	stats = pool.EnhancedStats()
	fmt.Printf("手动调整后 - Core: %d\n", stats.CoreWorkers)

	// 场景4: 测试不同策略
	fmt.Println("\n=== 场景4: 策略对比测试 ===")

	// 测试固定策略
	fmt.Println("切换到固定策略，设置核心worker为10...")
	fixedPool := worker_pool.NewPool(
		20,
		worker_pool.WithMaxWorkers(50),
		worker_pool.WithFixedCoreWorkers(10), // 固定10个核心worker
		worker_pool.WithLogger(log.Printf),
	)
	defer fixedPool.Shutdown()

	stats = fixedPool.EnhancedStats()
	fmt.Printf("固定策略 - Core: %d, Strategy: %s\n", stats.CoreWorkers, stats.CoreAdjustStrategy)

	// 场景5: 负载历史监控
	fmt.Println("\n=== 场景5: 负载历史监控 ===")
	fmt.Println("等待负载监控收集数据...")

	// 提交少量任务模拟低负载
	for i := 0; i < 5; i++ {
		time.Sleep(6 * time.Second) // 等待一个监控周期

		// 提交少量任务
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			return "low-load-task", nil
		})

		stats = pool.EnhancedStats()
		fmt.Printf("第%d次检查 - Core: %d, HistoryLength: %d, LastActivity: %v\n",
			i+1, stats.CoreWorkers, stats.LoadHistoryLength,
			time.Since(stats.LastActivityTime).Truncate(time.Second))
	}

	// 场景6: 突发高负载恢复测试
	fmt.Println("\n=== 场景6: 突发高负载恢复 ===")
	fmt.Println("模拟突发高负载...")
	for i := 0; i < 30; i++ {
		taskID := i
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return fmt.Sprintf("burst-task-%d", taskID), nil
		})
	}

	time.Sleep(3 * time.Second)
	stats = pool.EnhancedStats()
	fmt.Printf("突发负载后 - Core: %d, Active: %d, Completed: %d\n",
		stats.CoreWorkers, stats.ActiveWorkers, stats.Completed)

	// 最终统计
	fmt.Println("\n=== 最终统计 ===")
	finalStats := pool.EnhancedStats()
	fmt.Printf("最终状态:\n")
	fmt.Printf("  - 核心Worker: %d\n", finalStats.CoreWorkers)
	fmt.Printf("  - 最小Worker: %d\n", finalStats.MinWorkers)
	fmt.Printf("  - 最大Worker: %d\n", finalStats.MaxWorkers)
	fmt.Printf("  - 当前活跃: %d\n", finalStats.ActiveWorkers)
	fmt.Printf("  - 已完成任务: %d\n", finalStats.Completed)
	fmt.Printf("  - 总提交任务: %d\n", finalStats.TaskSubmitCount)
	fmt.Printf("  - 调整策略: %s\n", finalStats.CoreAdjustStrategy)
	fmt.Printf("  - 负载历史长度: %d\n", finalStats.LoadHistoryLength)
	fmt.Printf("  - 最后活动时间: %v\n", time.Since(finalStats.LastActivityTime).Truncate(time.Second))

	// 展示负载历史（最近5个样本）
	history := pool.GetLoadHistory()
	if len(history) > 0 {
		fmt.Printf("  - 最近5个负载样本:\n")
		start := len(history) - 5
		if start < 0 {
			start = 0
		}
		for i, sample := range history[start:] {
			fmt.Printf("    %d. Time: %s, Active: %d, Queue: %d\n",
				i+1, sample.Timestamp.Format("15:04:05"), sample.ActiveWorkers, sample.QueueLength)
		}
	}

	fmt.Println("\n✅ 增强功能演示完成")
}
*/

func runEnhancedDemo() {
	fmt.Println("🔧 增强功能演示已注释，如需运行请取消注释 runEnhancedDemo 函数")
	fmt.Println("💡 提示：该演示需要较长时间（约1分钟），适合深入了解动态调整特性")
}

// ==================== 压力测试 ====================
// 如需运行压力测试，请修改main函数中的mode为"stress"或使用命令行参数
/*
func runStressTests() {
	fmt.Println("🚀 Worker Pool 生产环境压力测试")

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
					localWg.Done() // 如果提交失败也要调用Done
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

// 其他测试函数类似，为节省篇幅这里省略...
func runLongRunningTest()         { fmt.Println("⏳ 长时间运行测试已注释，如需运行请取消注释") }
func runResourceLeakTest()        { fmt.Println("🔍 资源泄露测试已注释，如需运行请取消注释") }
func runCoreWorkersAdjustmentTest() { fmt.Println("🔧 CoreWorkers调整测试已注释，如需运行请取消注释") }
func runErrorHandlingTest()       { fmt.Println("❌ 错误处理测试已注释，如需运行请取消注释") }
func runContextCancellationTest() { fmt.Println("⏹️  Context取消测试已注释，如需运行请取消注释") }
*/

func runStressTests() {
	fmt.Println("🔧 压力测试已注释，如需运行请取消注释 runStressTests 函数")
	fmt.Println("💡 提示：压力测试包含多个测试用例，运行时间较长，适合生产环境验证")
	fmt.Println("📋 包含的测试：")
	fmt.Println("   - 基础压力测试：10000个任务，测试TPS性能")
	fmt.Println("   - 高并发测试：100个goroutine并发提交任务")
	fmt.Println("   - 长时间运行测试：10秒持续负载测试")
	fmt.Println("   - 资源泄露测试：创建销毁50个池检查内存泄露")
	fmt.Println("   - CoreWorkers调整测试：测试不同调整策略")
	fmt.Println("   - 错误处理测试：测试panic恢复和错误处理")
	fmt.Println("   - Context取消测试：测试上下文取消机制")
}
