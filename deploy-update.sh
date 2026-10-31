#!/usr/bin/env bash
# 已部署环境的后端更新。先 git pull，再运行 bash deploy-update.sh。
set -Eeuo pipefail

PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

fail() { printf '错误：%s\n' "$*" >&2; exit 1; }

DOMAIN="${WISEINVEST_DOMAIN:-hanxusong.asia}"
CERT_FILE="/home/ubuntu/ssl/hanxusong.asia_bundle.pem"
KEY_FILE="/home/ubuntu/ssl/hanxusong.asia.key"

[[ -f docker-compose.yml ]] || fail '找不到 docker-compose.yml'
[[ -f backend/.env.production ]] || fail '请先配置 backend/.env.production'
[[ -f "$CERT_FILE" ]] || fail "找不到 SSL 证书：$CERT_FILE"
[[ -f "$KEY_FILE" ]] || fail "找不到 SSL 私钥：$KEY_FILE"
command -v curl >/dev/null || fail '需要安装 curl'
command -v docker >/dev/null || fail '需要先安装并启动 Docker'

TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-120}"
[[ "$TIMEOUT" =~ ^[1-9][0-9]*$ ]] || fail 'DEPLOY_HEALTH_TIMEOUT 必须是正整数（秒）'

DOCKER=(docker)
USE_SUDO=false
if ! docker info >/dev/null 2>&1; then
    command -v sudo >/dev/null || fail '无法访问 Docker，请检查权限及服务状态'
    DOCKER=(sudo docker)
    USE_SUDO=true
fi
"${DOCKER[@]}" info >/dev/null 2>&1 || fail '无法连接 Docker 服务'

LEGACY_COMPOSE=false
if "${DOCKER[@]}" compose version >/dev/null 2>&1; then
    COMPOSE=("${DOCKER[@]}" compose)
elif command -v docker-compose >/dev/null; then
    COMPOSE=(docker-compose)
    if [[ "$USE_SUDO" == true ]]; then COMPOSE=(sudo docker-compose); fi
    "${COMPOSE[@]}" version >/dev/null 2>&1 || fail 'Docker Compose 不可用'
    LEGACY_COMPOSE=true
else
    fail '需要安装 Docker Compose V2 或 docker-compose V1'
fi
# 固定项目和配置文件，避免 COMPOSE_FILE 等环境变量选中其他服务。
COMPOSE+=(-f "$PROJECT_DIR/docker-compose.yml" --project-directory "$PROJECT_DIR")

for service in postgres redis nginx; do
    container_id="$("${COMPOSE[@]}" ps -q "$service")"
    [[ -n "$container_id" ]] || fail "$service 尚未启动；此脚本用于更新已部署的服务"
    running="$("${DOCKER[@]}" inspect --format '{{.State.Running}}' "$container_id")"
    [[ "$running" == true ]] || fail "$service 未运行，请先恢复该服务"
done

report_failure() {
    local status=$?
    trap - ERR
	printf '\n部署失败，请检查后端和 Nginx 日志：\n' >&2
	"${COMPOSE[@]}" logs --tail=80 backend nginx >&2 || true
    exit "$status"
}
trap report_failure ERR

wait_for_health() {
    local url=$1 deadline=$((SECONDS + TIMEOUT))
    while (( SECONDS < deadline )); do
        if curl -fsS --noproxy '*' --connect-timeout 2 --max-time 3 "$url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    printf '健康检查超时：%s\n' "$url" >&2
    return 1
}

wait_for_https() {
	local deadline=$((SECONDS + TIMEOUT))
	while (( SECONDS < deadline )); do
		if curl -fsS --noproxy '*' --resolve "$DOMAIN:443:127.0.0.1" --connect-timeout 2 --max-time 3 "https://$DOMAIN/health" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done
	printf 'HTTPS 健康检查超时：https://%s/health\n' "$DOMAIN" >&2
	return 1
}

if command -v ufw >/dev/null 2>&1; then
	if [[ "$EUID" -eq 0 ]]; then ufw allow 443/tcp >/dev/null; else sudo ufw allow 443/tcp >/dev/null; fi
fi

printf '[1/5] 构建后端镜像…\n'
"${COMPOSE[@]}" build backend

printf '[2/5] 更新后端容器，加载生产环境配置…\n'
if [[ "$LEGACY_COMPOSE" == true ]]; then
    # Compose 1.29.2 的 recreate 会读取新镜像中已缺失的 ContainerConfig。
    # 按项目/服务清理，也能匹配失败后被改成 <ID>_wiseinvest-api 的旧容器。
    # 后端无持久化卷；不传 -v，不触碰 PostgreSQL、Redis 或配置文件。
    printf '使用旧版 Compose：先清理旧后端容器，避开 ContainerConfig 兼容问题…\n'
    "${COMPOSE[@]}" rm -s -f backend
fi
"${COMPOSE[@]}" up -d --no-deps --force-recreate backend

printf '[3/5] 等待后端健康检查（最多 %s 秒）…\n' "$TIMEOUT"
wait_for_health http://127.0.0.1:8080/health

printf '[4/5] 使用新证书和配置检查 Nginx…\n'
"${COMPOSE[@]}" run --rm --no-deps nginx nginx -t

printf '[5/5] 重新创建 Nginx，应用 443 端口和证书挂载…\n'
if [[ "$LEGACY_COMPOSE" == true ]]; then
	"${COMPOSE[@]}" rm -s -f nginx
	"${COMPOSE[@]}" up -d --no-deps nginx
else
	"${COMPOSE[@]}" up -d --no-deps --force-recreate nginx
fi
wait_for_health http://127.0.0.1/health
wait_for_https

"${COMPOSE[@]}" ps backend nginx
printf '\n部署成功：HTTPS API https://%s/api/v1/\n' "$DOMAIN"
