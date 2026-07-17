# 插件引擎实现细节

## 核心结构

```go
type PluginEngine struct {
    mu       sync.RWMutex
    plugins  map[string]*LoadedPlugin  // name → 已加载插件
    basePath string                     // data/pluggins/
    adapter  SendAdapter               // 消息发送接口
}

type LoadedPlugin struct {
    Manifest Manifest      // pluggin.yaml 内容
    State    *lua.LState   // 独立 Lua VM
    Dir      string         // 插件目录路径
}
```

## 关键实现点

### 1. 独立 Lua VM (LState 隔离)

每个插件使用独立的 `*lua.LState`:
- 通过 `lua.NewState()` 创建新 VM
- VM 之间完全隔离, 全局变量互不影响
- 插件崩溃时 `LState.Close()` 释放资源, 不影响其他插件

### 2. 权限控制 (API 注入白名单)

`injectBaseAPI` 方法根据 `pluggin.yaml` 中的 `permissions` 决定注入哪些 API:

```go
hasPerm := func(p string) bool {
    for _, pp := range permissions {
        if pp == p || pp == "*" {
            return true
        }
    }
    return false
}

// 始终注入
L.SetGlobal("log", logTable)
L.SetGlobal("json", jsonTable)

// 按权限注入
if hasPerm("onebot11") && pe.adapter != nil {
    L.SetGlobal("onebot11", obTable)
}
if hasPerm("http") {
    L.SetGlobal("http", httpTable)
}
```

### 3. Go ↔ Lua 类型转换

**Go → Lua (`goToLuaValue`)**:
- `string` → `lua.LString`
- `float64/int64/int` → `lua.LNumber`
- `bool` → `lua.LBool`
- `map[string]any` → `*lua.LTable` (递归转换)
- `[]any` → `*lua.LTable` (1-indexed 数组)
- `nil` → `lua.LNil`
- 其他 → `lua.LString(json.Marshal)`

**Lua → Go (`luaValueToGo`)**:
- `lua.LString` → `string`
- `lua.LNumber` → `float64`
- `lua.LBool` → `bool`
- `*lua.LTable` (len>0) → `[]any` (数组)
- `*lua.LTable` (len=0) → `map[string]any` (字典)
- `*lua.LNilType` → `nil`

### 4. 事件处理 (`OnMessage`)

```go
func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
    pe.mu.RLock()
    defer pe.mu.RUnlock()

    for _, p := range pe.plugins {
        if !p.HasPermission("onebot11") {
            continue  // 无 onebot11 权限的插件不接收消息事件
        }

        fn := p.State.GetGlobal("on_message")
        if fn.Type() != lua.LTFunction {
            continue  // 没有定义 on_message, 跳过
        }

        table := eventToLuaTable(p.State, event)

        // 调用 Lua 函数: on_message(event) → (consumed, modified)
        p.State.Push(fn)
        p.State.Push(table)
        p.State.PCall(1, 2, nil)

        consumedRet := p.State.Get(-2)  // 第一个返回值
        p.State.Pop(2)

        if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
            return true  // 事件被消费, 后续插件和 Agent 不再处理
        }
    }

    return false
}
```

### 5. OneBot11 API 注入

`onebot11` 全局表通过 Go 闭包桥接 `SendAdapter` 接口:

```go
obTable := L.NewTable()
L.SetFuncs(obTable, map[string]lua.LGFunction{
    "send_group_msg": func(L *lua.LState) int {
        groupID := int64(L.CheckNumber(1))
        msg := L.CheckString(2)
        _, err := adapter.SendGroupMsg(groupID, msg)
        if err != nil {
            L.Push(lua.LBool(false))
            L.Push(lua.LString(err.Error()))
            return 2
        }
        L.Push(lua.LBool(true))
        return 1
    },
    // ...
})
L.SetGlobal("onebot11", obTable)
```

`L.SetFuncs` 将 Go 函数注册为 Lua 表的字段, 调用时自动进行 Lua → Go 参数转换。

### 6. YAML 解析

使用 `gopkg.in/yaml.v3` 解析 `pluggin.yaml`:

```go
func (pe *PluginEngine) readManifest(dir string) (*Manifest, error) {
    path := filepath.Join(dir, "pluggin.yaml")
    data, err := os.ReadFile(path)
    var m Manifest
    yaml.Unmarshal(data, &m)
    return &m, nil
}
```

### 7. 并发安全

- `sync.RWMutex` 保护 `plugins` map
- 读操作 (OnMessage, List) 使用 `RLock`
- 写操作 (Load, Unload, Reload) 使用 `Lock`
- 每个插件的 LState 是非并发安全的 (一个事件串行处理)

### 8. 事件循环集成

在 `agent/event.go` 中:

```go
if h.PluginEngine != nil {
    pluginEvent := pluggin.EventData{
        PostType:    "message",
        MessageType: ev.Message.MessageType,
        UserID:      ev.Message.UserID,
        GroupID:     ev.Message.GroupID,
        RawMessage:  ev.Message.RawMessage,
    }
    if h.PluginEngine.OnMessage(pluginEvent) {
        continue  // 插件消费, 跳过 Agent
    }
}
```

插件拦截在 ACL 检查之前, Skill 匹配之前, LLM 调用之前 — 即最早的消息处理阶段。
