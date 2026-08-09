# AGENTS.md

Guidance for agent sessions working in this repo. The codebase is fully
implemented and actively developed — `cmd/server/main.go` wires Postgres / Redis /
Core / Adapter / Agent / Plugin / Web API together. Trust code over `docs/` when
they conflict; docs are maintained in bulk and may lag behind recent commits.

## Toolchain

- Go 1.25 (see `go.mod`). Module path is the literal `JuanNiang-Neo` (case + hyphen
  matter for imports, e.g. `JuanNiang-Neo/internal/adapter`).
- Frontend: Node 18+ / npm (Vue 3 + Vite 6 + Vuetify 3 at `web/`).
- Baseline checks via the root `Makefile`:
  - `make build`     — full build (frontend → `web/dist` + Go binary `bin/juan-niang-neo`)
  - `make vet`       — `go vet ./...`
  - `make lint`      — `go vet` + `web-typecheck` (`vue-tsc` ≥ 2.x, supports Node 18–24)
  - `make test`      — `go test ./...`（已有 17 个 `*_test.go`，多数用内存 SQLite，可直接运行）
  - `make docker-up` — full stack via `deployments/docker-compose.yaml`
- CI: GitHub Actions (`.github/workflows/`) — `pr-check.yml` runs go build/vet/test/
  gofmt/tidy + frontend typecheck/build + Docker image build on every PR;
  `docker-build.yml` builds & pushes `ghcr.io/juanniangdev/juan` on main.

## Terminology traps (these have bitten past readers)

- The original design docs `docs/guidance.md` / `docs/provider.md` were folded into
  `docs/development.md` and no longer exist. Their historical misspellings
  (`inferstructure` for `infrastructure/`, `internal/provider` for `internal/adapter`)
  survive only as prose in the merged docs.
- "Provider" is overloaded in this codebase:
  - `internal/adapter.Provider` = the OneBot11 reverse-WebSocket adapter.
  - `internal/agent/provider` = the LLM provider group (OpenAI 兼容 / Anthropic /
    Gemini 协议，由 `api_mode` 区分；含 Eino ADK 适配器)。
  Always resolve by full import path, not the word "provider".
- `pluggin` (double-g, single-n) is the *intentional* spelling for the Lua plugin
  system — module `internal/pluggin`, config file `pluggin.yaml`, plugin dir
  `data/pluggins`. Do not "fix" it to `plugin`.

## Layout

Top-level:
- `cmd/server/main.go` — program entry, fully wired（基础设施 → core → adapter →
  agent → 插件 → Web API + 带 watchdog 的优雅退出）；`devconfig.go` 负责加载
  `dev.yaml`（优先级：环境变量 > dev.yaml > 内置默认值）。
- `internal/` — app code, nothing exported outside the module:
  - `adapter/` — OneBot11 反向 WS 服务端 + Webhook 适配器 + API 客户端 + 事件 +
    消息段 + `imgs://`/`stk://` base64 解析器。
  - `agent/` — Agent core built on Eino ADK; subpackages `cronjob`, `fishcal`,
    `mcp`, `memory`（shortterm/longterm/skillmem）, `prompt`, `provider`,
    `scheduledmsg`, `session`, `skill`, `tool`. `ConcurrencyManager` limits
    parallel Agent loops per ChatArea（默认 8）; `reply_strategy.go` 实现
    relevance 快路径/批量判断/缓存/降级管线; `event.go` 是批处理事件循环。
    Aggregated by `HagoCenter` in `agent.go`.
  - `api/` — Hertz web engine + `middleware`（JWT）+ `router` + `service` + `dto`。
    `/api/v1` 下共 121 条路由（`internal/api/router/router.go`），根路径仅
    `GET /health`。OpenAPI 规范见 `api/openapi.yaml`（与路由对齐）。
  - `core/` — `acl`, `cache`, `dao`, `handler`, `imgstore`, `models`
    （31 张表，由 `core.go::AutoMigrate` 创建）。
  - `pluggin/` — gopher-lua 引擎：加载器、命令树、内嵌 SDK + 系统插件
    （`//go:embed`）、插件商店客户端。
  - `logging/` — fatih/color 彩色输出 + JSON 格式化 + 调用栈 + Hub(SSE)。
  - `web/` — Frontend SPA serving helper (`SPAHandler`). Runtime reads `WEB_DIR`
    (default `web/dist`); `engine.New(addr, webDir, svc)` registers a `h.NoRoute`
    fallback: `/api/*` → standard 404 JSON envelope, anything else →
    file-or-`index.html` (Vue Router history mode). If `index.html` missing it
    serves a "build the frontend first" hint page. NOT embedded via `//go:embed`.
