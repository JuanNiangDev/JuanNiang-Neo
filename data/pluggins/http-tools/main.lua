-- ====================================================================
-- http-tools：HTTP 请求示例（同步版 + 异步版双示范）
-- ====================================================================
-- 覆盖内容：
--   同步版：http.get / http.post —— 适合低频快路径（会阻塞事件循环）
--   异步版：http.get_async / http.post_async —— 耗时请求推荐（不阻塞，
--           完成回调 on_http_response，调用现场用 ctx 保存并带回）
-- 每个功能都提供 sync / async 两个命令，业务处理逻辑共享。
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

-- ====================================================================
-- 业务处理（同步 / 异步共用；res = {status, body}）
-- ====================================================================

local function build_hitokoto_url(cat)
    local url = "https://v1.hitokoto.cn/?encode=json"
    if cat ~= "" and #cat == 1 then
        url = url .. "&c=" .. cat
    end
    return url
end

local function handle_hitokoto(ctx, res)
    if not res then
        reply_to(ctx, "请求失败")
        return
    end
    if res.status ~= 200 then
        reply_to(ctx, "API 返回异常状态: " .. tostring(res.status))
        return
    end
    local ok, data = pcall(json.decode, res.body)
    if not ok or not data.hitokoto then
        reply_to(ctx, "解析失败，响应: " .. tostring(res.body))
        return
    end
    local line = "「" .. data.hitokoto .. "」"
    if data.from then
        line = line .. "\n—— 出自 " .. data.from
    end
    reply_to(ctx, line)
end

local function handle_weather(ctx, res)
    if not res then
        reply_to(ctx, "天气查询失败")
        return
    end
    if res.status ~= 200 then
        reply_to(ctx, "天气 API 异常: " .. tostring(res.status))
        return
    end
    reply_to(ctx, "🌤️ " .. tostring(res.body))
end

local function handle_post(ctx, res)
    if not res then
        reply_to(ctx, "POST 失败")
        return
    end
    local ok, data = pcall(json.decode, res.body)
    if ok and data.json then
        reply_to(ctx, "httpbin 回显: " .. json.encode(data.json))
    else
        reply_to(ctx, "响应: " .. tostring(res.body))
    end
end

-- ====================================================================
-- 同步版命令（http.get / http.post）
-- 注意：同步请求会阻塞事件循环，仅适合低频快路径示范
-- ====================================================================

-- /hitokoto [类型] —— 同步 GET
jn.command.register("hitokoto", function(args, event)
    local ctx = { target = target_of(event) }
    local res, err = jn.http.get(build_hitokoto_url(args[1] or ""))
    if err then
        reply_to(ctx, "请求失败: " .. tostring(err))
        return true
    end
    handle_hitokoto(ctx, res)
    return true
end, {
    description = "一言金句（同步 http.get）",
    usage = "/hitokoto [a|b|c|d|e|f|g]",
})

-- /weather <城市> —— 同步 GET
jn.command.register("weather", function(args, event)
    local ctx = { target = target_of(event) }
    local city = args[1] or "北京"
    -- wttr.in 返回纯文本格式，?format=3 是精简版
    local res, err = jn.http.get("https://wttr.in/" .. city .. "?format=3")
    if err then
        reply_to(ctx, "天气查询失败: " .. tostring(err))
        return true
    end
    handle_weather(ctx, res)
    return true
end, {
    description = "查询天气（同步 http.get）",
    usage = "/weather <城市>",
})

-- /http post <文本> —— 同步 POST
jn.command.register("http post", function(args, event)
    local ctx = { target = target_of(event) }
    local payload = json.encode({ plugin = "http-tools", ts = os.time(), echo = args[1] or "hello" })
    local res, err = jn.http.post("https://httpbin.org/post", "application/json", payload)
    if err then
        reply_to(ctx, "POST 失败: " .. tostring(err))
        return true
    end
    handle_post(ctx, res)
    return true
end, {
    description = "演示 http.post（同步，httpbin 回显）",
    usage = "/http post <文本>",
})

-- ====================================================================
-- 异步版命令（http.get_async / http.post_async）
-- 立即返回 req_id，不阻塞事件循环；结果在 on_http_response 回调处理。
-- 调用时把回复目标等现场打包成 ctx，回调时原样带回。
-- ====================================================================

-- /hitokoto async [类型] —— 异步 GET
jn.command.register("hitokoto async", function(args, event)
    local ctx = { action = "hitokoto", target = target_of(event) }
    local rid = jn.http.get_async(build_hitokoto_url(args[1] or ""), ctx)
    if rid == 0 then
        reply_to(ctx, "异步请求提交失败")
    end
    return true
end, {
    description = "一言金句（异步 http.get_async）",
    usage = "/hitokoto async [a|b|c|d|e|f|g]",
})

-- /weather async <城市> —— 异步 GET
jn.command.register("weather async", function(args, event)
    local ctx = { action = "weather", target = target_of(event) }
    local city = args[1] or "北京"
    local rid = jn.http.get_async("https://wttr.in/" .. city .. "?format=3", ctx)
    if rid == 0 then
        reply_to(ctx, "天气查询提交失败")
    end
    return true
end, {
    description = "查询天气（异步 http.get_async）",
    usage = "/weather async <城市>",
})

-- /http post async <文本> —— 异步 POST（尾部 table 参数即 ctx）
jn.command.register("http post async", function(args, event)
    local ctx = { action = "post", target = target_of(event) }
    local payload = json.encode({ plugin = "http-tools", ts = os.time(), echo = args[1] or "hello" })
    local rid = jn.http.post_async("https://httpbin.org/post", "application/json", payload, ctx)
    if rid == 0 then
        reply_to(ctx, "POST 提交失败")
    end
    return true
end, {
    description = "演示 http.post_async（异步，httpbin 回显）",
    usage = "/http post async <文本>",
})

-- ====================================================================
-- 异步完成回调：on_http_response(req_id, ctx, result, err)
--   result = {status=number, body=string}；err 非 nil 表示失败
-- ====================================================================
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
        handle_hitokoto(ctx, result)
    elseif ctx.action == "weather" then
        handle_weather(ctx, result)
    elseif ctx.action == "post" then
        handle_post(ctx, result)
    end
end

log.info("http-tools 插件已加载")
