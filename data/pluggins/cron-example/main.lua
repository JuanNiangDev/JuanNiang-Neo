-- ====================================================================
-- Cron 示例插件
-- ====================================================================
-- 当 CronJob 触发此插件时，on_timer_call(event) 被调用。
-- event.payload 即 CronJob 配置的 Payload JSON 对象。
--
-- 示例 Payload:
--   {
--     "target_qq": 123456789,
--     "message": "⏰ 定时提醒：该喝水啦！",
--     "message_type": "private"
--   }
--
-- 字段说明:
--   target_qq    (number, 必填) 目标 QQ 号
--   message      (string, 必填) 发送的消息文本
--   message_type (string, 可选) "private"(默认) 或 "group"
--   group_id     (number, 可选) message_type=group 时的群号
-- ====================================================================

local jn = require("jn")

-- 定时任务回调
---@param event jn.Event
function on_timer_call(event)
    local payload = event.payload or {}
    local target_qq = payload.target_qq
    local message = payload.message
    local msg_type = payload.message_type or "private"
    local group_id = payload.group_id

    -- 参数校验
    if not message then
        jn.log.error("[cron-example] Payload 缺少必填字段 message")
        return
    end

    if msg_type == "group" then
        if not group_id then
            jn.log.error("[cron-example] message_type=group 需要提供 group_id")
            return
        end
        jn.onebot11.send_group_msg(group_id, message)
        jn.log.info(string.format("[cron-example] 已向群 %d 发送定时消息", group_id))
    else
        if not target_qq then
            jn.log.error("[cron-example] message_type=private 需要提供 target_qq")
            return
        end
        jn.onebot11.send_private_msg(target_qq, message)
        jn.log.info(string.format("[cron-example] 已向 QQ %d 发送定时消息", target_qq))
    end
end
