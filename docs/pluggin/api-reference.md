# Lua API 参考

JuanNiang-Neo 暴露给 Lua 插件的 API 函数。可用性由 `pluggin.yaml` 中 `permissions` 控制。

---

## 引入 SDK

JuanNiang-Neo 在二进制中内嵌了 Lua SDK（`internal/pluggin/sdk/jn.lua`），启动时由 `PluginEngine.ensureEmbeddedAssets()` 落盘到 `data/pluggins/sdk/jn.lua`，并通过 `injectSDK` 将该目录追加到每条 LState 的 `package.path`。

插件**推荐**通过 `require("jn")` 引入 SDK，以获得 IDE（sumneko lua-language-server）完整类型提示：

```lua
local jn = require("jn")

-- 通过 jn.<table>.<func> 调用，等价于直接使用全局 <table>.<func>
jn.log.info("插件启动")
local id, err = jn.t2i.generate("<h1>Hello</h1>")
```

SDK 仅是 Go 注入全局表的"重新导出"（`jn.log = log` / `jn.t2i = t2i` / ...），二者完全等价，可混用。下文示例同时给出"全局表"与"`jn.` 前缀"两种写法。

| SDK 字段 | 对应全局表 | 说明 |
|----------|-----------|------|
| `jn.log` | `log` | 日志 |
| `jn.json` | `json` | JSON 编解码 |
| `jn.onebot11` | `onebot11` | OneBot11 协议接口 |
| `jn.http` | `http` | HTTP 请求 |
| `jn.database` | `database` | 数据库访问（命名空间隔离） |
| `jn.cache` | `cache` | Redis 缓存（命名空间隔离） |
| `jn.t2i` | `t2i` | 文生图 |
| `jn.sandbox` | `sandbox` | 代码沙箱 |
| `jn.agent` | `agent` | Agent 操作接口 |
| `jn.command` | — | 多级命令注册（仅通过 SDK 暴露） |

> **说明**: `jn.command.register` 是命令注册的唯一入口，内部委托到 Go 侧 `__jn_internal.register_command` 全局函数。直接调用 `__jn_internal.*` 不被推荐，签名可能随版本调整。

---

## 全局表: `log`

日志输出到服务器 slog。

| 权限 | 始终可用 |

### `log.info(msg)` / `log.warn(msg)` / `log.error(msg)`

```lua
log.info("插件启动")
log.warn("配置缺失")
log.error("操作失败: " .. err)
```

---

## 全局表: `json`

JSON 编解码。| 权限 | 始终可用 |

### `json.encode(value) → string`

Lua 值 → JSON 字符串。

### `json.decode(str) → table`

JSON 字符串 → Lua table。

---

## 全局表: `onebot11`

| 权限 | `onebot11` |

所有函数返回 `(result, err)` — 成功时 err 为 nil。

### 消息发送

#### `onebot11.send_private_msg(user_id, message) → bool [, err]`

发送私聊消息。`user_id` number, `message` string。

#### `onebot11.send_group_msg(group_id, message) → bool [, err]`

发送群聊消息。

#### `onebot11.delete_msg(message_id) → bool [, err]`

撤回消息。

### 群信息查询

#### `onebot11.get_group_info(group_id) → table [, err]`

返回群信息: `{group_id, group_name, member_count, max_member_count}`。

#### `onebot11.get_group_member_list(group_id) → []table [, err]`

返回成员列表: `[{user_id, nickname, card, role, ...}]`。

#### `onebot11.get_group_member_info(group_id, user_id) → table [, err]`

返回单个成员信息。

#### `onebot11.get_group_honor_info(group_id) → table [, err]`

返回群荣誉: `{current_talkative, talkative_list, ...}`。

### 群管理

#### `onebot11.kick_group_member(group_id, user_id [, reject_add]) → bool [, err]`

踢出群成员。`reject_add` boolean, 默认 false。

#### `onebot11.ban_group_member(group_id, user_id, duration) → bool [, err]`

禁言。`duration` 秒。

#### `onebot11.set_group_whole_ban(group_id, enable) → bool [, err]`

全员禁言开关。

#### `onebot11.set_group_card(group_id, user_id, card) → bool [, err]`

设置群名片。

### 请求处理

#### `onebot11.handle_friend_request(flag, approve, remark) → bool [, err]`

处理好友申请。

#### `onebot11.handle_group_request(flag, sub_type, approve, reason) → bool [, err]`

处理群请求。`sub_type`: `"add"` / `"invite"`。

### 用户信息

#### `onebot11.get_login_info() → table [, err]`

