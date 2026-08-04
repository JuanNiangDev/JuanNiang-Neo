-- ====================================================================
-- agent-ops：Agent 运行时管理示例（多级命令）
-- ====================================================================
-- 覆盖内容：
--   1. agent 配置查询：providers / tools / skills / sessions / prompts / mcps / plugins
--   2. agent 运行时管理：switch_provider（仅 admin）、compact_memory
--   3. 多级命令：/agent providers 等
-- ====================================================================

local jn = require("jn")

local function reply(event, text)
    if event.message_type == "group" then
        jn.onebot11.send_group_msg(event.group_id, text)
    else
        jn.onebot11.send_private_msg(event.user_id, text)
    end
end

local function is_admin(user_id, event)
    if not event.admins then return false end
    local uid = tostring(user_id)
    for _, a in ipairs(event.admins) do
        if a == uid then return true end
    end
    return false
end

-- /agent 顶层分组
jn.command.register("agent", nil, {
    description = "Agent 管理命令分组",
    usage = "/agent <子命令>",
})

-- --------------------------------------------------------------------
-- /agent providers —— 运行时 Provider 列表
-- --------------------------------------------------------------------
jn.command.register("agent providers", function(args, event)
    local list, err = jn.agent.list_runtime_providers()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "运行时 Provider:" }
    for _, p in ipairs(list) do
        lines[#lines+1] = string.format("- [%s] id=%s %s (%s)",
            p.type, p.id, p.name, p.model)
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时已加载的 LLM Provider",
    usage = "/agent providers",
})

-- --------------------------------------------------------------------
-- /agent provider switch <id> —— 切换 Provider（仅 admin）
-- --------------------------------------------------------------------
jn.command.register("agent provider switch", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可切换 Provider")
        return true
    end
    local id = args[1]
    if not id then
        reply(event, "用法: /agent provider switch <provider_id>")
        return true
    end
    local ok, err = jn.agent.switch_provider(id)
    if ok then
        reply(event, "Provider 切换成功: " .. id)
    else
        reply(event, "切换失败: " .. tostring(err))
    end
    return true
end, {
    description = "切换主 Provider（仅管理员）",
    usage = "/agent provider switch <id>",
})

-- --------------------------------------------------------------------
-- /agent tools / /agent skills / /agent sessions / /agent prompts
-- /agent mcps / /agent plugins —— 各类配置查询
-- --------------------------------------------------------------------
jn.command.register("agent tools", function(args, event)
    local list, err = jn.agent.list_tools()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "运行时 Tool（前 15 个）:" }
    local n = math.min(#list, 15)
    for i = 1, n do
        lines[#lines+1] = string.format("- %s%s", list[i].name,
            list[i].active and "" or "（停用）")
    end
    if #list > n then lines[#lines+1] = "..." end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时已注册的 Tool",
    usage = "/agent tools",
})

jn.command.register("agent skills", function(args, event)
    local list, err = jn.agent.get_skills()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "已配置 Skill:" }
    for _, s in ipairs(list) do
        lines[#lines+1] = string.format("- %s%s", s.name,
            s.is_active and "" or "（停用）")
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出已配置的 Skill",
    usage = "/agent skills",
})

jn.command.register("agent sessions", function(args, event)
    local list, err = jn.agent.get_sessions()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "Session 列表:" }
    for _, s in ipairs(list) do
        lines[#lines+1] = string.format("- %s 模型=%s token=%s",
            s.id, s.model, tostring(s.token_usage))
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出 Session",
    usage = "/agent sessions",
})

jn.command.register("agent prompts", function(args, event)
    local list, err = jn.agent.get_prompts()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "Prompt 模板:" }
    for _, p in ipairs(list) do
        lines[#lines+1] = string.format("- %s [%s]%s",
            p.name, p.type, p.is_active and "" or "（停用）")
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出 Prompt 模板",
    usage = "/agent prompts",
})

jn.command.register("agent mcps", function(args, event)
    local list, err = jn.agent.list_mcps()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "运行时 MCP:" }
    for _, m in ipairs(list) do
        lines[#lines+1] = string.format("- %s%s (active=%s)",
            m.name, m.url and (" " .. m.url) or "", tostring(m.active))
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出运行时 MCP 服务器",
    usage = "/agent mcps",
})

jn.command.register("agent plugins", function(args, event)
    local list, err = jn.agent.get_plugins()
    if not list then
        reply(event, "获取失败: " .. tostring(err))
        return true
    end
    local lines = { "已安装插件:" }
    for _, p in ipairs(list) do
        lines[#lines+1] = string.format("- %s v%s%s", p.name, p.version,
            p.is_active and "" or "（停用）")
    end
    if #list == 0 then lines[#lines+1] = "（空）" end
    reply(event, table.concat(lines, "\n"))
    return true
end, {
    description = "列出已安装插件",
    usage = "/agent plugins",
})

-- --------------------------------------------------------------------
-- /agent memory compact —— 压缩当前会话短期记忆
-- --------------------------------------------------------------------
jn.command.register("agent memory compact", function(args, event)
    if not is_admin(event.user_id, event) then
        reply(event, "权限不足：仅 admins 可压缩记忆")
        return true
    end
    local out, err = jn.agent.compact_memory()
    if not out then
        reply(event, "压缩失败: " .. tostring(err))
        return true
    end
    reply(event, out)
    return true
end, {
    description = "压缩当前会话短期记忆（仅管理员）",
    usage = "/agent memory compact",
})

log.info("agent-ops 插件已加载")
