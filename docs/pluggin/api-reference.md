# Lua API 参考

本文档列出 JuanNiang-Neo 暴露给 Lua 插件的所有 API 函数。函数可用性由 `pluggin.yaml` 中的 `permissions` 字段控制。

---

## 全局表: `log`

日志输出, 输出到服务器 `slog` 日志流。

| 权限 | 始终可用 (无需声明) |

### `log.info(message)`

输出 INFO 级别日志。

```lua
log.info("插件启动完成")
-- 输出: [plugin:my-plugin] msg=插件启动完成
```

### `log.warn(message)`

输出 WARN 级别日志。

```lua
log.warn("配置项缺失, 使用默认值")
```

### `log.error(message)`

输出 ERROR 级别日志。

```lua
log.error("初始化失败: " .. err)
```

---

## 全局表: `json`

JSON 编解码。

| 权限 | 始终可用 |

### `json.encode(value) → string`

将 Lua 值编码为 JSON 字符串。

```lua
local data = {name = "test", count = 42}
local str = json.encode(data)
-- str = '{"name":"test","count":42}'
```

### `json.decode(str) → table`

将 JSON 字符串解码为 Lua table。

```lua
local str = '{"name":"test","count":42}'
local data = json.decode(str)
-- data.name = "test"
-- data.count = 42
```

---

## 全局表: `onebot11`

OneBot11 消息发送 API。

| 权限 | `onebot11` |

### `onebot11.send_group_msg(group_id, message) → bool [, error]`

发送群聊消息。

| 参数 | 类型 | 说明 |
|------|------|------|
| `group_id` | number | 目标群号 |
| `message` | string | 消息内容 (纯文本) |

| 返回值 | 类型 | 说明 |
|--------|------|------|
| 第一个 | boolean | 是否成功 |
| 第二个 | string | 失败时返回错误信息 |

```lua
local ok, err = onebot11.send_group_msg(123456789, "Hello World!")
if not ok then
    log.error("发送失败: " .. err)
end
```

### `onebot11.send_private_msg(user_id, message) → bool [, error]`

发送私聊消息。

| 参数 | 类型 | 说明 |
|------|------|------|
| `user_id` | number | 目标用户 QQ 号 |
| `message` | string | 消息内容 (纯文本) |

```lua
onebot11.send_private_msg(987654321, "你好!")
```

---

## 全局表: `http`

HTTP 请求 (预留, 当前仅返回占位信息)。

| 权限 | `http` |

### `http.get(url) → string`

发送 GET 请求 (预留)。

### `http.post(url, body) → string`

发送 POST 请求 (预留)。

> 注意: HTTP 功能计划在后续版本完善, 当前函数返回固定提示字符串。

---

## 回调函数

插件通过定义全局函数来响应系统事件。

### `on_message(event) → (consumed, modified)`

消息事件回调。每次收到 OneBot11 消息事件时调用。

**参数 `event`** (Lua table):

| 字段 | 类型 | 说明 |
|------|------|------|
| `post_type` | string | 固定为 `"message"` |
| `message_type` | string | `"private"` 或 `"group"` |
| `user_id` | number | 发送者 QQ 号 |
| `group_id` | number | 群号 (群消息时有效, 否则为 0) |
| `raw_message` | string | 消息原文 |

**返回值:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `consumed` | boolean | `true` 表示插件已处理, 跳过 Agent |
| `modified` | table | 修改后的事件 (当前仅原样返回) |

**示例:**

```lua
function on_message(event)
    -- 检测 /ping 命令
    if event.raw_message == "/ping" then
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, "pong!")
        elseif event.message_type == "private" then
            onebot11.send_private_msg(event.user_id, "pong!")
        end
        return true, event   -- 消费事件, 不触发 Agent
    end

    return false, event      -- 不消费, 继续 Agent 处理
end
```
