-- Ping 插件: 检测 /ping 命令并回复 pong

function on_message(event)
    local raw = event.raw_message or ""
    if raw == "/ping" then
        local msg_type = event.message_type
        if msg_type == "group" then
            onebot11.send_group_msg(event.group_id, "pong!")
        elseif msg_type == "private" then
            onebot11.send_private_msg(event.user_id, "pong!")
        end
        return true, event
    end
    return false, event
end
