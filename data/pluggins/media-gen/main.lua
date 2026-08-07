-- ====================================================================
-- media-gen：T2I 文生图 + Sandbox 沙箱执行示例（同步版 + 异步版双示范）
-- ====================================================================
-- 覆盖内容：
--   同步版：t2i.generate_url / sandbox.exec_python / exec_shell —— 会阻塞事件循环
--   异步版：t2i.generate_url_async / exec_python_async / exec_shell_async
--           —— 渲染/执行可能耗时（秒级~几十秒），推荐异步（不阻塞，
--              完成回调 on_t2i_response / on_sandbox_response，现场用 ctx 保存）
--   sandbox.create 保留同步（只返回 ID，快）；/sb status|del 同步（管理类快操作）
-- 每个功能都提供 sync / async 两个命令，业务处理逻辑共享。
-- ====================================================================
-- ⚠ T2I / Sandbox 未启用时相关 API 返回 (nil, "XX 服务未启用")
-- ====================================================================

local jn = require("jn")

local function target_of(event)
    if event.message_type == "group" then
        return { kind = "group", id = event.group_id }
    end
    return { kind = "private", id = event.user_id }
end

local function reply_to(ctx, text)
    if not ctx or not ctx.target then return end
    if ctx.target.kind == "group" then
        jn.onebot11.send_group_msg(ctx.target.id, text)
    else
        jn.onebot11.send_private_msg(ctx.target.id, text)
    end
end

-- ====================================================================
-- 业务处理（同步 / 异步共用）
-- ====================================================================

