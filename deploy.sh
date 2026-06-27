#!/bin/bash

# ============================================================
# WiseInvest 腾讯云部署脚本（IP 直连模式，无需域名）
# 在腾讯云轻量应用服务器 (Ubuntu 22.04) 上执行
# ============================================================

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || echo "YOUR_SERVER_IP")

echo -e "${GREEN}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║     🚀 WiseInvest 腾讯云一键部署脚本                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# ── Step 1: 系统环境 ──────────────────────────────────────
echo -e "${YELLOW}[1/4] 更新系统 & 安装基础工具...${NC}"
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl git ufw

# ── Step 2: 安装 Docker ──────────────────────────────────
echo -e "${YELLOW}[2/4] 安装 Docker & Docker Compose...${NC}"
if ! command -v docker &> /dev/null; then
    if curl -fsSL https://get.docker.com -o /tmp/get-docker.sh; then
        sudo sh /tmp/get-docker.sh
    else
        echo -e "${YELLOW}⚠️  Docker 官方源连接失败，改用 Ubuntu 软件源安装...${NC}"
        sudo apt install -y docker.io docker-compose
        sudo systemctl enable --now docker
    fi
    sudo usermod -aG docker $USER
    echo -e "${GREEN}✓ Docker 安装完成${NC}"
    echo -e "${RED}⚠️  Docker 组权限需要重新登录才生效${NC}"
    echo -e "${YELLOW}   请执行: exit 重新 SSH 登录后再次运行此脚本${NC}"
    exit 0
else
    echo -e "${GREEN}✓ Docker 已安装 $(docker --version)${NC}"
fi

# Docker Compose (优先 V2，国内网络下退回 Ubuntu 源的 docker-compose)
if sudo docker compose version &> /dev/null; then
    COMPOSE_CMD="sudo docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="sudo docker-compose"
else
    sudo apt install -y docker-compose-plugin || sudo apt install -y docker-compose
    if sudo docker compose version &> /dev/null; then
        COMPOSE_CMD="sudo docker compose"
    else
        COMPOSE_CMD="sudo docker-compose"
    fi
fi
echo -e "${GREEN}✓ Docker Compose 已就绪${NC}"

# ── Step 3: 防火墙 ──────────────────────────────────────
echo -e "${YELLOW}[3/4] 配置防火墙...${NC}"
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw --force enable
echo -e "${GREEN}✓ 防火墙已启用（开放 22/80）${NC}"

# ── Step 4: 启动服务 ────────────────────────────────────
echo -e "${YELLOW}[4/4] 构建 & 启动所有服务...${NC}"

# 检查生产环境配置
if [ ! -f backend/.env.production ]; then
    cp backend/.env.example backend/.env.production
    echo -e "${RED}❌ 请先编辑 backend/.env.production 填入生产配置${NC}"
    echo -e "${YELLOW}   nano backend/.env.production${NC}"
    echo -e "${YELLOW}   然后重新运行: bash deploy.sh${NC}"
    exit 1
fi

# 检查 JWT_SECRET 是否已修改
if grep -q "CHANGE_ME_TO_A_RANDOM_32_CHAR_STRING" backend/.env.production; then
    echo -e "${YELLOW}⚠️  自动生成 JWT_SECRET ...${NC}"
    JWT=$(openssl rand -hex 32)
    sed -i "s/CHANGE_ME_TO_A_RANDOM_32_CHAR_STRING/$JWT/" backend/.env.production
    echo -e "${GREEN}✓ JWT_SECRET 已自动生成${NC}"
fi

# 构建 & 启动（使用 sudo 确保权限）
$COMPOSE_CMD up -d --build

# 等待服务就绪
echo -e "${YELLOW}等待服务启动...${NC}"
MAX_WAIT=120
WAITED=0
while [ $WAITED -lt $MAX_WAIT ]; do
    if curl -s http://localhost/health > /dev/null 2>&1; then
        break
    fi
    sleep 3
    WAITED=$((WAITED + 3))
    echo -ne "\r${YELLOW}  已等待 ${WAITED}s / ${MAX_WAIT}s ...${NC}"
done
echo ""

if [ $WAITED -ge $MAX_WAIT ]; then
    echo -e "${RED}❌ 服务启动超时，请检查日志:${NC}"
    echo -e "${YELLOW}   docker compose logs backend${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗"
echo -e "║  ✅ WiseInvest 部署成功！                                ║"
echo -e "╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}📍 服务地址：${NC}"
echo -e "   API:       ${GREEN}http://${SERVER_IP}/api/v1/${NC}"
echo -e "   健康检查:  ${GREEN}http://${SERVER_IP}/health${NC}"
echo ""
echo -e "${YELLOW}📝 常用命令：${NC}"
echo -e "   查看日志:    ${GREEN}${COMPOSE_CMD} logs -f backend${NC}"
echo -e "   重启服务:    ${GREEN}${COMPOSE_CMD} restart backend${NC}"
echo -e "   停止所有:    ${GREEN}${COMPOSE_CMD} down${NC}"
echo -e "   更新部署:    ${GREEN}${COMPOSE_CMD} up -d --build backend${NC}"
echo -e "   数据库备份:  ${GREEN}docker exec wiseinvest-db pg_dump -U wiseinvest wiseinvest > backup.sql${NC}"
echo ""
echo -e "${YELLOW}📱 iOS App 配置：${NC}"
echo -e "   在 APIConfig.swift 中设置:${NC}"
echo -e "   ${GREEN}static let tunnelURL: String? = \"http://${SERVER_IP}\"${NC}"
echo ""