返回机器人自身信息: `{user_id, nickname}`。

#### `onebot11.get_stranger_info(user_id) → table [, err]`

返回陌生人信息。

#### `onebot11.get_friend_list() → []table [, err]`

返回好友列表。

#### `onebot11.get_group_list() → []table [, err]`

返回群列表。

### 其他

#### `onebot11.send_like(user_id, times) → bool [, err]`

发送赞。

#### `onebot11.get_status() → table [, err]`

返回适配器运行状态。

#### `onebot11.get_version_info() → table [, err]`

返回协议版本信息。

---

## 全局表: `http`

| 权限 | `http` |

### `http.get(url) → table`

发送 GET 请求。返回: `{status=number, body=string}`。

```lua
local r = http.get("https://api.example.com/data")
log.info("status: " .. r.status)
log.info("body: " .. r.body)
```

### `http.post(url [, content_type, body]) → table`

发送 POST 请求。

```lua
local r = http.post("https://api.example.com/submit", "application/json", '{"key":"value"}')
```

---

## 全局表: `database`

| 权限 | `database` |

**命名空间隔离**: 所有表名自动带有 `pluggin_<name>_` 前缀，不会与系统表冲突。

### `database.query(sql) → []table [, err]`

执行 SELECT 查询，返回结果行数组。

```lua
local rows = database.query("SELECT * FROM my_plugin_config")
for _, row in ipairs(rows) do
    log.info(row.key .. " = " .. row.value)
end
```

### `database.exec(sql) → number [, err]`

执行 INSERT/UPDATE/DELETE，返回影响行数。

```lua
local affected = database.exec("INSERT INTO counters VALUES (1, 0)")
```

> **注意:** 需自行在插件中创建所需的表。建议在加载时执行 `CREATE TABLE IF NOT EXISTS`。

---

## 全局表: `cache`

| 权限 | `cache` |

**命名空间隔离**: 所有 key 自动带有 `pluggin:<name>:` 前缀，与系统缓存隔离。

### `cache.get(key) → table`

读取缓存值。

```lua
local data = cache.get("my_key")
if data then
    log.info(data.value)
end
```

### `cache.set(key, value [, ttl]) → bool [, err]`

写入缓存。`ttl` 秒, 默认 0 (永不过期)。

```lua
cache.set("my_key", {value = 42, name = "test"}, 3600)  -- 1小时过期
```

### `cache.del(key) → bool [, err]`

删除缓存。

### `cache.exists(key) → number`

检查 key 是否存在。返回 0 或 1。

---

## 全局表: `t2i`

| 权限 | `t2i` |

> **开关检测:** T2I 服务未启用时调用 `generate` / `generate_url` 返回 `(nil, "T2I 服务未启用")`。
> 运行时通过 `AgentOperator.GetT2IClient()` 获取最新实例，支持热更新。

### `t2i.generate(html) → string [, err]`

根据 HTML 生成图片，返回图片 ID。

```lua
local jn = require("jn")
local id, err = jn.t2i.generate("<h1>Hello</h1>")
if not id then
    jn.log.error("T2I failed: " .. err)
end
```

### `t2i.generate_url(html) → string [, err]`

生成图片并返回公开 URL。

```lua
local url, err = t2i.generate_url("<p>Test</p>")
```

### `t2i.toggle(active) → bool [, err]`

启用或停用 T2I 服务。委托到 `AgentOperator.SetT2IActive`，会同步更新 DB 配置并重建客户端。

```lua
local ok, err = t2i.toggle(true)  -- 启用
```

### `t2i.is_active() → bool`

查询 T2I 服务当前是否启用（从 DB 配置读取）。`dao` 不可用时返回 `false`。

```lua
if not t2i.is_active() then
    log.warn("T2I 未启用")
end
```

### `t2i.get_config() → table [, err]`

返回 T2I 完整配置（base_url / timeout / is_active 等）。

---

## 全局表: `sandbox`

| 权限 | `sandbox` |

> **开关检测:** Sandbox 服务未启用时调用 `create` / `exec_shell` / `exec_python` 返回 `(nil, "Sandbox 服务未启用")`。
> 运行时通过 `AgentOperator.GetSandboxClient()` 获取最新实例，支持热更新。

### `sandbox.create() → table [, err]`

创建新的沙箱。返回: `{sandbox_id=string, status=string}`。

```lua
local jn = require("jn")
local sb = jn.sandbox.create()
if not sb then return end
jn.log.info("沙箱创建: " .. sb.sandbox_id)
```

### `sandbox.exec_shell(sandbox_id, command) → (output, exit_code) | (nil, err)`

