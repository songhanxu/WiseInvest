#!/bin/bash
# ============================================================
# WiseInvest - ngrok 公网隧道
#
# 首次使用前请配置 token（只需一次）：
#   1. 注册: https://dashboard.ngrok.com/signup
#   2. 获取 token: https://dashboard.ngrok.com/get-started/your-authtoken
#   3. 运行: ngrok config add-authtoken 你的token
# ============================================================

LOCAL_PORT=8080

# 检查后端是否已运行
if ! lsof -i :$LOCAL_PORT -sTCP:LISTEN -t > /dev/null 2>&1; then
    echo "⚠️  后端服务未运行，请先启动后端："
    echo "   cd ~/WiseInvest/backend && ./server"
    echo ""
fi

echo "🚀 启动 ngrok 公网隧道..."
echo "============================================================"
echo "地址就绪后，将 https://xxxx.ngrok-free.app 填入 APIConfig.swift："
echo "  static let tunnelURL: String? = \"https://xxxx.ngrok-free.app\""
echo "============================================================"
echo ""

ngrok http $LOCAL_PORT
