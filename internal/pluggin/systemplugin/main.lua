-- ====================================================================
-- JuanNiang-Neo 系统插件
-- ====================================================================
-- 提供 Agent / Provider / MCP / Tool / T2I / Sandbox / Session 的命令行管理。
-- 该插件随二进制内嵌分发，标记为 system: true，不允许删除或停用。
-- 用户可修改本文件以扩展命令，但 system 标志由 pluggin.yaml 控制。
-- ====================================================================

local jn = require("jn")

-- --------------------------------------------------------------------
-- 辅助函数：根据 event 回复消息
-- --------------------------------------------------------------------
local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

-- --------------------------------------------------------------------
-- 辅助函数：判断权限层级
-- --------------------------------------------------------------------
local function is_admin(user_id, event)
    if not event.admins then return false end
    local uid = tostring(user_id)
    for _, a in ipairs(event.admins) do
        if a == uid then return true end
    end
    return false
end

-- --------------------------------------------------------------------
-- 辅助函数：解析 on/off 参数
-- --------------------------------------------------------------------
local function parse_onoff(s)
    if s == "on" or s == "true" or s == "1" then return true end
    if s == "off" or s == "false" or s == "0" then return false end
    return nil
end

-- --------------------------------------------------------------------
-- 顶层命令: /system (纯分组节点，子命令自动生成帮助)
-- --------------------------------------------------------------------
jn.command.register("system", nil, {
    description = "系统管理命令分组",
    usage = "/system <子命令>",
})

