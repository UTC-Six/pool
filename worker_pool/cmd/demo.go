package main

// 所有导入已注释，如需运行请取消注释main函数和相关导入
/*
import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/UTC-Six/pool/worker_pool"
)
*/

// 如需运行基础演示，请取消注释下面的main函数
/*
func main() {
	// 创建协程池
	p := worker_pool.NewPool(2, worker_pool.WithMaxWorkers(5))
	defer p.Shutdown()

	// 提交普通任务
	err := p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool.Submit] hello from pool")
		return nil, nil
	})
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带返回值任务
	resultCh, _ := p.SubmitWithResult(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "result from pool", nil
	}, worker_pool.WithPriority(worker_pool.PriorityHigh))
	res := <-resultCh
	fmt.Println("[worker_pool.SubmitWithResult] result:", res.Result, "err:", res.Err)

	// 提交带超时的任务
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = p.Submit(ctx, func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(500 * time.Millisecond): // 缩短到500ms
			fmt.Println("[worker_pool] done")
		case <-ctx.Done():
			fmt.Println("[worker_pool] ctx cancelled 场景")
			return nil, ctx.Err()
		}
		return nil, nil
	}, worker_pool.WithTimeout(1*time.Second))
	if err != nil {
		fmt.Println("[worker_pool] submit error:", err)
	}

	// 提交带 recovery 的任务
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		panic("something wrong")
	}, worker_pool.WithRecovery(func(r interface{}) {
		fmt.Println("[worker_pool] recovered from:", r)
	}))

	// 批量提交任务，推荐用 Option 风格循环调用 Submit
	tasks := []struct {
		TaskFunc func(ctx context.Context) (interface{}, error)
		Tag      string
	}{
		{func(ctx context.Context) (interface{}, error) {
			fmt.Println("[worker_pool] batch task 1 running")
			return "A", nil
		}, "A"},
		{func(ctx context.Context) (interface{}, error) {
			fmt.Println("[worker_pool] batch task 2 running")
			return "B", nil
		}, "B"},
	}
	for _, task := range tasks {
		_ = p.Submit(context.Background(), task.TaskFunc, worker_pool.WithTag(task.Tag), worker_pool.WithLog(func(format string, args ...interface{}) { fmt.Printf("[TASK] "+format+"\n", args...) }))
	}

	// 任务前后钩子
	_ = p.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
		fmt.Println("[worker_pool] task with hooks running")
		return nil, nil
	}, worker_pool.WithBefore(func() { fmt.Println("[worker_pool] before hook") }), worker_pool.WithAfter(func() { fmt.Println("[worker_pool] after hook") }))

	// 动态调整
	p.SetMinWorkers(3)
	p.SetMaxWorkers(6)
	stats := p.Stats()
	fmt.Printf("[worker_pool] 动态调整后: min=%d, max=%d, active=%d, queued=%d\n", stats.MinWorkers, stats.MaxWorkers, stats.ActiveWorkers, stats.QueuedTasks)

	// 获取统计信息
	stats = p.Stats()
	fmt.Printf("[worker_pool] 活跃worker: %d, 排队: %d, 完成: %d\n", stats.ActiveWorkers, stats.QueuedTasks, stats.Completed)

	// 演示增强功能
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("以下是增强版Pool的功能演示:")
	fmt.Println(strings.Repeat("=", 50))
	runEnhancedExample()
}

// 运行增强版Pool示例的函数
func runEnhancedExample() {
	fmt.Println("=== 增强版Worker Pool示例 ===")

	// 创建支持动态调整的协程池
	pool := worker_pool.NewPool(
		10, // 初始minWorkers (减少以便演示)
		worker_pool.WithMaxWorkers(20),
		worker_pool.WithName("dynamic-pool"),
		worker_pool.WithLogger(log.Printf),

		// 动态调整配置
		worker_pool.WithAllowCoreTimeout(true),                             // 允许核心线程超时
		worker_pool.WithKeepAliveTime(60*time.Second),                      // 60秒空闲超时
		worker_pool.WithAdjustCheckInterval(1*time.Second),                 // 1秒检查一次（演示用）
		worker_pool.WithCoreAdjustStrategy(worker_pool.StrategyPercentage), // 百分比策略
		worker_pool.WithLowLoadThreshold(0.3),                              // 30%阈值
	)
	defer pool.Shutdown()

	// 场景1: 初始状态展示
	fmt.Println("\n=== 场景1: 初始状态 ===")
	stats := pool.DetailedStats()
	fmt.Printf("初始状态 - Workers: %d, Core: %d, Strategy: %s, Threshold: %.1f%%\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.CoreAdjustStrategy, stats.LowLoadThreshold*100)

	// 场景2: 手动调整核心worker数量
	fmt.Println("\n=== 场景2: 手动调整核心worker ===")
	fmt.Println("手动设置核心worker为5...")
	pool.SetCoreWorkers(5)

	stats = pool.DetailedStats()
	fmt.Printf("手动调整后 - Core: %d\n", stats.CoreWorkers)

	// 场景3: 测试不同策略
	fmt.Println("\n=== 场景3: 策略对比测试 ===")

	// 测试固定策略
	fmt.Println("创建固定策略池，设置核心worker为3...")
	fixedPool := worker_pool.NewPool(
		8,
		worker_pool.WithMaxWorkers(15),
		worker_pool.WithFixedCoreWorkers(3), // 固定3个核心worker
		worker_pool.WithName("fixed-pool"),
	)
	defer fixedPool.Shutdown()

	stats = fixedPool.DetailedStats()
	fmt.Printf("固定策略 - Core: %d, Strategy: %s\n", stats.CoreWorkers, stats.CoreAdjustStrategy)

	// 场景4: 提交一些任务测试
	fmt.Println("\n=== 场景4: 任务提交测试 ===")
	fmt.Println("提交5个任务...")
	for i := 0; i < 5; i++ {
		pool.Submit(context.Background(), func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return fmt.Sprintf("enhanced-task-%d", i), nil
		})
	}

	time.Sleep(1 * time.Second) // 等待任务处理
	stats = pool.DetailedStats()
	fmt.Printf("任务提交后 - Workers: %d, Core: %d, Queue: %d, Completed: %d\n",
		stats.ActiveWorkers, stats.CoreWorkers, stats.QueuedTasks, stats.Completed)

	// 等待一个监控周期看看是否有调整
	fmt.Println("等待2秒观察监控...")
	time.Sleep(2 * time.Second)
	stats = pool.DetailedStats()
	fmt.Printf("监控后 - Core: %d, HistoryLength: %d\n", stats.CoreWorkers, stats.LoadHistoryLength)

	// 最终统计
	fmt.Println("\n=== 最终统计 ===")
	finalStats := pool.DetailedStats()
	fmt.Printf("增强Pool最终状态:\n")
	fmt.Printf("  - 核心Worker: %d\n", finalStats.CoreWorkers)
	fmt.Printf("  - 最小Worker: %d\n", finalStats.MinWorkers)
	fmt.Printf("  - 最大Worker: %d\n", finalStats.MaxWorkers)
	fmt.Printf("  - 当前活跃: %d\n", finalStats.ActiveWorkers)
	fmt.Printf("  - 已完成任务: %d\n", finalStats.Completed)
	fmt.Printf("  - 总提交任务: %d\n", finalStats.TaskSubmitCount)
	fmt.Printf("  - 调整策略: %s\n", finalStats.CoreAdjustStrategy)
	fmt.Printf("  - 负载历史长度: %d\n", finalStats.LoadHistoryLength)

	if finalStats.TaskSubmitCount > 0 {
		fmt.Printf("  - 最后活动时间: %v前\n", time.Since(finalStats.LastActivityTime).Truncate(time.Second))
	}
}
*/
