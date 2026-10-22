#!/bin/bash

# WiseInvest 一键启动脚本
# 用于快速启动整个项目（不使用 Docker）

set -e

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                                                          ║"
echo "║              🚀 WiseInvest 启动脚本                      ║"
echo "║                                                          ║"
echo "║     智能加密货币投资助手 - 一键启动                      ║"
echo "║                                                          ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo ""

# 检查环境
echo -e "${YELLOW}📋 检查环境...${NC}"

# 检查 Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go 未安装，请先安装 Go 1.21+${NC}"
    echo -e "${YELLOW}   安装命令: brew install go${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go 已安装 ($(go version))${NC}"

# 检查 PostgreSQL
if ! command -v psql &> /dev/null; then
    echo -e "${RED}❌ PostgreSQL 未安装${NC}"
    echo -e "${YELLOW}   安装命令: brew install postgresql@15${NC}"
    exit 1
fi
echo -e "${GREEN}✓ PostgreSQL 已安装${NC}"

# 检查 Redis
if ! command -v redis-cli &> /dev/null; then
    echo -e "${RED}❌ Redis 未安装${NC}"
    echo -e "${YELLOW}   安装命令: brew install redis${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Redis 已安装${NC}"

echo ""

# 检查服务状态
echo -e "${YELLOW}🔍 检查服务状态...${NC}"

# 检查 PostgreSQL 是否运行
if ! pg_isready -h localhost -p 5432 &> /dev/null; then
    echo -e "${YELLOW}⚠️  PostgreSQL 未运行，正在启动...${NC}"
    brew services start postgresql@15
    sleep 3
    
    if ! pg_isready -h localhost -p 5432 &> /dev/null; then
        echo -e "${RED}❌ PostgreSQL 启动失败${NC}"
        exit 1
    fi
fi
echo -e "${GREEN}✓ PostgreSQL 正在运行${NC}"

# 检查 Redis 是否运行
if ! redis-cli ping &> /dev/null; then
    echo -e "${YELLOW}⚠️  Redis 未运行，正在启动...${NC}"
    brew services start redis
    sleep 2
    
    if ! redis-cli ping &> /dev/null; then
        echo -e "${RED}❌ Redis 启动失败${NC}"
        exit 1
    fi
fi
echo -e "${GREEN}✓ Redis 正在运行${NC}"

echo ""

# 进入后端目录
cd backend

# 检查 .env 文件
if [ ! -f .env ]; then
    echo -e "${YELLOW}📝 创建 .env 文件...${NC}"
    cp .env.example .env
    echo -e "${YELLOW}⚠️  请编辑 backend/.env 文件，填入你的 OPENAI_API_KEY${NC}"
    echo -e "${YELLOW}   然后重新运行此脚本${NC}"
    exit 1
fi

# 检查 OPENAI_API_KEY 是否已配置（排除占位符）
OPENAI_KEY=$(grep "^OPENAI_API_KEY=" .env | cut -d'=' -f2)
if [ -z "$OPENAI_KEY" ] || [ "$OPENAI_KEY" = "your_openai_or_deepseek_api_key_here" ]; then
    echo -e "${RED}❌ 请在 backend/.env 文件中配置 OPENAI_API_KEY${NC}"
    echo -e "${YELLOW}   编辑 backend/.env，将 OPENAI_API_KEY 设置为你的 API Key${NC}"
    echo -e "${YELLOW}   支持 OpenAI (sk-...) 或 DeepSeek API Key${NC}"
    exit 1
fi

echo -e "${GREEN}✓ 配置文件检查通过${NC}"
echo ""

# 初始化数据库
echo -e "${YELLOW}🗄️  初始化数据库...${NC}"

