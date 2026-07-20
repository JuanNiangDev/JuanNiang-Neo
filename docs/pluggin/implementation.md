# 插件引擎实现细节

## 核心结构

```go
type PluginEngine struct {
    mu         sync.RWMutex
    plugins    map[string]*LoadedPlugin
    basePath   string
    adapter    SendAdapter              // OneBot11 完整 API
    db         *gorm.DB                 // 数据库 (命名空间隔离)
    cache      *cache.Cache             // Redis (命名空间隔离)
    t2i        *t2icaller.Client        // T2I 服务 (nil = 未启用)
    sandbox    *sandboxcaller.Client    // Sandbox 服务 (nil = 未启用)
    dao        *dao.Bundle              // Agent 配置查询
    agentOp    AgentOperator            // Provider/MCP/Tool/T2I/Sandbox 切换 + Compact
    currentEv  EventData                // 当前事件上下文
    commands   *CommandRegistry         // 多级命令注册表
}

type LoadedPlugin struct {
    Manifest Manifest
    State    *lua.LState
    Dir      string
}

type Manifest struct {
    Name        string
    Version     string
    Author      string
    Description string
    Entry       string
    Permissions []string
    System      bool   // true = 系统插件，触发三层守卫
}
```

## 关键实现点

### 1. 独立 Lua VM (LState 隔离)

每个插件使用独立的 `*lua.LState`，通过 `lua.NewState()` 创建，VM 之间完全隔离。

### 2. SDK 注入 (`injectSDK`)

```go
//go:embed sdk/jn.lua
var jnSDKSource string

func (pe *PluginEngine) injectSDK(L *lua.LState, pluginName string) {
    sdkDir := filepath.Join(pe.basePath, "sdk")
    // 将 SDK 目录追加到 package.path，使 require("jn") 与 IDE 都能找到
    pathScript := fmt.Sprintf(`package.path = "%s/?.lua;" .. (package.path or "")`,
        strings.ReplaceAll(sdkDir, "\\", "/"))
    if err := L.DoString(pathScript); err != nil {
        slog.Warn("设置 package.path 失败", "plugin", pluginName, "err", err)
    }
}
```

SDK (`data/pluggins/sdk/jn.lua`) 内部仅做 `M.log = log`、`M.t2i = t2i` 等字段重新导出，`M.command.register` 委托到 `__jn_internal.register_command` 全局函数（由 `injectCommandAPI` 注入）。

### 3. 内嵌资源落盘 (`ensureEmbeddedAssets`)

```go
//go:embed sdk/jn.lua
var jnSDKSource string
//go:embed systemplugin/pluggin.yaml
var systemPluginManifest string
//go:embed systemplugin/main.lua
var systemPluginMain string

func (pe *PluginEngine) ensureEmbeddedAssets() {
    // 1. SDK 总是覆盖（保持与二进制版本同步，IDE 类型与运行时一致）
    sdkFile := filepath.Join(pe.basePath, "sdk", "jn.lua")
    os.MkdirAll(filepath.Join(pe.basePath, "sdk"), 0o755)
    os.WriteFile(sdkFile, []byte(jnSDKSource), 0o644)

    // 2. system 插件仅在不存在时写入（允许用户自定义修改）
    sysYaml := filepath.Join(pe.basePath, "system", "pluggin.yaml")
    if _, err := os.Stat(sysYaml); os.IsNotExist(err) {
        os.MkdirAll(filepath.Join(pe.basePath, "system"), 0o755)
        os.WriteFile(sysYaml, []byte(systemPluginManifest), 0o644)
        os.WriteFile(filepath.Join(pe.basePath, "system", "main.lua"),
            []byte(systemPluginMain), 0o644)
    }
}
```

`LoadAll()` 启动时先调用 `ensureEmbeddedAssets()`，再遍历 `data/pluggins/` 加载所有插件（跳过 `sdk/` 目录）。

### 4. 多级命令系统 (`CommandRegistry`)

