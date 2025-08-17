package main

// 如需运行缩容测试，请取消注释下面的导入
/*
import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)
*/

// 如需运行缩容测试，请取消注释下面的main函数
/*
func main() {
	fmt.Println("🔥 测试新的缩容逻辑")

	// 创建池，启用缩容功能
	pool := worker_pool.NewPool(10,
		worker_pool.WithMaxWorkers(20),
		worker_pool.WithAllowCoreTimeout(true),               // 关键：允许超时
		worker_pool.WithKeepAliveTime(5*time.Second),         // 5秒超时（快速测试）
		worker_pool.WithAdjustCheckInterval(2*time.Second),   // 2秒检查一次（快速测试）
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage),
		worker_pool.WithLowLoadThreshold(0.3),                // 30%阈值
		worker_pool.WithLogger(log.Printf),
		worker_pool.WithName("shrink-test-pool"),
	)
	defer pool.Shutdown()

	fmt.Printf("初始状态: ")
	printStats(pool)

	// 阶段1: 提交大量任务触发扩容
	fmt.Println("\n🚀 阶段1: 提交20个任务触发扩容")
	for i := 0; i < 20; i++ {
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(100 * time.Millisecond) // 短暂工作
			return fmt.Sprintf("task-%d", i), nil
		})
	}

	time.Sleep(2 * time.Second) // 等待任务处理和可能的扩容
	fmt.Printf("扩容后状态: ")
	printStats(pool)

	// 阶段2: 等待任务完成，观察缩容
	fmt.Println("\n⏳ 阶段2: 等待任务完成，观察是否会缩容...")
	for i := 0; i < 15; i++ { // 等待30秒，观察缩容过程
		time.Sleep(2 * time.Second)
		fmt.Printf("第%d次检查 (等待%d秒): ", i+1, (i+1)*2)
		printStats(pool)

		stats := pool.DetailedStats()
		if stats.ActiveWorkers <= stats.CoreWorkers {
			fmt.Println("✅ 缩容完成！")
			break
		}
	}

	// 阶段3: 手动调整CoreWorkers测试
	fmt.Println("\n🔧 阶段3: 手动设置CoreWorkers=5，观察进一步缩容")
	pool.SetCoreWorkers(5)

	for i := 0; i < 10; i++ { // 等待20秒观察
		time.Sleep(2 * time.Second)
		fmt.Printf("手动调整后第%d次检查: ", i+1)
		printStats(pool)

		stats := pool.DetailedStats()
		if stats.ActiveWorkers <= 5 {
			fmt.Println("✅ 手动调整后缩容完成！")
			break
		}
	}

	fmt.Println("\n🎉 缩容逻辑测试完成")
}

func printStats(pool *worker_pool.Pool) {
	stats := pool.DetailedStats()
	fmt.Printf("Workers: %d, Core: %d, Queue: %d, Completed: %d\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.QueuedTasks, stats.Completed)
}
*/
