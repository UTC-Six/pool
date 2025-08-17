package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)

func simpleTest() {
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

	stats := pool.DetailedStats()
	fmt.Printf("初始状态 - Min: %d, Max: %d, Core: %d, Strategy: %s\n",
		stats.MinWorkers, stats.MaxWorkers, stats.CoreWorkers, stats.CoreAdjustStrategy)

	// 测试2: 任务提交
	fmt.Println("\n--- 测试2: 任务提交 ---")
	for i := 0; i < 3; i++ {
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			fmt.Printf("任务 %d 执行中\n", i)
			time.Sleep(100 * time.Millisecond)
			return fmt.Sprintf("result-%d", i), nil
		})
	}

	time.Sleep(500 * time.Millisecond)
	stats = pool.DetailedStats()
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

	fixedStats := fixedPool.DetailedStats()
	fmt.Printf("固定策略 - Core: %d, Strategy: %s\n", fixedStats.CoreWorkers, fixedStats.CoreAdjustStrategy)

	// 百分比策略
	percentPool := worker_pool.NewPool(
		6,
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage),
		worker_pool.WithLowLoadThreshold(0.4),
		worker_pool.WithName("percent-pool"),
	)
	defer percentPool.Shutdown()

	percentStats := percentPool.DetailedStats()
	fmt.Printf("百分比策略 - Core: %d, Strategy: %s, Threshold: %.1f%%\n",
		percentStats.CoreWorkers, percentStats.CoreAdjustStrategy, percentStats.LowLoadThreshold*100)

	fmt.Println("\n=== 测试完成 ===")
}

func main() {
	simpleTest()
}
