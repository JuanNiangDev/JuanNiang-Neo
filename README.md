# JuanNiang-Neo

> 复活吧卷娘 — 基于 OneBot11 协议的 QQ 聊天 Agent 系统

JuanNiang-Neo 是一个类 AstrBot 的 LLM 驱动的 QQ 聊天 Agent：支持多轮对话、MCP 工具调用、长期/短期记忆、Lua 插件扩展，以及 Vue 3 管理面板。

## 特性

- **OneBot11 反向 WebSocket**：同时支持反向 WS（`8081`）与 Webhook（`8091`，可选）两种接入。
- **Agent 核心**：`HagoCenter` 聚合 Provider / MCP / Memory / Prompt / Session / Skill / Tool；长任务用 errgroup 风格后台执行，独立 Drainer Agent 排空缓冲并发送 QQ 消息。
- **多 Provider**：OpenAI 兼容协议的 Text / Image / Embedding Provider，运行时通过 Web API 热切换。
- **MCP 集成**：通过 [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) 接入 MCP SSE 服务器作为 Agent 工具源。
- **记忆**：短期滑动窗口（Redis）+ 长期记忆（Postgres + HotArea LRU）+ 后台任务缓冲。
- **Lua 插件**：gopher-lua 引擎，全量 API（`onebot11`/`http`/`database`/`cache`/`t2i`/`sandbox`/`agent`）；插件目录 `data/pluggins/<name>/`。
- **Web 管理面板**：Vue 3 + Vuetify 仪表盘，JWT 鉴权（可选 OIDC SSO），单项管理全部资源。
- **可插拔基础设施**：T2I 文生图 / Sandbox 代码执行容器未配置时自动返回未启用提示，可在 Web 面板热启用。
- **单二进制 + 可选前端静态服务**：后端二进制独立运行，生产环境经 `WEB_DIR` 指向前端 `dist` 即同端口服务 SPA。

## 架构一览

```
┌─────────────────────────────────────────────────────────────┐
│                      cmd/server/main.go                     │
│                    (组装 & 启动入口)                          │
├──────────┬──────────┬──────────┬──────────┬────────────────┤
│  api/    │  pluggin/│  agent/  │  core/   │ infrastructure/│
│ (Web API)│ (Lua引擎)│ (Agent)  │ (核心库) │   (基础设施)    │
│ Hertz    │ gopher-lua│HagoCenter│ GORM    │ postgres/redis  │
│ + SPA    │ 命名空间  │ bgtask    │ cache   │ sandbox/t2i     │
│ + JWT    │          │ Drainer   │ acl     │                 │
├──────────┴──────────┴──────────┴──────────┴────────────────┤
│                     adapter/ (OneBot11 WS)                  │
└─────────────────────────────────────────────────────────────┘
```

详细架构见 [`docs/architecture.md`](docs/architecture.md)。事件流见 [`docs/event-flow.md`](docs/event-flow.md)，调用栈见 [`docs/call-stack.md`](docs/call-stack.md)，实现细节见 [`docs/implementation.md`](docs/implementation.md)。

## 快速开始

### 依赖

- Go 1.25+
- Node.js 18+ 与 npm（开发前端 / 构建生产 dist）
- Postgres 16+ 与 Redis 7+（或使用随附的 `docker compose` 一键拉起）
- Make（可选，但推荐使用根 `Makefile`）

### 一键开发（前后端并行）

```bash
# 1) 准备 PG / Redis (任选其一)
docker compose -f deployments/docker-compose.yaml up -d postgres redis
# 或本地已有即可

# 2) 安装前端依赖并启动开发环境
make web-install      # cd web && npm ci
make dev              # 并行启动 Vite (:3000) + Go (:8090)
```

打开浏览器访问 `http://localhost:3000`（Vite 热更新，自动代理 `/api` 到 `:8090`）。

### 生产构建（单二进制 + 前端 dist）

```bash
make build            # 前端 dist + Go 二进制 (输出 bin/juan-niang-neo)
WEB_DIR=web/dist ./bin/juan-niang-neo
# 浏览器访问 http://localhost:8090, 后端同时服务前端 SPA
```

### Docker 一键起整套

