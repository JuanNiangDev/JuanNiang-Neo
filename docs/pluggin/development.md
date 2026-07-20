# 插件开发指南

## 快速开始

### 1. 创建插件目录

```bash
mkdir -p data/pluggins/my-plugin
```

### 2. 编写 pluggin.yaml

```yaml
name: my-plugin
version: "1.0.0"
author: YourName
description: "我的插件"
entry: main.lua
permissions:
  - onebot11
  - http
  - cache
```

### 3. 编写 main.lua

推荐通过 `require("jn")` 引入 SDK，使用 `jn.command.register` 注册 `/` 开头的命令式交互。SDK 提供完整 IDE 类型提示，命令注册后由系统统一派发并自动生成 `/help`。

```lua
local jn = require("jn")

jn.log.info("my-plugin 已加载")

-- 注册 /ping 命令；系统自动派发并回复，无需在 on_message 中匹配
jn.command.register("ping", function(args, event)
    return true, "pong!"  -- consumed=true, reply="pong!"
end, {
    description = "Ping 测试",
    usage = "/ping",
})

-- 纯事件监听场景仍可使用 on_message
function on_message(event)
    -- 命令已由 CommandRegistry 派发，此处只处理非命令消息
    if event.raw_message:sub(1, 1) == "/" then
        return false, event
    end
    -- ... 其他逻辑
    return false, event
end
```

### 4. 启动服务

```bash
go run ./cmd/server/
# 或
docker compose -f deployments/docker-compose.yaml up
```

启动后即可在 QQ 中发送 `/ping` 测试，发送 `/help` 查看所有可用命令。

---

## 完整示例

> **约定**: 下列示例统一使用 `local jn = require("jn")` 引入 SDK。`jn.<table>.<func>` 与全局 `<table>.<func>` 完全等价，可按需混用。涉及 `/` 命令的示例使用 `jn.command.register` 注册，由系统自动派发与回复。

### 示例 1: Ping 插件 (onebot11)

```lua
-- permissions: [onebot11]
local jn = require("jn")

jn.command.register("ping", function(args, event)
    return true, "pong!"
end, { description = "Ping 测试", usage = "/ping" })
```

### 示例 2: 群管理插件 (onebot11)

```lua
-- permissions: [onebot11]
local jn = require("jn")

-- /kick <user_id>
jn.command.register("kick", function(args, event)
    local target = tonumber(args[1])
    if not target then
        return true, "用法: /kick <user_id>"
    end
    local ok, err = jn.onebot11.kick_group_member(event.group_id, target)
    if not ok then
        jn.log.error("踢人失败: " .. err)
        return true, "踢人失败: " .. err
    end
    return true, "已踢出 " .. target
end, { description = "踢出群成员", usage = "/kick <user_id>" })

-- /ban <user_id> <duration_seconds>
jn.command.register("ban", function(args, event)
    local target = tonumber(args[1])
    local duration = tonumber(args[2])
    if not target or not duration then
        return true, "用法: /ban <user_id> <duration_seconds>"
    end
    local ok, err = jn.onebot11.ban_group_member(event.group_id, target, duration)
    return true, ok and ("已禁言 " .. target .. " " .. duration .. "s") or ("失败: " .. err)
end, { description = "禁言成员", usage = "/ban <user_id> <duration_seconds>" })

-- /group_info
jn.command.register("group_info", function(args, event)
    local info = jn.onebot11.get_group_info(event.group_id)
    if not info then
        return true, "查询失败"
    end
    return true, string.format("群名: %s\n人数: %d", info.group_name, info.member_count)
end, { description = "查询群信息", usage = "/group_info" })
```

### 示例 3: HTTP 请求插件

```lua
-- permissions: [onebot11, http]
local jn = require("jn")

jn.command.register("joke", function(args, event)
    local r = jn.http.get("https://api.example.com/joke")
    if r.status ~= 200 then
        jn.log.error("API 返回: " .. r.status)
        return true, "API 不可用"
    end
    local data = jn.json.decode(r.body)
    return true, data.content or "无内容"
end, { description = "随机笑话", usage = "/joke" })
```

