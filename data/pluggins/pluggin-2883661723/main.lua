-- Ping 插件示例：使用 JuanNiang-Neo Lua SDK 注册多级命令
-- 演示 jn.command.register + jn.onebot11.send_group_msg / send_private_msg 的标准用法

local jn = require("jn")

-- --------------------------------------------------------------------
-- /ping 顶层命令
-- --------------------------------------------------------------------
jn.command.register("ping", function(args, event)
    local reply_text = "pong!"
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, reply_text)
    else
        jn.onebot11.send_private_msg(event.user_id, reply_text)
    end
    return true
end, {
    description = "Ping 测试命令，回复 pong",
    usage = "/ping",
})

-- --------------------------------------------------------------------
-- 兼容旧 on_message 入口：命令系统未命中时走这里
-- （因为 /ping 已注册到命令表，此处实际不会被调用；
--   保留是为了演示 SDK 之外的常规消息处理路径。）
-- --------------------------------------------------------------------
function on_message(event)
    local raw = event.raw_message or ""
    if raw == "/ping" then
        if event.message_type == "group" then
            jn.onebot11.send_group_msg(event.group_id, "pong!")
        else
            jn.onebot11.send_private_msg(event.user_id, "pong!")
        end
        return true, event
    end
    return false, event
end

log.info("ping 插件已加载（SDK 模式）")
