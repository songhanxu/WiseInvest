#!/bin/bash

# WiseInvest 诊断工具
# 用于快速诊断环境问题

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                                                          ║"
echo "║           🔍 WiseInvest 诊断工具                         ║"
echo "║                                                          ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo ""

# 1. 检查 Go
echo -e "${YELLOW}1. 检查 Go 环境${NC}"
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    echo -e "${GREEN}✓ Go 已安装: $GO_VERSION${NC}"
else
    echo -e "${RED}✗ Go 未安装${NC}"
    echo -e "${YELLOW}  安装命令: brew install go${NC}"
fi
echo ""

# 2. 检查 PostgreSQL
echo -e "${YELLOW}2. 检查 PostgreSQL${NC}"
if command -v psql &> /dev/null; then
    PSQL_VERSION=$(psql --version)
    echo -e "${GREEN}✓ PostgreSQL 已安装: $PSQL_VERSION${NC}"
    
    # 检查服务状态
    if pg_isready -h localhost -p 5432 &> /dev/null; then
        echo -e "${GREEN}✓ PostgreSQL 正在运行${NC}"
        
        # 检查数据库
        if psql -h localhost -U wiseinvest -d wiseinvest -c "SELECT 1;" &> /dev/null; then
            echo -e "${GREEN}✓ 数据库 wiseinvest 可访问${NC}"
        else
            echo -e "${RED}✗ 数据库 wiseinvest 不可访问${NC}"
            echo -e "${YELLOW}  修复命令: cd backend && make db-init${NC}"
        fi
    else
        echo -e "${RED}✗ PostgreSQL 未运行${NC}"
        echo -e "${YELLOW}  启动命令: brew services start postgresql@15${NC}"
    fi
else
    echo -e "${RED}✗ PostgreSQL 未安装${NC}"
    echo -e "${YELLOW}  安装命令: brew install postgresql@15${NC}"
fi
echo ""

# 3. 检查 Redis
echo -e "${YELLOW}3. 检查 Redis${NC}"
if command -v redis-cli &> /dev/null; then
    REDIS_VERSION=$(redis-cli --version)
    echo -e "${GREEN}✓ Redis 已安装: $REDIS_VERSION${NC}"
    
    # 检查服务状态
    if redis-cli ping &> /dev/null; then
        echo -e "${GREEN}✓ Redis 正在运行${NC}"
    else
        echo -e "${RED}✗ Redis 未运行${NC}"
        echo -e "${YELLOW}  启动命令: brew services start redis${NC}"
    fi
else
    echo -e "${RED}✗ Redis 未安装${NC}"
    echo -e "${YELLOW}  安装命令: brew install redis${NC}"
fi
echo ""

# 4. 检查端口占用
echo -e "${YELLOW}4. 检查端口占用${NC}"
if lsof -i :8080 &> /dev/null; then
    echo -e "${YELLOW}⚠️  端口 8080 已被占用${NC}"
    lsof -i :8080
    echo -e "${YELLOW}  停止命令: ./stop.sh${NC}"
else
    echo -e "${GREEN}✓ 端口 8080 可用${NC}"
fi
echo ""

# 5. 检查配置文件
echo -e "${YELLOW}5. 检查配置文件${NC}"
if [ -f backend/.env ]; then
    echo -e "${GREEN}✓ .env 文件存在${NC}"
    
    # 检查必需的配置项
    if grep -q "OPENAI_API_KEY=sk-" backend/.env; then
        echo -e "${GREEN}✓ OPENAI_API_KEY 已配置${NC}"
    else
        echo -e "${RED}✗ OPENAI_API_KEY 未配置或格式错误${NC}"
        echo -e "${YELLOW}  请编辑 backend/.env 文件${NC}"
    fi
    
    # 显示配置（隐藏敏感信息）
    echo -e "${BLUE}配置摘要:${NC}"
    grep -v "API_KEY\|PASSWORD" backend/.env | grep "=" | while read line; do
        echo "  $line"
    done
else
    echo -e "${RED}✗ .env 文件不存在${NC}"
    echo -e "${YELLOW}  创建命令: cd backend && cp .env.example .env${NC}"
fi
echo ""

# 6. 检查 Go 依赖
echo -e "${YELLOW}6. 检查 Go 依赖${NC}"
if [ -f backend/go.mod ]; then
    echo -e "${GREEN}✓ go.mod 文件存在${NC}"
    
    if [ -f backend/go.sum ]; then
        echo -e "${GREEN}✓ go.sum 文件存在${NC}"
    else
        echo -e "${YELLOW}⚠️  go.sum 文件不存在${NC}"
        echo -e "${YELLOW}  修复命令: cd backend && go mod download && go mod tidy${NC}"
    fi