```go
type CommandRegistry struct {
    mu   sync.RWMutex
    root *CommandNode // 虚拟根节点
}

type CommandNode struct {
    Name        string
    Opts        CommandOpts
    Handler     CommandHandler
    PluginName  string
    Children    map[string]*CommandNode
}

type CommandHandler func(args []string, event EventData) (consumed bool, reply string, err error)
```

**注册**: `Register(plugin, path, opts, handler)` 按路径逐级创建节点，叶节点持有 `handler` 与 `pluginName`；同一路径重复注册会覆盖旧 handler/opts。

**派发**: `Dispatch(raw, event)` 解析 `/cmd subcmd args...`，沿树逐级匹配，记录最后一个 `Handler != nil` 的节点（最长前缀匹配）。命中后 `args = tokens[matchedPathLen:]`。未命中 handler 但停在非根节点 → 列出子命令作为提示。

**卸载联动**: `UnregisterPlugin(plugin)` 递归清除该插件注册的所有命令节点（设 `Handler=nil / PluginName=""`，并清理空叶子节点）。

**查询**: `ListByPlugin(plugin)` 返回指定插件注册的所有叶命令路径（供 `ListMaps` 附加到插件响应）。

**`/help` 自动生成**: `FormatHelp(path)` 生成帮助文本，被内置 `/help` 命令调用。

### 5. 命令注册 API (`injectCommandAPI`)

```go
func (pe *PluginEngine) injectCommandAPI(L *lua.LState, pluginName string) {
    internal := L.NewTable()
    L.SetFuncs(internal, map[string]lua.LGFunction{
        "register_command": func(L *lua.LState) int {
            // 参数: path (string|table), handler (function), opts (table, optional)
            // 解析 path → []string
            // 校验 handler 类型
            // 保留 handler 引用防止 GC: L.SetGlobal(refKey, handlerFn)
            // 包装为 CommandHandler: 在插件 LState 中通过 L.CallByParam 调用
            // 调用 registry.Register(pluginName, path, opts, handler)
            return 1  // true 成功 / false+err 失败
        },
    })
    L.SetGlobal("__jn_internal", internal)
}
```

**handler 引用保活**: Go 侧通过 `L.SetGlobal(refKey, handlerFn)` 保留 handler 引用，防止 Lua GC 回收。`refKey` 格式为 `__jn_cmd_handler_<plugin>_<path>`。

**handler 调用**: 在 `CommandHandler` 闭包中，构造 `argTable`（1-indexed）与 `evTable`，通过 `L.CallByParam` 在插件 LState 中调用 handler，期望返回 2 个值 `(consumed, reply)`。

### 6. 内置 `/help` 命令 (`registerBuiltinCommands`)

```go
func (pe *PluginEngine) registerBuiltinCommands() {
    pe.commands.Register("system", []string{"help"}, CommandOpts{
        Description: "查看所有可用命令，或查看某个命令的子命令与用法",
        Usage:       "/help [命令路径...]",
    }, func(args []string, event EventData) (bool, string, error) {
        reply := pe.commands.FormatHelp(args)
        return true, reply, nil
    })
}
```

在 `NewPluginEngine` 构造时调用一次，注册到 `system` 插件名下，路径 `["system", "help"]`。

### 7. `system` 系统插件

`internal/pluggin/systemplugin/` 由 `//go:embed` 内嵌：
- `pluggin.yaml` — 含 `system: true`
- `main.lua` — 通过 `require("jn")` + `jn.command.register` 注册系统命令组

`main.lua` 注册的命令（部分）：
- `/system` — 顶层分组（无 handler 实际行为，列出子命令）
- `/system status` — 系统状态总览（Provider / MCP / Tool / T2I / Sandbox 计数）
- `/system provider list|switch` — Provider 列表与切换
- `/system mcp list|toggle` — MCP 列表与启停
- `/system tool list|toggle` — Tool 列表与启停
- `/system memory compact` — Compact 短期记忆
- `/system t2i status|toggle` — T2I 状态与启停
- `/system sandbox status|toggle` — Sandbox 状态与启停
- `/system session list` — Session 列表

