# JuanNiang-Neo 开发引导 (guidance.md)

> 本文件是 OpenCode 会话与人类贡献者的开发约定 + 历史拼写坑提示。代码与文档冲突时以代码为准。

## 1. 工程入口与组装顺序

`cmd/server/main.go` 是唯一可执行入口, 启动顺序固定:

1. `signal.NotifyContext` 注册退出信号 → 优雅退出贯穿全程
2. **日志**: `logging.NewHandler` 同时输出到 stdio 与 `LogHub` (最近 250 条 + SSE 推送)
3. **Postgres** via `infrastructure/postgres` (GORM)
4. **Redis** via `infrastructure/redis`
5. **Core** (`core.Init`): DAO bundle + Cache + ACL
6. **JWT** 密钥注入 middleware (`JWT_SECRET` env)
7. **Adapter** (OneBot11 反向 WS, `:8081` 默认) + **Webhook Adapter** (可选, `:8091`)
8. **Agent** (`agent.HagoCenter.Init` + `Start`): 聚合 Provider / MCP / Memory / Prompt / Session / Skill / Tool, 启动 `BackgroundTaskExecutor` + `DrainerAgent`
9. **Plugin Engine** (`internal/pluggin`), 扫描 `data/pluggins/`
10. **Web API** (`internal/api/engine`): Hertz `:8090`, 注册全量 `/api/v1/*` + `/health` + 前端 SPA 兜底 (NEW)
11. **T2I / Sandbox**: 从 DB 加载配置, 可通过 Web API 热更新 (回调 `svc.OnUpdateT2I` / `OnUpdateSandbox`)
12. 等待 `<-ctx.Done()`, 触发 `defer` 反向关停

## 2. 模块路径与命名陷阱

- 真实 infra 模块路径是顶层 `infrastructure/`。**`docs/dev/guidance.md` (本文件的旧版本) 历史上把它拼成 `inferstructure`, 这是已知错误, 以代码为准。**
- `docs/dev/provider.md` 写的 `internal/provider` 实际是 `internal/adapter` (包名 `adapter`)。
- "Provider" 在本项目中是 overloaded 术语:
  - `internal/adapter.Provider` = OneBot11 反向 WebSocket 适配器
  - `internal/agent/provider` = LLM Provider 组 (OpenAI 兼容等)
  始终以完整 import path 为准, 不要看字面词。
- **`pluggin` (双 g 单 n) 是有意的拼写**, 模块 `internal/pluggin`, 配置文件 `pluggin.yaml`, 插件目录 `data/pluggins`。不要"修正"为 `plugin`。

## 3. 前后端协作约定

### 3.1 API 路由

- 全部业务路由位于 `/api/v1/*`; 仅 `GET /health` 在 root。
- 鉴权: `middleware.JWTAuth()`, 例外只有 `POST /login` 与 `GET /health`。
- 信封: `{ "status": 0, "info": "OK", "data": <any|null> }`。成功 `status === 0`, 错误码详见 `internal/api/dto/response.go`。
- CORS 全局开放 (`*`), 便于前端 dev 直连。

### 3.2 前端 SPA 静态服务 (NEW)

- `internal/web/web.go.SPAHandler(webDir)` 通过 `h.NoRoute` 兜底:
  - `/api/*` 未命中 → 标准 404 信封
  - 其它路径 → 文件存在 serve 文件, 不存在回退 `index.html`
  - `index.html` 缺失 → 200 引导提示页
- **不嵌入二进制**: 前端以磁盘文件形式存在 `WEB_DIR` (默认 `web/dist`)。
- 开发模式前端走 Vite `:3000`, `vite.config.ts` 代理 `/api` → `:8090`。
- 改后端路由或 DTO 时, 同步更新 `web/src/api/index.ts` 与 `web/src/views/*.vue`。后端为准。

### 3.3 Makefile & Docker

