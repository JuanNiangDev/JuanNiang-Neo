-- ====================================================================
-- http-tools：HTTP 请求示例（异步版）
-- ====================================================================
-- 覆盖内容：
--   1. http.get_async  —— 一言金句 / wttr.in 天气（异步 + 调用现场 ctx）
--   2. http.post_async —— 演示 JSON POST（异步）
-- 说明：外部 HTTP 请求可能耗时（秒级），用异步版不阻塞事件循环；
--   调用时把回复目标等现场打包成 ctx，完成回调 on_http_response 里
--   按 ctx.action 分发处理结果并回复。
-- ====================================================================

local jn = require("jn")

local function target_of(event)
    if event.message_type == "group" then
        return { kind = "group", id = event.group_id }
    end
    return { kind = "private", id = event.user_id }
end

local function reply_to(ctx, text)
    if not ctx or not ctx.target then return end
    if ctx.target.kind == "group" then
        jn.onebot11.send_group_msg(ctx.target.id, text)
    else
        jn.onebot11.send_private_msg(ctx.target.id, text)
    end
end

-- --------------------------------------------------------------------
-- /hitokoto [类型] —— 一言金句（异步）
-- 类型: a动画 b漫画 c游戏 d小说 e原创 f网络 g其他
-- --------------------------------------------------------------------
jn.command.register("hitokoto", function(args, event)
    local cat = args[1] or ""
    local url = "https://v1.hitokoto.cn/?encode=json"
    if cat ~= "" and #cat == 1 then
        url = url .. "&c=" .. cat
    end
    -- 现场保存：动作类型 + 回复目标，回调时原样带回
    local ctx = { action = "hitokoto", target = target_of(event) }
    local rid = jn.http.get_async(url, ctx)
    if rid == 0 then
        reply_to(ctx, "异步请求提交失败")
    end
    return true
end, {
    description = "获取一言金句",
    usage = "/hitokoto [a|b|c|d|e|f|g]",
})

-- --------------------------------------------------------------------
-- /weather <城市> —— wttr.in 简版天气（异步）
-- --------------------------------------------------------------------
jn.command.register("weather", function(args, event)
    local city = args[1] or "北京"
    local ctx = { action = "weather", target = target_of(event) }
    -- wttr.in 返回纯文本格式，?format=3 是精简版
    local rid = jn.http.get_async("https://wttr.in/" .. city .. "?format=3", ctx)
    if rid == 0 then
        reply_to(ctx, "天气查询提交失败")
    end
    return true
end, {
    description = "查询天气（简版）",
    usage = "/weather <城市>",
})

-- --------------------------------------------------------------------
-- /http post —— 演示 POST + JSON 编码（异步）
-- 尾部 table 参数即调用现场 ctx（与 body 字符串区分）
-- --------------------------------------------------------------------
jn.command.register("http post", function(args, event)
    local payload = json.encode({ plugin = "http-tools", ts = os.time(), echo = args[1] or "hello" })
    local ctx = { action = "post", target = target_of(event) }
    local rid = jn.http.post_async("https://httpbin.org/post", "application/json", payload, ctx)
    if rid == 0 then
        reply_to(ctx, "POST 提交失败")
    end
    return true
end, {
    description = "演示 http.post_async（httpbin 回显）",
    usage = "/http post <文本>",
})

-- --------------------------------------------------------------------
-- 异步完成回调：on_http_response(req_id, ctx, result, err)
--   result = {status=number, body=string}；err 非 nil 表示失败
-- --------------------------------------------------------------------
function on_http_response(req_id, ctx, result, err)
    if not ctx then
        log.warn("on_http_response: 无调用现场 ctx，忽略回调")
        return
    end
    if err then
        reply_to(ctx, "请求失败: " .. tostring(err))
        return
    end

    if ctx.action == "hitokoto" then
        if result.status ~= 200 then
            reply_to(ctx, "API 返回异常状态: " .. tostring(result.status))
            return
        end
        local ok, data = pcall(json.decode, result.body)
        if not ok or not data.hitokoto then
            reply_to(ctx, "解析失败，响应: " .. tostring(result.body))
            return
        end
        local line = "「" .. data.hitokoto .. "」"
        if data.from then
            line = line .. "\n—— 出自 " .. data.from
        end
        reply_to(ctx, line)

    elseif ctx.action == "weather" then
        if result.status ~= 200 then
            reply_to(ctx, "天气 API 异常: " .. tostring(result.status))
            return
        end
        reply_to(ctx, "🌤️ " .. tostring(result.body))

    elseif ctx.action == "post" then
        local ok, data = pcall(json.decode, result.body)
        if ok and data.json then
            reply_to(ctx, "httpbin 回显: " .. json.encode(data.json))
        else
            reply_to(ctx, "响应: " .. tostring(result.body))
        end
    end
end

log.info("http-tools 插件已加载")