### 8. 系统插件三层守卫

| 层级 | 守卫点 | 行为 |
|------|--------|------|
| 1 | `Manifest.System` (YAML `system: true`) | 标记来源 |
| 2 | `PluginEngine.IsSystem(name)` | 读取已加载 manifest 的 System 字段 |
| 3 | `PluginEngine.Unload(name)` | 拒绝卸载，返回 `"system 插件 %q 不允许卸载"` |
| 3' | Service 层 `TogglePlugin` | `data.IsActive == false && IsSystem(id)` → 返回 `40028 PluginIsSystem`（允许"启用"以支持幂等场景） |
| 3'' | Service 层 `DeletePlugin` | `IsSystem(id)` → 返回 `40028 PluginIsSystem` |

### 9. 权限控制 + 服务开关检测

`injectBaseAPI` 根据 `pluggin.yaml` 中的 `permissions` 注入 API，同时检测服务的可用性：

```go
// 始终注入 (无需权限)
L.SetGlobal("log", logTable)    // 3 函数: info/warn/error
L.SetGlobal("json", jsonTable)  // 2 函数: encode/decode

// 按权限 + 服务可用性注入
if hasPerm("onebot11") && pe.adapter != nil {
    pe.injectOneBot11(L, pluginName)     // 20 个函数
}
if hasPerm("http") {
    pe.injectHTTP(L, pluginName)         // 2 函数: get/post
}
if hasPerm("database") && pe.db != nil {
    pe.injectDatabase(L, pluginName)     // 2 函数: query/exec
}
if hasPerm("cache") && pe.cache != nil {
    pe.injectCache(L, pluginName)        // 4 函数: get/set/del/exists
}
if hasPerm("t2i") {
    pe.injectT2I(L, pluginName)          // 5 函数: generate/generate_url/toggle/is_active/get_config
}
if hasPerm("sandbox") {
    pe.injectSandbox(L, pluginName)      // 6 函数: create/exec_shell/exec_python/toggle/is_active/get_config
}
if hasPerm("agent") && pe.dao != nil {
    pe.injectAgent(L)                    // 16 函数: 查询+管理+切换+Compact
}
```

T2I/Sandbox 内部通过 `getCurrentClient()` 优先从 `agentOp.GetT2IClient()` / `agentOp.GetSandboxClient()` 获取最新运行时实例，支持热更新；若为 nil 则 `generate` / `create` / `exec_*` 返回 `(nil, "服务未启用")`，但 `toggle` / `is_active` / `get_config` 仍可通过 `agentOp` / `dao` 工作。

### 10. OneBot11 完整 API (20 函数)

通过 `SendAdapter` 接口暴露，`AdapterWrapper` 将 `adapter.Provider` 的强类型返回值转为 `map[string]any`:

```go
type SendAdapter interface {
    SendPrivateMsg(userID int64, message any) (int64, error)
    SendGroupMsg(groupID int64, message any) (int64, error)
    DeleteMsg(messageID int64) error
    GetGroupInfo(groupID int64) (map[string]any, error)
    GetGroupMemberList(groupID int64) ([]map[string]any, error)
    GetGroupMemberInfo(groupID, userID int64) (map[string]any, error)
    GetGroupHonorInfo(groupID int64) (map[string]any, error)
    KickGroupMember(groupID, userID int64, rejectAdd bool) error
    BanGroupMember(groupID, userID int64, duration int) error
    SetGroupWholeBan(groupID int64, enable bool) error
    SetGroupCard(groupID, userID int64, card string) error
    HandleFriendRequest(flag, approve, remark) error
    HandleGroupRequest(flag, subType, approve, reason) error
    GetLoginInfo() (map[string]any, error)
    GetStrangerInfo(userID int64) (map[string]any, error)
    GetFriendList() ([]map[string]any, error)
    GetGroupList() ([]map[string]any, error)
    SendLike(userID int64, times int) error
    GetStatus() (map[string]any, error)
    GetVersionInfo() (map[string]any, error)
}
```

