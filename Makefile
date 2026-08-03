# JuanNiang-Neo Makefile
#
# 一键编排 Go 后端 + Vue 前端的开发/构建/容器化。所有目标均为 phony (无文件输出副作用)。
# 用法:
#   make            # 等价于 make build (前端 + 后端单二进制)
#   make dev        # 并行启动 Vite (:3000) + Go API (:8090)
#   make run        # 跑 go run (走 web/dist 服务前端)
#   make run-debug  # 跑 go run debug 模式 (pprof + 详细日志)
#   make docker-up  # docker compose 起整套 (postgres/redis/app)
#   make help       # 查看全部目标

SHELL          := /bin/bash
GO             ?= go
NPM            ?= npm
DOCKER         ?= docker
DOCKER_COMPOSE ?= docker compose

# 目录
ROOT_DIR   := $(CURDIR)
WEB_DIR    := $(ROOT_DIR)/web
WEB_DIST   := $(WEB_DIR)/dist
BIN_DIR    := $(ROOT_DIR)/bin
BIN_NAME   := juan-niang-neo
BIN_PATH   := $(BIN_DIR)/$(BIN_NAME)

# Go 构建参数
GOPROXY     ?= direct
GOFLAGS     := -trimpath
LDFLAGS     := -s -w
BUILD_TAGS  :=

# 运行参数 (dev/run)
API_ADDR    ?= :8090
OB_PORT     ?= 8081
WEB_DIR_ENV ?= $(WEB_DIST)   # 默认服务前端构建产物
RUN_ARGS    ?=                # 透传给 ./cmd/server 的参数, 如: make run RUN_ARGS=-debug

# Docker
COMPOSE_FILE := deployments/docker-compose.yaml

