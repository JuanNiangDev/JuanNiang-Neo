-- ====================================================================
-- 戳一戳回复插件
-- 监听 on_notice(notify, poke) 事件，回复消息。
-- 支持判断戳的发送者和目标，避免戳自己时重复回复。
-- ====================================================================

local jn = require("jn")

-- 可自定义回复语列表
local REPLIES = {
    "别戳了别戳了！(〃＞＿＜;〃)",
    "戳我干嘛！",
    "再戳我要生气了！(#`Д´)ﾉ",
    "戳一戳已收到，已读不回。",
    "你戳我一下，我回你一下 🤏",
}

---@param event jn.Event
function on_notice(event)
    -- 戳一戳事件：notice_type = "notify", sub_type = "poke"
    if event.notice_type ~= "notify" or event.sub_type ~= "poke" then
        return
    end

    -- target_id 是被戳的人，user_id 是戳的人
    local from_qq = event.user_id       -- 发起戳一戳的人
    local to_qq = event.target_id       -- 被戳的人（可能是机器人自己）
    local group_id = event.group_id

    if group_id == 0 then
        return  -- 私聊戳一戳暂不处理（私聊戳一戳通常没有 group_id）
    end

    -- 随机选一条回复
    local reply_text = REPLIES[math.random(#REPLIES)]

    -- 用 CQ 码 @ 发起戳一戳的人
    local msg = string.format("[CQ:at,qq=%d] %s", from_qq, reply_text)
    jn.onebot11.send_group_msg(group_id, msg)

    jn.log.info(string.format("[poke-reply] %d 戳了 %d 在群 %d", from_qq, to_qq, group_id))
end