### 11. HTTP API (真实请求)

```go
httpClient := &http.Client{Timeout: 30 * time.Second}

"get": func(L *lua.LState) int {
    url := L.CheckString(1)
    resp, err := httpClient.Get(url)
    // 返回 {status=200, body="..."}
}

"post": func(L *lua.LState) int {
    url := L.CheckString(1)
    contentType := L.CheckString(2)   // 可选
    body := L.CheckString(3)          // 可选
    resp, err := httpClient.Post(url, contentType, bytes.NewBufferString(body))
    // 返回 {status=200, body="..."}
}
```

### 12. 数据库 API (命名空间隔离)

```go
// 隔离: 表名前缀 pluggin_<name>_
db.Exec(sql)     // INSERT/UPDATE/DELETE → 返回影响行数
db.Query(sql)    // SELECT → 返回 []map[string]any
```

使用 `gorm.DB.Raw()` 执行原始 SQL，插件代码需自行管理表创建。

### 13. 缓存 API (命名空间隔离)

```go
// 隔离: Key 前缀 pluggin:<name>:
cache.Get(key)      // 读取 → map[string]any
cache.Set(key, val [, ttl])  // 写入 → bool
cache.Del(key)      // 删除 → bool
cache.Exists(key)   // 检查 → 0 or 1
```

通过 `cache.Cache` 封装，使用 Redis 后端。

### 14. T2I / Sandbox 服务开关

服务为 nil 时注入返回 error 的占位函数：

```go
if pe.t2i == nil {
    L.SetFuncs(t2iTable, map[string]lua.LGFunction{
        "generate": func(L *lua.LState) int {
            L.Push(lua.LNil)
            L.Push(lua.LString("T2I 服务未启用"))
            return 2
        },
    })
    return
}
```

`toggle` / `is_active` / `get_config` 不依赖客户端实例，通过 `agentOp` / `dao` 工作。

### 15. AgentOperator (Provider/MCP/Tool/T2I/Sandbox 切换 + Compact)

```go
type AgentOperator interface {
    // Provider 管理
    SetProviderActive(ctx, id, active) error
    SwitchProvider(ctx, id) error
    GetProviderGroup() ProviderGroupAccess

    // MCP 管理
    SetMCPActive(ctx, id, active) error
    GetMCPGroup() MCPGroupAccess

    // Tool 管理
    SetToolActive(ctx, name, active) error
    GetToolRegistry() ToolRegistryAccess

    // T2I / Sandbox 启停
    SetT2IActive(ctx, active) error
    SetSandboxActive(ctx, active) error
    GetT2IClient() *t2icaller.Client
    GetSandboxClient() *sandboxcaller.Client

    // 记忆与上下文
    CompactMemory(ctx, chatAreaID) error
    GetChatAreaID(userID, groupID, messageType) string
}

// 暴露给插件的子接口
type ProviderGroupAccess interface {
    List() []ProviderInfo
    GetActive(id string) bool
}

type MCPGroupAccess interface {
    ListMCPs() []MCPInfo
    IsConnected(id string) bool
}

type ToolRegistryAccess interface {
    ListTools() []ToolInfo
    IsActive(name string) bool
}
```

`HagoCenter` 实现此接口 (`agent_operator.go`):
- `SetProviderActive`: 更新 DB → 从 ProviderGroup 添加/移除
- `SwitchProvider`: 切换主 Provider，停用同类型其他 Provider
- `SetMCPActive`: 更新 DB → 停用时断开 MCP 连接
- `SetToolActive`: 更新 DB → 停用时从 ToolRegistry 移除（内置工具除外）
- `SetT2IActive` / `SetSandboxActive`: 更新 DB 配置 + 重建客户端实例 + 通过 `OnUpdateT2I` / `OnUpdateSandbox` 回调同步到 HagoCenter
- `CompactMemory`: 调用 `MemoryGroup.CompactShortTermMemory` (需要 LLM)
- `GetChatAreaID`: 根据 userID/groupID/type 获取或创建 ChatArea

