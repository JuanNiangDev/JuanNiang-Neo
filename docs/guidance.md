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
- **Lua SDK 与 system 插件**: 由 `//go:embed` 内嵌到二进制，启动时由 `PluginEngine.ensureEmbeddedAssets()` 落盘到 `data/pluggins/sdk/jn.lua`（SDK 总是覆盖）与 `data/pluggins/system/`（system 插件仅首次落盘，允许用户修改）。这两份文件**不应加入 .gitignore 之外的手动管理**，运行时由二进制保证一致性。
- **命令注册表 (`CommandRegistry`)** 是 PluginEngine 内的运行时内存对象，命令注册**不持久化到 DB**；每次启动由各插件的 `main.lua` 通过 `jn.command.register` 重新注册。这是有意设计——命令与插件 LState 生命周期绑定，卸载即清除。
- **系统锁定提示词 (`IsSystem=true`)**: 启动时由 `PromptManager.EnsureSystemPrompt(ctx)` 幂等播种到 DB（name=`__system_locked__`），二进制版本更新时会覆盖内容。API 层（`UpdatePrompt` / `DeletePrompt` / `TogglePrompt`）禁止修改系统锁定提示词，返回 `40029 PromptIsSystem`；用户创建 `Type=system` 的提示词也被拒绝。
- 服务开关: T2I 与 Sandbox 未配置时 API 自动返回"未启用"提示, 无需提前手动配置。运行时通过 `AgentOperator.GetT2IClient()` / `GetSandboxClient()` 获取最新实例，支持热更新。
- Web 控台鉴权: JWT (HS256), 默认 72h 有效期; 可选 OIDC SSO; 单管理员; 初始密码 `Admin123` (首登必改)。

## 5. Agent 与长任务模型

- OneBot11 API 函数被注册为 Agent Tools; 新增 OneBot11 工具应 wrap `internal/adapter.Provider` 方法, 不要重新实现。
- 长任务 (MCP / 工具调用) 用 errgroup 风格后台执行, 结果写入 `bgtask` memory; 独立的 **DrainerAgent** (非对话 Agent) 排空缓冲并发送最终的 QQ 消息。
- Memory: ShortTerm (Redis 滑窗) + LongTerm (Postgres + HotArea LRU) + BgTask (缓冲); 三者由 `memory.MemoryGroup` 聚合。
- Skill 命中后注入 prompt + tool; 命中规则是 关键词 OR 正则。
- **SystemPrompt 拼接优先级**（高 → 低）: SystemLocked (`IsSystem=true`) → system (跳过 IsSystem) → personality → custom。内容直接 `strings.Join("\n\n")` 拼接，**不再使用 `text/template` 渲染**（`Variables` 字段已删除）。
- **内置工具 ID 前缀 `builtin:`**: `ListTools` 合并 `ToolRegistry.List()` 内置工具与 DB `ToolConfig`，内置工具响应 `ID = "builtin:" + name`、`IsBuiltin = true`。`ToggleTool` 收到 `builtin:` 前缀返回 `40030 ToolIsBuiltin`，禁止切换内置工具状态。

## 5.5. Lua 插件系统约定 (NEW)

- **推荐使用 SDK**: 插件 `main.lua` 开头 `local jn = require("jn")`，通过 `jn.<table>.<func>` 调用 Go API，可获得 sumneko lua-language-server 完整类型提示。SDK 字段与全局表完全等价，可混用。
- **命令注册优先**: 涉及 `/` 开头的命令式交互，**必须**使用 `jn.command.register(path, handler, opts)` 注册，由 `CommandRegistry` 统一派发、自动回复、`/help` 自动生成。**不要**在 `on_message` 中手动 `string.match` 解析 `/cmd`——命令系统在 `on_message` 之前派发，未命中的 `/` 消息才到达 `on_message`。
- **多级命令**: `jn.command.register({"foo", "bar", "baz"}, handler, opts)` 注册多级路径，派发时按最长前缀匹配。
- **`/help` 内置命令**: 由 `registerBuiltinCommands()` 在 `NewPluginEngine` 时注册，路径 `["system", "help"]`，挂在 `system` 插件名下。
- **`system` 系统插件**: 由 `//go:embed` 内嵌 `pluggin.yaml` + `main.lua`，清单中 `system: true`。三层守卫禁止卸载 / 停用 / 删除（详见 `docs/pluggin/implementation.md` §8）。`ensureEmbeddedAssets` 仅在 `data/pluggins/system/pluggin.yaml` 不存在时落盘，**允许用户修改 `main.lua` 扩展命令**，但 `system: true` 标志由清单控制不应移除。
- **`on_message` 适用场景**: 纯事件监听（不回复）、关键词触发（非 `/` 前缀）、无需固定命令模式的副作用逻辑。`on_message` 返回 `consumed=true` 阻止 Agent 处理。
- **AgentOperator 接口**: `HagoCenter` 实现，暴露 Provider / MCP / Tool / T2I / Sandbox 管理 + Compact + 上下文查询。插件通过 `jn.agent.*` 调用，运行时状态查询用 `list_runtime_providers` / `list_mcps` / `list_tools`，切换用 `switch_provider` / `toggle_mcp` / `toggle_tool` / `t2i.toggle` / `sandbox.toggle`。

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

