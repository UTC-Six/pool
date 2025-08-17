package main

// 如需运行增强功能演示，请取消注释下面的导入和main函数
/*
import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)

func main() {
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
	stats := pool.DetailedStats()
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
	stats = pool.DetailedStats()
	fmt.Printf("高负载后 - Workers: %d, Core: %d, Queue: %d, Completed: %d\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.QueuedTasks, stats.Completed)

	// 场景3: 手动调整核心worker数量
	fmt.Println("\n=== 场景3: 手动调整核心worker ===")
	fmt.Println("手动设置核心worker为15...")
	pool.SetCoreWorkers(15)

	stats = pool.DetailedStats()
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

	stats = fixedPool.DetailedStats()
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

		stats = pool.DetailedStats()
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
	stats = pool.DetailedStats()
	fmt.Printf("突发负载后 - Core: %d, Active: %d, Completed: %d\n",
		stats.CoreWorkers, stats.ActiveWorkers, stats.Completed)

	// 最终统计
	fmt.Println("\n=== 最终统计 ===")
	finalStats := pool.DetailedStats()
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
}
*/