- `infrastructure/` — `postgres`, `redis`, `sandbox`, `t2i` adapters（每个含
  `handler` 调用方子包）。
- `web/` — Vue 3 + Vite 6 + Vuetify 3 dashboard（28 个视图页，full implementation）。
  Build output goes to `web/dist/` (gitignored). `vite.config.ts` proxies `/api`
  → `http://127.0.0.1:8090` in dev mode.
- `data/` — runtime data; `data/pluggins/` holds Lua plugins（13 个示例 +
  `sdk` + `system`）+ their `pluggin.yaml` configs（未提交）；`data/imgs/` 图床
  图片；`data/plugin_store.json` 插件商店配置。
- `docs/` — 完整文档:
  - `api.md` — Web API 全路由文档（27 章，与 121 条路由对齐）
  - `project-details.md` — 架构 / 事件循环 / 调用栈 / 数据模型（mermaid/ASCII）
  - `plugin-development.md` — Lua 插件开发 + API 参考
  - `plugin-store.md` — 插件商店机制与动态配置
  - `deployment.md` — 部署与调试指南 (env var / docker / systemd / 反代 / FAQ)
  - `development.md` — 二次开发指南（该读什么/该改什么/不该动什么）
  - `external-services.md` — 外部服务接入（T2I/Sandbox/Provider 热更新）
  - `webhook-cronjob.md` — Webhook / CronJob 触发 Lua 插件
  - `llm-provider-adaptation-plan.md` — LLM 协议适配规划
  - `changelog/` — 按日期记录的变更日志（问题-根因-修复）
- `api/` — holds `openapi.yaml` (the OpenAPI 3.0 spec covering all 121 routes).
  NOT a Go package.
- `sql/` — `init.sql` is a documentation reference; tables are actually created by
  GORM `AutoMigrate` at startup.
- `config/` — `config.yaml` reference file; the binary reads env vars at runtime,
  this file is advisory only.
- `deployments/` — `Dockerfile`（3-stage: node → go → alpine runtime, frontend
  runs from `/app/web/dist` via `WEB_DIR`）+ `docker-compose.yaml`（postgres +
  redis + app, healthchecks, restart policy, named network, bind-mount `../data`
  → `/app/data`）。
- `pkg/` — empty placeholder. `src/` 与 `scripts/` 已移除。

## Frontend serving

- `cmd/server/main.go` reads `WEB_DIR` (default `web/dist`) and passes it to
  `engine.New(addr, webDir, svc)`.
- `internal/web/web.go` provides `SPAHandler(webDir)` which:
  - Returns standard `{status,info,data}` 404 envelope for any unmatched
    `/api/*` route (keeps API errors uniform).
  - For all other paths: serves the file if present, falls back to `index.html`
    for client-side routing. Path traversal is blocked via `filepath.Rel`.
  - If `web/dist` doesn't exist or lacks `index.html`, returns a 200 hint page
    (so operators see actionable guidance rather than a bare 404).
- **NOT embedded**: the binary depends on `web/dist` being present on disk. This
  is intentional — keeps binary rebuild-free for frontend-only updates, matches
  the project's "config on disk" philosophy.
- Dev mode: Vite serves the SPA at `:3000` and proxies `/api` → `:8090`, so the
  Go fallback is never reached.

## Makefile & docker

- Root `Makefile` orchestrates frontend + backend + docker. `make build` =
  `web-build` → `build-go`; `make dev` runs Vite + Go in parallel; `make
  docker-up` builds & starts the whole compose stack.
- `.dockerignore` excludes `web/node_modules`, `web/dist`, `.git`, `bin/`,
  `docs/`, `data/`, etc. to keep build context small.
- `.env.example` documents all supported env vars for compose; `docker compose`
  reads a sibling `.env` automatically.

## Source-of-truth rules

