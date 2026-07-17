# AGENTS.md

Guidance for OpenCode sessions working in this repo. The codebase is at an early
stage (entry point `cmd/server/main.go` is a `TODO` stub; many `internal/agent/*`
functions are declaration-only stubs). Trust code over `docs/` when they
conflict.

## Toolchain

- Go 1.25 (see `go.mod`). Module path is the literal `JuanNiang-Neo` (case + hyphen
  matter for imports, e.g. `JuanNiang-Neo/internal/adapter`).
- No Makefile, no CI, no test files yet. Run `go build ./...` and `go vet ./...`
  as the baseline checks; do not assume `go test` has anything to run.

## Terminology traps (these have bitten past readers)

- `docs/guidance.md` misspells the infra module as `inferstructure`. The real
  path is top-level `infrastructure/` (postgres, redis, sandbox, t2i).
- `docs/provider.md` documents an `internal/provider` package, but the actual
  import path is `internal/adapter` (package `adapter`).
- "Provider" is overloaded in this codebase:
  - `internal/adapter.Provider` = the OneBot11 reverse-WebSocket adapter.
  - `internal/agent/provider` = the LLM provider group (OpenAI-compatible etc.).
  Always resolve by full import path, not the word "provider".
- `pluggin` (double-g, single-n) is the *intentional* spelling for the Lua plugin
  system — module `internal/pluggin`, config file `pluggin.yaml`, plugin dir
  `data/pluggins`. Do not "fix" it to `plugin`.

## Layout

Top-level:
- `cmd/server/main.go` — program entry (currently a stub).
- `internal/` — app code, nothing exported outside the module:
  - `adapter/` — OneBot11 WS server + API + events + message segments.
  - `agent/` — Agent core; subpackages `mcp`, `memory`, `prompt`, `provider`,
    `session`, `skill`, `tool`. Aggregated by `HagoCenter` in `agent.go`.
  - `api/` — Hertz web engine + `middleware` + `router` + `service` (web admin).
  - `core/` — `acl`, `cache`, `dao`, `handler`, `models`.
  - `pluggin/` — Lua plugin engine.
- `infrastructure/` — `postgres`, `redis`, `sandbox`, `t2i` adapters.
- `data/` — runtime data; `data/pluggins/` holds Lua plugins + their
  `pluggin.yaml` configs (not committed).
- `docs/` — 完整文档:
  - `architecture.md` — 项目架构
  - `event-flow.md` — 事件流 & Agent 处理流程
  - `call-stack.md` — 调用栈
  - `implementation.md` — 实现细节
  - `api.md` — Web API 文档
  - `openapi.yaml` — OpenAPI 3.0 规范
  - `pluggin/` — 插件开发文档 (API参考/开发指南/架构/实现)
  - `dev/` — 原始设计文档 (`guidance.md`, `provider.md`)。
- `sql/`, `scripts/`, `deployments/`, `config/`, `web/`, `api/`, `pkg/` —
  currently Empty placeholders.

## Source-of-truth rules (from `docs/guidance.md`)

- Persistent state lives in Postgres + Redis cache. Agent/session/skill/plugin
  state must sync back to DB; do not add in-memory-only state.
- Lua plugins are the exception: their config is `data/pluggins/<name>/pluggin.yaml`.
- Web console auth: JWT, optional OIDC SSO, single admin user, initial password
  `Admin123` (change on first boot).
- OneBot11 API functions are registered as Agent Tools; new OneBot11-capable
  tools should wrap `internal/adapter.Provider` methods rather than re-implement.
- Long-running steps (MCP/Tool calls) run as background tasks and write results
  into bgtask memory; a separate Agent (not the chat Agent) drains the buffer
  and sends the final QQ message. Model errgroup-style concurrency.

## Conventions

- Logging via `log/slog` (structured). Match existing `slog.Info/Error(...,
  "key", val)` style; do not introduce `fmt.Println` or third-party loggers.
- Imports follow the std → third-party → `JuanNiang-Neo/...` block layout seen in
  `internal/adapter/provider.go`. Preserve that ordering.
- Comments and identifiers are mixed Chinese/English — keep that style in the
  file you are editing; do not translate.