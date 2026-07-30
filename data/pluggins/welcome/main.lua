-- ====================================================================
-- 入群欢迎插件
-- 监听 on_notice(group_increase) 事件，向群内发送欢迎语。
-- 支持两种入群方式：管理员同意 (approve) 和邀请入群 (invite)。
-- ====================================================================

local jn = require("jn")

---@param event jn.Event
function on_notice(event)
    if event.notice_type ~= "group_increase" then return end

    local user_id = event.user_id
    local group_id = event.group_id

    if event.sub_type == "invite" then
        local inviter = event.operator_id
        jn.onebot11.send_group_msg(group_id,
            string.format("🎉 欢迎新同学 [CQ:at,qq=%d]！由群友 [CQ:at,qq=%d] 邀请入群～", user_id, inviter))
        jn.log.info(string.format("[welcome] %d 被 %d 邀请加入群 %d", user_id, inviter, group_id))
    else
        jn.onebot11.send_group_msg(group_id,
            string.format("👋 欢迎新同学 [CQ:at,qq=%d] 加入本群！(´▽`ʃ♡ƪ)", user_id))
        jn.log.info(string.format("[welcome] %d 加入了群 %d", user_id, group_id))
    end
end