# 检查数据库是否存在
if ! psql -h localhost -U wiseinvest -d wiseinvest -c "SELECT 1" &> /dev/null; then
    echo -e "${YELLOW}创建数据库用户和数据库...${NC}"
    
    # 创建用户（如果不存在）
    psql postgres -c "CREATE USER wiseinvest WITH PASSWORD 'wiseinvest';" 2>/dev/null || true
    
    # 创建数据库
    psql postgres -c "CREATE DATABASE wiseinvest OWNER wiseinvest;" 2>/dev/null || true
    
    # 授权
    psql postgres -c "GRANT ALL PRIVILEGES ON DATABASE wiseinvest TO wiseinvest;" 2>/dev/null || true
    
    echo -e "${GREEN}✓ 数据库创建成功${NC}"
else
    echo -e "${GREEN}✓ 数据库已存在${NC}"
fi

echo ""

# 安装 Go 依赖
echo -e "${YELLOW}📦 安装 Go 依赖...${NC}"
go mod download
go mod tidy
echo -e "${GREEN}✓ 依赖安装完成${NC}"

echo ""

# 启动后端服务
echo -e "${BLUE}🚀 启动后端服务...${NC}"
echo ""

# 创建日志目录
mkdir -p ../logs

# 在后台启动
nohup go run cmd/server/main.go > ../logs/backend.log 2>&1 &
BACKEND_PID=$!

# 等待服务启动（最多等待 60 秒）
echo -e "${YELLOW}等待服务启动（首次启动需编译，可能较慢）...${NC}"
MAX_WAIT=60
WAITED=0
while [ $WAITED -lt $MAX_WAIT ]; do
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 后端服务启动成功 (PID: $BACKEND_PID)，耗时 ${WAITED}s${NC}"
        break
    fi
    sleep 2
    WAITED=$((WAITED + 2))
    echo -ne "\r${YELLOW}  已等待 ${WAITED}s / ${MAX_WAIT}s ...${NC}"
done

if [ $WAITED -ge $MAX_WAIT ]; then
    echo ""
    echo -e "${RED}❌ 后端服务启动超时（${MAX_WAIT}s），请查看日志: logs/backend.log${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                          ║${NC}"
echo -e "${BLUE}║  ${GREEN}✅ WiseInvest 启动成功！${BLUE}                              ║${NC}"
echo -e "${BLUE}║                                                          ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

echo -e "${YELLOW}📍 服务地址：${NC}"
echo -e "   后端 API:  ${GREEN}http://localhost:8080${NC}"
echo -e "   健康检查:  ${GREEN}http://localhost:8080/health${NC}"
echo -e "   PostgreSQL: ${GREEN}localhost:5432${NC}"
echo -e "   Redis:      ${GREEN}localhost:6379${NC}"
echo ""

echo -e "${YELLOW}📝 后端进程信息：${NC}"
echo -e "   PID: ${GREEN}$BACKEND_PID${NC}"
echo -e "   日志: ${GREEN}logs/backend.log${NC}"
echo ""

echo -e "${YELLOW}🧪 测试 API：${NC}"
echo -e "   ${GREEN}curl http://localhost:8080/health${NC}"
echo -e "   ${GREEN}curl http://localhost:8080/api/v1/agents${NC}"
echo ""

echo -e "${YELLOW}📱 启动 iOS 应用：${NC}"
echo -e "   ${GREEN}cd ../ios${NC}"
echo -e "   ${GREEN}open WiseInvest.xcodeproj${NC}"
echo ""

echo -e "${YELLOW}📚 查看日志：${NC}"
echo -e "   ${GREEN}tail -f logs/backend.log${NC}"
echo ""

echo -e "${YELLOW}🛑 停止服务：${NC}"
echo -e "   ${GREEN}kill $BACKEND_PID${NC}"
echo -e "   或运行: ${GREEN}./stop.sh${NC}"
echo ""

# 保存 PID 到文件
echo $BACKEND_PID > ../logs/backend.pid

echo -e "${GREEN}🎉 开始使用 WiseInvest！${NC}"
echo ""
