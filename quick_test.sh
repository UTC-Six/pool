#!/bin/bash
# Worker Pool 快速验证脚本
# 作者: carey
# 用途: 快速验证 Worker Pool 的核心功能和稳定性

echo "🚀 开始 Worker Pool 快速验证..."
echo "测试环境: $(go version)"
echo "CPU核心数: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo "未知")"
echo ""

# 进入worker_pool目录
cd worker_pool || {
    echo "❌ 找不到 worker_pool 目录"
    exit 1
}

# 基础功能测试
echo "1. 基础功能测试..."
if go test -v -run TestBasicFunctionality; then
    echo "✅ 基础功能测试通过"
else
    echo "❌ 基础功能测试失败"
    exit 1
fi
echo ""

# 并发安全测试  
echo "2. 并发安全测试..."
if go test -race -run TestRaceConditions; then
    echo "✅ 并发安全测试通过"
else
    echo "❌ 并发安全测试失败"
    exit 1
fi
echo ""

# 性能测试
echo "3. 性能测试..."
if go test -v -run TestHighLoad; then
    echo "✅ 性能测试通过"
else
    echo "❌ 性能测试失败"
    exit 1
fi
echo ""

# 资源泄露测试
echo "4. 资源泄露测试..."
if go test -v -run TestMemoryLeak; then
    echo "✅ 资源泄露测试通过"
else
    echo "❌ 资源泄露测试失败"
    exit 1
fi
echo ""

# CoreWorkers功能测试
echo "5. CoreWorkers功能测试..."
if go test -v -run TestCoreWorkersAdjustment; then
    echo "✅ CoreWorkers功能测试通过"
else
    echo "❌ CoreWorkers功能测试失败"
    exit 1
fi
echo ""

# 边界情况测试
echo "6. 边界情况测试..."
if go test -v -run TestEdgeCases; then
    echo "✅ 边界情况测试通过"
else
    echo "❌ 边界情况测试失败"
    exit 1
fi
echo ""

echo "🎉 所有核心测试通过！"
echo ""
echo "📊 快速基准测试..."
go test -bench=BenchmarkTaskSubmission -benchtime=1s
echo ""

echo "✅ Worker Pool 验证完成，可以安全使用！"
echo ""
echo "💡 完整测试套件运行命令:"
echo "   go test -v                    # 运行所有测试"
echo "   go test -race                 # 竞态条件检测"  
echo "   go test -bench=.              # 性能基准测试"
echo "   go run cmd/stress_demo.go     # 压力测试演示"
echo "   go run cmd/main.go            # 功能演示"
