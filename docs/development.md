# JuanNiang-Neo 项目开发文档

本文档面向二次开发者，整合原 `docs/implementation.md`、`docs/dev/guidance.md`、`docs/dev/provider.md` 中对开发最有价值的内容，按"该读什么、该改什么、不该动什么"组织。代码为准；与 `docs/` 冲突时以代码为准。

## 工具链

- Go 1.25（见 `go.mod`）。模块路径 `JuanNiang-Neo`（大小写与连字符都重要，如 `JuanNiang-Neo/internal/adapter`）
- 前端：Node 18+ / npm，Vue 3 + Vite 6 + Vuetify 3（位于 `web/`）
- 基线检查（根 `Makefile`）：
  - `make build` — 前端 → `web/dist` + Go 二进制 `bin/juan-niang-neo`
  - `make vet` — `go vet ./...`
  - `make lint` — `go vet` + `web-typecheck` (`vue-tsc` ≥ 2.x)
  - `make dev` — Vite + Go 并行
- **无 CI，无 `*_test.go` 体系**（仅 `internal/agent/skill/skill_test.go` 一个；`make test` 是占位）

## 术语陷阱（别再被坑）

- `docs/guidance.md` 把 infra 模块拼成了 `inferstructure`，真实路径是 `infrastructure/`。
- `internal/adapter.Provider` ≠ `internal/agent/provider.ProviderGroup`：前者是 OneBot11 反向 WS 适配器，后者是 LLM Provider 组。永远按完整 import path 解析，别只看 "Provider" 这个词。
- `pluggin`（双 g 单 n）是**有意**拼写：模块 `internal/pluggin`、配置 `pluggin.yaml`、插件目录 `data/pluggins`。不要"修复"为 `plugin`。
- `web/dist` 不嵌入二进制；改前端不必重编 Go。

## 该读什么（按开发目标定位）

| 你想做 | 起点 |
|--------|------|
| 加一条 Web API | `internal/api/router/router.go`（68 路由在此注册）、`internal/api/service/service.go`（handler）、`internal/api/dto/` |
| 加 Agent 内置工具 | `internal/agent/tool/builtin.go::RegisterBuiltinTools`；参考既有 `send_*_msg`/`browser_search` 等 |
| 接新 LLM 协议 | `internal/agent/provider/provider.go`（现 OpenAI 兼容 + Eino ADK adapter），实现 `Provider` 接口 |
| 调整 Agent 并发限制 | `internal/agent/concurrency.go`（默认 8/ChatArea） |
| 修改分段回复算法 | `internal/agent/event.go::splitMessages`（Maibot 式自然断句） |
| 加 ACL 维度 | `internal/core/acl/acl.go::Check` + `models.ACLRule` |
| 写 Lua 插件 | 读 [pluggin/development.md](pluggin/development.md) |
| 改前端页面 | `web/src/views/*.vue`（22 页）、`web/src/api/*`（typed endpoints）、`web/src/router/index.ts` |
| 改 Plugin SDK | `internal/pluggin/sdk/jn.lua`（`//go:embed`，带 LuaCATS 注解） |
| 加数据模型 | `internal/core/models/` 加 GORM model + `core.go::AutoMigrate` 注册 + `internal/core/dao/` DAO + `dao.NewBundle` 接入 |

## 项目目录速查

```
cmd/server/main.go            入口 (组装/启动/退出)
internal/
  adapter/      OneBot11 反向 WS + Webhook (Adapter / WebhookAdapter / Event / Segment)
  agent/        Agent 核心 (HagoCenter + 11 子包)
    provider/   OpenAI 兼容客户端 (Chat / Vision)
    mcp/        MCP 客户端 (mark3labs/mcp-go SSE)
    memory/     三层记忆 (shortterm Redis / longterm PG+HotArea / skillmem 全局技能记忆)
    prompt/     系统锁定提示词 + 拼接
    session/    会话 + ChatRecord 持久化
    skill/      关键词/正则技能匹配
    tool/       ToolRegistry + 内置工具
    cronjob/    robfig/cron 调度器
    concurrency.go  每 ChatArea 并发令牌 (chan struct{})
    eino_middleware.go  Eino ADK 中间件 (ACL + BeforeAgent 注入)
  api/          Hertz Web (engine + middleware + router + service)
  core/         Init / dao.Bundle / models (22 表) / acl / cache
  pluggin/      Lua 引擎 + 命令树 + 内嵌 SDK + 系统插件
  web/          SPAHandler (NoRoute 兜底)
  logging/      fatih/color 彩色 stdout + JSON 格式化 + 调用栈 + Hub(SSE)
infrastructure/
  postgres/ redis/      基础客户端 (功能选项)
  sandbox/ t2i/         含 /handler (caller 子包, 真正 Client)
web/                    Vue 3 SPA (22 views)
data/pluggins/          Lua 插件 (配置 pluggin.yaml, 非入库)
deployments/            Dockerfile (3 段) + docker-compose.yaml
docs/                   本文档树
```

## 当前实现状态（哪些是真实现，哪些是桩）