### 后续变更 (Lua 插件系统重构 + 系统锁定提示词 + Adapter/Tool 改进)

- **Lua SDK (`internal/pluggin/sdk/jn.lua`)**: `//go:embed` 内嵌，启动时落盘到 `data/pluggins/sdk/jn.lua`，插件通过 `require("jn")` 引入获得 IDE 类型提示。SDK 重新导出 Go 注入的全局表 + 提供 `jn.command.register` 入口。
- **多级命令系统 (`internal/pluggin/command.go`)**: `CommandRegistry` + `CommandNode` 树，支持 `/cmd subcmd args...` 最长前缀匹配派发。`PluginEngine.OnMessage` 在 `on_message` 之前优先派发命令，命中后自动回复。
- **内置 `/help` 命令**: 由 `registerBuiltinCommands()` 注册到 `system` 插件名下，路径 `["system", "help"]`，调用 `FormatHelp(args)` 输出顶层命令或子命令详情。
- **`system` 系统插件 (`internal/pluggin/systemplugin/`)**: 由 `//go:embed` 内嵌，清单中 `system: true`，触发三层守卫（Manifest.System + PluginEngine.IsSystem + Service 层）。注册 `/system status/provider/mcp/tool/memory/t2i/sandbox/session` 等命令。
- **系统锁定提示词 (`internal/agent/prompt/prompt.go`)**: 新增 `IsSystem bool` 字段（`models.Prompt`）、`SystemLockedPromptName = "__system_locked__"` 常量、`EnsureSystemPrompt(ctx)` 启动时幂等播种。`BuildSystemPrompt` 拼接优先级 SystemLocked→system→personality→custom，删除 `text/template` 渲染与 `Variables` 字段。Service 层守卫返回 `40029 PromptIsSystem`。
- **Adapter 重构 (`internal/adapter/`)**: `listenAddr()` 规范化兼容 host/:port/host:port/空串；`Stop()` 修复 close of closed channel panic（关闭后置 nil）；`newWSServer` 改用 `context.Background()` 避免 SyncConfig 5s 超时级联取消；`SyncConfig` 简化为 Stop+Start；新增 `wsConn.remoteAddr` 字段、`ConnDetail` 结构、`connDetails()` 方法、`ProviderStatus.Conns` 字段。
- **Tool 管理增强 (`internal/api/service/service.go`)**: `ListTools` 合并 `ToolRegistry.List()` 内置工具与 DB `ToolConfig`，内置工具 ID 用 `builtin:` 前缀；`ToggleTool` 收到 `builtin:` 前缀返回 `40030 ToolIsBuiltin`。
- **Plugin 管理增强**: `ListMaps` 返回 `permissions` / `is_system` / `commands` 字段；`TogglePlugin` / `DeletePlugin` 系统插件守卫返回 `40028 PluginIsSystem`。
- **AgentOperator 接口扩展**: 新增 `SetToolActive` / `SwitchProvider` / `SetT2IActive` / `SetSandboxActive` / `GetT2IClient` / `GetSandboxClient` / `GetProviderGroup` / `GetMCPGroup` / `GetToolRegistry`；Lua 侧 `agent.*` 新增 `list_mcps` / `toggle_mcp` / `list_tools` / `toggle_tool` / `list_runtime_providers` / `switch_provider`，`t2i.*` / `sandbox.*` 新增 `toggle` / `is_active` / `get_config`。
- **新 API 错误码**: `40028 PluginIsSystem` / `40029 PromptIsSystem` / `40030 ToolIsBuiltin`。
- **`GetLogs` 返回顺序**: 改为最新最前（反转为倒序），前端无需自行排序。
- **`Onebot11Adapter.AdminQQNumbers` 字段**: 持久化到 DB（GORM `serializer:json`），由 `UpdateAdapterConfig` 同步 DB 与运行时。
- **T2I/Sandbox 启动时 `InitConfig`**: `loadT2IFromDB` / `loadSandboxFromDB` 在 DB 无配置时初始化默认行，保证前端读取不报错。