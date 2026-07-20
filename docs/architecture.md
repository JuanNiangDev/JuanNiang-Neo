# JuanNiang-Neo 项目架构

## 概述

JuanNiang-Neo 是一个基于 OneBot11 协议的 QQ 聊天 Agent 系统，类 AstrBot。支持 LLM 驱动的多轮对话、MCP 工具调用、Lua 插件扩展，以及 Web 管理面板。

## 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                      cmd/server/main.go                     │
│                    (组装 & 启动入口)                          │
├──────────┬──────────┬──────────┬──────────┬────────────────┤
│  api/    │  pluggin/│  agent/  │  core/   │ infrastructure/│
│ (Web API)│ (Lua引擎)│ (Agent)  │ (核心库) │   (基础设施)    │
│          │          │          │          │                │
│ engine   │ 引擎管理  │ mcp      │ models   │  postgres      │
│ middlwr  │ API暴露  │ memory   │ dao      │  redis         │
│ router   │ 热加载   │ prompt   │ cache    │  sandbox       │
│ service  │ 事件拦截 │ provider │ acl      │  t2i           │
│          │          │ session  │ handler  │                │
│          │          │ skill    │          │                │
│          │          │ tool     │          │                │
├──────────┴──────────┴──────────┴──────────┴────────────────┤
│                     adapter/ (OneBot11 WS)                  │
│                 事件接收 · API调用 · 消息构造                 │
└─────────────────────────────────────────────────────────────┘
```

### 模块职责

| 模块 | 包路径 | 职责 |
|------|--------|------|
| **入口** | `cmd/server/main.go` | 组装所有模块, 启动服务, 优雅退出 |
| **适配器** | `internal/adapter/` | OneBot11 反向 WS 服务端: 事件解析、API 封装、消息段构造 |
| **Agent** | `internal/agent/` | Agent 核心: MCP、记忆、提示词、Provider、会话、技能、工具 |
| **核心库** | `internal/core/` | 数据模型 (GORM)、DAO、缓存 (Redis)、ACL |
| **Web API** | `internal/api/` | Hertz Web 管理面板: JWT 鉴权、CRUD API |
| **插件** | `internal/pluggin/` | gopher-lua 引擎: 插件生命周期、API 暴露、事件拦截 |
| **基础设施** | `infrastructure/` | Postgres、Redis、Sandbox、T2I 客户端 |

---

## 数据模型

```
AdminUser ─── 管理员用户 (单用户, JWT 登录)
Provider  ─── LLM Provider 配置 (Text/Image/Embedding)
MCPServer ─── MCP SSE 服务器配置
Skill     ─── 技能定义 (关键词/正则匹配 → Prompt+Tool)
ToolConfig ── 工具配置 (非内置工具；内置工具 ID 形如 builtin:<name>)
Prompt    ─── 提示词模板 (system/personality/custom; IsSystem=true 表示系统锁定不可改)
Onebot11Adapter ── OneBot11 适配器配置 (含 AdminQQNumbers 列表)
ChatArea  ─── 聊天区域 (private/group, Session+Memory 的集合)
  ├─ Session       ─── 会话: 标识符 + 消息历史 (Redis) + Token 统计
  ├─ ShortTermMemory ── 短期记忆: 滑动窗口, Postgres ChatRecord 查询
  ├─ LongTermMemory  ── 长期记忆: Postgres + HotArea (LRU)
  └─ BackgroundTask  ── 后台任务: DAG 步骤 + 结果缓冲区
ChatRecord ─── 聊天记录 (user/assistant/tool)
Plugin     ─── Lua 插件元数据 (Manifest.System=true 表示系统插件不可删/不可停用)
ACLRule    ─── ACL 规则 (用户×ChatArea×权限)
```

> **模型新增字段:**
> - `Prompt.IsSystem bool` — 标记系统锁定提示词，`BuildSystemPrompt` 强制拼接，Service 层 Update/Delete/Toggle 拒绝；启动时 `EnsureSystemPrompt(ctx)` 幂等播种名为 `__system_locked__` 的种子。
> - `Manifest.System bool` (Lua 插件清单字段) — 标记系统插件，三层守卫禁止卸载/删除/停用。
> - `Onebot11Adapter.AdminQQNumbers []string` — 管理员 QQ 号列表，由前端 `v-combobox` 编辑，运行时通过 `Adapter.Admins()` 透传给事件。
> - 已删除: `Prompt.Variables` 字段（模板渲染功能移除）。

### 模型关系

```
ChatArea (1) ──< Session (1:1)
ChatArea (1) ──< ShortTermMemory (1:1)
ChatArea (1) ──< LongTermMemory (1:1)
ChatArea (1) ──< ChatRecord (1:N)
ChatArea (1) ──< BackgroundTask (1:N)
ChatArea (1) ──< ACLRule (1:N per user)

