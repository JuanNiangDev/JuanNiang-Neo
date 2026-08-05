-- ====================================================================
-- memo：便签示例插件 —— jn.file API 完整演示
-- ====================================================================
-- 覆盖内容（file 权限）：
--   1. jn.file.append_line —— 追加一行（add）
--   2. jn.file.read_lines   —— 按行读取全部（load_memos / set / del）
--   3. jn.file.read_line    —— 读取第 N 行（get；越界返回 nil 可作循环终止）
--   4. jn.file.write_line   —— 改写第 N 行（set）
--   5. jn.file.write_lines  —— 覆盖写入多行（del）
--   6. jn.file.read         —— 整体读取（list 演示手动按行拆分）
--   7. jn.file.write        —— 整体覆盖写入（clear）
--   8. jn.file.exists       —— 判断文件是否存在（clear 前置检查）
-- 数据存储：data/pluggins/memo/data/<QQ号>.txt，每个用户一个文件，一行一条便签。
-- 所有路径均相对插件自身目录，禁止 .. 越界（由引擎强制）。
-- ====================================================================

local jn = require("jn")

-- --------------------------------------------------------------------
-- 辅助函数：按消息类型回复（群聊发群、私聊发私聊）
-- --------------------------------------------------------------------
local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- 某用户的便签文件路径（相对插件目录）
local function memo_path(user_id)
    return "data/" .. tostring(user_id) .. ".txt"
end

-- --------------------------------------------------------------------
-- 辅助函数：读取某用户全部便签；文件不存在视为空列表
-- （演示 jn.file.read_lines —— 按行读取）
-- --------------------------------------------------------------------
local function load_memos(user_id)
    local lines, err = jn.file.read_lines(memo_path(user_id))
    if err then
        -- 文件尚未创建（read_lines 对不存在的文件返回 err）
        return {}
    end
    return lines
end

-- --------------------------------------------------------------------
-- 辅助函数：群白名单检查（返回 true 表示已拦截并回复）
-- （演示 jn.config.get 读取 list 类型配置）
-- --------------------------------------------------------------------
local function reject_if_not_allowed(event)
    if event.message_type ~= "group" then
        return false
    end
    local whitelist = jn.config.get("group_whitelist") or {}
    if #whitelist == 0 then
        return false
    end
    local gid = tostring(event.group_id)
    for _, v in ipairs(whitelist) do
        if tostring(v) == gid then
            return false
        end
    end
    reply(event, "该群未开放便签功能~")
    return true
end

