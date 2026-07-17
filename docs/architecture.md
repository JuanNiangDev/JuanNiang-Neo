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
ToolConfig ── 工具配置 (非内置工具)
Prompt    ─── 提示词模板 (system/personality/custom)
ChatArea  ─── 聊天区域 (private/group, Session+Memory 的集合)
  ├─ Session       ─── 会话: 标识符 + 消息历史 (Redis) + Token 统计
  ├─ ShortTermMemory ── 短期记忆: 滑动窗口, Postgres ChatRecord 查询
  ├─ LongTermMemory  ── 长期记忆: Postgres + HotArea (LRU)
  └─ BackgroundTask  ── 后台任务: DAG 步骤 + 结果缓冲区
ChatRecord ─── 聊天记录 (user/assistant/tool)
Plugin     ─── Lua 插件元数据
ACLRule    ─── ACL 规则 (用户×ChatArea×权限)
```

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

---

## 状态管理

遵循 `docs/guidance.md` 的约定:

- **持久化状态**: Postgres (所有配置 + 数据)
- **缓存状态**: Redis (Session 消息历史、短期记忆热数据)
- **例外**: Lua 插件配置由 `data/pluggins/<name>/pluggin.yaml` 管理
- **原则**: 内存中的有状态模块 (Agent/Memory/Skill) 最终与 DB 同步