### 16. `OnMessage` 命令优先派发

```go
func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
    pe.mu.RLock()
    defer pe.mu.RUnlock()
    pe.currentEv = event

    // 1. 优先派发给命令注册表（/cmd subcmd ...）
    if strings.HasPrefix(strings.TrimSpace(event.RawMessage), "/") {
        c, reply, err := pe.commands.Dispatch(event.RawMessage, event)
        if err != nil {
            slog.Error("命令派发错误", "raw", event.RawMessage, "err", err)
        }
        if c {
            if reply != "" {
                pe.sendReply(event, reply)  // 自动回复
            }
            return true  // consumed
        }
    }

    // 2. 没有命令命中，按原逻辑派发给插件的 on_message
    for _, p := range pe.plugins {
        if !p.HasPermission("onebot11") {
            continue
        }
        fn := p.State.GetGlobal("on_message")
        if fn.Type() != lua.LTFunction {
            continue
        }
        // ... 调用 on_message(event) ...
    }
    return false
}

// sendReply 根据 message_type 回复到对应会话
func (pe *PluginEngine) sendReply(event EventData, content string) {
    switch event.MessageType {
    case "private":
        pe.adapter.SendPrivateMsg(event.UserID, content)
    case "group":
        pe.adapter.SendGroupMsg(event.GroupID, content)
    }
}
```

### 17. `ListMaps` 增强

```go
func (pe *PluginEngine) ListMaps() []map[string]any {
    pe.mu.RLock()
    defer pe.mu.RUnlock()
    out := make([]map[string]any, 0, len(pe.plugins))
    for _, p := range pe.plugins {
        m := p.Manifest
        entry := map[string]any{
            "name":        m.Name,
            "version":     m.Version,
            "author":      m.Author,
            "description": m.Description,
            "permissions": m.Permissions,
            "is_system":   m.System,
            "is_active":   true,
            "commands":    pe.commands.ListByPlugin(m.Name),  // []PluginCommandInfo
        }
        out = append(out, entry)
    }
    return out
}

type PluginCommandInfo struct {
    Path        []string `json:"path"`         // 完整路径, 如 ["system","provider","switch"]
    Description string   `json:"description"`
    Usage       string   `json:"usage"`
    IsLeaf      bool     `json:"is_leaf"`      // handler != nil
}
```

### 18. Current Event 上下文

```go
pe.currentEv = event  // 在 OnMessage 开头存储

// agent.get_current_chat_area() 读取 currentEv 并调用 agentOp.GetChatAreaID()
```

### 19. Go ↔ Lua 类型转换

```go
goToLuaValue(v any) lua.LValue    // Go → Lua
luaValueToGo(v lua.LValue) any    // Lua → Go
pushResult(L, err) int             // bool + err
pushResultJSON(L, v, err) int      // table + err
```

### 20. 并发安全

- `sync.RWMutex` 保护 `plugins` map 和 `currentEv`
- `CommandRegistry` 内部独立 `sync.RWMutex` 保护命令树
- 读操作 (OnMessage, List, ListMaps) 使用 `RLock`
- 写操作 (Load, Unload, Reload) 使用 `Lock`
- 每个插件的 LState 串行处理 (一个事件串行调用)

### 21. 事件循环集成

```go
// agent/event.go
if h.PluginEngine != nil {
    pluginEvent := pluggin.EventData{...}
    if h.PluginEngine.OnMessage(pluginEvent) {
        continue  // 插件/命令消费, 跳过 Agent
    }
}
```

插件拦截（含命令派发）在 ACL 检查之前、Skill 匹配之前、LLM 调用之前。
