# Lua API 参考

Jonathan-Neo 暴露给 Lua 插件的 API 函数。可用性由 `pluggin.yaml` 中 `permissions` 控制。

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

> **开关检测:** T2I 服务未启用时调用返回 `(nil, "T2I 服务未启用")`。

### `t2i.generate(html) → string [, err]`

根据 HTML 生成图片，返回图片 ID。

```lua
local id, err = t2i.generate("<h1>Hello</h1>")
if not id then
    log.error("T2I failed: " .. err)
end
```

### `t2i.generate_url(html) → string [, err]`

生成图片并返回公开 URL。

```lua
local url, err = t2i.generate_url("<p>Test</p>")
```

---

## 全局表: `sandbox`

| 权限 | `sandbox` |

> **开关检测:** Sandbox 服务未启用时调用返回 `(nil, "Sandbox 服务未启用")`。

### `sandbox.create() → table [, err]`

创建新的沙箱。返回: `{sandbox_id=string, status=string}`。

```lua
local sb = sandbox.create()
if not sb then return end
log.info("沙箱创建: " .. sb.sandbox_id)
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

---

## 全局表: `agent`

| 权限 | `agent` |

提供 Agent 配置的只读查询。

### `agent.get_providers() → []table [, err]`

返回所有 LLM Provider 配置。

### `agent.get_mcp_servers() → []table [, err]`

返回所有 MCP 服务器配置。

### `agent.get_skills() → []table [, err]`

返回所有 Skill 配置。

### `agent.get_sessions() → []table [, err]`

返回所有 Session。

### `agent.get_prompts() → []table [, err]`

返回所有 Prompt 模板。

### `agent.get_tools() → []table [, err]`

返回所有 Tool 配置。

### `agent.get_plugins() → []table [, err]`

返回所有已安装插件信息。

### `agent.set_provider_active(id, active) → bool [, err]`

启用/停用 LLM Provider。停用时从运行环境中移除，启用的 Provider 会被加载。

```lua
agent.set_provider_active("uuid", false)  -- 停用
agent.set_provider_active("uuid", true)   -- 启用
```

### `agent.set_mcp_active(id, active) → bool [, err]`

启用/停用 MCP 服务器。停用时会断开连接。

### `agent.get_current_chat_area() → table`

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

### `agent.compact_memory() → string [, err]`

Compact 当前 Chat-Area 的短期记忆：调用 LLM 将窗口内消息压缩为摘要，写入长期记忆并清空窗口。

```lua
local ok, err = agent.compact_memory()
if err then
    log.error("Compact 失败: " .. err)
end
```

> **注意:** 需要已配置 Text LLM Provider 才能执行 Compact。

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