在沙箱中执行 Shell 命令。

```lua
local output, code = sandbox.exec_shell(sb.sandbox_id, "ls -la")
log.info("exit code: " .. code)
log.info("output: " .. output)
```

### `sandbox.exec_python(sandbox_id, code) → (output, error) | (nil, err)`

在沙箱中执行 Python 代码。

```lua
local out, err = sandbox.exec_python(sb.sandbox_id, "print('hello')")
if err then log.info("stderr: " .. err) end
log.info("stdout: " .. out)
```

### `sandbox.toggle(active) → bool [, err]`

启用或停用 Sandbox 服务。委托到 `AgentOperator.SetSandboxActive`，同步更新 DB 配置并重建客户端。

### `sandbox.is_active() → bool`

查询 Sandbox 服务当前是否启用（从 DB 配置读取）。`dao` 不可用时返回 `false`。

### `sandbox.get_config() → table [, err]`

返回 Sandbox 完整配置（base_url / api_key / timeout / is_active 等）。

---

## 全局表: `agent`

| 权限 | `agent` |

提供 Agent 配置的查询与运行时管理（共 16 个函数）。

### 配置查询（从 DB 读取）

#### `agent.get_providers() → []table [, err]`

返回所有 LLM Provider 配置。

#### `agent.get_mcp_servers() → []table [, err]`

返回所有 MCP 服务器配置。

#### `agent.get_skills() → []table [, err]`

返回所有 Skill 配置。

#### `agent.get_sessions() → []table [, err]`

返回所有 Session。

#### `agent.get_prompts() → []table [, err]`

返回所有 Prompt 模板。

#### `agent.get_tools() → []table [, err]`

返回所有 Tool 配置。

#### `agent.get_plugins() → []table [, err]`

返回所有已安装插件信息。

### Provider 管理

#### `agent.set_provider_active(id, active) → bool [, err]`

启用/停用 LLM Provider。停用时从运行环境中移除，启用的 Provider 会被加载。

```lua
agent.set_provider_active("uuid", false)  -- 停用
agent.set_provider_active("uuid", true)   -- 启用
```

#### `agent.list_runtime_providers() → []table [, err]`

返回当前运行时已加载的 Provider 列表（来自 `ProviderGroup.List()`，仅包含已 active 的）。每项结构：

```lua
{
    id = "uuid",
    name = "openai",
    type = "text_model",  -- "text_model" | "image_model" | "embedding_model"
    model = "gpt-4",
    active = true
}
```

#### `agent.switch_provider(id) → bool [, err]`

切换主 Provider（将指定 Provider 标记为活跃，停用同类型的其他 Provider）。委托到 `AgentOperator.SwitchProvider`。

```lua
local ok, err = agent.switch_provider("uuid")
```

### MCP 管理

#### `agent.set_mcp_active(id, active) → bool [, err]`

启用/停用 MCP 服务器。停用时会断开连接。

#### `agent.list_mcps() → []table [, err]`

返回当前运行时已加载的 MCP 列表（来自 `MCPGroup.ListMCPs()`）。每项结构：

```lua
{
    id = "uuid",
    name = "weather-mcp",
    url = "http://localhost:8080/sse",
    active = true
}
```

#### `agent.toggle_mcp(id, active) → bool [, err]`

启用/停用 MCP 服务器，等价于 `set_mcp_active`，提供语义更直观的别名。

### Tool 管理

#### `agent.list_tools() → []table [, err]`

返回当前运行时已注册的 Tool 列表（来自 `ToolRegistry.ListTools()`）。每项结构：

```lua
{
    name = "send_group_msg",
    description = "发送群消息",
    builtin = true,
    long_running = false,
    active = true
}
```

#### `agent.toggle_tool(name, active) → bool [, err]`

启用/停用指定 Tool。`name` 是工具名（非 ID），停用时从 `ToolRegistry` 移除。

> **注意**: 内置工具运行时常驻，停用后仍保留在注册表中；用户自定义工具停用后会被 `Unregister`。

### 上下文与记忆

#### `agent.get_current_chat_area() → table`

返回当前正在处理的消息所属 Chat-Area 信息。

```lua
local area = agent.get_current_chat_area()
-- area = {
--   post_type = "message",
--   message_type = "group",
--   user_id = 123456,
--   group_id = 789012,
--   chat_area_id = "uuid"
-- }
```

#### `agent.compact_memory() → string [, err]`

Compact 当前 Chat-Area 的短期记忆：调用 LLM 将窗口内消息压缩为摘要，写入长期记忆并清空窗口。

