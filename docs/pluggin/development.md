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

```lua
log.info("my-plugin 已加载")

function on_message(event)
    if event.raw_message == "/ping" then
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, "pong!")
        else
            onebot11.send_private_msg(event.user_id, "pong!")
        end
        return true, event
    end
    return false, event
end
```

### 4. 启动服务

```bash
go run ./cmd/server/
# 或
docker compose -f deployments/docker-compose.yaml up
```

---

## 完整示例

### 示例 1: Ping 插件 (onebot11)

```lua
-- permissions: [onebot11]
function on_message(event)
    if event.raw_message == "/ping" then
        local reply = "pong!"
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, reply)
        else
            onebot11.send_private_msg(event.user_id, reply)
        end
        return true, event
    end
    return false, event
end
```

### 示例 2: 群管理插件 (onebot11)

```lua
-- permissions: [onebot11]
function on_message(event)
    local msg = event.raw_message

    -- 踢人
    if msg:match("^/kick (%d+)") then
        local target = tonumber(msg:match("^/kick (%d+)"))
        local ok, err = onebot11.kick_group_member(event.group_id, target)
        if not ok then
            log.error("踢人失败: " .. err)
        end
        return true, event
    end

    -- 禁言
    if msg:match("^/ban (%d+) (%d+)") then
        local target, duration = msg:match("^/ban (%d+) (%d+)")
        onebot11.ban_group_member(event.group_id, tonumber(target), tonumber(duration))
        return true, event
    end

    -- 查询群信息
    if msg == "/group_info" then
        local info = onebot11.get_group_info(event.group_id)
        if info then
            local reply = string.format("群名: %s\n人数: %d", info.group_name, info.member_count)
            onebot11.send_group_msg(event.group_id, reply)
        end
        return true, event
    end

    return false, event
end
```

### 示例 3: HTTP 请求插件

```lua
-- permissions: [onebot11, http]
function on_message(event)
    if event.raw_message == "/joke" then
        local r = http.get("https://api.example.com/joke")
        if r.status == 200 then
            local data = json.decode(r.body)
            if event.message_type == "group" then
                onebot11.send_group_msg(event.group_id, data.content)
            else
                onebot11.send_private_msg(event.user_id, data.content)
            end
        else
            log.error("API 返回: " .. r.status)
        end
        return true, event
    end
    return false, event
end
```

### 示例 4: 数据库 + 缓存插件

```lua
-- permissions: [onebot11, database, cache]

-- 加载时创建表
database.exec([[
  CREATE TABLE IF NOT EXISTS counters (
    name TEXT PRIMARY KEY,
    value INTEGER DEFAULT 0
  )
]])

function on_message(event)
    local msg = event.raw_message

    -- 计数器: /count <name>
    if msg:match("^/count (.+)") then
        local name = msg:match("^/count (.+)")

        -- 先查缓存
        local cached = cache.get("counter:" .. name)
        local count
        if cached then
            count = cached.value + 1
        else
            -- 缓存未命中, 查数据库
            local rows = database.query(
                "SELECT value FROM counters WHERE name = '" .. name .. "'"
            )
            if #rows > 0 then
                count = rows[1].value + 1
            else
                count = 1
            end
        end

        -- 更新数据库
        database.exec(string.format(
            "INSERT INTO counters (name, value) VALUES ('%s', %d) ON CONFLICT UPDATE SET value = %d",
            name, count, count
        ))

        -- 更新缓存 (5分钟)
        cache.set("counter:" .. name, {value = count}, 300)

        onebot11.send_group_msg(event.group_id, name .. ": " .. count)
        return true, event
    end

    return false, event
end
```

### 示例 5: Agent 管理插件

```lua
-- permissions: [onebot11, agent]

function on_message(event)
    local msg = event.raw_message

    -- 列出所有 Provider
    if msg == "/providers" then
        local providers = agent.get_providers()
        local lines = {"Provider 列表:"}
        for _, p in ipairs(providers) do
            table.insert(lines, string.format("- %s (%s) [%s]", p.name, p.type, p.model))
        end
        onebot11.send_group_msg(event.group_id, table.concat(lines, "\n"))
        return true, event
    end

    -- 获取当前 Chat-Area
    if msg == "/area" then
        local area = agent.get_current_chat_area()
        local reply = string.format(
            "ChatArea ID: %s\ntype: %s\nuser: %d\ngroup: %d",
            area.chat_area_id, area.message_type, area.user_id, area.group_id
        )
        onebot11.send_group_msg(event.group_id, reply)
        return true, event
    end

    -- Compact 记忆
    if msg == "/compact" then
        local result, err = agent.compact_memory()
        if err then
            onebot11.send_group_msg(event.group_id, "Compact 失败: " .. err)
        else
            onebot11.send_group_msg(event.group_id, result)
        end
        return true, event
    end

    return false, event
end
```

### 示例 6: T2I 图片生成

```lua
-- permissions: [onebot11, t2i]

function on_message(event)
    local msg = event.raw_message

    if msg:match("^/draw (.+)") then
        local prompt = msg:match("^/draw (.+)")
        local html = string.format("<h1>%s</h1><p>Generated by JuanNiang</p>", prompt)

        local id, err = t2i.generate(html)
        if not id then
            onebot11.send_group_msg(event.group_id, "图片生成失败: " .. err)
        else
            onebot11.send_group_msg(event.group_id, "图片已生成，ID: " .. id)
        end
        return true, event
    end

    return false, event
end
```

### 示例 7: Sandbox 代码执行

```lua
-- permissions: [onebot11, sandbox]

function on_message(event)
    local msg = event.raw_message

    if msg:match("^/py (.+)") then
        local code = msg:match("^/py (.+)")

        local sb = sandbox.create()
        if not sb then
            onebot11.send_group_msg(event.group_id, "沙箱创建失败")
            return true, event
        end

        local output, stderr = sandbox.exec_python(sb.sandbox_id, code)
        local reply = "Output:\n" .. output
        if stderr and stderr ~= "" then
            reply = reply .. "\n\nErrors:\n" .. stderr
        end

        onebot11.send_group_msg(event.group_id, reply)
        return true, event
    end

    return false, event
end
```

---

## 开发规范

### 错误处理

Lua 中的运行时错误会被 `PluginEngine` 捕获并记录:

```
ERROR [plugin:my-plugin] on_message 错误 err=<string>:4: attempt to ...
```

建议对关键操作使用 `pcall`:

```lua
local ok, err = pcall(function()
    onebot11.send_group_msg(group_id, complex_message)
end)
if not ok then
    log.error("发送失败: " .. tostring(err))
end
```

### 性能注意事项

- `on_message` 会阻塞事件循环 (每条消息串行处理)
- 避免在 `on_message` 中执行长时间网络请求
- 数据库查询使用索引、缓存热点数据

### 命名空间

- 数据库表名自动带 `pluggin_<name>_` 前缀，不会与系统表冲突
- 缓存 Key 自动带 `pluggin:<name>:` 前缀，不会与系统缓存冲突
- 无需手动添加前缀

### 服务开关

T2I 和 Sandbox 为可插拔服务。在代码中检查返回值即可：

```lua
local result, err = t2i.generate(html)
if not result then
    -- 服务未启用或生成失败
    log.error("T2I: " .. err)
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