```bash
cp .env.example .env            # 修改 JWT_SECRET 等
make docker-up                  # docker compose up --build -d
# 浏览器访问 http://localhost:8090
make docker-logs                # 看 app 容器日志
make docker-down                # 停止整套
```

## 默认管理员

首次启动自动初始化：

| 用户名 | 密码     |
|--------|----------|
| admin  | Admin123 |

**首次登录后请立即在 Web 面板「设置」页修改密码**。JWT 默认有效期 72h；签发密钥为 `JWT_SECRET` 环境变量（生产环境务必修改）。

## 环境变量

完整列表见 [`.env.example`](.env.example)，最常用：

| 变量 | 默认 | 说明 |
|------|------|------|
| `WEB_DIR` | `web/dist` | 前端构建产物目录；留空则后端不服务 SPA（开发模式推荐留空走 Vite） |
| `API_ADDR` | `:8090` | Web 管理面板 + API 监听地址 |
| `OB_PORT` | `8081` | OneBot11 反向 WebSocket 监听端口 |
| `JWT_SECRET` | `change-me-in-production` | JWT 签名密钥 |
| `OB_ADMINS` | （空） | 管理员 QQ 号，逗号分隔 |
| `DB_*` / `REDIS_*` | 见 .env | Postgres / Redis 连接 |
| `OB_TOKEN` | （空） | OneBot11 反向 WS 接入校验 token |

## 项目布局

```
cmd/server/main.go          程序入口 (组装/启动/优雅退出)
internal/
  adapter/                  OneBot11 反向 WS + Webhook, 事件/API/消息段
  agent/                    HagoCenter: mcp/memory/prompt/provider/session/skill/tool
                            + BackgroundTaskExecutor + DrainerAgent
  api/                      Hertz web (engine/middleware/router/service)
  core/                     models(GORM) / dao / cache / acl
  pluggin/                  gopher-lua 引擎 (注意拼写: pluggin)
  web/                      前端 SPA 静态服务 (运行时磁盘读取)
infrastructure/             postgres / redis / sandbox / t2i 客户端
web/                        Vue 3 + Vite + Vuetify 前端 (构建产物 web/dist)
data/pluggins/<name>/       Lua 插件: main.lua + pluggin.yaml
config/config.yaml          参考配置 (实际启动用环境变量)
api/openapi.yaml            OpenAPI 3.0 规范
sql/init.sql                GORM AutoMigrate 后的表结构参考
deployments/                Dockerfile + docker-compose.yaml
docs/                       完整文档 (architecture/event-flow/...)

# 空或仅占位: pkg/
```

> 拼写约定（来自 `AGENTS.md`）：
> - 模块名 `internal/pluggin` 与目录 `data/pluggins` 是 **有意** 拼写（double-g, single-n），不要"修正"为 `plugin`。
> - `infrastructure/` 是真实模块路径；`docs/guidance.md` 历史上把它拼成 `inferstructure`，以代码为准。

## 常用 `make` 目标

```
make help          查看所有目标
make build         前端 + 后端单二进制 (默认目标)
make build-go      仅构建后端 (复用已有 web/dist)
make web-build     仅构建前端
make dev           并行起 Vite (:3000) + Go (:8090)
make run           go run (WEB_DIR=web/dist)
make vet / lint    go vet + 前端 typecheck
make docker-up     docker compose 起整套
make docker-logs   跟随 app 容器日志
make docker-down   停止整套
make clean         清理 bin/ 与 web/dist
```

## 已知限制

- `sql/init.sql` 仅为参考；表结构由 GORM `AutoMigrate` 自动创建，无需手动执行。
- 前端 SSE 日志流（后端 `GET /api/v1/logs/stream`）暂未在 UI 展示，前端按轮询 `/logs` 使用。

## 文档索引

- [架构](docs/architecture.md) · [事件流](docs/event-flow.md) · [调用栈](docs/call-stack.md) · [实现细节](docs/implementation.md)
- [API 文档](docs/api.md) · [OpenAPI 规范](api/openapi.yaml)
- [插件开发](docs/pluggin/) · [原始设计](docs/dev/) · [开发引导](docs/guidance.md)
- [OpenCode 代理指引](AGENTS.md)

## License

见 [LICENSE](LICENSE)。