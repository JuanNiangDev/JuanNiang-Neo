# 插件架构

## 概述

JuanNiang-Neo 的 Lua 插件系统基于 `gopher-lua` (Go-Lua 绑定), 允许用户通过 Lua 脚本扩展机器人功能。插件可以：

- 拦截 OneBot11 消息事件
- 调用完整 OneBot11 API (20 个函数)
- 发起 HTTP 请求
- 操作数据库和缓存 (命名空间隔离)
- 使用文生图 (T2I) 和沙箱 (Sandbox) 服务
- 管理 Agent 配置 (Provider 切换、MCP 管理、记忆 Compact)

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    PluginEngine                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ Plugin A │  │ Plugin B │  │ Plugin C │  (独立 LState)   │
│  │ LState₁  │  │ LState₂  │  │ LState₃  │                 │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                 │
│       │              │              │                       │
│       ▼              ▼              ▼                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │          Shared Go API Layer (按权限注入)            │   │
│  │  ┌──────────┬──────────┬──────────┬──────────────┐  │   │
│  │  │ log.*    │ json.*   │ onebot11 │ http.*       │  │   │
│  │  │ info     │ encode   │  .20 fn  │ get/post     │  │   │
│  │  │ warn     │ decode   │          │              │  │   │
│  │  │ error    │          │          │              │  │   │
│  │  ├──────────┼──────────┼──────────┼──────────────┤  │   │
│  │  │ database │ cache    │ t2i.*    │ sandbox.*    │  │   │
│  │  │ query    │ get/set  │ generate │ create       │  │   │
│  │  │ exec     │ del/exist│ gen_url  │ exec_shell   │  │   │
│  │  ├──────────┴──────────┴──────────┴──────────────┤  │   │
│  │  │ agent.*                                       │  │   │
│  │  │ get_providers  set_provider_active            │  │   │
│  │  │ get_mcp_servers set_mcp_active                │  │   │
│  │  │ get_current_chat_area  compact_memory         │  │   │
│  │  │ get_skills/sessions/prompts/tools/plugins     │  │   │
│  │  └───────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                         │                                   │
│         ┌───────────────┼───────────────┐                  │
│         ▼               ▼               ▼                  │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐          │
│  │ SendAdapter│  │ *gorm.DB │   │  AgentOperator│         │
│  │ (OneBot11) │  │ (命名空间)│   │ (Provider/MCP  │        │
│  └──────────┘   └──────────┘   │  Compact/Ctx) │          │
│                                └──────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

## 事件流中的插件位置

```
OneBot11 Event → PluginEngine.OnMessage(event)
                    │
                    │ 存储 currentEv (供 agent.* 查询当前上下文)
                    │
                    ├─ for each plugin:
                    │    └─ lua.call("on_message", event)
                    │         ├─ consumed=true  → 跳过 Agent
                    │         └─ consumed=false → 继续
                    │
                    ▼
                 Agent 处理
```

## 插件目录结构

```
data/pluggins/
├── ping/                  # 插件名 = 目录名
│   ├── pluggin.yaml       # 插件清单 (必需)
│   └── main.lua           # 入口脚本 (默认)
└── my-plugin/
    ├── pluggin.yaml
    ├── main.lua
    └── utils.lua          # 插件内部依赖
```

## pluggin.yaml 格式

```yaml
name: my-plugin
version: "1.0.0"
author: YourName
description: "插件描述"
entry: main.lua
permissions:
  - onebot11               # OneBot11 消息发送/群管理/信息查询
  - http                   # HTTP 请求
  - database               # 数据库访问 (命名空间: pluggin_<name>_)
  - cache                  # 缓存访问   (命名空间: pluggin:<name>:)
  - t2i                    # 文生图 (服务未启用时返回 nil+err)
  - sandbox                # 沙箱 (服务未启用时返回 nil+err)
  - agent                  # Agent 配置查询 + Provider/MCP 切换 + Compact
```

### 命名空间隔离

| 资源 | 隔离方式 | 示例 |
|------|---------|------|
| 数据库 | 表名前缀 `pluggin_<name>_` | `pluggin_myplugin_config` |
| 缓存 | Key 前缀 `pluggin:<name>:` | `pluggin:myplugin:mykey` |

### 服务开关检测

T2I 和 Sandbox 为可插拔服务。如果服务未配置或未启用：
- **Lua 调用**: 返回 `(nil, "服务未启用")`
- **LLM ToolCall**: 返回工具调用失败提示
- 无需在 config.yaml 或环境变量中手动配置

## 插件生命周期

```
加载 (Load):
  1. 读取 pluggin.yaml → 解析 Manifest
  2. 创建 LState (lua.NewState)
  3. injectBaseAPI: 根据 permissions 注入 Go 函数到 Lua 全局表
     - 始终注入: log, json
     - 按权限注入: onebot11, http, database, cache, t2i, sandbox, agent
     - T2I/Sandbox nil 检测: 服务不可用时注入返回 error 的占位函数
  4. L.DoFile(entry.lua): 执行插件脚本
  5. 注册到 Plugins map

运行:
  - on_message(event) 被事件循环调用
  - PluginEngine 自动存储 currentEv 供 agent.get_current_chat_area() 查询
  - 插件可调用所有已注入的 Go API

热加载 (Reload):
  1. Unload: LState.Close() → 从 map 删除
  2. Load: 重新执行上述流程

卸载 (Unload):
  1. LState.Close() → 释放 Lua VM
  2. 从 Plugins map 删除
```

## 依赖注入

`PluginEngine` 通过构造函数接收所有外部依赖：

```go
func NewPluginEngine(
    basePath string,      // 插件目录
    adapter SendAdapter,  // OneBot11 适配器 (消息发送)
    db *gorm.DB,          // 数据库 (命名空间隔离)
    cache *cache.Cache,   // Redis 缓存 (命名空间隔离)
    t2i *t2icaller.Client,     // T2I 服务 (可为 nil)
    sandbox *sandboxcaller.Client, // Sandbox 服务 (可为 nil)
    dao *dao.Bundle,       // DAO (Agent 配置查询)
    agentOp AgentOperator, // Agent 操作 (Provider/MCP 切换, Compact, 上下文)
) *PluginEngine
```

### AgentOperator 接口

```go
type AgentOperator interface {
    SetProviderActive(ctx, id, active) error     // 启用/停用 Provider
    SetMCPActive(ctx, id, active) error          // 启用/停用 MCP
    CompactMemory(ctx, chatAreaID) error         // Compact 短期记忆
    GetChatAreaID(userID, groupID, messageType) string  // 获取 Chat-Area ID
}
```

`HagoCenter` 实现此接口，使插件可以操作 Agent 的核心功能。

## 内存模型

每个插件拥有独立的 `lua.LState`, 这意味着:
- 插件间的全局变量互不干扰
- 每个插件的 Lua VM 完全隔离
- 插件崩溃不影响其他插件或主进程 (gopher-lua 的 panic 被 recover 捕获)
- `currentEv` 字段在 `OnMessage` 期间存储当前事件上下文, 供 `agent.*` API 使用

Go ←→ Lua 类型映射:

| Go 类型 | Lua 类型 |
|---------|---------|
| `string` | `string` |
| `int / int64 / float64` | `number` |
| `bool` | `boolean` |
| `map[string]any` | `table` (key-value) |
| `[]any` | `table` (array, 1-indexed) |
| `nil` | `nil` |