-- --------------------------------------------------------------------
-- /system status —— 系统总览
-- --------------------------------------------------------------------
jn.command.register("system status", function(args, event)
    local lines = {"== JuanNiang-Neo 系统状态 =="}

    local providers = jn.agent.list_runtime_providers()
    if providers then
        lines[#lines+1] = string.format("已加载 Provider: %d", #providers)
        for _, p in ipairs(providers) do
            lines[#lines+1] = string.format("  - [%s] %s (%s)", p.type, p.name, p.model)
        end
    end

    local mcps = jn.agent.list_mcps()
    if mcps then
        lines[#lines+1] = string.format("已连接 MCP: %d", #mcps)
        for _, m in ipairs(mcps) do
            lines[#lines+1] = string.format("  - %s (active=%s)", m.name, tostring(m.active))
        end
    end

    local tools = jn.agent.list_tools()
    if tools then
        lines[#lines+1] = string.format("已注册 Tool: %d", #tools)
    end

    lines[#lines+1] = string.format("T2I 启用: %s", tostring(jn.t2i.is_active()))
    lines[#lines+1] = string.format("Sandbox 启用: %s", tostring(jn.sandbox.is_active()))

    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看 Agent / Provider / MCP / Tool / T2I / Sandbox 总体状态",
    usage = "/system status",
})

-- --------------------------------------------------------------------
-- /system provider —— 默认显示列表
-- --------------------------------------------------------------------
jn.command.register("system provider", function(args, event)
    local list, err = jn.agent.list_runtime_providers()
    if not list then
        reply(event, "获取 Provider 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 Provider 列表:"}
    for _, p in ipairs(list) do
        lines[#lines+1] = string.format("- id=%s  type=%s  model=%s  name=%s",
            p.id, p.type, p.model, p.name)
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: list, switch"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 LLM Provider（默认列出）",
    usage = "/system provider [list|switch <id>]",
})

-- --------------------------------------------------------------------
-- /system provider list|switch
-- --------------------------------------------------------------------
jn.command.register("system provider list", function(args, event)
    local list, err = jn.agent.list_runtime_providers()
    if not list then
        reply(event, "获取 Provider 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 Provider 列表:"}
    for _, p in ipairs(list) do
        lines[#lines+1] = string.format("- id=%s  type=%s  model=%s  name=%s",
            p.id, p.type, p.model, p.name)
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时已加载的 LLM Provider",
    usage = "/system provider list",
})

jn.command.register("system provider switch", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 Provider")
        return true
    end
    local id = args[1]
    if not id then
        reply(event, "用法: /system provider switch <provider_id>")
        return true
    end
    local ok, err = jn.agent.switch_provider(id)
    if ok then
        reply(event, "Provider 切换成功: " .. id)
    else
        reply(event, "Provider 切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "切换同类型 Provider (停用其它同类，激活指定 Provider)",
    usage = "/system provider switch <provider_id>",
})

-- --------------------------------------------------------------------
-- /system mcp —— 默认显示列表
-- --------------------------------------------------------------------
jn.command.register("system mcp", function(args, event)
    local list, err = jn.agent.list_mcps()
    if not list then
        reply(event, "获取 MCP 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 MCP 列表:"}
    for _, m in ipairs(list) do
        lines[#lines+1] = string.format("- id=%s  name=%s  active=%s",
            m.id, m.name, tostring(m.active))
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: list, toggle"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 MCP 服务器（默认列出）",
    usage = "/system mcp [list|toggle <id> <on|off>]",
})

-- --------------------------------------------------------------------
-- /system mcp list|toggle
-- --------------------------------------------------------------------
jn.command.register("system mcp list", function(args, event)
    local list, err = jn.agent.list_mcps()
    if not list then
        reply(event, "获取 MCP 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 MCP 列表:"}
    for _, m in ipairs(list) do
        lines[#lines+1] = string.format("- id=%s  name=%s  active=%s",
            m.id, m.name, tostring(m.active))
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时 MCP 服务器",
    usage = "/system mcp list",
})

jn.command.register("system mcp toggle", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 MCP")
        return true
    end
    local id = args[1]
    local onoff = args[2]
    if not id or not onoff then
        reply(event, "用法: /system mcp toggle <mcp_id> <on|off>")
        return true
    end
    local active = parse_onoff(onoff)
    if active == nil then
        reply(event, "无效的开关值，请用 on/off")
        return true
    end
    local ok, err = jn.agent.toggle_mcp(id, active)
    if ok then
        reply(event, string.format("MCP %s 已%s", id, active and "启用" or "停用"))
    else
        reply(event, "MCP 切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "启用或停用指定 MCP 服务器",
    usage = "/system mcp toggle <mcp_id> <on|off>",
})

-- --------------------------------------------------------------------
-- /system tool —— 默认显示列表
-- --------------------------------------------------------------------
jn.command.register("system tool", function(args, event)
    local list, err = jn.agent.list_tools()
    if not list then
        reply(event, "获取 Tool 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 Tool 列表:"}
    for _, t in ipairs(list) do
        local tag = t.builtin and "[builtin]" or "[custom]"
        local lr = t.long_running and " (long-running)" or ""
        lines[#lines+1] = string.format("- %s %s%s — %s",
            tag, t.name, lr, t.description or "")
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: list, toggle"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 Tool（默认列出）",
    usage = "/system tool [list|toggle <name> <on|off>]",
})

-- --------------------------------------------------------------------
-- /system tool list|toggle
-- --------------------------------------------------------------------
jn.command.register("system tool list", function(args, event)
    local list, err = jn.agent.list_tools()
    if not list then
        reply(event, "获取 Tool 失败: " .. tostring(err))
        return true
    end
    local lines = {"运行时 Tool 列表:"}
    for _, t in ipairs(list) do
        local tag = t.builtin and "[builtin]" or "[custom]"
        local lr = t.long_running and " (long-running)" or ""
        lines[#lines+1] = string.format("- %s %s%s — %s",
            tag, t.name, lr, t.description or "")
    end
    if #list == 0 then
        lines[#lines+1] = "(空)"
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时 Tool",
    usage = "/system tool list",
})

jn.command.register("system tool toggle", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 Tool")
        return true
    end
    local name = args[1]
    local onoff = args[2]
    if not name or not onoff then
        reply(event, "用法: /system tool toggle <tool_name> <on|off>")
        return true
    end
    local active = parse_onoff(onoff)
    if active == nil then
        reply(event, "无效的开关值，请用 on/off")
        return true
    end
    local ok, err = jn.agent.toggle_tool(name, active)
    if ok then
        reply(event, string.format("Tool %s 已%s", name, active and "启用" or "停用"))
    else
        reply(event, "Tool 切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "启用或停用指定 Tool",
    usage = "/system tool toggle <tool_name> <on|off>",
})

-- --------------------------------------------------------------------
-- /system memory compact
-- --------------------------------------------------------------------
jn.command.register("system memory compact", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可执行记忆 Compact")
        return true
    end
    local result, err = jn.agent.compact_memory()
    if result then
        reply(event, result)
    else
        reply(event, "Compact 失败: " .. tostring(err))
    end
    return true
end, {
    description = "压缩当前 ChatArea 的短期记忆并写入长期记忆",
    usage = "/system memory compact",
})

-- --------------------------------------------------------------------
-- /system t2i —— 默认显示状态
-- --------------------------------------------------------------------
jn.command.register("system t2i", function(args, event)
    local active = jn.t2i.is_active()
    local lines = {"T2I 状态:"}
    lines[#lines+1] = "  启用: " .. tostring(active)
    if active then
        local cfg = jn.t2i.get_config()
        if cfg then
            lines[#lines+1] = "  base_url: " .. tostring(cfg.base_url or "")
            lines[#lines+1] = "  timeout: " .. tostring(cfg.timeout or 0)
        end
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: status, toggle"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 T2I 服务（默认显示状态）",
    usage = "/system t2i [status|toggle <on|off>]",
})

-- --------------------------------------------------------------------
-- /system t2i status|toggle
-- --------------------------------------------------------------------
jn.command.register("system t2i status", function(args, event)
    local active = jn.t2i.is_active()
    local lines = {"T2I 状态:"}
    lines[#lines+1] = "  启用: " .. tostring(active)
    if active then
        local cfg = jn.t2i.get_config()
        if cfg then
            lines[#lines+1] = "  base_url: " .. tostring(cfg.base_url or "")
            lines[#lines+1] = "  timeout: " .. tostring(cfg.timeout or 0)
        end
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看 T2I 服务状态",
    usage = "/system t2i status",
})

jn.command.register("system t2i toggle", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 T2I")
        return true
    end
    local onoff = args[1]
    if not onoff then
        reply(event, "用法: /system t2i toggle <on|off>")
        return true
    end
    local active = parse_onoff(onoff)
    if active == nil then
        reply(event, "无效的开关值，请用 on/off")
        return true
    end
    local ok, err = jn.t2i.toggle(active)
    if ok then
        reply(event, "T2I 已" .. (active and "启用" or "停用"))
    else
        reply(event, "T2I 切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "启用或停用 T2I 服务",
    usage = "/system t2i toggle <on|off>",
})

-- --------------------------------------------------------------------
-- /system sandbox —— 默认显示状态
-- --------------------------------------------------------------------
jn.command.register("system sandbox", function(args, event)
    local active = jn.sandbox.is_active()
    local lines = {"Sandbox 状态:"}
    lines[#lines+1] = "  启用: " .. tostring(active)
    if active then
        local cfg = jn.sandbox.get_config()
        if cfg then
            lines[#lines+1] = "  base_url: " .. tostring(cfg.base_url or "")
            lines[#lines+1] = "  timeout: " .. tostring(cfg.timeout or 0)
        end
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: status, toggle"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 Sandbox 服务（默认显示状态）",
    usage = "/system sandbox [status|toggle <on|off>]",
})

-- --------------------------------------------------------------------
-- /system sandbox status|toggle
-- --------------------------------------------------------------------
jn.command.register("system sandbox status", function(args, event)
    local active = jn.sandbox.is_active()
    local lines = {"Sandbox 状态:"}
    lines[#lines+1] = "  启用: " .. tostring(active)
    if active then
        local cfg = jn.sandbox.get_config()
        if cfg then
            lines[#lines+1] = "  base_url: " .. tostring(cfg.base_url or "")
            lines[#lines+1] = "  timeout: " .. tostring(cfg.timeout or 0)
        end
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看 Sandbox 服务状态",
    usage = "/system sandbox status",
})

jn.command.register("system sandbox toggle", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 Sandbox")
        return true
    end
    local onoff = args[1]
    if not onoff then
        reply(event, "用法: /system sandbox toggle <on|off>")
        return true
    end
    local active = parse_onoff(onoff)
    if active == nil then
        reply(event, "无效的开关值，请用 on/off")
        return true
    end
    local ok, err = jn.sandbox.toggle(active)
    if ok then
        reply(event, "Sandbox 已" .. (active and "启用" or "停用"))
    else
        reply(event, "Sandbox 切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "启用或停用 Sandbox 服务",
    usage = "/system sandbox toggle <on|off>",
})

-- --------------------------------------------------------------------
-- /system session —— 默认显示列表
-- --------------------------------------------------------------------
jn.command.register("system session", function(args, event)
    local sessions, err = jn.agent.get_sessions()
    if not sessions then
        reply(event, "获取 Session 失败: " .. tostring(err))
        return true
    end
    local lines = {"Session 列表:"}
    for _, s in ipairs(sessions) do
        lines[#lines+1] = string.format("- id=%s  area=%s  model=%s  tokens=%d",
            s.id, s.chat_area_id, s.model or "?", s.token_usage or 0)
    end
    if #sessions == 0 then
        lines[#lines+1] = "(空)"
    end
    lines[#lines+1] = ""
    lines[#lines+1] = "子命令: list, info"
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "管理 Session（默认列出）",
    usage = "/system session [list|info]",
})

-- --------------------------------------------------------------------
-- /system session list|info
-- --------------------------------------------------------------------
jn.command.register("system session list", function(args, event)
    local sessions, err = jn.agent.get_sessions()
    if not sessions then
        reply(event, "获取 Session 失败: " .. tostring(err))
        return true
    end
    local lines = {"Session 列表:"}
    for _, s in ipairs(sessions) do
        lines[#lines+1] = string.format("- id=%s  area=%s  model=%s  tokens=%d",
            s.id, s.chat_area_id, s.model or "?", s.token_usage or 0)
    end
    if #sessions == 0 then
        lines[#lines+1] = "(空)"
    end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出所有会话",
    usage = "/system session list",
})

jn.command.register("system session info", function(args, event)
    local area = jn.agent.get_current_chat_area()
    local lines = {"当前会话信息:"}
    lines[#lines+1] = "  chat_area_id: " .. tostring(area.chat_area_id)
    lines[#lines+1] = "  message_type: " .. tostring(area.message_type)
    lines[#lines+1] = "  user_id: " .. tostring(area.user_id)
    lines[#lines+1] = "  group_id: " .. tostring(area.group_id)
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "查看当前会话的 ChatArea 上下文",
    usage = "/system session info",
})

-- ====================================================================
-- 兼容旧的 on_message：捕获 /system 不带子命令的情况
-- 命令系统已经处理了 "/system" 开头的消息，此处仅作日志
-- 注：on_message 仅返回 skip_reply（不消费消息、不修改事件）
-- ====================================================================
function on_message(event)
    -- 所有 /system* 命令已由 jn.command 注册表处理
    -- 此处仅用于不匹配任何命令时的兜底
    return false, false  -- consumed, skip_reply
end

log.info("system 插件已加载")
