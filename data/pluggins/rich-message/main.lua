-- ====================================================================
-- rich-message：富文本消息与 onebot11 查询示例
-- ====================================================================
-- 覆盖内容：
--   1. 消息段数组（text / at / image / face）—— image 支持三种来源
--   2. onebot11 群信息查询（get_group_info / get_group_member_list）
--   3. send_group_sticker —— 发送图床表情（stk:// 短 UUID）
-- ====================================================================

local jn = require("jn")

local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- --------------------------------------------------------------------
-- /send card —— 富文本消息段
-- --------------------------------------------------------------------
-- 图片 file 支持三种来源：
--   相对路径 img/dot.png  → 从插件目录读取并自动转 base64
--   https://...           → 直接透传 URL
--   imgs://<图床图片ID>    → 图床图片引用（发送层自动转 base64）
-- --------------------------------------------------------------------
jn.command.register("send card", function(args, event)
    local target
    if event.message_type == "group" then
        target = event.group_id
    else
        target = event.user_id
    end

    local segments = {
        { type = "text", data = { text = "这是一条富文本消息，包含多种消息段：\n" } },
        { type = "text", data = { text = "1) @ 某人：" } },
        { type = "at", data = { qq = tostring(event.user_id) } },
        { type = "text", data = { text = "\n2) 表情：" } },
        { type = "face", data = { id = "66" } },
        { type = "text", data = { text = "\n3) 插件本地图片（自动转 base64）：" } },
        { type = "image", data = { file = "img/dot.png" } },
        { type = "text", data = { text = "\n4) 网络图片 URL：" } },
        { type = "image", data = { file = "https://httpbin.org/image/png" } },
        { type = "text", data = { text = "\n5) 图床图片（需先在图床传图，替换下面的 ID）：" } },
        { type = "image", data = { file = "imgs://REPLACE_WITH_IMAGE_ID" } },
    }

    local ok, err = jn.onebot11.send_group_msg(target, segments)
    if not ok then
        reply(event, "发送失败: " .. tostring(err))
    end
    return true
end, {
    description = "发送一条富文本消息（演示消息段）",
    usage = "/send card",
})

-- --------------------------------------------------------------------
-- /group info —— 查询群信息
-- /group members —— 查询群成员列表
-- --------------------------------------------------------------------
jn.command.register("group info", function(args, event)
    if event.message_type ~= "group" then
        reply(event, "请在群聊中使用")
        return true
    end
    local info, err = jn.onebot11.get_group_info(event.group_id)
    if not info then
        reply(event, "查询失败: " .. tostring(err))
        return true
    end
    reply(event, string.format(
        "群名: %s\n群号: %d\n成员数: %d / %d",
        info.group_name, info.group_id, info.member_count, info.max_member_count))
    return true
end, {
    description = "查看当前群信息",
    usage = "/group info",
})

jn.command.register("group members", function(args, event)
    if event.message_type ~= "group" then
        reply(event, "请在群聊中使用")
        return true
    end
    local members, err = jn.onebot11.get_group_member_list(event.group_id)
    if not members then
        reply(event, "查询失败: " .. tostring(err))
        return true
    end
    local lines = { "群成员列表（前 10 个）:" }
    local count = math.min(#members, 10)
    for i = 1, count do
        local m = members[i]
        lines[#lines+1] = string.format("- %s (%d) [%s]", m.card or m.nickname, m.user_id, m.role)
    end
    if #members > count then
        lines[#lines+1] = "..." .. tostring(#members - count) .. " 人未显示"
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看当前群成员列表",
    usage = "/group members",
})

-- --------------------------------------------------------------------
-- /sticker <表情ID> —— 发送图床表情（stk:// 短 UUID）
-- --------------------------------------------------------------------
-- 表情 ID 在 Web「表情包库」页面查看；发送时插件只接触短 UUID，
-- 底层由发送层自动映射到图床图片并转 base64（subType=1）。
jn.command.register("sticker", function(args, event)
    local stickerID = args[1]
    if not stickerID then
        reply(event, "用法: /sticker <表情ID>（在 Web 表情包库查看）")
        return true
    end
    local ok, err
    if event.message_type == "group" then
        ok, err = jn.onebot11.send_group_sticker(event.group_id, stickerID)
    else
        ok, err = jn.onebot11.send_private_sticker(event.user_id, stickerID)
    end
    if not ok then
        reply(event, "发送表情失败: " .. tostring(err))
    end
    return true
end, {
    description = "发送表情包库中的表情",
    usage = "/sticker <表情ID>",
})

log.info("rich-message 插件已加载")
