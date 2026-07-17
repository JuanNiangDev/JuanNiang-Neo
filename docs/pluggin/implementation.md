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
    agentOp    AgentOperator            // Provider/MCP 切换 + Compact
    currentEv  EventData                // 当前事件上下文
}

type LoadedPlugin struct {
    Manifest Manifest
    State    *lua.LState
    Dir      string
}
```

## 关键实现点

### 1. 独立 Lua VM (LState 隔离)

每个插件使用独立的 `*lua.LState`，通过 `lua.NewState()` 创建，VM 之间完全隔离。

### 2. 权限控制 + 服务开关检测

`injectBaseAPI` 根据 `pluggin.yaml` 中的 `permissions` 注入 API，同时检测服务的可用性：

```go
// 始终注入 (无需权限)
L.SetGlobal("log", logTable)
L.SetGlobal("json", jsonTable)

// 按权限 + 服务可用性注入
if hasPerm("onebot11") && pe.adapter != nil {
    pe.injectOneBot11(L, pluginName)     // 20 个函数
}
if hasPerm("t2i") {
    pe.injectT2I(L, pluginName)          // nil → 注入返回 error 的占位函数
}
if hasPerm("sandbox") {
    pe.injectSandbox(L, pluginName)      // nil → 注入返回 error 的占位函数
}
if hasPerm("agent") && pe.dao != nil {
    pe.injectAgent(L)                    // 查询 + 管理 + Compact
}
```

### 3. OneBot11 完整 API (20 函数)

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

### 4. HTTP API (真实请求)

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

### 5. 数据库 API (命名空间隔离)

```go
// 隔离: 表名前缀 pluggin_<name>_
db.Exec(sql)     // INSERT/UPDATE/DELETE → 返回影响行数
db.Query(sql)    // SELECT → 返回 []map[string]any
```

使用 `gorm.DB.Raw()` 执行原始 SQL，插件代码需自行管理表创建。

### 6. 缓存 API (命名空间隔离)

```go
// 隔离: Key 前缀 pluggin:<name>:
cache.Get(key)      // 读取 → map[string]any
cache.Set(key, val [, ttl])  // 写入 → bool
cache.Del(key)      // 删除 → bool
cache.Exists(key)   // 检查 → 0 or 1
```

通过 `cache.Cache` 封装，使用 Redis 后端。

### 7. T2I / Sandbox 服务开关

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

### 8. AgentOperator (Provider/MCP 切换 + Compact)

```go
type AgentOperator interface {
    SetProviderActive(ctx, id, active) error
    SetMCPActive(ctx, id, active) error
    CompactMemory(ctx, chatAreaID) error
    GetChatAreaID(userID, groupID, messageType) string
}
```

`HagoCenter` 实现此接口 (`agent_operator.go`):
- `SetProviderActive`: 更新 DB → 从 ProviderGroup 添加/移除
- `SetMCPActive`: 更新 DB → 停用时断开 MCP 连接
- `CompactMemory`: 调用 `MemoryGroup.CompactShortTermMemory` (需要 LLM)
- `GetChatAreaID`: 根据 userID/groupID/type 获取或创建 ChatArea

### 9. Current Event 上下文

```go
func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
    pe.currentEv = event  // 存储当前上下文
    // ... 调用插件 on_message ...
}
```

`agent.get_current_chat_area()` 读取 `currentEv` 并调用 `agentOp.GetChatAreaID()` 获取持久化 ID。

### 10. Go ↔ Lua 类型转换

```go
goToLuaValue(v any) lua.LValue    // Go → Lua
luaValueToGo(v lua.LValue) any    // Lua → Go
pushResult(L, err) int             // bool + err
pushResultJSON(L, v, err) int      // table + err
```

### 11. 并发安全

- `sync.RWMutex` 保护 `plugins` map 和 `currentEv`
- 读操作 (OnMessage, List) 使用 `RLock`
- 写操作 (Load, Unload, Reload) 使用 `Lock`
- 每个插件的 LState 串行处理 (一个事件串行调用)

### 12. 事件循环集成

```go
// agent/event.go
if h.PluginEngine != nil {
    pluginEvent := pluggin.EventData{...}
    if h.PluginEngine.OnMessage(pluginEvent) {
        continue  // 插件消费, 跳过 Agent
    }
}
```

插件拦截在 ACL 检查之前、Skill 匹配之前、LLM 调用之前。