```lua
local ok, err = agent.compact_memory()
if err then
    log.error("Compact 失败: " .. err)
end
```

> **注意:** 需要已配置 Text LLM Provider 才能执行 Compact。

---

## SDK 模块: `jn.command`

多级命令注册。仅通过 SDK 暴露，需先 `local jn = require("jn")`。

命令系统是 `CommandRegistry` 维护的一棵 `CommandNode` 树，插件通过 `jn.command.register` 在树上挂载自己的命令。`PluginEngine.OnMessage` 在派发到 `on_message` 之前，会先检查 `event.RawMessage` 是否以 `/` 开头，若是则调用 `commands.Dispatch` 进行最长前缀匹配：

- 命中可执行 handler → 自动回复 `reply`（非空时），并 `consumed=true` 跳过 Agent 与 `on_message`
- 未命中 handler 但停在某个非根节点 → 自动列出该节点的子命令作为提示
- 完全未命中 → fallback 到插件的 `on_message` 回调

### `jn.command.register(path, handler [, opts]) → bool [, err]`

注册一条命令。

**参数**:
- `path` — 命令路径，可为字符串（按空格切分，如 `"foo bar"`）或字符串数组（如 `{"foo", "bar"}`）
- `handler` — 处理函数，签名 `function(args, event): consumed, reply`
  - `args` — 命令路径之后的所有空格分隔参数（`string[]`）
  - `event` — 触发命令的事件上下文（`jn.Event`）
  - `consumed` — 是否消费此命令（true 跳过 Agent 处理）
  - `reply` — 若非空，由系统自动回复给用户
- `opts` — 选项表（可选）：
  - `description` — 命令描述（用于 `/help` 自动生成）
  - `usage` — 用法示例（如 `"/system provider switch <id>"`）

**返回**: `true` 成功；`false, err` 失败（参数非法时）。

> **handler 引用保活**: Go 侧通过 `L.SetGlobal(refKey, handlerFn)` 保留 handler 引用，防止 Lua GC 回收。

```lua
local jn = require("jn")

-- 注册 /greet <name> 命令
jn.command.register("greet", function(args, event)
    local name = args[1] or "朋友"
    return true, "你好，" .. name .. "！"
end, {
    description = "打招呼",
    usage = "/greet [名字]",
})

-- 注册多级命令 /myplugin subcmd1 subcmd2
jn.command.register({"myplugin", "subcmd1", "subcmd2"}, function(args, event)
    -- args 是 subcmd2 之后的所有 token
    return true, "收到参数: " .. table.concat(args, " ")
end, {
    description = "多级命令示例",
    usage = "/myplugin subcmd1 subcmd2 [args...]",
})
```

### 内置 `/help` 命令

`PluginEngine.registerBuiltinCommands()` 在初始化时注册了 `/help` 命令（路径 `["system", "help"]`，挂在 `system` 插件名下）：

- `/help` — 列出所有顶层命令
- `/help <cmd>` — 查看 `<cmd>` 的子命令与用法
- `/help <cmd> <subcmd>` — 查看更深层级

示例输出：

```
可用命令：
- /greet — 打招呼
- /help — 查看所有可用命令，或查看某个命令的子命令与用法
- /system — 系统管理命令组

使用 /help <命令> 查看该命令的子命令与用法。
```

---

## 回调: `on_message`

```lua
function on_message(event) → (consumed, modified)
```

| 事件字段 | 类型 | 说明 |
|----------|------|------|
| `post_type` | string | `"message"` |
| `message_type` | string | `"private"` / `"group"` |
| `user_id` | number | 发送者 QQ 号 |
| `group_id` | number | 群号 |
| `raw_message` | string | 消息原文 |

`consumed=true` → 跳过 Agent 处理。

> **命令优先**: 若 `event.raw_message` 以 `/` 开头，`PluginEngine.OnMessage` 会先调用 `CommandRegistry.Dispatch`。命中命令后会自动回复并 `consumed=true`，**不会**再调用任何插件的 `on_message`。仅当未命中任何命令时才 fallback 到 `on_message` 回调链。插件应优先使用 `jn.command.register` 注册命令式交互，将 `on_message` 用于纯事件监听场景。

---

## 权限速查

| 权限 | 暴露的全局表 |
|------|-------------|
| `*` | 所有 |
| `onebot11` | `onebot11.*` |
| `http` | `http.*` |
| `database` | `database.*` |
| `cache` | `cache.*` |
| `t2i` | `t2i.*` |
| `sandbox` | `sandbox.*` |
| `agent` | `agent.*` |
