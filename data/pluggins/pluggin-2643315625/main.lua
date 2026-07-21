-- ====================================================================
-- JuanNiang-Neo 签到插件
-- ====================================================================
-- 命令:
--   /qd    — 每日签到，随机获得 1-10 分
--   /rank  — 查看积分排名（前十名）
-- ====================================================================

local jn = require("jn")

-- --------------------------------------------------------------------
-- 辅助函数：根据 event 回复消息
-- --------------------------------------------------------------------
local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- --------------------------------------------------------------------
-- 辅助函数：获取用户显示名称
-- --------------------------------------------------------------------
local function get_user_name(event)
    if event.message_type == "group" then
        local info = jn.onebot11.get_group_member_info(event.group_id, event.user_id)
        if info then
            return info.nickname and info.nickname ~= "" and info.nickname or tostring(event.user_id)
        end
    end
    return tostring(event.user_id)
end

-- --------------------------------------------------------------------
-- 辅助函数：获取今日日期 (YYYY-MM-DD)
-- --------------------------------------------------------------------
local function today()
    return os.date("%Y-%m-%d")
end

-- --------------------------------------------------------------------
-- 辅助函数：获取一句金句 (from vv314/quotes -> hitokoto API)
-- --------------------------------------------------------------------
local function get_quote()
    local resp = jn.http.get("https://v1.hitokoto.cn/?c=a&c=b&c=c&c=d&c=e&c=f&c=g&c=h&c=i&c=j&c=k&c=l")
    if resp and resp.status == 200 and resp.body then
        local data = jn.json.decode(resp.body)
        if data and data.hitokoto then
            local from = data.from and ("—— " .. data.from) or ""
            return data.hitokoto .. " " .. from
        end
    end
    -- 兜底金句
    local fallbacks = {
        "今天也是充满希望的一天！",
        "努力不一定会成功，但不努力一定很轻松——开个玩笑，还是要加油！",
        "生活不止眼前的苟且，还有明天的苟且。",
        "每一天都是新的一天，今天也要元气满满！",
        "人生没有白走的路，每一步都算数。",
    }
    return fallbacks[math.random(#fallbacks)]
end

-- --------------------------------------------------------------------
-- 初始化数据库表
-- --------------------------------------------------------------------
local function init_db()
    local sql = [[
        CREATE TABLE IF NOT EXISTS pluggin_checkin_records (
            id SERIAL PRIMARY KEY,
            user_id BIGINT NOT NULL,
            group_id BIGINT NOT NULL DEFAULT 0,
            user_name TEXT DEFAULT '',
            score INTEGER DEFAULT 0,
            check_date TEXT NOT NULL,
            created_at TEXT DEFAULT ''
        )
    ]]
    local _, err = jn.database.exec(sql)
    if err then
        jn.log.error("签到插件：初始化数据库失败: " .. err)
    end
end

-- --------------------------------------------------------------------
-- 检查今日是否已签到
-- --------------------------------------------------------------------
local function has_checked_in(user_id, group_id)
    local sql = string.format(
        "SELECT id FROM pluggin_checkin_records WHERE user_id = %d AND group_id = %d AND check_date = '%s' LIMIT 1",
        user_id, group_id, today()
    )
    local rows, err = jn.database.query(sql)
    if err then
        jn.log.error("签到插件：查询签到记录失败: " .. err)
        return nil
    end
    return rows and #rows > 0
end

-- --------------------------------------------------------------------
-- 执行签到
-- --------------------------------------------------------------------
local function do_checkin(user_id, group_id, user_name)
    local score = math.random(1, 10)
    local date = today()
    local sql = string.format(
        "INSERT INTO pluggin_checkin_records (user_id, group_id, user_name, score, check_date, created_at) VALUES (%d, %d, '%s', %d, '%s', '%s')",
        user_id, group_id, user_name, score, date, os.date("%Y-%m-%d %H:%M:%S")
    )
    local _, err = jn.database.exec(sql)
    if err then
        jn.log.error("签到插件：签到失败: " .. err)
        return nil, err
    end
    return score, nil
end

-- --------------------------------------------------------------------
-- 获取用户总积分
-- --------------------------------------------------------------------
local function get_user_score(user_id, group_id)
    local sql = string.format(
        "SELECT COALESCE(SUM(score), 0) AS total FROM pluggin_checkin_records WHERE user_id = %d AND group_id = %d",
        user_id, group_id
    )
    local rows, err = jn.database.query(sql)
    if err then
        jn.log.error("签到插件：查询积分失败: " .. err)
        return 0
    end
    if rows and #rows > 0 then
        local val = rows[1]["total"]
        if val then
            return tonumber(val) or 0
        end
    end
    return 0
end

-- --------------------------------------------------------------------
-- 命令: /qd —— 签到
-- --------------------------------------------------------------------
jn.command.register("qd", function(args, event)
    local user_id = event.user_id
    local group_id = event.group_id or 0

    -- 检查今日是否已签到
    local checked = has_checked_in(user_id, group_id)
    if checked == nil then
        reply(event, "签到查询出错，请稍后再试~")
        return true
    end
    if checked then
        reply(event, "你今天已经签到过了，明天再来吧~")
        return true
    end

    -- 获取用户名称
    local user_name = get_user_name(event)

    -- 执行签到
    local score, err = do_checkin(user_id, group_id, user_name)
    if err then
        reply(event, "签到失败，请稍后再试~")
        return true
    end

    -- 获取总积分
    local total = get_user_score(user_id, group_id)

    -- 获取金句
    local quote = get_quote()

    -- 构造回复
    local lines = {
        string.format("签到成功！%s", user_name),
        string.format("本次获得 +%d 分 | 累计 %d 分", score, total),
        "",
        quote,
    }
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "每日签到，随机获得 1-10 积分",
    usage = "/qd",
})

-- --------------------------------------------------------------------
-- 命令: /rank —— 查看排名（前十名）
-- --------------------------------------------------------------------
jn.command.register("rank", function(args, event)
    local group_id = event.group_id or 0

    local sql = string.format(
        [[SELECT user_id, user_name, COALESCE(SUM(score), 0) AS total, COUNT(*) AS check_days
          FROM pluggin_checkin_records
          WHERE group_id = %d
          GROUP BY user_id, user_name
          ORDER BY total DESC
          LIMIT 10]],
        group_id
    )
    local rows, err = jn.database.query(sql)
    if err then
        jn.log.error("签到插件：查询排名失败: " .. err)
        reply(event, "查询排名失败，请稍后再试~")
        return true
    end

    if not rows or #rows == 0 then
        reply(event, "暂无签到记录，快来签到吧~")
        return true
    end

    local lines = {"签到排行榜 (Top 10):"}
    local medals = { "🥇", "🥈", "🥉" }
    for i, row in ipairs(rows) do
        local medal = medals[i] or string.format("%d.", i)
        local name = row["user_name"] and row["user_name"] ~= "" and row["user_name"] or tostring(row["user_id"])
        local total = tonumber(row["total"]) or 0
        local days = tonumber(row["check_days"]) or 0
        lines[#lines + 1] = string.format("%s %s — %d 分 (%d 天)", medal, name, total, days)
    end

    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看签到积分排行榜（前十名）",
    usage = "/rank",
})

-- --------------------------------------------------------------------
-- on_message 兜底
-- --------------------------------------------------------------------
function on_message(event)
    return false, event
end

-- 初始化
init_db()
jn.log.info("签到插件已加载")
