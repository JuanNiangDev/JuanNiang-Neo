# syntax=docker/dockerfile:1.6
# JuanNiang-Neo 多阶段构建:
#   1) web-builder : Node 20 构建前端 -> /app/web/dist
#   2) go-builder  : Go 1.25 编译后端二进制 (前后端不嵌入, 运行时从 /app/web/dist 读取)
#   3) runtime     : Alpine + 非 root 用户, 仅含 binary + config + web/dist + 健康检查

ARG GO_VERSION=1.25-alpine
ARG NODE_VERSION=20-alpine

# ===== Stage 1: 前端构建 =====
FROM node:${NODE_VERSION} AS web-builder
WORKDIR /app/web

# 先拷 lockfile, 利用层缓存
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund

# 拷剩余前端源码并构建
COPY web/ ./
RUN npm run build && test -d dist


# ===== Stage 2: Go 后端构建 =====
FROM golang:${GO_VERSION} AS go-builder
WORKDIR /app

# golang:alpine 不含 git，go mod download 需要 git 拉取依赖
RUN apk --no-cache add git

ENV CGO_ENABLED=0 \
    GOPROXY=direct \
    GONOSUMCHECK=* \
    GONOSUMDB=*

# 先拷 module 描述, 利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 仅编译后端, 不嵌入前端 (运行时由 WEB_DIR 指向容器内 /app/web/dist)
RUN go build -trimpath -ldflags "-s -w" -o /juan-niang-neo ./cmd/server/


# ===== Stage 3: 运行时镜像 =====
FROM alpine:latest

# 安全 & 时区支持
RUN apk --no-cache add ca-certificates tzdata wget \
    && addgroup -S jn && adduser -S -G jn jn

WORKDIR /app

# binary + 配置 + 前端产物
COPY --from=go-builder /juan-niang-neo /usr/local/bin/juan-niang-neo
COPY --from=web-builder  /app/web/dist /app/web/dist
COPY config/ /etc/juan-niang-neo/config

# 运行时数据 (plugin 目录由 compose bind-mount 进来; 这里只准备兜底空目录)
RUN mkdir -p /app/data/pluggins \
    && chown -R jn:jn /app /etc/juan-niang-neo

ENV WEB_DIR=/app/web/dist \
    TZ=Asia/Shanghai \
    OB_PORT=8081 \
    API_ADDR=:8090 \
    DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=postgres \
    DB_PASSWORD=postgres \
    DB_NAME=juan \
    REDIS_ADDR=redis:6379 \
    REDIS_PASSWORD=root

EXPOSE 8081 8090

USER jn

# 健康检查: 后端 /health 在 root 路径, 无需鉴权。
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8090/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["juan-niang-neo"]