-- --------------------------------------------------------------------
-- /memo add <内容> —— 追加一条便签（jn.file.append_line）
-- --------------------------------------------------------------------
jn.command.register("memo add", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local content = table.concat(args, " ")
    if content == "" then
        reply(event, "用法: /memo add <内容>")
        return true
    end
    -- 去掉内容里的换行，保证"一行一条"的存储格式
    content = content:gsub("[\r\n]+", " ")

    -- 超长截断（读 string 类型配置）
    local max_len = tonumber(jn.config.get("max_len")) or 200
    if #content > max_len then
        content = content:sub(1, max_len) .. "…"
    end

    -- 条数上限（append 前先数一下已有条数）
    local max_notes = tonumber(jn.config.get("max_notes")) or 50
    local lines = load_memos(event.user_id)
    if #lines >= max_notes then
        reply(event, "便签已满（最多 " .. max_notes .. " 条），请先 /memo del 一些~")
        return true
    end

    local ok, err = jn.file.append_line(memo_path(event.user_id), content)
    if not ok then
        jn.log.error("memo: 追加便签失败: " .. tostring(err))
        reply(event, "保存失败，请稍后再试~")
        return true
    end
    reply(event, string.format("已保存第 %d 条便签 📝", #lines + 1))
    return true
end, {
    description = "添加一条便签",
    usage = "/memo add <内容>",
})

-- --------------------------------------------------------------------
-- /memo list —— 列出全部便签（jn.file.read 整体读取 + 手动按行拆分）
-- --------------------------------------------------------------------
jn.command.register("memo list", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local content, err = jn.file.read(memo_path(event.user_id))
    if content == nil then
        -- 文件不存在 → read 返回 (nil, err)
        reply(event, "你还没有便签，用 /memo add <内容> 添加第一条吧~")
        return true
    end

    -- 演示整体读取后自行拆分（read_lines 是等价的行级 API）
    local lines = {}
    for line in content:gmatch("[^\r\n]+") do
        lines[#lines + 1] = line
    end

    if #lines == 0 then
        reply(event, "你还没有便签，用 /memo add <内容> 添加第一条吧~")
        return true
    end
    local out = { string.format("📒 你的便签（共 %d 条）：", #lines) }
    for i, line in ipairs(lines) do
        out[#out + 1] = string.format("%d. %s", i, line)
    end
    reply(event, table.concat(out, "\n"))
    return true
end, {
    description = "列出全部便签",
    usage = "/memo list",
})

-- --------------------------------------------------------------------
-- /memo get <n> —— 查看第 n 条（jn.file.read_line；越界返回 nil）
-- --------------------------------------------------------------------
jn.command.register("memo get", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local n = tonumber(args[1])
    if not n or n < 1 then
        reply(event, "用法: /memo get <序号>")
        return true
    end

    local line, err = jn.file.read_line(memo_path(event.user_id), n)
    if line == nil then
        -- 越界/文件不存在都返回 nil（read_line 约定越界不是错误）
        reply(event, "第 " .. n .. " 条便签不存在~")
        return true
    end
    reply(event, string.format("第 %d 条：\n%s", n, line))
    return true
end, {
    description = "查看第 n 条便签",
    usage = "/memo get <序号>",
})

-- --------------------------------------------------------------------
-- /memo set <n> <内容> —— 改写第 n 条（jn.file.write_line）
-- --------------------------------------------------------------------
jn.command.register("memo set", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local n = tonumber(args[1])
    local content = table.concat(args, " ", 2)
    if not n or n < 1 or content == "" then
        reply(event, "用法: /memo set <序号> <内容>")
        return true
    end
    content = content:gsub("[\r\n]+", " ")

    -- write_line 对超出行号会自动补空行，这里先校验范围避免产生空行
    local lines = load_memos(event.user_id)
    if n > #lines then
        reply(event, "第 " .. n .. " 条便签不存在（当前共 " .. #lines .. " 条）~")
        return true
    end

    local ok, err = jn.file.write_line(memo_path(event.user_id), n, content)
    if not ok then
        jn.log.error("memo: 改写便签失败: " .. tostring(err))
        reply(event, "改写失败，请稍后再试~")
        return true
    end
    reply(event, "已改写第 " .. n .. " 条便签 ✅")
    return true
end, {
    description = "改写第 n 条便签",
    usage = "/memo set <序号> <内容>",
})

-- --------------------------------------------------------------------
-- /memo del <n> —— 删除第 n 条（jn.file.read_lines + write_lines 重写）
-- --------------------------------------------------------------------
jn.command.register("memo del", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local n = tonumber(args[1])
    if not n or n < 1 then
        reply(event, "用法: /memo del <序号>")
        return true
    end

    local lines = load_memos(event.user_id)
    if n > #lines then
        reply(event, "第 " .. n .. " 条便签不存在（当前共 " .. #lines .. " 条）~")
        return true
    end

    table.remove(lines, n)
    local ok, err = jn.file.write_lines(memo_path(event.user_id), lines)
    if not ok then
        jn.log.error("memo: 删除便签失败: " .. tostring(err))
        reply(event, "删除失败，请稍后再试~")
        return true
    end
    reply(event, "已删除第 " .. n .. " 条便签 🗑️")
    return true
end, {
    description = "删除第 n 条便签",
    usage = "/memo del <序号>",
})

-- --------------------------------------------------------------------
-- /memo clear —— 清空全部（jn.file.exists + jn.file.write 整体覆盖）
-- --------------------------------------------------------------------
jn.command.register("memo clear", function(args, event)
    if reject_if_not_allowed(event) then return true end

    local path = memo_path(event.user_id)
    if not jn.file.exists(path) then
        reply(event, "你还没有便签，无需清空~")
        return true
    end

    local ok, err = jn.file.write(path, "")
    if not ok then
        jn.log.error("memo: 清空便签失败: " .. tostring(err))
        reply(event, "清空失败，请稍后再试~")
        return true
    end
    reply(event, "已清空全部便签 🧹")
    return true
end, {
    description = "清空全部便签",
    usage = "/memo clear",
})

-- --------------------------------------------------------------------
-- on_message 兜底：不消费消息，交由 Agent 处理
-- --------------------------------------------------------------------
function on_message(event)
    return false, event, false
end

jn.log.info("memo 插件已加载（jn.file API 示例）")
