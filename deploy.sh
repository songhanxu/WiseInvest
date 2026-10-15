#!/bin/bash

# ============================================================
# WiseInvest 腾讯云部署脚本（IP 直连模式，无需域名）
# 在腾讯云轻量应用服务器 (Ubuntu 22.04) 上执行
# ============================================================

set -e

PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

# 已运行的实例共用日常更新流程，避免两个入口的兼容逻辑再次分叉。
if [ -f backend/.env.production ] && command -v docker >/dev/null; then
    DOCKER_CHECK=(docker)
    if ! docker info >/dev/null 2>&1 && command -v sudo >/dev/null; then
        DOCKER_CHECK=(sudo docker)
    fi
    RUNNING_SERVICES=$("${DOCKER_CHECK[@]}" inspect --format '{{.State.Running}}' \
        wiseinvest-db wiseinvest-redis wiseinvest-nginx 2>/dev/null) || RUNNING_SERVICES=""
    if [[ "$RUNNING_SERVICES" == $'true\ntrue\ntrue' ]]; then
        echo '检测到已有服务，切换到后端更新流程…'
        exec bash "$PROJECT_DIR/deploy-update.sh"
    fi
fi

TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-120}"
if ! [[ "$TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
    echo 'DEPLOY_HEALTH_TIMEOUT 必须是正整数（秒）' >&2
    exit 1
fi

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

SERVER_IP=$(curl -fsS --connect-timeout 3 --max-time 5 https://api.ipify.org 2>/dev/null || echo "YOUR_SERVER_IP")

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
LEGACY_COMPOSE=false
if sudo docker compose version &> /dev/null; then
    COMPOSE_CMD="sudo docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="sudo docker-compose"
    LEGACY_COMPOSE=true
else
    sudo apt install -y docker-compose-plugin || sudo apt install -y docker-compose
    if sudo docker compose version &> /dev/null; then
        COMPOSE_CMD="sudo docker compose"
    else
        COMPOSE_CMD="sudo docker-compose"
        LEGACY_COMPOSE=true
    fi
fi
# 使用显式配置路径，避免 COMPOSE_FILE 指向其他项目。
COMPOSE_ARGS=(-f "$PROJECT_DIR/docker-compose.yml" --project-directory "$PROJECT_DIR")
echo -e "${GREEN}✓ Docker Compose 已就绪${NC}"

# ── Docker Hub 镜像加速 ──────────────────────────────────
# 国内云服务器直连 Docker Hub 经常超时，优先配置腾讯云内网镜像。
if [ ! -f /etc/docker/daemon.json ] || ! sudo grep -q "registry-mirrors" /etc/docker/daemon.json; then
    echo -e "${YELLOW}配置 Docker 镜像加速...${NC}"
    sudo mkdir -p /etc/docker
    sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'
{
  "registry-mirrors": [
    "https://mirror.ccs.tencentyun.com"
  ]
}
EOF
    sudo systemctl daemon-reload
    sudo systemctl restart docker
    echo -e "${GREEN}✓ Docker 镜像加速已配置${NC}"
fi

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

# 先完成构建；构建失败时不停止现有后端。
$COMPOSE_CMD "${COMPOSE_ARGS[@]}" build backend
if [[ "$LEGACY_COMPOSE" == true ]]; then
    echo -e "${YELLOW}旧版 Compose：清理旧后端容器，避开 ContainerConfig 兼容问题...${NC}"
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" rm -s -f backend
    # 保留已有数据库、Redis 和 Nginx，避免旧 Compose 重建它们。
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" up -d --no-recreate
else
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" up -d
fi

wait_for_health() {
    local url=$1 deadline=$((SECONDS + TIMEOUT))
    while (( SECONDS < deadline )); do
        if curl -fsS --noproxy '*' --connect-timeout 2 --max-time 3 "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    echo -e "${RED}❌ 健康检查超时：${url}${NC}" >&2
    return 1
}

deployment_failed() {
    local status=$?
    trap - ERR
    echo -e "${RED}❌ 部署失败，后端最近日志：${NC}" >&2
    $COMPOSE_CMD "${COMPOSE_ARGS[@]}" logs --tail=80 backend >&2 || true
    exit "$status"
}
trap deployment_failed ERR

echo -e "${YELLOW}等待后端启动...${NC}"
wait_for_health http://127.0.0.1:8080/health
$COMPOSE_CMD "${COMPOSE_ARGS[@]}" exec -T nginx nginx -t
$COMPOSE_CMD "${COMPOSE_ARGS[@]}" exec -T nginx nginx -s reload
wait_for_health http://127.0.0.1/health

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
echo -e "   更新部署:    ${GREEN}bash deploy-update.sh${NC}"
echo -e "   数据库备份:  ${GREEN}docker exec wiseinvest-db pg_dump -U wiseinvest wiseinvest > backup.sql${NC}"
echo ""
echo -e "${YELLOW}📱 iOS App 配置：${NC}"
echo -e "   在 APIConfig.swift 中设置:${NC}"
echo -e "   ${GREEN}static let tunnelURL: String? = \"http://${SERVER_IP}\"${NC}"
echo ""
