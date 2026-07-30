-- ====================================================================
-- 富文本图片示例插件
-- 注册 /rich 命令，使用消息段发送带文字的图片消息。
-- 图片 test.png 需放在插件目录下（data/pluggins/rich-demo/test.png）。
-- ====================================================================

local jn = require("jn")

--- /rich 命令：发送富文本示例
---@param args string[]
---@param event jn.Event
jn.command.register("rich", function(args, event)
    local target_id = event.group_id ~= 0 and event.group_id or event.user_id
    local is_group = event.group_id ~= 0

    -- 组装消息段：文本 + 图片 + 表情
    local segments = {
        { type = "text", data = { text = "这是一张示例图片 👇\n" } },
        {
            type = "image",
            data = { file = "test.png" }   -- 插件目录下的 test.png，自动转 base64
        },
        { type = "text", data = { text = "\n👆 以上是插件目录中的 test.png" } },
        { type = "face", data = { id = "66" } },  -- CQ 表情：[爱心]
    }

    if is_group then
        jn.log.info(string.format("[rich-demo] 群 %d 触发 /rich", event.group_id))
    else
        jn.log.info(string.format("[rich-demo] 私聊 %d 触发 /rich", event.user_id))
    end

    -- 返回 reply 字符串会自动发送，但因为我们是富文本，需要手动发
    -- 同时返回 consumed=true 防止走 Agent
    -- 注意：reply 返回空，手动调用 send
    if is_group then
            jn.onebot11.send_group_msg(target_id, segments)
        else
            jn.onebot11.send_private_msg(target_id, segments)
        end
    return true, ""
end, {
    description = "发送带图片的富文本消息示例",
    usage = "/rich"
})