### 示例 4: 数据库 + 缓存插件

```lua
-- permissions: [onebot11, database, cache]
local jn = require("jn")

-- 加载时创建表（表名自动加 pluggin_my-plugin_ 前缀）
jn.database.exec([[
  CREATE TABLE IF NOT EXISTS counters (
    name TEXT PRIMARY KEY,
    value INTEGER DEFAULT 0
  )
]])

-- /count <name>
jn.command.register("count", function(args, event)
    local name = args[1]
    if not name then
        return true, "用法: /count <name>"
    end

    -- 先查缓存
    local cached = jn.cache.get("counter:" .. name)
    local count
    if cached then
        count = cached.value + 1
    else
        -- 缓存未命中, 查数据库
        local rows = jn.database.query(
            "SELECT value FROM counters WHERE name = '" .. name .. "'"
        )
        if #rows > 0 then
            count = rows[1].value + 1
        else
            count = 1
        end
    end

    -- 更新数据库
    jn.database.exec(string.format(
        "INSERT INTO counters (name, value) VALUES ('%s', %d) ON CONFLICT UPDATE SET value = %d",
        name, count, count
    ))

    -- 更新缓存 (5分钟)
    jn.cache.set("counter:" .. name, {value = count}, 300)

    return true, name .. ": " .. count
end, { description = "计数器", usage = "/count <name>" })
```

### 示例 5: Agent 管理插件

```lua
-- permissions: [onebot11, agent]
local jn = require("jn")

-- /providers — 列出所有 Provider
jn.command.register("providers", function(args, event)
    local providers = jn.agent.get_providers()
    local lines = {"Provider 列表:"}
    for _, p in ipairs(providers) do
        table.insert(lines, string.format("- %s (%s) [%s]", p.name, p.type, p.model))
    end
    return true, table.concat(lines, "\n")
end, { description = "列出所有 Provider", usage = "/providers" })

-- /area — 获取当前 Chat-Area
jn.command.register("area", function(args, event)
    local area = jn.agent.get_current_chat_area()
    return true, string.format(
        "ChatArea ID: %s\ntype: %s\nuser: %d\ngroup: %d",
        area.chat_area_id, area.message_type, area.user_id, area.group_id
    )
end, { description = "获取当前 Chat-Area", usage = "/area" })

-- /compact — Compact 记忆
jn.command.register("compact", function(args, event)
    local result, err = jn.agent.compact_memory()
    if err then
        return true, "Compact 失败: " .. err
    end
    return true, result
end, { description = "Compact 短期记忆", usage = "/compact" })

-- /switch_provider <id> — 切换主 Provider（多级命令示例）
jn.command.register({"myplugin", "switch_provider"}, function(args, event)
    local id = args[1]
    if not id then
        return true, "用法: /myplugin switch_provider <id>"
    end
    local ok, err = jn.agent.switch_provider(id)
    return true, ok and "已切换" or ("失败: " .. err)
end, { description = "切换主 Provider", usage = "/myplugin switch_provider <id>" })
```

### 示例 6: T2I 图片生成

```lua
-- permissions: [onebot11, t2i]
local jn = require("jn")

-- /draw <prompt>
jn.command.register("draw", function(args, event)
    local prompt = table.concat(args, " ")
    if prompt == "" then
        return true, "用法: /draw <prompt>"
    end
    local html = string.format("<h1>%s</h1><p>Generated by JuanNiang</p>", prompt)

    local id, err = jn.t2i.generate(html)
    if not id then
        return true, "图片生成失败: " .. err
    end
    return true, "图片已生成，ID: " .. id
end, { description = "文生图", usage = "/draw <prompt>" })
```

### 示例 7: Sandbox 代码执行