- 持久化状态落 Postgres（Redis 承担缓存与短期记忆）。Agent / session / skill /
  plugin 配置必须同步回 DB。运行时*热*数据——群成员信息缓存、热聊统计、
  知识库 LRU、活跃循环监控（`LoopTracker`）——是有意的内存态
  （见 `internal/agent/agent.go`），不要把它当持久化。
- Lua 插件配置是例外：`data/pluggins/<name>/pluggin.yaml`。
- Web console auth: JWT (HS256), single admin user, initial password `Admin123`
  (change on first boot)。生产环境必须设置 `JWT_SECRET`。
- OneBot11 API functions are registered as Agent Tools; new OneBot11-capable
  tools should wrap `internal/adapter.Provider` methods rather than re-implement.
- 工具调用在 Eino ADK ReAct 循环内同步执行；Agent 的消息发送统一走
  `DeferredSendQueue`（任务执行完再按序发送）。`ConcurrencyManager` 限制每
  ChatArea 的并行 Agent 循环数。

## Conventions

- Logging via `log/slog` (structured) 或 `internal/logging` 模块日志器
  （`logging.NewModule`）。Match existing `slog.Info/Error(..., "key", val)`
  style; do not introduce `fmt.Println` or third-party loggers.
- Imports follow the std → third-party → `JuanNiang-Neo/...` block layout seen in
  `internal/adapter/provider.go`. Preserve that ordering.
- Comments and identifiers are mixed Chinese/English — keep that style in the
  file you are editing; do not translate.
- 开发时 `cp dev.yaml.example dev.yaml`; `make run` 自动读取 dev.yaml 中的基础设施连接端点。

## 分支保护与贡献规则

主分支（`main`）已启用严格分支保护，**禁止直接向主分支提交代码**：

- **仓库内贡献者（读写权限）**：所有代码修改必须在**新建的分支**（如 `feature/xxx`、`fix/xxx`、`docs/xxx`）上进行，然后通过 **Pull Request** 合并到主分支；直接 push 到 `main` 会被拒绝。
- **Fork 贡献者**：请在自 fork 的仓库中**新建分支**开发，再向本仓库发起 Pull Request；**禁止从 fork 仓库的主分支（`main`/`master`）直接发起 PR**，此类 PR 将被拒绝。
- 主分支的合并只能通过 Pull Request 完成（详见 README「贡献指南」）。

## 提交信息规范（重要）

遵循 [Conventional Commits 约定式提交](https://www.conventionalcommits.org/zh-hans/v1.0.0/)。

- **格式**：`<type>(<scope>): <subject>`，subject 后空一行接 body，末尾可选 footer
- **type**（必选）：

  | type | 用途 |
  |---|---|
  | `feat` | 新功能 |
  | `fix` | 缺陷修复 |
  | `docs` | 仅文档变更 |
  | `style` | 格式/样式，不影响逻辑 |
  | `refactor` | 重构，不改行为 |
  | `perf` | 性能优化 |
  | `test` | 测试 |
  | `build` | 构建系统/依赖 |
  | `ci` | CI 配置 |
  | `chore` | 其他不修改 src/test 的变更 |
  | `revert` | 回退先前的提交 |

- **scope**（可选）：影响范围（模块/组件/文件名）。本项目常用：
  `web`（前端面板）、`agent`（Agent 核心）、`api`（Web API）、`pluggin`（Lua 插件系统）、
  `docs`（文档内容）、`config`（工程配置）、`deps`（依赖）
- **subject**：中文、简短（≤50 字），概括本次提交的动机而非过程
- **body**：说明改动点、影响范围与必要背景；用**多个独立 `-m`** 组织
  （第一个 `-m` 为标题，后续每个 `-m` 一段无序列表项）；
  **禁止**用 `\n` 把多条说明塞进单个 `-m` 伪装多段
- **footer**（可选）：`BREAKING CHANGE:` 等；如需决策记录可用
  `Constraint:` / `Rejected:` / `Directive:` / `Tested:` trailer
- 示例：

  ```bash
  git commit \
    -m "fix(web): 修复登录页在暗色模式下对比度不足" \
    -m "- 调整按钮与背景色阶" \
    -m "- 补充 hover 态边框"
  ```
