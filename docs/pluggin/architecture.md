# 插件架构

## 概述

JuanNiang-Neo 的 Lua 插件系统基于 `gopher-lua` (Go-Lua 绑定), 允许用户通过 Lua 脚本扩展机器人功能。插件可以：

- 拦截 OneBot11 消息事件
- 调用完整 OneBot11 API (20 个函数)
- 发起 HTTP 请求
- 操作数据库和缓存 (命名空间隔离)
- 使用文生图 (T2I) 和沙箱 (Sandbox) 服务
- 管理 Agent 配置 (Provider/MCP/Tool 切换、记忆 Compact、运行时查询)
- **通过 `jn.command.register` 注册多级命令**，由系统统一派发与 `/help` 自动生成
- **通过 `require("jn")` 引入内嵌 SDK**，获得 IDE 完整类型提示

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    PluginEngine                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ Plugin A │  │ Plugin B │  │ system   │  (独立 LState)   │
│  │ LState₁  │  │ LState₂  │  │ LState₃  │  system: true   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                 │
│       │              │              │                       │
│       ▼              ▼              ▼                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   SDK Layer (//go:embed sdk/jn.lua)                 │   │
│  │   local jn = require("jn")                          │   │
│  │   jn.log / jn.json / jn.onebot11 / jn.http / ...    │   │
│  │   jn.command.register (委托 __jn_internal)          │   │
│  └─────────────────────────────────────────────────────┘   │
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
│  │  │          │          │ toggle   │ exec_python  │  │   │
│  │  │          │          │ is_active│ toggle       │  │   │
│  │  │          │          │ get_cfg  │ is_active    │  │   │
│  │  │          │          │          │ get_cfg      │  │   │
│  │  ├──────────┴──────────┴──────────┴──────────────┤  │   │
│  │  │ agent.* (16 fn)                                │  │   │
│  │  │ 查询: providers/mcp_servers/skills/sessions/   │  │   │
│  │  │       prompts/tools/plugins                    │  │   │
│  │  │ 管理: set_provider_active / switch_provider    │  │   │
│  │  │       set_mcp_active / toggle_mcp / list_mcps  │  │   │
│  │  │       toggle_tool / list_tools                 │  │   │
│  │  │       list_runtime_providers                   │  │   │
│  │  │ 上下文: get_current_chat_area / compact_memory │  │   │
│  │  └───────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                         │                                   │
│         ┌───────────────┼───────────────┐                  │
│         ▼               ▼               ▼                  │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐          │
│  │ SendAdapter│  │ *gorm.DB │   │  AgentOperator│         │
│  │ (OneBot11) │  │ (命名空间)│   │ (Provider/MCP/ │        │
│  └──────────┘   └──────────┘   │  Tool/T2I/Sandbox│       │
│                                │  Compact/Ctx)    │       │
│                                └──────────────┘          │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   CommandRegistry (多级命令树)                       │   │
│  │   root → system → help (内置)                        │   │
│  │        → system → status / provider / mcp / ...      │   │
│  │        → <plugin> → <subcmd> → ...                   │   │
│  │   Dispatch("/cmd sub args", event)                   │   │
│  │   ListByPlugin(name) / FormatHelp(path)              │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 事件流中的插件位置

```
OneBot11 Event → PluginEngine.OnMessage(event)
                    │
                    │ 存储 currentEv (供 agent.* 查询当前上下文)
                    │
                    ├─ 若 raw_message 以 "/" 开头:
                    │    └─ CommandRegistry.Dispatch(raw, event)
                    │         ├─ 命中 handler → sendReply(reply) + consumed=true
                    │         ├─ 停在非根节点 → 列出子命令提示 + consumed=true
                    │         └─ 完全未命中 → fallback 到 on_message
                    │
                    ├─ for each plugin (按 Map 迭代顺序):
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
├── sdk/                  # 内嵌 SDK (启动时由 ensureEmbeddedAssets 落盘, 总是覆盖)
│   └── jn.lua            # SDK 源码 (//go:embed sdk/jn.lua)
├── system/               # 系统插件 (启动时若不存在则落盘, 不覆盖)
│   ├── pluggin.yaml      # system: true
│   └── main.lua          # 注册 /system status/provider/mcp/tool/...
├── ping/                 # 用户插件, 名 = 目录名
│   ├── pluggin.yaml      # 插件清单 (必需)
│   └── main.lua          # 入口脚本 (默认)
└── my-plugin/
    ├── pluggin.yaml
    ├── main.lua
    └── utils.lua         # 插件内部依赖
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
  - t2i                    # 文生图 (5 函数: generate/generate_url/toggle/is_active/get_config)
  - sandbox                # 沙箱 (6 函数: create/exec_shell/exec_python/toggle/is_active/get_config)
  - agent                  # Agent 操作 (16 函数: 查询+管理+切换+Compact)
# system: true             # 仅系统插件需要, 标记后禁止 API 停用/删除/卸载
```

`system: true` 字段标记系统插件，由 `Manifest.System` 接收。该字段触发三层守卫：
1. `PluginEngine.IsSystem(name)` 读取已加载 manifest
2. `PluginEngine.Unload(name)` 拒绝卸载
3. Service 层 `TogglePlugin` / `DeletePlugin` 返回 `40028 PluginIsSystem`

### 命名空间隔离

| 资源 | 隔离方式 | 示例 |
|------|---------|------|
| 数据库 | 表名前缀 `pluggin_<name>_` | `pluggin_myplugin_config` |
| 缓存 | Key 前缀 `pluggin:<name>:` | `pluggin:myplugin:mykey` |

### 服务开关检测

T2I 和 Sandbox 为可插拔服务。如果服务未配置或未启用：
- **Lua 调用** (`generate` / `create` / `exec_*`): 返回 `(nil, "服务未启用")`
- **管理函数** (`toggle` / `is_active` / `get_config`): 通过 `AgentOperator` / `dao` 查询，不依赖客户端实例
- 运行时通过 `agentOp.GetT2IClient()` / `agentOp.GetSandboxClient()` 获取最新实例，支持热更新

## Lua SDK

`internal/pluggin/sdk/jn.lua` 由 `//go:embed` 内嵌到二进制，启动时由 `ensureEmbeddedAssets()` 落盘到 `data/pluggins/sdk/jn.lua`（**每次启动总是覆盖**，保持与二进制版本同步）。

`injectSDK(L, pluginName)` 在每条 LState 中将 SDK 目录追加到 `package.path`，使插件可通过：

```lua
local jn = require("jn")
```

引入。SDK 仅捕获 Go 注入的全局表并重新暴露为模块字段（`jn.log = log`、`jn.t2i = t2i`、...），不引入额外行为。`jn.command.register` 是唯一例外，它内部委托到 Go 侧 `__jn_internal.register_command` 全局函数。

SDK 附带 sumneko lua-language-server 的 `---@class` / `---@field` / `---@alias` 类型注解，IDE 可获得：
- 函数签名提示（参数类型、返回值类型）
- 字段补全（`jn.t2i.` 弹出 `generate` / `generate_url` / `toggle` / `is_active` / `get_config`）
- 内联文档（hover 显示描述）

> **混用**: SDK 字段与全局表完全等价，可在同一插件中混用 `jn.log.info(...)` 与 `log.info(...)`。

## 多级命令系统

`CommandRegistry` (`internal/pluggin/command.go`) 维护一棵 `CommandNode` 树，根节点为虚拟节点。

```go
type CommandNode struct {
    Name        string
    Opts        CommandOpts    // description / usage
    Handler     CommandHandler // nil = 分组节点
    PluginName  string         // 注册方插件名
    Children    map[string]*CommandNode
}
```

### 派发流程

`PluginEngine.OnMessage` 在 `raw_message` 以 `/` 开头时调用 `commands.Dispatch(raw, event)`：

1. 按 `strings.Fields` 切分 token，去掉 `tokens[0]` 的前导 `/`
2. 沿命令树逐级匹配，记录最后一个 `Handler != nil` 的节点（最长前缀匹配）
3. 命中 handler → 调用 `handler(args, event)`，`args` 为命中路径之后的所有 token
   - `consumed=true` → `PluginEngine.sendReply(event, reply)` 自动回复（若 `reply` 非空），跳过 Agent 与 `on_message`
   - `consumed=false` 且 `reply == ""` 且 `err == nil` → 视为未消费，fallback 到 `on_message`
4. 未命中 handler 但停在某个非根节点 → 列出该节点的子命令作为提示，`consumed=true`
5. 完全未命中 → `consumed=false`，fallback 到 `on_message`

### 命令注册

插件通过 `jn.command.register(path, handler, opts)` 注册命令：

```lua
local jn = require("jn")
jn.command.register({"myplugin", "greet"}, function(args, event)
    return true, "你好！"  -- consumed=true, reply="你好！"
end, { description = "打招呼", usage = "/myplugin greet" })
```

Go 侧 `injectCommandAPI` 注入 `__jn_internal.register_command` 全局函数：
- 解析 `path` 参数（string 或 string[]）
- 校验 `handler` 类型
- 通过 `L.SetGlobal(refKey, handlerFn)` 保留 handler 引用防止 GC
- 调用 `registry.Register(pluginName, path, opts, handler)`，handler 在插件 LState 中通过 `L.CallByParam` 调用

### 卸载联动

`PluginEngine.Unload(name)` 调用 `commands.UnregisterPlugin(name)`，递归清除该插件注册的所有命令节点（设 `Handler=nil / PluginName=""`，并清理空叶子节点）。

### `/help` 自动生成

`registerBuiltinCommands()` 在 `NewPluginEngine` 时注册 `/help` 命令（路径 `["system", "help"]`，挂在 `system` 插件名下），调用 `commands.FormatHelp(args)` 输出：
- 无参数 → 顶层命令列表
- 有参数 → 该路径节点的描述/用法 + 子命令列表 + 来源插件

## `system` 系统插件

`internal/pluggin/systemplugin/` 由 `//go:embed` 内嵌 `pluggin.yaml` + `main.lua`：

- 清单中 `system: true`，触发三层守卫
- `ensureEmbeddedAssets()` 仅在 `data/pluggins/system/pluggin.yaml` 不存在时落盘（**允许用户自定义修改**，不覆盖）
- `main.lua` 通过 `require("jn")` + `jn.command.register` 注册系统命令组：
  - `/system status` — 系统状态概览
  - `/system provider` — Provider 管理（list / switch / toggle）
  - `/system mcp` — MCP 管理（list / toggle）
  - `/system tool` — Tool 管理（list / toggle）
  - `/system memory` — 记忆管理（compact）
  - `/system t2i` — T2I 服务管理（status / toggle）
  - `/system sandbox` — Sandbox 服务管理（status / toggle）
  - `/system session` — Session 查询

## 插件生命周期

```
启动 (LoadAll):
  1. ensureEmbeddedAssets():
     - SDK 总是覆盖到 data/pluggins/sdk/jn.lua
     - system 插件仅在不存在时落盘到 data/pluggins/system/
  2. 遍历 data/pluggins/ 下所有子目录 (跳过 sdk/):
     - 调用 Load(name) 加载每个插件

加载 (Load):
  1. 读取 pluggin.yaml → 解析 Manifest (含 System 字段)
  2. 创建 LState (lua.NewState)
  3. injectSDK(L, name): 将 data/pluggins/sdk 追加到 package.path
  4. injectBaseAPI(L, name, permissions): 按 permissions 注入 Go 函数到 Lua 全局表
     - 始终注入: log, json
     - 按权限注入: onebot11, http, database, cache, t2i, sandbox, agent
     - T2I/Sandbox nil 检测: 服务不可用时注入返回 error 的占位函数
  5. injectCommandAPI(L, name): 注入 __jn_internal.register_command
  6. L.DoFile(entry.lua): 执行插件脚本 (此时 require("jn") 可用)
  7. 注册到 Plugins map

运行:
  - OnMessage(event): 命令优先派发, 未命中再调用 on_message
  - PluginEngine 自动存储 currentEv 供 agent.get_current_chat_area() 查询
  - 插件可调用所有已注入的 Go API

热加载 (Reload):
  1. Unload: commands.UnregisterPlugin(name) → LState.Close() → 从 map 删除
  2. Load: 重新执行上述流程

卸载 (Unload):
  1. 系统插件守卫: Manifest.System=true → 拒绝卸载
  2. commands.UnregisterPlugin(name): 移除该插件注册的所有命令
  3. LState.Close() → 释放 Lua VM
  4. 从 Plugins map 删除
```

## 依赖注入

`PluginEngine` 通过构造函数接收所有外部依赖：

```go
func NewPluginEngine(
    basePath string,      // 插件目录
    adapter SendAdapter,  // OneBot11 适配器 (消息发送)
    db *gorm.DB,          // 数据库 (命名空间隔离)
    cache *cache.Cache,   // Redis 缓存 (命名空间隔离)
    t2i *t2icaller.Client,     // T2I 服务 (可为 nil, 运行时通过 agentOp.GetT2IClient() 获取)
    sandbox *sandboxcaller.Client, // Sandbox 服务 (可为 nil, 运行时通过 agentOp.GetSandboxClient() 获取)
    dao *dao.Bundle,       // DAO (Agent 配置查询)
    agentOp AgentOperator, // Agent 操作 (Provider/MCP/Tool/T2I/Sandbox/Compact/上下文)
) *PluginEngine
```

构造函数末尾调用 `registerBuiltinCommands()` 注册 `/help` 内置命令，并初始化 `CommandRegistry`。

### AgentOperator 接口（增强版）

```go
type AgentOperator interface {
    // Provider 管理
    SetProviderActive(ctx, id, active) error
    SwitchProvider(ctx, id) error                // 切换主 Provider
    GetProviderGroup() ProviderGroupAccess       // 暴露 List() / GetActive(id)

    // MCP 管理
    SetMCPActive(ctx, id, active) error
    GetMCPGroup() MCPGroupAccess                 // 暴露 ListMCPs() / IsConnected(id)

    // Tool 管理
    SetToolActive(ctx, name, active) error
    GetToolRegistry() ToolRegistryAccess         // 暴露 ListTools() / IsActive(name)

    // T2I / Sandbox 启停
    SetT2IActive(ctx, active) error
    SetSandboxActive(ctx, active) error
    GetT2IClient() *t2icaller.Client             // 运行时最新客户端
    GetSandboxClient() *sandboxcaller.Client

    // 记忆与上下文
    CompactMemory(ctx, chatAreaID) error
    GetChatAreaID(userID, groupID, messageType) string
}
```

`HagoCenter` 实现此接口，使插件可以操作 Agent 的核心功能。

## 内存模型

每个插件拥有独立的 `lua.LState`, 这意味着:
- 插件间的全局变量互不干扰
- 每个插件的 Lua VM 完全隔离
- 插件崩溃不影响其他插件或主进程 (gopher-lua 的 panic 被 recover 捕获)
- `currentEv` 字段在 `OnMessage` 期间存储当前事件上下文, 供 `agent.*` API 使用
- `CommandRegistry` 是 PluginEngine 内的运行时内存对象，命令注册不持久化到 DB；每次启动时由各插件的 `main.lua` 重新注册

Go ←→ Lua 类型映射:

| Go 类型 | Lua 类型 |
|---------|---------|
| `string` | `string` |
| `int / int64 / float64` | `number` |
| `bool` | `boolean` |
| `map[string]any` | `table` (key-value) |
| `[]any` | `table` (array, 1-indexed) |
| `nil` | `nil` |
