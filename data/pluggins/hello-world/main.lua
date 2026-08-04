-- ====================================================================
-- hello-world：JuanNiang-Neo 插件入门示例
-- ====================================================================
-- 覆盖内容：
--   1. jn.command.register —— 注册 /hello 命令
--   2. on_message          —— 纯事件监听（ping/pong）
--   3. log / json          —— 日志与 JSON 编解码
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

-- --------------------------------------------------------------------
-- /hello [name] —— 简单命令
-- --------------------------------------------------------------------
jn.command.register("hello", function(args, event)
    local name = args[1] or "朋友"
    local hour = tonumber(os.date("%H")) or 12
    local greeting
    if hour < 6 then
        greeting = "凌晨好"
    elseif hour < 12 then
        greeting = "早上好"
    elseif hour < 18 then
        greeting = "下午好"
    else
        greeting = "晚上好"
    end
    return true, greeting .. "，" .. name .. "！我是卷娘，欢迎来找我玩~"
end, {
    description = "打招呼",
    usage = "/hello [名字]",
})

-- --------------------------------------------------------------------
-- on_message —— 监听消息事件
-- --------------------------------------------------------------------
-- 返回值: (consumed, modified_event, skip_reply)
--   consumed=true  → 跳过 Agent 与后续插件
--   modified_event → 可修改后的事件表（nil/原表=不修改）
--   skip_reply     → true 跳过回复策略评估
function on_message(event)
    local msg = event.raw_message or ""

    -- ping/pong：纯事件监听，不阻塞 Agent
    if msg == "ping" then
        log.info("收到 ping，来自 " .. tostring(event.user_id))
        reply(event, "pong 🏓")
        return false, event, false
    end

    -- 演示 json 编解码：/json {"k":"v"} 回显解析结果
    if msg:sub(1, 6) == "/json " then
        local ok, decoded = pcall(json.decode, msg:sub(7))
        if ok and decoded then
            local encoded = json.encode(decoded)
            reply(event, "解析成功: " .. tostring(encoded))
        else
            reply(event, "JSON 解析失败，请检查格式")
        end
        return true, event, false
    end

    return false, event, false
end

log.info("hello-world 插件已加载")
