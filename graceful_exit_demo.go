package main

// 优雅退出机制演示
// 如需运行演示，请取消注释下面的导入和main函数
/*
import (
	"context"
	"fmt"
	"sync"
	"time"
)

func main() {
	fmt.Println("🛡️ 优雅退出 vs 强制退出对比演示")

	// 演示1: 强制退出的问题（不推荐的做法）
	fmt.Println("\n❌ 错误示例：强制退出的问题")
	demonstrateForcefulExit()

	// 演示2: 优雅退出的正确做法
	fmt.Println("\n✅ 正确示例：优雅退出机制")
	demonstrateGracefulExit()

	// 演示3: Worker Pool中的优雅退出
	fmt.Println("\n🏭 Worker Pool中的优雅退出实现")
	demonstrateWorkerPoolGracefulExit()
}

// ❌ 强制退出示例（问题重重）
func demonstrateForcefulExit() {
	fmt.Println("创建一个'强制退出'的worker...")

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("  Worker清理完成")

		for {
			select {
			case <-stopChan:
				fmt.Println("  Worker收到停止信号，但正在处理重要任务...")

				// 模拟正在处理重要任务（比如写数据库）
				fmt.Println("  正在保存重要数据...")
				time.Sleep(2 * time.Second) // 模拟耗时操作
				fmt.Println("  数据保存完成")
				return // 优雅退出

			default:
				// 正常工作
				fmt.Println("  Worker正在工作...")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	time.Sleep(1 * time.Second)
	fmt.Println("发送停止信号...")
	close(stopChan)

	// 如果这里不等待，就是"强制退出"
	fmt.Println("等待worker优雅退出...")
	wg.Wait()
	fmt.Println("Worker已优雅退出")
}

// ✅ 优雅退出示例
func demonstrateGracefulExit() {
	fmt.Println("创建优雅退出的worker...")

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("  Worker优雅退出完成")

		for {
			select {
			case <-ctx.Done():
				fmt.Println("  收到退出信号，开始清理...")

				// 清理资源
				fmt.Println("  关闭数据库连接...")
				fmt.Println("  保存未完成的工作...")
				fmt.Println("  释放内存资源...")

				return // 优雅退出：通过return正常结束

			default:
				// 检查是否有工作要做
				fmt.Println("  执行工作单元...")

				// 模拟可中断的工作
				select {
				case <-time.After(300 * time.Millisecond):
					// 工作完成
				case <-ctx.Done():
					fmt.Println("  工作被中断，但会在清理后退出")
					return
				}
			}
		}
	}()

	time.Sleep(1 * time.Second)
	fmt.Println("请求worker优雅退出...")
	cancel() // 发送取消信号

	wg.Wait() // 等待优雅退出完成
	fmt.Println("所有worker已优雅退出")
}

// 🏭 Worker Pool中的优雅退出实现
func demonstrateWorkerPoolGracefulExit() {
	fmt.Println("演示Worker Pool的优雅退出机制...")

	// 模拟我们的worker pool中的优雅退出逻辑
	type MockPool struct {
		workers     int
		coreWorkers int
		allowCoreTimeout bool
		keepAliveTime    time.Duration
		shutdown         bool
		mu               sync.Mutex
		taskCond         *sync.Cond
		wg               sync.WaitGroup
	}

	pool := &MockPool{
		workers:          5,
		coreWorkers:      2,
		allowCoreTimeout: true,
		keepAliveTime:    2 * time.Second,
		shutdown:         false,
	}
	pool.taskCond = sync.NewCond(&pool.mu)

	// 启动几个worker演示优雅退出
	for i := 0; i < 3; i++ {
		workerID := i
		pool.wg.Add(1)

		go func() {
			defer pool.wg.Done()
			defer fmt.Printf("    Worker-%d 优雅退出完成\n", workerID)

			fmt.Printf("    Worker-%d 开始工作\n", workerID)

			for {
				pool.mu.Lock()

				// 🎯 关键：检查是否可以优雅退出
				canTimeout := pool.allowCoreTimeout && pool.workers > pool.coreWorkers

				if !pool.shutdown && canTimeout {
					fmt.Printf("    Worker-%d 检测到可以退出（workers=%d > coreWorkers=%d）\n",
						workerID, pool.workers, pool.coreWorkers)

					pool.mu.Unlock()

					// 🛡️ 优雅退出：使用超时等待而不是立即退出
					ctx, cancel := context.WithTimeout(context.Background(), pool.keepAliveTime)

					fmt.Printf("    Worker-%d 等待%v后优雅退出...\n", workerID, pool.keepAliveTime)

					select {
					case <-ctx.Done():
						cancel()
						// 再次检查条件
						pool.mu.Lock()
						if pool.workers > pool.coreWorkers {
							pool.workers-- // 原子地减少worker数量
							fmt.Printf("    Worker-%d 优雅退出：workers %d -> %d\n",
								workerID, pool.workers+1, pool.workers)
							pool.mu.Unlock()
							return // 🎯 优雅退出：通过return正常结束goroutine
						}
						pool.mu.Unlock()
					}
					continue
				}

				// 检查关闭信号
				if pool.shutdown {
					fmt.Printf("    Worker-%d 收到关闭信号，准备退出\n", workerID)
					pool.mu.Unlock()
					return // 优雅退出
				}

				pool.mu.Unlock()

				// 模拟工作
				time.Sleep(500 * time.Millisecond)
			}
		}()
	}

	// 让worker运行一段时间
	time.Sleep(1 * time.Second)

	// 模拟调整coreWorkers触发缩容
	fmt.Println("  调整coreWorkers从2到1，触发优雅缩容...")
	pool.mu.Lock()
	pool.coreWorkers = 1
	pool.mu.Unlock()

	// 等待缩容完成
	time.Sleep(3 * time.Second)

	// 关闭池
	fmt.Println("  关闭worker pool...")
	pool.mu.Lock()
	pool.shutdown = true
	pool.taskCond.Broadcast() // 唤醒所有等待的worker
	pool.mu.Unlock()

	pool.wg.Wait()
	fmt.Println("  所有worker已优雅退出")
}
*/
