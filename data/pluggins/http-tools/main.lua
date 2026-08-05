-- ====================================================================
-- http-tools：HTTP 请求示例
-- ====================================================================
-- 覆盖内容：
--   1. http.get  —— 一言金句 / wttr.in 天气
--   2. http.post —— 演示 JSON POST
-- ====================================================================

local jn = require("jn")

local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- --------------------------------------------------------------------
-- /hitokoto [类型] —— 一言金句
-- 类型: a动画 b漫画 c游戏 d小说 e原创 f网络 g其他
-- --------------------------------------------------------------------
jn.command.register("hitokoto", function(args, event)
    local cat = args[1] or ""
    local url = "https://v1.hitokoto.cn/?encode=json"
    if cat ~= "" and #cat == 1 then
        url = url .. "&c=" .. cat
    end
    local res, err = jn.http.get(url)
    if not res then
        reply(event, "请求失败: " .. tostring(err))
        return true
    end
    if res.status ~= 200 then
        reply(event, "API 返回异常状态: " .. tostring(res.status))
        return true
    end
    local ok, data = pcall(json.decode, res.body)
    if not ok or not data.hitokoto then
        reply(event, "解析失败，响应: " .. tostring(res.body))
        return true
    end
    local line = "「" .. data.hitokoto .. "」"
    if data.from then
        line = line .. "\n—— 出自 " .. data.from
    end
    reply(event, line)
    return true
end, {
    description = "获取一言金句",
    usage = "/hitokoto [a|b|c|d|e|f|g]",
})

-- --------------------------------------------------------------------
-- /weather <城市> —— wttr.in 简版天气
-- --------------------------------------------------------------------
jn.command.register("weather", function(args, event)
    local city = args[1] or "北京"
    -- wttr.in 返回纯文本格式，?format=3 是精简版
    local res, err = jn.http.get("https://wttr.in/" .. city .. "?format=3")
    if not res then
        reply(event, "天气查询失败: " .. tostring(err))
        return true
    end
    if res.status ~= 200 then
        reply(event, "天气 API 异常: " .. tostring(res.status))
        return true
    end
    reply(event, "🌤️ " .. (res.body or ""))
    return true
end, {
    description = "查询天气（简版）",
    usage = "/weather <城市>",
})

-- --------------------------------------------------------------------
-- /http post —— 演示 POST + JSON 编码
-- --------------------------------------------------------------------
jn.command.register("http post", function(args, event)
    local payload = json.encode({ plugin = "http-tools", ts = os.time(), echo = args[1] or "hello" })
    local res, err = jn.http.post("https://httpbin.org/post", "application/json", payload)
    if not res then
        reply(event, "POST 失败: " .. tostring(err))
        return true
    end
    local ok, data = pcall(json.decode, res.body)
    if ok and data.json then
        reply(event, "httpbin 回显: " .. json.encode(data.json))
    else
        reply(event, "响应: " .. tostring(res.body))
    end
    return true
end, {
    description = "演示 http.post（httpbin 回显）",
    usage = "/http post <文本>",
})

log.info("http-tools 插件已加载")
