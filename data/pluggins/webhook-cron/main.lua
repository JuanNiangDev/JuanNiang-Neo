-- ====================================================================
-- webhook-cron：外部集成示例
-- ====================================================================
-- 覆盖内容：
--   1. on_webhook —— 外部 HTTP POST 触发（定向模式 /webhook/webhook-cron）
--   2. on_cronjob —— CronJob 定时触发（payload 指定目标群与消息）
-- ====================================================================

local jn = require("jn")

-- --------------------------------------------------------------------
-- on_webhook —— 外部 HTTP 触发
-- --------------------------------------------------------------------
-- 定向模式：POST /webhook/webhook-cron（带 WebhookToken）→ 只有本插件收到
-- 广播模式：POST /webhook  → 广播给所有有 webhook 权限的插件
--
-- 返回值：(consumed, reply)
--   reply 在定向模式下会作为 HTTP 响应的 metadata 返回给调用方
function on_webhook(event)
    local p = event.webhook and event.webhook.payload or {}

    log.info("收到 webhook", "path", event.webhook and event.webhook.path or "",
        "method", event.webhook and event.webhook.method or "")

    -- GitHub 风格推送通知示例
    if p.action == "opened" or p.action == "push" then
        local title = p.title or p.ref or "更新"
        local who = p.sender and p.sender.login or "unknown"
        jn.onebot11.send_group_msg(1076723599,
            string.format("🔔 仓库有%s：%s（by %s）", p.action, title, who))
        return true, "notified"
    end

    -- 通用消息广播
    if p.message then
        local group_id = tonumber(p.group_id or 1076723599)
        jn.onebot11.send_group_msg(group_id, tostring(p.message))
        return true, "broadcasted to " .. group_id
    end

    return false, "unhandled payload"
end

-- --------------------------------------------------------------------
-- on_cronjob —— CronJob 定时触发
-- --------------------------------------------------------------------
-- 触发方式：在 Web「CronJob」页新建任务，message_type 选 cronjob，
-- 选中本插件，payload 配置如下：
--   {"target_group": 1076723599, "message": "整点报时：现在 ..."}
-- 或
--   {"target_qq": 123456789, "message": "每日提醒"}
function on_cronjob(event)
    local p = event.payload or {}
    local msg = p.message
    if not msg then
        log.warn("on_cronjob 缺少 payload.message")
        return
    end
    if p.target_qq then
        jn.onebot11.send_private_msg(tonumber(p.target_qq), msg)
    elseif p.target_group then
        jn.onebot11.send_group_msg(tonumber(p.target_group), msg)
    else
        jn.onebot11.send_group_msg(1076723599, msg)
    end
    log.info("cronjob 已发送", "msg", msg)
end

log.info("webhook-cron 插件已加载")
