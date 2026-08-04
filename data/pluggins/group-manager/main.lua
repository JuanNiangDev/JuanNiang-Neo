-- ====================================================================
-- group-manager：群管理 + 通知/请求事件示例
-- ====================================================================
-- 覆盖内容：
--   1. onebot11 群管理 API（禁言/踢人/设名片）—— 高危操作仅 admin 可用
--   2. on_notice  —— 入群欢迎
--   3. on_request —— 好友申请自动处理（暗号）
-- ====================================================================
-- ⚠ 安全说明：event.admins 透传 OB_ADMINS 配置的管理员 QQ 列表。
--   高危操作（禁言/踢人）必须校验 is_admin，防止非管理员滥用。
-- ====================================================================

local jn = require("jn")

-- --------------------------------------------------------------------
-- 辅助：判断是否为管理员
-- --------------------------------------------------------------------
local function is_admin(user_id, event)
    if not event.admins then return false end
    local uid = tostring(user_id)
    for _, a in ipairs(event.admins) do
        if a == uid then return true end
    end
    return false
end

local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- --------------------------------------------------------------------
-- /ban <qq> <秒数> —— 禁言（仅 admin）
-- --------------------------------------------------------------------
jn.command.register("ban", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可执行禁言")
        return true
    end
    local qq = tonumber(args[1])
    local duration = tonumber(args[2]) or 60
    if not qq then
        reply(event, "用法: /ban <QQ号> <秒数>")
        return true
    end
    local ok, err = jn.onebot11.ban_group_member(event.group_id, qq, duration)
    if ok then
        reply(event, string.format("已禁言 %d 号 %d 秒", qq, duration))
    else
        reply(event, "禁言失败: " .. tostring(err))
    end
    return true
end, {
    description = "禁言群成员（仅管理员）",
    usage = "/ban <QQ号> <秒数>",
})

-- --------------------------------------------------------------------
-- /kick <qq> —— 踢人（仅 admin）
-- --------------------------------------------------------------------
jn.command.register("kick", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可执行踢人")
        return true
    end
    local qq = tonumber(args[1])
    if not qq then
        reply(event, "用法: /kick <QQ号>")
        return true
    end
    local ok, err = jn.onebot11.kick_group_member(event.group_id, qq, false)
    if ok then
        reply(event, "已踢出 " .. qq)
    else
        reply(event, "踢人失败: " .. tostring(err))
    end
    return true
end, {
    description = "踢出群成员（仅管理员）",
    usage = "/kick <QQ号>",
})

-- --------------------------------------------------------------------
-- /card <qq> <名片> —— 设置群名片（仅 admin）
-- --------------------------------------------------------------------
jn.command.register("card", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可设置群名片")
        return true
    end
    local qq = tonumber(args[1])
    local card = args[2]
    if not qq or not card then
        reply(event, "用法: /card <QQ号> <新名片>")
        return true
    end
    local ok, err = jn.onebot11.set_group_card(event.group_id, qq, card)
    if ok then
        reply(event, "已将 " .. qq .. " 的名片改为「" .. card .. "」")
    else
        reply(event, "设置失败: " .. tostring(err))
    end
    return true
end, {
    description = "设置群名片（仅管理员）",
    usage = "/card <QQ号> <新名片>",
})

-- --------------------------------------------------------------------
-- on_notice —— 通知事件（入群欢迎）
-- --------------------------------------------------------------------
function on_notice(event)
    if event.notice_type == "group_increase" then
        jn.onebot11.send_group_msg(event.group_id,
            "[CQ:at,qq=" .. event.user_id .. "] 欢迎加入本群！请查看群公告~")
    end
    -- 戳一戳自动回应
    if event.notice_type == "notify" and event.sub_type == "poke" then
        jn.onebot11.send_group_msg(event.group_id,
            "[CQ:at,qq=" .. event.user_id .. "] 别戳啦，再戳要散架了！")
    end
end

-- --------------------------------------------------------------------
-- on_request —— 请求事件（好友申请暗号自动同意）
-- --------------------------------------------------------------------
function on_request(event)
    if event.request_type == "friend" then
        if event.comment and event.comment:find("卷娘") then
            jn.onebot11.handle_friend_request(event.flag, true, "暗号正确，欢迎")
            log.info("已自动同意好友申请", event.user_id)
        else
            jn.onebot11.handle_friend_request(event.flag, false, "验证信息不符")
        end
    end
end

log.info("group-manager 插件已加载")
