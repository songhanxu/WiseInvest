#!/bin/bash

# WiseInvest API 测试脚本
# 用于快速测试所有 API 端点

set -e

BASE_URL="http://localhost:8080"
API_URL="${BASE_URL}/api/v1"

echo "🚀 WiseInvest API 测试脚本"
echo "=========================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试函数
test_endpoint() {
    local name=$1
    local method=$2
    local endpoint=$3
    local data=$4
    
    echo -e "${YELLOW}测试: ${name}${NC}"
    echo "请求: ${method} ${endpoint}"
    
    if [ -z "$data" ]; then
        response=$(curl -s -X ${method} "${endpoint}")
    else
        response=$(curl -s -X ${method} "${endpoint}" \
            -H "Content-Type: application/json" \
            -d "${data}")
    fi
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ 成功${NC}"
        echo "响应: ${response}" | jq '.' 2>/dev/null || echo "${response}"
    else
        echo -e "${RED}✗ 失败${NC}"
    fi
    echo ""
}

# 1. 健康检查
test_endpoint "健康检查" "GET" "${BASE_URL}/health"

# 2. 获取可用 Agent
test_endpoint "获取可用 Agent" "GET" "${API_URL}/agents"

# 3. 创建对话
echo -e "${YELLOW}创建对话...${NC}"
conversation_response=$(curl -s -X POST "${API_URL}/conversations" \
    -H "Content-Type: application/json" \
    -d '{
        "user_id": 1,
        "agent_type": "investment_advisor",
        "title": "API 测试对话"
    }')

conversation_id=$(echo $conversation_response | jq -r '.id')
echo -e "${GREEN}✓ 对话创建成功${NC}"
echo "对话 ID: ${conversation_id}"
echo "响应: ${conversation_response}" | jq '.'
echo ""

# 4. 获取对话详情
test_endpoint "获取对话详情" "GET" "${API_URL}/conversations/${conversation_id}"

# 5. 发送消息
test_endpoint "发送消息" "POST" "${API_URL}/messages" \
    "{
        \"conversation_id\": ${conversation_id},
        \"content\": \"帮我分析一下 BTC 的投资风险\"
    }"

# 6. 获取用户对话列表
test_endpoint "获取用户对话列表" "GET" "${API_URL}/conversations/user/1"

# 7. 流式消息测试
echo -e "${YELLOW}测试流式消息...${NC}"
echo "请求: POST ${API_URL}/messages/stream"
curl -X POST "${API_URL}/messages/stream" \
    -H "Content-Type: application/json" \
    -d "{
        \"conversation_id\": ${conversation_id},
        \"content\": \"当前市场适合投资吗？\"
    }" \
    --no-buffer

echo ""
echo -e "${GREEN}✓ 流式消息测试完成${NC}"
echo ""

# 8. 删除对话
echo -e "${YELLOW}清理测试数据...${NC}"
curl -s -X DELETE "${API_URL}/conversations/${conversation_id}" > /dev/null
echo -e "${GREEN}✓ 测试对话已删除${NC}"
echo ""

echo "=========================="
echo -e "${GREEN}🎉 所有测试完成！${NC}"