else
    echo -e "${RED}✗ go.mod 文件不存在${NC}"
fi
echo ""

# 7. 检查后端服务
echo -e "${YELLOW}7. 检查后端服务${NC}"
if curl -s http://localhost:8080/health &> /dev/null; then
    HEALTH=$(curl -s http://localhost:8080/health)
    echo -e "${GREEN}✓ 后端服务正在运行${NC}"
    echo -e "${BLUE}健康检查响应: $HEALTH${NC}"
else
    echo -e "${YELLOW}⚠️  后端服务未运行${NC}"
    echo -e "${YELLOW}  启动命令: ./start.sh${NC}"
fi
echo ""

# 8. 检查日志文件
echo -e "${YELLOW}8. 检查日志文件${NC}"
if [ -d logs ]; then
    echo -e "${GREEN}✓ logs 目录存在${NC}"
    
    if [ -f logs/backend.log ]; then
        LOG_SIZE=$(du -h logs/backend.log | cut -f1)
        echo -e "${GREEN}✓ backend.log 存在 (大小: $LOG_SIZE)${NC}"
        
        # 显示最后几行日志
        echo -e "${BLUE}最近的日志:${NC}"
        tail -5 logs/backend.log 2>/dev/null | sed 's/^/  /'
    else
        echo -e "${YELLOW}⚠️  backend.log 不存在${NC}"
    fi
    
    if [ -f logs/backend.pid ]; then
        PID=$(cat logs/backend.pid)
        if ps -p $PID > /dev/null; then
            echo -e "${GREEN}✓ 后端进程运行中 (PID: $PID)${NC}"
        else
            echo -e "${YELLOW}⚠️  PID 文件存在但进程未运行${NC}"
        fi
    fi
else
    echo -e "${YELLOW}⚠️  logs 目录不存在${NC}"
    mkdir -p logs
    echo -e "${GREEN}✓ 已创建 logs 目录${NC}"
fi
echo ""

# 9. 系统信息
echo -e "${YELLOW}9. 系统信息${NC}"
echo -e "${BLUE}操作系统: $(uname -s)${NC}"
echo -e "${BLUE}架构: $(uname -m)${NC}"
echo -e "${BLUE}内核版本: $(uname -r)${NC}"
echo ""

# 10. 磁盘空间
echo -e "${YELLOW}10. 磁盘空间${NC}"
DISK_USAGE=$(df -h . | tail -1 | awk '{print $5}')
echo -e "${BLUE}当前目录磁盘使用率: $DISK_USAGE${NC}"
echo ""

# 总结
echo -e "${BLUE}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                    诊断总结                              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

# 统计问题
ISSUES=0

if ! command -v go &> /dev/null; then ((ISSUES++)); fi
if ! command -v psql &> /dev/null; then ((ISSUES++)); fi
if ! command -v redis-cli &> /dev/null; then ((ISSUES++)); fi
if ! pg_isready -h localhost -p 5432 &> /dev/null; then ((ISSUES++)); fi
if ! redis-cli ping &> /dev/null; then ((ISSUES++)); fi
if [ ! -f backend/.env ]; then ((ISSUES++)); fi

if [ $ISSUES -eq 0 ]; then
    echo -e "${GREEN}✅ 所有检查通过！环境配置正常。${NC}"
    echo ""
    echo -e "${YELLOW}下一步:${NC}"
    echo -e "  1. 运行 ${GREEN}./start.sh${NC} 启动服务"
    echo -e "  2. 访问 ${GREEN}http://localhost:8080/health${NC} 验证"
else
    echo -e "${YELLOW}⚠️  发现 $ISSUES 个问题需要修复${NC}"
    echo ""
    echo -e "${YELLOW}建议操作:${NC}"
    echo -e "  1. 查看上面的错误信息"
    echo -e "  2. 按照提示修复问题"
    echo -e "  3. 重新运行诊断: ${GREEN}./diagnose.sh${NC}"
    echo -e "  4. 查看详细文档: ${GREEN}cat TROUBLESHOOTING.md${NC}"
fi

echo ""
echo -e "${BLUE}📚 相关文档:${NC}"
echo -e "  - 安装指南: ${GREEN}INSTALL.md${NC}"
echo -e "  - 快速启动: ${GREEN}QUICKSTART.md${NC}"
echo -e "  - 故障排除: ${GREEN}TROUBLESHOOTING.md${NC}"
echo ""
