package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/UTC-Six/pool/monitor"
)

// =============================================================================
// 模拟 logz 日志系统
// =============================================================================

// logzInfof 模拟 logz 日志函数
func logzInfof(ctx context.Context, format string, args ...interface{}) {
	// 从 ctx 中提取 traceID（模拟 logz 的行为）
	traceID := "unknown"
	if traceValue := ctx.Value("trace-id"); traceValue != nil {
		if traceStr, ok := traceValue.(string); ok {
			traceID = traceStr
		}
	}

	// 输出带 traceID 的日志
	log.Printf("[TraceID=%s] "+format, append([]interface{}{traceID}, args...)...)
}

// =============================================================================
// 自定义 Context 处理器
// =============================================================================

// customContextProcessor 自定义的context处理器
func customContextProcessor(ctx context.Context) context.Context {
	// 如果ctx为空，返回Background
	if ctx == nil {
		return context.Background()
	}

	// 创建一个新的 context
	asyncCtx := context.Background()

	// 复制原 ctx 中的关键信息到新的 ctx
	if traceValue := ctx.Value("trace-id"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "trace-id", traceValue)
	}
	if traceValue := ctx.Value("uber-trace-id"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "uber-trace-id", traceValue)
	}
	if traceValue := ctx.Value("X-B3-TraceId"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "X-B3-TraceId", traceValue)
	}
	if traceValue := ctx.Value("traceparent"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "traceparent", traceValue)
	}
	if traceValue := ctx.Value("request-id"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "request-id", traceValue)
	}
	if traceValue := ctx.Value("correlation-id"); traceValue != nil {
		asyncCtx = context.WithValue(asyncCtx, "correlation-id", traceValue)
	}

	// 如果没有找到任何traceID，返回Background
	if asyncCtx.Value("trace-id") == nil &&
		asyncCtx.Value("uber-trace-id") == nil &&
		asyncCtx.Value("X-B3-TraceId") == nil {
		return context.Background()
	}

	return asyncCtx
}

// =============================================================================
// 业务函数示例
// =============================================================================

// UserService 用户服务
type UserService struct{}

// GetUser 获取用户信息
func (s *UserService) GetUser(ctx context.Context, userID string) (*User, error) {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "UserService.GetUser", nil)

	// 模拟数据库查询
	time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)

	// 模拟偶尔的错误
	if rand.Float32() < 0.1 {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	return &User{ID: userID, Name: "John Doe", Email: "john@example.com"}, nil
}

// CreateUser 创建用户
func (s *UserService) CreateUser(ctx context.Context, user *User) error {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "UserService.CreateUser", nil)

	// 模拟数据库插入
	time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)

	// 模拟偶尔的错误
	if rand.Float32() < 0.05 {
		return fmt.Errorf("failed to create user: %s", user.Email)
	}

	return nil
}

// User 用户模型
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// =============================================================================
// API 处理器示例
// =============================================================================

// APIHandler API处理器
type APIHandler struct {
	userService *UserService
}

// NewAPIHandler 创建API处理器
func NewAPIHandler() *APIHandler {
	return &APIHandler{
		userService: &UserService{},
	}
}

// GetUserHandler 获取用户API处理器
func (h *APIHandler) GetUserHandler(ctx context.Context, userID string) (*User, error) {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "GET /api/users/{id}", nil)

	// 调用业务逻辑
	return h.userService.GetUser(ctx, userID)
}

// CreateUserHandler 创建用户API处理器
func (h *APIHandler) CreateUserHandler(ctx context.Context, user *User) error {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "POST /api/users", nil)

	// 调用业务逻辑
	return h.userService.CreateUser(ctx, user)
}

// =============================================================================
// 数据库操作示例
// =============================================================================

// DatabaseService 数据库服务
type DatabaseService struct{}

// Query 数据库查询
func (s *DatabaseService) Query(ctx context.Context, sql string, args ...interface{}) ([]map[string]interface{}, error) {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "DatabaseService.Query", nil)

	// 模拟数据库查询
	time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

	// 模拟查询结果
	result := []map[string]interface{}{
		{"id": 1, "name": "John", "email": "john@example.com"},
		{"id": 2, "name": "Jane", "email": "jane@example.com"},
	}

	return result, nil
}

// Transaction 数据库事务
func (s *DatabaseService) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	start := time.Now()
	defer monitor.Monitor(ctx, start, "DatabaseService.Transaction", nil)

	// 模拟事务开始
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	// 执行事务函数
	if err := fn(ctx); err != nil {
		// 模拟回滚
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
		return err
	}

	// 模拟提交
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	return nil
}

// =============================================================================
// 外部服务调用示例
// =============================================================================

// ExternalService 外部服务
type ExternalService struct{}

