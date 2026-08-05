-- ====================================================================
-- data-store：数据持久化示例
-- ====================================================================
-- 覆盖内容：
--   1. cache  —— Redis 缓存（计数器，键自动加 pluggin:<name>: 前缀）
--   2. database —— Postgres 笔记 CRUD（自定义表需加前缀 + IF NOT EXISTS）
-- ====================================================================
-- ⚠ database 无命名空间隔离，自定义表必须加自己的前缀（data_store_）
-- ====================================================================

local jn = require("jn")

local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- 加载时自动建表（幂等）
database.exec([[
  CREATE TABLE IF NOT EXISTS data_store_notes (
    id SERIAL PRIMARY KEY,
    content TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
  )
]])

-- --------------------------------------------------------------------
-- /counter —— Redis 计数器（演示 cache.set/get/incr 思路）
-- --------------------------------------------------------------------
jn.command.register("counter", function(args, event)
    local key = "counter:" .. tostring(event.user_id)
    local cur = jn.cache.get(key) or 0
    cur = tonumber(cur) or 0
    cur = cur + 1
    -- cache 存的是 JSON 值；存数字时用 json.encode
    jn.cache.set(key, json.encode(cur), 86400)  -- 24h 过期
    reply(event, string.format("你已经打卡 %d 次啦！", cur))
    return true
end, {
    description = "Redis 计数器（按人，24h 有效）",
    usage = "/counter",
})

-- --------------------------------------------------------------------
-- /note add <内容> / /note list / /note del <id>
-- --------------------------------------------------------------------
jn.command.register("note add", function(args, event)
    local content = table.concat(args, " ")
    if content == "" then
        reply(event, "用法: /note add <内容>")
        return true
    end
    -- 参数化占位：使用 ? 占位符
    local n, err = database.exec(
        "INSERT INTO data_store_notes (content, created_by) VALUES (?, ?)",
        content, tostring(event.user_id))
    if not n then
        reply(event, "写入失败: " .. tostring(err))
        return true
    end
    reply(event, "笔记已保存 ✔")
    return true
end, {
    description = "保存一条笔记",
    usage = "/note add <内容>",
})

jn.command.register("note list", function(args, event)
    local rows, err = database.query(
        "SELECT id, content, created_at FROM data_store_notes ORDER BY id DESC LIMIT 10")
    if not rows then
        reply(event, "查询失败: " .. tostring(err))
        return true
    end
    if #rows == 0 then
        reply(event, "还没有笔记，用 /note add 保存第一条")
        return true
    end
    local lines = { "最近 10 条笔记:" }
    for _, r in ipairs(rows) do
        lines[#lines+1] = string.format("#%s %s（%s）", r.id, r.content, r.created_at)
    end
    lines[#lines+1] = "删除: /note del <id>"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出最近的笔记",
    usage = "/note list",
})

jn.command.register("note del", function(args, event)
    local id = tonumber(args[1])
    if not id then
        reply(event, "用法: /note del <id>")
        return true
    end
    local n, err = database.exec("DELETE FROM data_store_notes WHERE id = ?", id)
    if not n then
        reply(event, "删除失败: " .. tostring(err))
        return true
    end
    reply(event, "已删除 " .. n .. " 条笔记")
    return true
end, {
    description = "删除笔记",
    usage = "/note del <id>",
})

-- --------------------------------------------------------------------
-- /cache demo —— 演示 cache API 全家桶
-- --------------------------------------------------------------------
jn.command.register("cache demo", function(args, event)
    local key = "demo:" .. tostring(event.user_id)
    jn.cache.set(key, json.encode({ seen = true, at = os.time() }), 300)
    local exists = jn.cache.exists(key)
    local val = jn.cache.get(key)
    local out = string.format("exists=%d value=%s", exists, tostring(val))
    jn.cache.del(key)
    out = out .. "\nafter_del exists=" .. tostring(jn.cache.exists(key))
    reply(event, out)
    return true
end, {
    description = "演示 cache 全套 API",
    usage = "/cache demo",
})

log.info("data-store 插件已加载")
