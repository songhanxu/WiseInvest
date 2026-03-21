# ============================================================
# WiseInvest Backend - Multi-stage Dockerfile
# ============================================================

# ---- Stage 1: Build ----
FROM golang:1.24-alpine AS builder

# 使用阿里云 Alpine 镜像源（国内服务器加速）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 缓存依赖层
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

# 编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/server \
    cmd/server/main.go

# ---- Stage 2: Runtime ----
FROM alpine:3.19

# 使用阿里云 Alpine 镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata curl

# 设置时区（定时任务依赖 Asia/Shanghai）
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从 builder 阶段复制二进制
COPY --from=builder /app/server .

# APNs 密钥文件（如果有的话）
# COPY apns_key.p8 .

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["./server"]
