#!/bin/bash

# ============================================================
# WiseInvest 腾讯云部署脚本
# 在腾讯云轻量应用服务器 (Ubuntu 22.04) 上执行
# ============================================================

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}"
echo "╔══════════════════════════════════════════════════════════╗"
echo "║     🚀 WiseInvest 腾讯云一键部署脚本                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# ── Step 1: 系统环境 ──────────────────────────────────────
echo -e "${YELLOW}[1/6] 更新系统 & 安装基础工具...${NC}"
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl git ufw

# ── Step 2: 安装 Docker ──────────────────────────────────
echo -e "${YELLOW}[2/6] 安装 Docker & Docker Compose...${NC}"
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com | sudo sh
    sudo usermod -aG docker $USER
    echo -e "${GREEN}✓ Docker 安装完成（请重新登录以生效 docker 组权限）${NC}"
else
    echo -e "${GREEN}✓ Docker 已安装$(docker --version)${NC}"
fi

# Docker Compose (V2 已内置于 Docker)
if ! docker compose version &> /dev/null; then
    sudo apt install -y docker-compose-plugin
fi
echo -e "${GREEN}✓ Docker Compose $(docker compose version --short)${NC}"

# ── Step 3: 防火墙 ──────────────────────────────────────
echo -e "${YELLOW}[3/6] 配置防火墙...${NC}"
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw --force enable
echo -e "${GREEN}✓ 防火墙已启用（开放 22/80/443）${NC}"

# ── Step 4: 拉取代码 ────────────────────────────────────
echo -e "${YELLOW}[4/6] 准备项目代码...${NC}"
PROJECT_DIR=/opt/wiseinvest
if [ ! -d "$PROJECT_DIR" ]; then
    sudo mkdir -p $PROJECT_DIR
    sudo chown $USER:$USER $PROJECT_DIR
    echo -e "${YELLOW}请将项目代码上传到 $PROJECT_DIR${NC}"
    echo -e "${YELLOW}方式一：git clone <your-repo> $PROJECT_DIR${NC}"
    echo -e "${YELLOW}方式二：scp -r ./WiseInvest/* user@server:$PROJECT_DIR/${NC}"
fi

# ── Step 5: SSL 证书 ────────────────────────────────────
echo -e "${YELLOW}[5/6] 申请 SSL 证书...${NC}"
echo -e "${YELLOW}请先确保域名已解析到此服务器 IP${NC}"
echo ""
read -p "请输入你的域名（如 api.wiseinvest.com）: " DOMAIN
read -p "请输入你的邮箱（用于 SSL 证书通知）: " EMAIL

if [ ! -z "$DOMAIN" ] && [ ! -z "$EMAIL" ]; then
    # 替换 Nginx 配置中的域名占位符
    sed -i "s/YOUR_DOMAIN/$DOMAIN/g" $PROJECT_DIR/nginx/conf.d/wiseinvest.conf

    # 先用 HTTP 模式启动 Nginx 以通过 ACME 验证
    mkdir -p $PROJECT_DIR/certbot/conf $PROJECT_DIR/certbot/www

    # 首次申请证书（使用 standalone 模式）
    sudo docker run --rm \
        -v $PROJECT_DIR/certbot/conf:/etc/letsencrypt \
        -v $PROJECT_DIR/certbot/www:/var/www/certbot \
        -p 80:80 \
        certbot/certbot certonly \
        --standalone \
        --email $EMAIL \
        --agree-tos \
        --no-eff-email \
        -d $DOMAIN

    echo -e "${GREEN}✓ SSL 证书申请成功${NC}"
else
    echo -e "${YELLOW}⚠️  跳过 SSL 配置，稍后可手动执行${NC}"
fi

# ── Step 6: 启动服务 ────────────────────────────────────
echo -e "${YELLOW}[6/6] 启动所有服务...${NC}"
cd $PROJECT_DIR

# 检查生产环境配置
if [ ! -f backend/.env.production ]; then
    cp backend/.env.example backend/.env.production
    echo -e "${RED}❌ 请先编辑 $PROJECT_DIR/backend/.env.production 填入生产配置${NC}"
    exit 1
fi

# 构建 & 启动
docker compose up -d --build

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗"
echo -e "║  ✅ WiseInvest 部署成功！                                ║"
echo -e "╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}📍 服务地址：${NC}"
echo -e "   API:    ${GREEN}https://$DOMAIN${NC}"
echo -e "   健康检查: ${GREEN}https://$DOMAIN/health${NC}"
echo ""
echo -e "${YELLOW}📝 常用命令：${NC}"
echo -e "   查看日志:    ${GREEN}docker compose logs -f backend${NC}"
echo -e "   重启服务:    ${GREEN}docker compose restart backend${NC}"
echo -e "   停止所有:    ${GREEN}docker compose down${NC}"
echo -e "   更新部署:    ${GREEN}docker compose up -d --build backend${NC}"
echo -e "   数据库备份:  ${GREEN}docker exec wiseinvest-db pg_dump -U wiseinvest wiseinvest > backup.sql${NC}"
echo ""