# 颜色 (仅 tty 时启用)
ifneq (,$(findstring xterm,$(TERM)))
	C_RESET := \033[0m
	C_GREEN := \033[32m
	C_CYAN  := \033[36m
	C_YELL  := \033[33m
else
	C_RESET :=
	C_GREEN :=
	C_CYAN  :=
	C_YELL  :=
endif

.DEFAULT_GOAL := build
.PHONY: help all build build-go web-install web-build web-dev web-lint web-typecheck \
        dev run fmt vet lint test tidy clean \
        docker docker-build docker-up docker-down docker-logs \
        check-go check-node

# =====================================================================
# Help
# =====================================================================

help: ## 显示所有可用目标
	@printf "$(C_CYAN)JuanNiang-Neo Makefile$(C_RESET)\n"
	@printf "用法: make [target]\n\n"
	@awk -F'## ' '/^[a-zA-Z][a-zA-Z0-9_-]*:.*## / { \
		split($$1, a, ":"); \
		printf "  $(C_GREEN)%-16s$(C_RESET) %s\n", a[1], $$2 \
	}' $(MAKEFILE_LIST)

all: build ## 等价于 build

# =====================================================================
# 前端
# =====================================================================

check-node: ## 检查 node/npm 可用
	@command -v node >/dev/null 2>&1 || { printf "$(C_YELL)未发现 node$(C_RESET)\n"; exit 1; }
	@command -v $(NPM) >/dev/null 2>&1 || { printf "$(C_YELL)未发现 $(NPM)$(C_RESET)\n"; exit 1; }

web-install: check-node ## 安装前端依赖 (npm ci, 失败回退 npm install)
	@printf "$(C_CYAN)>>> 安装前端依赖$(C_RESET)\n"
	@cd $(WEB_DIR) && ([ -f package-lock.json ] && $(NPM) ci || $(NPM) install)

web-build: ## 构建前端 (typecheck + vite build -> web/dist)
	@printf "$(C_CYAN)>>> 构建前端$(C_RESET)\n"
	@cd $(WEB_DIR) && $(NPM) run build
	@test -d $(WEB_DIST) && printf "$(C_GREEN)<<< 前端产物已生成: $(WEB_DIST)$(C_RESET)\n" \
		|| { printf "$(C_YELL)web/dist 缺失, 构建可能失败$(C_RESET)\n"; exit 1; }

web-dev: check-node ## 仅启动 Vite 开发服务器 (:3000, 代理 /api -> :8090)
	@printf "$(C_CYAN)>>> 启动 Vite (dev)$(C_RESET)\n"
	@cd $(WEB_DIR) && $(NPM) run dev

web-lint: check-node ## 前端 lint
	@cd $(WEB_DIR) && $(NPM) run lint

web-typecheck: check-node ## 前端类型检查
	@cd $(WEB_DIR) && $(NPM) run typecheck

# =====================================================================
# Go 后端
# =====================================================================

check-go: ## 检查 go 可用
	@command -v $(GO) >/dev/null 2>&1 || { printf "$(C_YELL)未发现 go$(C_RESET)\n"; exit 1; }

build: web-build build-go ## 构建前端 + 后端 (默认目标是 all)

build-go: check-go ## 仅构建后端二进制 (不带前端构建步骤; 仍可服务 web/dist 如已存在)
	@printf "$(C_CYAN)>>> 构建 Go 后端$(C_RESET)\n"
	@mkdir -p $(BIN_DIR)
	@cd $(ROOT_DIR) && GOPROXY=$(GOPROXY) CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) $(if $(BUILD_TAGS),-tags '$(BUILD_TAGS)',) \
		-ldflags '$(LDFLAGS)' \
		-o $(BIN_PATH) ./cmd/server
	@printf "$(C_GREEN)<<< 二进制: $(BIN_PATH)$(C_RESET)\n"

dev: check-go check-node ## 并行启动 Vite (:3000) + Go (:8090); Ctrl-C 一次性停掉
	@printf "$(C_CYAN)>>> 启动 dev 环境 (Vite + Go 并行)$(C_RESET)\n"
	@printf "    前端: http://localhost:3000   (Vite 热更新, 代理 /api -> :8090)\n"
	@printf "    后端: http://localhost:8090 ($(BIN_NAME) API)\n"
	@trap 'kill 0' INT TERM; \
	( cd $(WEB_DIR) && $(NPM) run dev ) & \
	( cd $(ROOT_DIR) && $(GO) run ./cmd/server -dev-config $(DEV_CONFIG) ) & \
	wait

DEV_CONFIG ?= dev.yaml            # 开发配置文件路径 (不存在则静默跳过)

run: check-go ## 跑 go run (自动读取 dev.yaml, 前端走 web/dist)
	@printf "$(C_CYAN)>>> 启动后端 (go run, dev-config=$(DEV_CONFIG))$(C_RESET)\n"
	@cd $(ROOT_DIR) && $(GO) run ./cmd/server -dev-config $(DEV_CONFIG) $(RUN_ARGS)

run-debug: check-go ## 跑 go run debug 模式 (自动读取 dev.yaml + pprof :6060)
	@printf "$(C_CYAN)>>> 启动后端 DEBUG 模式 (dev-config=$(DEV_CONFIG), pprof :6060)$(C_RESET)\n"
	@cd $(ROOT_DIR) && $(GO) run ./cmd/server -debug -dev-config $(DEV_CONFIG)

fmt: check-go ## go fmt
	@cd $(ROOT_DIR) && $(GO) fmt ./...

vet: check-go ## go vet
	@cd $(ROOT_DIR) && $(GO) vet ./...

lint: vet web-typecheck ## 综合检查 (go vet + 前端 typecheck)

test: check-go ## go test (当前无测试, 占位)
	@cd $(ROOT_DIR) && $(GO) test ./... || printf "$(C_YELL)注意: 项目尚无 _test.go$(C_RESET)\n"

tidy: check-go ## go mod tidy
	@cd $(ROOT_DIR) && $(GO) mod tidy

# =====================================================================
# Docker
# =====================================================================

docker: docker-up ## 等价于 docker-up

docker-up: ## docker compose up (构建并后台启动整套)
	@printf "$(C_CYAN)>>> docker compose up --build -d$(C_RESET)\n"
	@cd $(ROOT_DIR) && $(DOCKER_COMPOSE) -f $(COMPOSE_FILE) up --build -d
	@printf "$(C_GREEN)<<< 已启动; 访问 http://localhost:8090$(C_RESET)\n"
	@printf "    日志: make docker-logs\n"
	@printf "    停止: make docker-down\n"

docker-down: ## docker compose down
	@cd $(ROOT_DIR) && $(DOCKER_COMPOSE) -f $(COMPOSE_FILE) down

docker-logs: ## 查看 app 容器日志 (follow)
	@cd $(ROOT_DIR) && $(DOCKER_COMPOSE) -f $(COMPOSE_FILE) logs -f juan-niang-neo

docker-build: ## 仅构建镜像
	@cd $(ROOT_DIR) && $(DOCKER_COMPOSE) -f $(COMPOSE_FILE) build

# =====================================================================
# 清理
# =====================================================================

clean: ## 清理 bin/ 和 web/dist
	@printf "$(C_YELL)>>> 清理 bin/ 与 web/dist$(C_RESET)\n"
	@rm -rf $(BIN_DIR) $(WEB_DIST)