- 根 `Makefile` 一键编排: `make build` / `make dev` / `make docker-up` / `make vet` / `make lint` / `make clean`。
- `make lint` = `go vet` + `make web-typecheck` (`vue-tsc` ≥ 2.x, 支持 Node 18–24)。此前 `vue-tsc@1.8.x` 在 Node 24 上的 `Search string not found` 兼容问题已通过升级解决。
- `deployments/Dockerfile` 3 阶段: `node:20-alpine` 构前端 → `golang:1.25-alpine` 构 Go → `alpine:latest` 运行 (非 root 用户 `jn`, `WEB_DIR=/app/web/dist`, `HEALTHCHECK` 走 `/health`)。
- `deployments/docker-compose.yaml` 起 postgres + redis + app, 全部带 healthcheck + `restart: unless-stopped`, `depends_on: condition: service_healthy`, 网络 `juanniang-net`, 插件 bind-mount `../data/pluggins` → `/app/data/pluggins` (注意: 必须挂到 `/app/data/pluggins`, 因 `cmd/server/main.go` 硬编码加载 `"data/pluggins"` 且 CWD 为 `/app`)。
- `.env.example` 列出所有 env var; `docker compose` 自动读取同目录 `.env`。

## 4. 状态管理铁律

- 持久状态在 Postgres + Redis cache; Agent / Session / Skill / Plugin 状态最终与 DB 同步。**禁止纯内存有状态对象。**
- Lua 插件是唯一例外: 配置文件 `data/pluggins/<name>/pluggin.yaml` 直读盘。
- 服务开关: T2I 与 Sandbox 未配置时 API 自动返回"未启用"提示, 无需提前手动配置。
- Web 控台鉴权: JWT (HS256), 默认 72h 有效期; 可选 OIDC SSO; 单管理员; 初始密码 `Admin123` (首登必改)。

## 5. Agent 与长任务模型

- OneBot11 API 函数被注册为 Agent Tools; 新增 OneBot11 工具应 wrap `internal/adapter.Provider` 方法, 不要重新实现。
- 长任务 (MCP / 工具调用) 用 errgroup 风格后台执行, 结果写入 `bgtask` memory; 独立的 **DrainerAgent** (非对话 Agent) 排空缓冲并发送最终的 QQ 消息。
- Memory: ShortTerm (Redis 滑窗) + LongTerm (Postgres + HotArea LRU) + BgTask (缓冲); 三者由 `memory.MemoryGroup` 聚合。
- Skill 命中后注入 prompt + tool; 命中规则是 关键词 OR 正则。

## 6. 编码约定

- 日志 `log/slog` 结构化, `slog.Info("event", "key", val)` 风格。**禁止 `fmt.Println` 或第三方 logger**。
- Import 顺序: std → 第三方 → `JuanNiang-Neo/...`, 参见 `internal/adapter/provider.go`。
- 注释与标识符混合中英文, **不要翻译**当前文件里已有的语言风格。
- 提交前跑: `make vet` (必过), `make lint` (前端 typecheck 在 Node 24 上可能跳过, 见 3.3)。

## 7. 历史变更摘要 (本会话)

- 后端 `internal/web/web.go` + `engine.New(addr, webDir, svc)` 新增前端 SPA 服务。
- 前端 `web/src/api/index.ts.PromptResp` 补 `is_system: boolean`; `web/src/views/PromptsPage.vue` 加系统 badge + 禁用编辑/删除/启停。
- 根 `Makefile` 新增 (构建/dev/docker/清理)。
- `web/package.json` 升级 `vue-tsc` 到 ≥ 2.x (修复 Node 24 兼容), 顺带修了 5 个 `.vue` 中 `v-switch @update:model-value` 回调的类型签名错误 (`(v: boolean)` → `(v) => toggle(id, !!v)`)。
- `deployments/Dockerfile` 改为 3 阶段 (含前端); `deployments/docker-compose.yaml` 加 healthcheck + 网络 + .env 支持。
- 根 `.dockerignore` + `.env.example` 新增。
- `README.md` 重写; `AGENTS.md` 修正布局与前端服务说明; `docs/architecture.md` 装一节"前端 SPA 静态服务"; `docs/api.md` 装第 20 章; 本文件从空文件补全。