| 项 | 状态 |
|----|------|
| Adapter 反向 WS / Webhook / API | ✅ 完整实现 |
| EventLoop / processEvent / handleMessage / handleToolCalls | ✅ |
| BgTaskExecutor (errgroup DAG) / DrainerAgent | ✅ |
| CronJobManager (robfig/cron) | ✅ |
| Provider (OpenAI 兼容 + 流式 + Vision) | ✅ |
| MCP (SSE) | ✅ |
| Memory 三层 (shortterm/longterm+HotArea/bgtask) | ✅ |
| Prompt (SystemLocked + BuildFullContext) | ✅ |
| ToolRegistry + 内置工具 | ✅（除 `vision` builtin 只返回提示，真 Vision 走 reply_strategy.go）|
| Lua 插件引擎 + 命令树 + 系统 SDK + 系统插件 | ✅ |
| Web API 68 路由 + JWT + SSE 日志 | ✅ |
| 前端 22 页 (Vue 3 + Vuetify 3) | ✅ |
| AgentLite 模式 / StripMarkdown / 分消息段 | ✅ |
| `internal/agent/memory/root.go::Memory` 接口 | ⚠ 空 stub (无方法) |
| `internal/agent/skill/root.go`、`prompt/root.go` | ⚠ 占位 (实现在 .go) |
| `HagoCenter.Stop()` | ⚠ 仅关两路 channel，不显式停 BG/Drainer/EventLoop/CronJob ctx (赖外层 ctx) |
| `HagoCenter.SetToolActive` | ⚠ 停用只能 Unregister，无法重新注册已 Unregister 的 builtin |
| `internal/core/handler/` | ⚠ 空目录占位 |
| `database` 插件权限的 `prefixSQL` | ⚠ 桩，未生效，任意 SQL |
| 内置 `vision` 工具 | ⚠ 返回提示，不真正取图（真 Vision 见 reply_strategy.go:70） |

## 约定（必须遵守）

- **持久化**：所有有状态模块（Agent/Session/Skill/Memory/Provider/MCP/Plugin 元数据等）的状态必须同步回 Postgres；**不引入纯内存状态**。Redis 仅作 Session 消息历史、短期记忆窗口、PubSub 与插件/Agent 缓存。
- **日志**：`log/slog` 结构化，匹配既有 `slog.Info("...", "key", val)` 风格；**不引入** `fmt.Println` 或第三方 logger。
- **导入顺序**：std → 第三方 → `JuanNiang-Neo/...` 三段。见 `internal/adapter/provider.go`。
- **注释与标识符**：混合中英文，保持所在文件原有风格，**不要翻译**。
- **OneBot11 API 复用**：新增 OneBot11 能力应包装 `internal/adapter.Provider` 方法，不要再实现一份。
- **长任务**：所有 Tool 调用同步执行，Eino ADK ChatModelAgent 驱动 ReAct 循环，ConcurrencyManager 限制每 ChatArea 并行 Agent 数。
- **开发配置**：开发时使用 `dev.yaml` 配置基础设施连接端点（`make run` 自动读取；`cp dev.yaml.example dev.yaml`）。
- **Web 控制台**：JWT 鉴权，单管理员，初始化 `admin / Admin123`（首次启动务必改）。
- **插件配置仍走磁盘**：`data/pluggins/<name>/pluggin.yaml`，不入 DB。
- **错误码**：业务错误用 `dto.Response{...}`（如 `dto.AdapterNotInitialized`），通过 `dto.GenFinalResponse` 包装，HTTP 200。

## 数据模型与持久化策略

- Postgres 拥有**所有**持久状态（22 张表，见 `internal/core/core.go::AutoMigrate`）
- Redis 仅作：短期记忆滑动窗口（`shortterm:msgs:<areaID>` List）、PubSub 任务结果通知、插件/Agent 任意缓存
- `ChatRecord.id` 为自增 int64（不是 UUID，多数表用 UUID），保留这个差异
- 单行配置表（`Onebot11Adapter`/`WebhookConfig`/`T2IConfig`/`SandboxConfig`）固定 `id=1`，首次 `InitConfig` 用 `OnConflict DoNothing` 建默认行
- `ReplyStrategyConfig` 无 `DeletedAt`（单例）
- 长期记忆 `Embedding []byte` 字段已就位但**当前未做向量检索**，搜索走 `ILIKE` 内容匹配

## 改动检查流程

改完代码后推荐：

```bash
make vet           # go vet
make lint          # go vet + 前端 typecheck
make build         # 全量构建验证 (前端 + Go 二进制)
# 若改了 web/src: make web-lint
```

没有单元测试 CI，请在日志 + Web 面板手工验证关键路径。

## 写 Agent 工具的最小范式

```go
// internal/agent/tool/builtin.go, 在 RegisterBuiltinTools 内:
h.Register(...)
tool := NewTool("my_tool", "用途描述",
    StringParam("arg1"), /* JSON Schema 参数 */
    func(ctx context.Context, args map[string]any) (string, error) {
        // args["arg1"].(string)
        // 调 adapter / MCP / DB
        return "结果文本（喂给 LLM 的 tool-role msg）", nil
    })
// 是否长任务？设置 BaseTool.longRunning=true 让 handleToolCalls 走 BgTaskExecutor
```

参数帮助函数：`StringParam/Int64Param/MessageParam/GroupIDUserIDParams/TimeParams`（`tool/root.go`）。

## 写 Web API 的最小范式

```go
// internal/api/router/router.go 在 /api/v1 group 内注册:
g.POST("/myresource", svc.AddMyResource)

// internal/api/service/service.go 在 *Service 上加 handler:
func (s *Service) AddMyResource(ctx *app.RequestContext) {
    var req MyReq
    if err := ctx.BindAndValidate(&req); err != nil {
       c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{...}))
       return
    }
    // s.DAO.<...> 操作
    c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, resp))
}
```

DTO 转换集中在 `dto/transfer.go`，枚举预定义在 `dto/response.go`。

## 提交时注意

- 不要把 `web/node_modules/`、`web/dist/`、`bin/`、`data/`、`docs/` 改动意外提交（`.gitignore` 已排除 `data/pluggins/`）
- 系统插件 `internal/pluggin/systemplugin/` 是内嵌资源，修改需在 `go:embed` 范围内
- 不要修复 `pluggin` 拼写、不要翻译中英混排的注释/标识符