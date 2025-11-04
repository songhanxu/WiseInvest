#!/bin/bash

# WiseInvest 停止脚本

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}🛑 停止 WiseInvest 服务...${NC}"
echo ""

# 检查 PID 文件
if [ -f logs/backend.pid ]; then
    BACKEND_PID=$(cat logs/backend.pid)
    
    # 检查进程是否存在
    if ps -p $BACKEND_PID > /dev/null; then
        echo -e "${YELLOW}停止后端服务 (PID: $BACKEND_PID)...${NC}"
        kill $BACKEND_PID
        echo -e "${GREEN}✓ 后端服务已停止${NC}"
    else
        echo -e "${YELLOW}⚠️  后端服务未运行${NC}"
    fi
    
    # 删除 PID 文件
    rm logs/backend.pid
else
    echo -e "${YELLOW}⚠️  未找到 PID 文件，尝试查找进程...${NC}"
    
    # 查找并停止所有相关进程
    pkill -f "go run cmd/server/main.go" || true
    echo -e "${GREEN}✓ 已尝试停止所有相关进程${NC}"
fi

echo ""
echo -e "${GREEN}✅ WiseInvest 服务已停止${NC}"
echo ""

echo -e "${YELLOW}💡 提示：${NC}"
echo -e "   PostgreSQL 和 Redis 仍在运行"
echo -e "   如需停止它们，请运行："
echo -e "   ${GREEN}brew services stop postgresql@15${NC}"
echo -e "   ${GREEN}brew services stop redis${NC}"
echo ""
