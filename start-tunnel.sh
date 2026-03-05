#!/bin/bash

# WiseInvest 隧道启动脚本
# 将本地后端（:8080）通过 ngrok 暴露到公网，供 iOS 真机和企微 Webhook 使用

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                                                          ║"
echo "║         WiseInvest 隧道启动脚本 (ngrok)                 ║"
echo "║                                                          ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# ── 检查 ngrok ────────────────────────────────────────────────────────────────
if ! command -v ngrok &> /dev/null; then
    echo -e "${RED}❌ ngrok 未安装${NC}"
    echo ""
    echo -e "${YELLOW}安装方式（任选其一）：${NC}"
    echo -e "  ${GREEN}brew install ngrok/ngrok/ngrok${NC}"
    echo -e "  或前往 https://ngrok.com/download 下载"
    echo ""
    echo -e "${YELLOW}安装后需登录（免费账号即可）：${NC}"
    echo -e "  ${GREEN}ngrok config add-authtoken <你的_token>${NC}"
    exit 1
fi
echo -e "${GREEN}✓ ngrok 已安装 ($(ngrok version 2>/dev/null | head -1))${NC}"

# ── 检查后端是否已启动 ────────────────────────────────────────────────────────
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  后端服务未运行，正在启动...${NC}"
    cd "$(dirname "$0")"
    bash start.sh &
    START_PID=$!

    echo -e "${YELLOW}等待后端就绪...${NC}"
    for i in $(seq 1 20); do
        sleep 2
        if curl -s http://localhost:8080/health > /dev/null 2>&1; then
            echo -e "${GREEN}✓ 后端已就绪${NC}"
            break
        fi
        if [ $i -eq 20 ]; then
            echo -e "${RED}❌ 后端启动超时，请先手动运行 ./start.sh${NC}"
            exit 1
        fi
    done
    cd - > /dev/null
else
    echo -e "${GREEN}✓ 后端服务已在运行${NC}"
fi

echo ""

# ── 创建日志目录 ──────────────────────────────────────────────────────────────
mkdir -p logs

# ── 启动 ngrok 隧道 ───────────────────────────────────────────────────────────
echo -e "${BLUE}🚀 启动 ngrok 隧道（端口 8080）...${NC}"
echo ""

# 以 JSON log 模式后台运行，方便解析公网 URL
nohup ngrok http 8080 --log=stdout --log-format=json > logs/ngrok.log 2>&1 &
NGROK_PID=$!
echo $NGROK_PID > logs/ngrok.pid

# 等待 ngrok API 就绪（本地 4040 端口）
echo -e "${YELLOW}等待隧道建立...${NC}"
for i in $(seq 1 15); do
    sleep 1
    PUBLIC_URL=$(curl -s http://127.0.0.1:4040/api/tunnels 2>/dev/null \
        | grep -o '"public_url":"https://[^"]*"' \
        | head -1 \
        | sed 's/"public_url":"//;s/"//')
    if [ -n "$PUBLIC_URL" ]; then
        break
    fi
    if [ $i -eq 15 ]; then
        echo -e "${RED}❌ 隧道建立超时，请查看日志：logs/ngrok.log${NC}"
        exit 1
    fi
done

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                          ║${NC}"
echo -e "${BLUE}║  ${GREEN}✅ 隧道启动成功！${BLUE}                                    ║${NC}"
echo -e "${BLUE}║                                                          ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}📍 公网地址：${NC}"
echo -e "   ${GREEN}${PUBLIC_URL}${NC}"
echo ""
echo -e "${YELLOW}📱 iOS App 配置（APIConfig.swift）：${NC}"
echo -e "   将 baseURL 改为：${GREEN}${PUBLIC_URL}${NC}"
echo ""
echo -e "${YELLOW}🤖 企微机器人回调地址（如需要）：${NC}"
echo -e "   ${GREEN}${PUBLIC_URL}/api/v1/...${NC}"
echo ""
echo -e "${YELLOW}📊 ngrok 控制台（查看请求详情）：${NC}"
echo -e "   ${GREEN}http://127.0.0.1:4040${NC}"
echo ""
echo -e "${YELLOW}📝 日志：${NC}"
echo -e "   ${GREEN}tail -f logs/ngrok.log${NC}"
echo ""
echo -e "${YELLOW}🛑 停止隧道：${NC}"
echo -e "   ${GREEN}kill $NGROK_PID${NC}  或  ${GREEN}./stop-tunnel.sh${NC}"
echo ""

# ── 同步更新 APIConfig baseURL（可选，需要 sed 支持）────────────────────────
API_CONFIG="ios/WiseInvest/WiseInvest/Data/Network/APIConfig.swift"
if [ -f "$API_CONFIG" ]; then
    echo -e "${YELLOW}🔧 是否自动更新 iOS APIConfig.swift 的 baseURL？(y/N)${NC}"
    read -r -t 10 REPLY || REPLY="n"
    if [[ "$REPLY" =~ ^[Yy]$ ]]; then
        # 备份原文件
        cp "$API_CONFIG" "${API_CONFIG}.bak"
        # 替换 baseURL（匹配形如 http://... 或 https://... 的行）
        sed -i '' "s|static let baseURL.*=.*\"https\?://[^\"]*\"|static let baseURL = \"${PUBLIC_URL}\"|g" "$API_CONFIG"
        echo -e "${GREEN}✓ APIConfig.swift 已更新为 ${PUBLIC_URL}${NC}"
        echo -e "${YELLOW}  原文件已备份为 ${API_CONFIG}.bak${NC}"
    else
        echo -e "${YELLOW}跳过自动更新，请手动将 iOS APIConfig.swift 中的 baseURL 改为：${NC}"
        echo -e "   ${GREEN}${PUBLIC_URL}${NC}"
    fi
fi

echo ""
echo -e "${GREEN}🎉 隧道运行中，Ctrl+C 或 kill $NGROK_PID 可停止${NC}"
echo ""

# 保持脚本前台运行以便查看状态，Ctrl+C 时清理
trap "echo ''; echo -e '${YELLOW}正在停止隧道...${NC}'; kill $NGROK_PID 2>/dev/null; rm -f logs/ngrok.pid; echo -e '${GREEN}隧道已停止${NC}'" EXIT INT TERM

wait $NGROK_PID