LongTermMemory (1) ──< LongTermMemoryItem (1:N)
```

### 插件 API 分组

| 权限 | 全局表 / SDK 字段 | 函数数 | 说明 |
|------|--------|--------|------|
| 始终 | `log` (`jn.log`) | 3 | info/warn/error |
| 始终 | `json` (`jn.json`) | 2 | encode/decode |
| `onebot11` | `onebot11` (`jn.onebot11`) | 20 | 消息发送/群管理/信息查询/请求处理 |
| `http` | `http` (`jn.http`) | 2 | get/post (真实 HTTP 请求) |
| `database` | `database` (`jn.database`) | 2 | query/exec (命名空间隔离) |
| `cache` | `cache` (`jn.cache`) | 4 | get/set/del/exists (命名空间隔离) |
| `t2i` | `t2i` (`jn.t2i`) | 5 | generate/generate_url + toggle/is_active/get_config |
| `sandbox` | `sandbox` (`jn.sandbox`) | 6 | create/exec_shell/exec_python + toggle/is_active/get_config |
| `agent` | `agent` (`jn.agent`) | 16 | 配置查询 + Provider/MCP/Tool 切换 + Switch + Compact + 上下文 |
| 内置 | `jn.command` | 1 | register 多级命令 |

> **Lua SDK (NEW):** `internal/pluggin/sdk/jn.lua` 由 Go 二进制内嵌（`//go:embed`），启动时落盘到 `data/pluggins/sdk/jn.lua`。插件通过 `local jn = require("jn")` 引入，在 sumneko lua-language-server 中获得完整代码提示。SDK 仅捕获 Go 注入的全局表（log/json/onebot11/http/database/cache/t2i/sandbox/agent）作为模块字段，不引入额外行为；命令注册通过 `jn.command.register` 委托到 Go 侧 `__jn_internal.register_command`。

> **多级命令系统 (NEW):** `internal/pluggin/command.go` 实现 `CommandRegistry` + `CommandNode` 树形结构，支持多级命令派发（如 `/system provider switch <id>`）。最长前缀匹配，未命中 Handler 时友好列出子命令。命令 handler 签名：`func(args []string, event EventData) (consumed bool, reply string, err error)`。`/help` 在 Go 侧 `registerBuiltinCommands()` 注册，列出所有 Lua 插件注册的顶级命令；`/help <cmd> [sub...]` 列出该命令的子命令与用法。

> **system 系统插件 (NEW):** `internal/pluggin/systemplugin/`（main.lua + pluggin.yaml），manifest 标 `system: true`，封装 Agent/T2I/Sandbox/Session 操作。命令包括 `/system status`、`/system provider list|switch`、`/system mcp list|toggle`、`/system tool list|toggle`、`/system memory compact`、`/system t2i status|toggle`、`/system sandbox status|toggle`、`/system session list|info`。系统插件不允许删除或停用（三层保护：Manifest.System 字段 + PluginEngine.IsSystem() + Service 层 Toggle/Delete 守卫）。

---

## 状态管理

遵循 `docs/guidance.md` 的约定:

- **持久化状态**: Postgres (所有配置 + 数据)
- **缓存状态**: Redis (Session 消息历史)
- **插件数据**: DB 和 Cache 均通过命名空间隔离 (`pluggin_<name>_` / `pluggin:<name>:`)
- **例外**: Lua 插件配置由 `data/pluggins/<name>/pluggin.yaml` 管理
- **服务开关**: T2I 和 Sandbox 为可插拔服务, 未配置时自动返回未启用提示, 无需手动配置；启动时若 DB 无配置则调用 `InitConfig` 创建默认行，避免前端读取报"record not found"
- **原则**: 内存中的有状态模块 (Agent/Memory/Skill) 最终与 DB 同步
- **命令注册表**: `CommandRegistry` 为运行时内存对象，由 Lua 插件 `jn.command.register` 与 Go 侧 `registerBuiltinCommands(/help)` 共同填充；插件卸载时通过 `UnregisterPlugin(name)` 清理其注册的所有命令

## 前端 SPA 静态服务

后端复用 Hertz 引擎同端口 (默认 `:8090`) 服务前端 SPA, 不引入额外前端服务器:

```
浏览器 ──> :8090
              ├── /api/v1/*    -> Hertz 路由 (业务 API, JWT 鉴权)
              ├── /health       -> 内联健康检查
              └── 其它路径      -> internal/web.SPAHandler
                                   ├── /api/* (未命中) -> 标准 404 信封
                                   ├── 文件存在       -> serve 文件
                                   ├── 文件缺失        -> 回退 index.html (Vue Router history)
                                   └── 无 index.html  -> 200 引导提示页
```

- 入口: `cmd/server/main.go` 读取 `WEB_DIR` (默认 `web/dist`) 并传入 `engine.New(addr, webDir, svc)`。
- 实现: `internal/web/web.go` 的 `SPAHandler(webDir)`。挂在 `h.NoRoute(...)` 上, 兜底所有未命中路由。
- **不嵌入二进制**: 前端以磁盘文件形式存在 `WEB_DIR`, 便于"只换前端不重编 Go"的部署节奏; 与项目"配置在盘上"哲学一致。
- **开发模式**: 前端走 Vite `:3000` 热更新, `vite.config.ts` 代理 `/api` → `:8090`, 因此开发期 Go 的 SPA fallback 不会被触发。
- **生产模式 (容器)**: `deployments/Dockerfile` 多阶段构建把 `web/dist` 拷到 `/app/web/dist`, `ENV WEB_DIR=/app/web/dist`, 单容器同端口暴露 Web 面板 + API + 前端。
- 路径穿越防护: `SPAHandler` 通过 `filepath.Rel` 校验目标文件必须落在 `webDir` 之内。