// CallAPI 调用外部API
func (s *ExternalService) CallAPI(ctx context.Context, url string, method string) ([]byte, error) {
	start := time.Now()
	defer monitor.Monitor(ctx, start, fmt.Sprintf("%s %s", method, url), nil)

	// 模拟HTTP请求
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

	// 模拟偶尔的超时或错误
	if rand.Float32() < 0.1 {
		return nil, fmt.Errorf("timeout calling %s", url)
	}

	return []byte(`{"status": "success"}`), nil
}

// =============================================================================
// 工具函数
// =============================================================================

// createContextWithTraceID 创建带traceID的context
func createContextWithTraceID(traceID string) context.Context {
	return context.WithValue(context.Background(), "trace-id", traceID)
}

// createContextWithJaegerTraceID 创建带Jaeger traceID的context
func createContextWithJaegerTraceID(traceID string) context.Context {
	return context.WithValue(context.Background(), "uber-trace-id", traceID)
}

// createContextWithZipkinTraceID 创建带Zipkin traceID的context
func createContextWithZipkinTraceID(traceID string) context.Context {
	return context.WithValue(context.Background(), "X-B3-TraceId", traceID)
}

// =============================================================================
// 主函数
// =============================================================================

func main() {
	fmt.Println("=== Monitor 包完整示例 ===")
	fmt.Println("展示所有功能和使用场景")
	fmt.Println()

	// 1. 演示不同的初始化方式
	demonstrateInitialization()

	// 2. 演示基本监控功能
	demonstrateBasicMonitoring()

	// 3. 演示业务函数监控
	demonstrateBusinessFunctionMonitoring()

	// 4. 演示API监控
	demonstrateAPIMonitoring()

	// 5. 演示数据库操作监控
	demonstrateDatabaseMonitoring()

	// 6. 演示外部服务调用监控
	demonstrateExternalServiceMonitoring()

	// 7. 演示不同链路追踪系统
	demonstrateTracingSystems()

	// 8. 演示错误场景
	demonstrateErrorScenarios()

	// 等待异步日志输出
	time.Sleep(200 * time.Millisecond)

	fmt.Println("\n=== 示例完成 ===")
	printSummary()
}

// =============================================================================
// 演示函数
// =============================================================================

func demonstrateInitialization() {
	fmt.Println("1. 演示不同的初始化方式:")

	// 默认配置
	fmt.Println("   - 默认配置:")
	monitor1 := monitor.NewPerformanceMonitor()
	fmt.Printf("     默认logger: %T\n", monitor1)
	fmt.Printf("     默认contextProcessor: %T\n", monitor1)

	// 使用 WithLogger
	fmt.Println("   - 使用 WithLogger:")
	monitor2 := monitor.NewPerformanceMonitor(monitor.WithLogger(logzInfof))
	fmt.Printf("     自定义logger: %T\n", monitor2)

	// 使用 WithCtx
	fmt.Println("   - 使用 WithCtx:")
	monitor3 := monitor.NewPerformanceMonitor(monitor.WithCtx(customContextProcessor))
	fmt.Printf("     自定义contextProcessor: %T\n", monitor3)

	// 组合使用
	fmt.Println("   - 同时使用 WithLogger 和 WithCtx:")
	monitor4 := monitor.NewPerformanceMonitor(
		monitor.WithLogger(logzInfof),
		monitor.WithCtx(customContextProcessor),
	)
	fmt.Printf("     自定义logger: %T\n", monitor4)
	fmt.Printf("     自定义contextProcessor: %T\n", monitor4)
	fmt.Println()
}

func demonstrateBasicMonitoring() {
	fmt.Println("2. 演示基本监控功能:")

	ctx := context.Background()

	// 基本函数监控
	monitor.Monitor(ctx, time.Now(), "basicFunction", nil)

	// 带自定义logger的监控
	monitor.Monitor(ctx, time.Now(), "functionWithCustomLogger", logzInfof)

	// 空context监控
	monitor.Monitor(nil, time.Now(), "nilContext", logzInfof)

	// 无traceID的context监控
	emptyCtx := context.WithValue(ctx, "other-key", "other-value")
	monitor.Monitor(emptyCtx, time.Now(), "emptyTraceContext", logzInfof)

	fmt.Println()
}

func demonstrateBusinessFunctionMonitoring() {
	fmt.Println("3. 演示业务函数监控:")

	ctx := context.Background()
	userService := &UserService{}

	// 获取用户
	user, err := userService.GetUser(ctx, "user123")
	if err != nil {
		fmt.Printf("   获取用户失败: %v\n", err)
	} else {
		fmt.Printf("   获取用户成功: %s\n", user.Name)
	}

	// 创建用户
	newUser := &User{ID: "user456", Name: "Jane Doe", Email: "jane@example.com"}
	err = userService.CreateUser(ctx, newUser)
	if err != nil {
		fmt.Printf("   创建用户失败: %v\n", err)
	} else {
		fmt.Printf("   创建用户成功: %s\n", newUser.Name)
	}

	fmt.Println()
}