```lua
-- permissions: [onebot11, sandbox]
local jn = require("jn")

-- /py <code>
jn.command.register("py", function(args, event)
    local code = table.concat(args, " ")
    if code == "" then
        return true, "用法: /py <code>"
    end

    local sb = jn.sandbox.create()
    if not sb then
        return true, "沙箱创建失败"
    end

    local output, stderr = jn.sandbox.exec_python(sb.sandbox_id, code)
    local reply = "Output:\n" .. output
    if stderr and stderr ~= "" then
        reply = reply .. "\n\nErrors:\n" .. stderr
    end
    return true, reply
end, { description = "执行 Python 代码", usage = "/py <code>" })
```

### 示例 8: `on_message` 事件监听（非命令场景）

`on_message` 适合无固定命令模式的纯事件监听。命令优先派发保证 `/` 开头的消息先由 `CommandRegistry` 处理，未命中才 fallback 到 `on_message`。

```lua
-- permissions: [onebot11]
local jn = require("jn")

-- 监听所有非命令消息，记录到日志
function on_message(event)
    if event.raw_message:sub(1, 1) == "/" then
        return false, event  -- 命令消息，让命令系统处理
    end
    jn.log.info("收到消息: " .. event.raw_message)
    return false, event  -- 不消费，继续走 Agent
end
```

---

## 开发规范

### 命令注册 vs `on_message`

| 场景 | 推荐方式 |
|------|---------|
| 固定 `/cmd` 交互 | `jn.command.register` — 系统统一派发、自动回复、`/help` 自动生成 |
| 多级命令（`/foo bar baz`） | `jn.command.register({"foo", "bar", "baz"}, ...)` |
| 关键词触发（非 `/` 前缀） | `on_message` + `string.match` |
| 纯事件监听（不回复） | `on_message` 返回 `false` |

**注意**: 不要在 `on_message` 中再处理 `/` 开头的消息——命令系统会先消费命中命令，未命中的 `/` 消息才到达 `on_message`，但通常应视为未知命令而非自定义逻辑。

### 错误处理

Lua 中的运行时错误会被 `PluginEngine` 捕获并记录:

```
ERROR [plugin:my-plugin] on_message 错误 err=<string>:4: attempt to ...
```

建议对关键操作使用 `pcall`:

```lua
local ok, err = pcall(function()
    jn.onebot11.send_group_msg(group_id, complex_message)
end)
if not ok then
    jn.log.error("发送失败: " .. tostring(err))
end
```

命令 handler 中的 error 也会被捕获并记录到日志，但不会自动回复给用户——若需要错误回复，请在 handler 中显式 `return true, "错误: " .. tostring(err)`。

### 性能注意事项

- `on_message` 与命令 handler 都会阻塞事件循环（每条消息串行处理）
- 避免在其中执行长时间网络请求
- 数据库查询使用索引、缓存热点数据

### 命名空间

- 数据库表名自动带 `pluggin_<name>_` 前缀，不会与系统表冲突
- 缓存 Key 自动带 `pluggin:<name>:` 前缀，不会与系统缓存冲突
- 无需手动添加前缀

### 服务开关

T2I 和 Sandbox 为可插拔服务。在代码中检查返回值即可：

```lua
local result, err = jn.t2i.generate(html)
if not result then
    -- 服务未启用或生成失败
    jn.log.error("T2I: " .. err)
end
```

也可通过 `is_active()` 主动查询：

```lua
if not jn.t2i.is_active() then
    return true, "T2I 服务未启用"
end
```

---

## 调试

```bash
# 查看日志
go run ./cmd/server/

# Docker
docker compose -f deployments/docker-compose.yaml logs -f juan-niang-neo
```

### 热加载

修改插件后重启服务即可 (后续版本支持 Web API 热加载)。

### IDE 类型提示

将 `data/pluggins/sdk/jn.lua` 加入工作区即可在 VS Code（安装 sumneko lua-language-server 扩展）中获得：
- `require("jn")` 返回值的字段补全
- 函数参数/返回值类型提示
- hover 显示 `---@class` / `---@field` 文档

SDK 文件由二进制启动时自动落盘到 `data/pluggins/sdk/jn.lua`，无需手动创建。