local function build_poster_html(text)
    return string.format([[
      <div style="font-family:'Microsoft YaHei',sans-serif;width:480px;padding:40px;
        background:linear-gradient(135deg,#667eea,#764ba2);color:#fff;text-align:center;
        border-radius:16px;">
        <h1 style="margin:0;font-size:38px;">🖼️ %s</h1>
        <p style="opacity:.85;margin:12px 0 0;">—— 来自 media-gen 插件</p>
      </div>
    ]], text)
end

-- 发送图片（image URL → 富文本消息段）
local function send_image(ctx, url)
    local msg = {
        { type = "text", data = { text = "给你做了一张图：" } },
        { type = "image", data = { file = url } },
    }
    if ctx.target.kind == "group" then
        jn.onebot11.send_group_msg(ctx.target.id, msg)
    else
        jn.onebot11.send_private_msg(ctx.target.id, msg)
    end
end

-- 沙箱 ID 缓存（每次沙箱创建后复用）
local my_sandbox_id = nil

local function get_sandbox()
    if my_sandbox_id then return my_sandbox_id end
    -- 沙箱创建较快（返回 ID 即可），保留同步；耗时的是代码执行
    local sb, err = jn.sandbox.create()
    if not sb then
        return nil, err or "Sandbox 创建失败"
    end
    my_sandbox_id = sb.sandbox_id
    log.info("沙箱已创建", sb.sandbox_id)
    return my_sandbox_id
end

-- 格式化 Python 执行结果（同步返回 output / error 两值，异步返回 {output, error} 表）
local function handle_python(ctx, output, e)
    if e ~= nil and e ~= "" then
        reply_to(ctx, "```\n" .. tostring(e) .. "\n```")
    else
        reply_to(ctx, "```\n" .. tostring(output) .. "\n```")
    end
end

local function handle_shell(ctx, output, exit)
    reply_to(ctx, "exit=" .. tostring(exit) .. "\n```\n" .. tostring(output) .. "\n```")
end

-- ====================================================================
-- 同步版命令（t2i.generate_url / exec_python / exec_shell）
-- 注意：同步调用会阻塞事件循环，仅适合低频快路径示范
-- ====================================================================

-- /img sync <文字> —— 同步 T2I
jn.command.register("img sync", function(args, event)
    local ctx = { target = target_of(event) }
    local text = table.concat(args, " ") or "摸鱼中"
    if text == "" then text = "摸鱼中" end
    local url, err = jn.t2i.generate_url(build_poster_html(text))
    if not url then
        reply_to(ctx, "T2I 生成失败: " .. tostring(err) ..
            "（请先在 Web「T2I」页启用服务）")
        return true
    end
    send_image(ctx, url)
    return true
end, {
    description = "T2I 生成文字海报（同步 t2i.generate_url）",
    usage = "/img sync <文字>",
})

-- /run sync <python代码> —— 同步执行 Python
jn.command.register("run sync", function(args, event)
    local ctx = { target = target_of(event) }
    local code = table.concat(args, " ")
    if code == "" then
        reply_to(ctx, "用法: /run sync <python代码>，如 /run sync print(1+1)")
        return true
    end
    local sid, err = get_sandbox()
    if not sid then
        reply_to(ctx, "沙箱不可用: " .. tostring(err) ..
            "（请先在 Web「Sandbox」页启用服务）")
        return true
    end
    local output, e = jn.sandbox.exec_python(sid, code)
    if output == nil and e ~= "" then
        reply_to(ctx, "执行失败: " .. tostring(e))
        return true
    end
    handle_python(ctx, output, e)
    return true
end, {
    description = "在沙箱中执行 Python（同步 exec_python）",
    usage = "/run sync <python代码>",
})

-- /sb shell sync <命令> —— 同步执行 Shell
jn.command.register("sb shell sync", function(args, event)
    local ctx = { target = target_of(event) }
    local cmd = table.concat(args, " ")
    if cmd == "" then
        reply_to(ctx, "用法: /sb shell sync <命令>")
        return true
    end
    local sid, err = get_sandbox()
    if not sid then
        reply_to(ctx, "沙箱不可用: " .. tostring(err))
        return true
    end
    local output, exit = jn.sandbox.exec_shell(sid, cmd)
    if output == nil and exit == nil then
        reply_to(ctx, "执行失败: " .. tostring(err or "unknown"))
        return true
    end
    handle_shell(ctx, output, exit)
    return true
end, {
    description = "在沙箱中执行 Shell（同步 exec_shell）",
    usage = "/sb shell sync <命令>",
})

-- ====================================================================
-- 异步版命令（generate_url_async / exec_python_async / exec_shell_async）
-- 立即返回 req_id，不阻塞事件循环；结果在 on_xxx_response 回调处理。
-- 调用时把回复目标、沙箱 ID 等现场打包成 ctx，回调时原样带回。
-- ====================================================================

-- /img <文字> —— 异步 T2I
jn.command.register("img", function(args, event)
    local ctx = { action = "img", target = target_of(event) }
    local text = table.concat(args, " ") or "摸鱼中"
    if text == "" then text = "摸鱼中" end
    local rid = jn.t2i.generate_url_async(build_poster_html(text), nil, ctx)
    if rid == 0 then
        reply_to(ctx, "T2I 生成提交失败（请先在 Web「T2I」页启用服务）")
    end
    return true
end, {
    description = "T2I 生成文字海报（异步 generate_url_async）",
    usage = "/img <文字>",
})

-- /run <python代码> —— 异步执行 Python
jn.command.register("run", function(args, event)
    local code = table.concat(args, " ")
    if code == "" then
        reply_to({ target = target_of(event) }, "用法: /run <python代码>，如 /run print(1+1)")
        return true
    end
    local sid, err = get_sandbox()
    if not sid then
        reply_to({ target = target_of(event) }, "沙箱不可用: " .. tostring(err) ..
            "（请先在 Web「Sandbox」页启用服务）")
        return true
    end
    -- 现场保存：动作类型 + 沙箱 ID + 回复目标
    local ctx = { action = "run", sid = sid, target = target_of(event) }
    local rid = jn.sandbox.exec_python_async(sid, code, ctx)
    if rid == 0 then
        reply_to(ctx, "执行提交失败")
    end
    return true
end, {
    description = "在沙箱中执行 Python（异步 exec_python_async）",
    usage = "/run <python代码>",
})

-- /sb shell <命令> —— 异步执行 Shell
jn.command.register("sb shell", function(args, event)
    local cmd = table.concat(args, " ")
    if cmd == "" then
        reply_to({ target = target_of(event) }, "用法: /sb shell <命令>")
        return true
    end
    local sid, err = get_sandbox()
    if not sid then
        reply_to({ target = target_of(event) }, "沙箱不可用: " .. tostring(err))
        return true
    end
    local ctx = { action = "shell", sid = sid, target = target_of(event) }
    local rid = jn.sandbox.exec_shell_async(sid, cmd, ctx)
    if rid == 0 then
        reply_to(ctx, "执行提交失败")
    end
    return true
end, {
    description = "在沙箱中执行 Shell（异步 exec_shell_async）",
    usage = "/sb shell <命令>",
})

-- ====================================================================
-- 管理类（同步，快操作，无需异步）
-- ====================================================================

-- /sb del —— 删除当前沙箱
jn.command.register("sb del", function(args, event)
    if not my_sandbox_id then
        reply_to({ target = target_of(event) }, "还没有创建沙箱")
        return true
    end
    local ok, err = jn.sandbox.delete(my_sandbox_id)
    if ok then
        reply_to({ target = target_of(event) }, "沙箱已删除")
        my_sandbox_id = nil
    else
        reply_to({ target = target_of(event) }, "删除失败: " .. tostring(err))
    end
    return true
end, {
    description = "删除当前沙箱",
    usage = "/sb del",
})

-- /sb status —— 沙箱与 T2I 状态查询
jn.command.register("sb status", function(args, event)
    local t2iActive = jn.t2i.is_active()
    local sbActive = jn.sandbox.is_active()
    local lines = {
        string.format("T2I 启用: %s", tostring(t2iActive)),
        string.format("Sandbox 启用: %s", tostring(sbActive)),
    }
    local t2iCfg = jn.t2i.get_config()
    if t2iCfg then
        lines[#lines+1] = string.format("T2I 服务: %s (timeout=%s)", t2iCfg.base_url, t2iCfg.timeout)
    end
    local sbCfg = jn.sandbox.get_config()
    if sbCfg then
        lines[#lines+1] = string.format("Sandbox 服务: %s", sbCfg.base_url)
    end
    reply_to({ target = target_of(event) }, table.concat(lines, "\n"))
    return true
end, {
    description = "查看 T2I / Sandbox 状态",
    usage = "/sb status",
})

-- ====================================================================
-- 异步完成回调
--   on_t2i_response(req_id, ctx, result, err)      —— result = 图片 URL
--   on_sandbox_response(req_id, ctx, result, err)  —— result = {output, exit_code|error}
-- ====================================================================
function on_t2i_response(req_id, ctx, result, err)
    if not ctx then return end
    if err then
        reply_to(ctx, "T2I 渲染失败: " .. tostring(err))
        return
    end
    if ctx.action == "img" then
        send_image(ctx, result)
    end
end

function on_sandbox_response(req_id, ctx, result, err)
    if not ctx then return end
    if err then
        reply_to(ctx, "执行失败: " .. tostring(err))
        return
    end
    if ctx.action == "run" then
        handle_python(ctx, result.output, result.error)
    elseif ctx.action == "shell" then
        handle_shell(ctx, result.output, result.exit_code)
    end
end

log.info("media-gen 插件已加载")