func demonstrateAPIMonitoring() {
	fmt.Println("4. 演示API监控:")

	ctx := context.Background()
	apiHandler := NewAPIHandler()

	// GET API
	user, err := apiHandler.GetUserHandler(ctx, "user789")
	if err != nil {
		fmt.Printf("   GET /api/users/{id} 失败: %v\n", err)
	} else {
		fmt.Printf("   GET /api/users/{id} 成功: %s\n", user.Name)
	}

	// POST API
	newUser := &User{ID: "user999", Name: "Bob Smith", Email: "bob@example.com"}
	err = apiHandler.CreateUserHandler(ctx, newUser)
	if err != nil {
		fmt.Printf("   POST /api/users 失败: %v\n", err)
	} else {
		fmt.Printf("   POST /api/users 成功: %s\n", newUser.Name)
	}

	fmt.Println()
}

func demonstrateDatabaseMonitoring() {
	fmt.Println("5. 演示数据库操作监控:")

	ctx := context.Background()
	dbService := &DatabaseService{}

	// 数据库查询
	results, err := dbService.Query(ctx, "SELECT * FROM users WHERE status = ?", "active")
	if err != nil {
		fmt.Printf("   数据库查询失败: %v\n", err)
	} else {
		fmt.Printf("   数据库查询成功，返回 %d 条记录\n", len(results))
	}

	// 数据库事务
	err = dbService.Transaction(ctx, func(ctx context.Context) error {
		// 模拟事务内的操作
		time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)
		return nil
	})
	if err != nil {
		fmt.Printf("   数据库事务失败: %v\n", err)
	} else {
		fmt.Printf("   数据库事务成功\n")
	}

	fmt.Println()
}

func demonstrateExternalServiceMonitoring() {
	fmt.Println("6. 演示外部服务调用监控:")

	ctx := context.Background()
	externalService := &ExternalService{}

	// 调用外部API
	response, err := externalService.CallAPI(ctx, "https://api.example.com/users", "GET")
	if err != nil {
		fmt.Printf("   调用外部API失败: %v\n", err)
	} else {
		fmt.Printf("   调用外部API成功，响应长度: %d\n", len(response))
	}

	// 调用另一个外部API
	response, err = externalService.CallAPI(ctx, "https://api.example.com/orders", "POST")
	if err != nil {
		fmt.Printf("   调用外部API失败: %v\n", err)
	} else {
		fmt.Printf("   调用外部API成功，响应长度: %d\n", len(response))
	}

	fmt.Println()
}

func demonstrateTracingSystems() {
	fmt.Println("7. 演示不同链路追踪系统:")

	// 自定义 traceID
	ctx1 := createContextWithTraceID("trace-12345")
	monitor.Monitor(ctx1, time.Now(), "customTraceID", logzInfof)

	// Jaeger traceID
	ctx2 := createContextWithJaegerTraceID("1-5759e988-bd862e3fe1be46a994272793")
	monitor.Monitor(ctx2, time.Now(), "jaegerTraceID", logzInfof)

	// Zipkin traceID
	ctx3 := createContextWithZipkinTraceID("bd862e3fe1be46a994272793")
	monitor.Monitor(ctx3, time.Now(), "zipkinTraceID", logzInfof)

	// OpenTelemetry traceparent
	ctx4 := context.WithValue(context.Background(), "traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	monitor.Monitor(ctx4, time.Now(), "openTelemetryTraceID", logzInfof)

	fmt.Println()
}

func demonstrateErrorScenarios() {
	fmt.Println("8. 演示错误场景:")

	ctx := context.Background()

	// 模拟panic场景
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("   捕获到panic: %v\n", r)
			}
		}()

		start := time.Now()
		defer monitor.Monitor(ctx, start, "panicFunction", logzInfof)

		// 模拟panic
		panic("模拟panic场景")
	}()

	// 模拟长时间运行的函数
	start := time.Now()
	defer monitor.Monitor(ctx, start, "longRunningFunction", logzInfof)

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("   长时间运行函数完成\n")

	fmt.Println()
}

func printSummary() {
	fmt.Println("=== 设计优势总结 ===")
	fmt.Println("1. 极简设计：只有一个 Monitor 方法，简洁高效")
	fmt.Println("2. Optional 配置：使用 WithLogger 和 WithCtx 的优雅选项方式")
	fmt.Println("3. 零性能影响：监控逻辑完全异步，不影响原函数执行时间")
	fmt.Println("4. 完美集成：与 logz 系统无缝集成，支持自动 traceID 提取")
	fmt.Println("5. Context 安全：异步执行时 traceID 仍然有效")
	fmt.Println("6. 兜底处理：空 context 或无 traceID 时返回 context.Background()")
	fmt.Println("7. 灵活使用：支持各种监控场景，从简单函数到复杂业务逻辑")
	fmt.Println("8. 易于维护：代码简洁，逻辑清晰，维护成本低")
}
