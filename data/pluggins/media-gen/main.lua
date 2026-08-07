-- ====================================================================
-- media-gen：T2I 文生图 + Sandbox 沙箱执行示例（异步版）
-- ====================================================================
-- 覆盖内容：
--   1. t2i.generate_url_async        —— HTML 模板渲染图片（异步 + 现场 ctx）
--   2. sandbox.create（同步，快速）   —— 获取沙箱 ID
--   3. sandbox.exec_python_async / exec_shell_async（异步）
-- 说明：T2I 渲染与沙箱执行代码都可能耗时（秒级~几十秒），用异步版
--   不阻塞事件循环；调用时把回复目标等现场打包成 ctx，完成回调
--   on_t2i_response / on_sandbox_response 里按 ctx.action 分发并回复。
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

-- --------------------------------------------------------------------
-- /img <文字> —— T2I 生成文字海报并发群（异步）
-- --------------------------------------------------------------------
jn.command.register("img", function(args, event)
    local text = table.concat(args, " ") or "摸鱼中"
    if text == "" then text = "摸鱼中" end
    local html = string.format([[
      <div style="font-family:'Microsoft YaHei',sans-serif;width:480px;padding:40px;
        background:linear-gradient(135deg,#667eea,#764ba2);color:#fff;text-align:center;
        border-radius:16px;">
        <h1 style="margin:0;font-size:38px;">🖼️ %s</h1>
        <p style="opacity:.85;margin:12px 0 0;">—— 来自 media-gen 插件</p>
      </div>
    ]], text)

    local ctx = { action = "img", target = target_of(event) }
    local rid = jn.t2i.generate_url_async(html, nil, ctx)
    if rid == 0 then
        reply_to(ctx, "T2I 生成提交失败（请先在 Web「T2I」页启用服务）")
    end
    return true
end, {
    description = "T2I 生成文字海报",
    usage = "/img <文字>",
})

-- --------------------------------------------------------------------
-- /run <python代码> —— Sandbox 异步执行 Python（单行）
-- /sb shell <命令>   —— Sandbox 异步执行 Shell
-- /sb del             —— 删除当前沙箱
-- --------------------------------------------------------------------
local my_sandbox_id = nil  -- 插件内保存沙箱 ID（每次沙箱创建后复用）

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
    description = "在沙箱中执行 Python 代码",
    usage = "/run <python代码>",
})

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
    description = "在沙箱中执行 Shell 命令",
    usage = "/sb shell <命令>",
})

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

-- --------------------------------------------------------------------
-- /sb status —— 沙箱与 T2I 状态查询（同步快查询，无需异步）
-- --------------------------------------------------------------------
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

-- --------------------------------------------------------------------
-- 异步完成回调
--   on_t2i_response(req_id, ctx, result, err)      —— result = 图片 URL
--   on_sandbox_response(req_id, ctx, result, err)  —— result = {output, exit_code|error}
-- --------------------------------------------------------------------
function on_t2i_response(req_id, ctx, result, err)
    if not ctx then return end
    if err then
        reply_to(ctx, "T2I 渲染失败: " .. tostring(err))
        return
    end
    if ctx.action == "img" then
        local msg = {
            { type = "text", data = { text = "给你做了一张图：" } },
            { type = "image", data = { file = result } },
        }
        if ctx.target.kind == "group" then
            jn.onebot11.send_group_msg(ctx.target.id, msg)
        else
            jn.onebot11.send_private_msg(ctx.target.id, msg)
        end
    end
end

function on_sandbox_response(req_id, ctx, result, err)
    if not ctx then return end
    if err then
        reply_to(ctx, "执行失败: " .. tostring(err))
        return
    end
    if ctx.action == "run" then
        local e = result.error or ""
        local out = result.output or ""
        if e ~= "" then
            reply_to(ctx, "```\n" .. e .. "\n```")
        else
            reply_to(ctx, "```\n" .. out .. "\n```")
        end
    elseif ctx.action == "shell" then
        reply_to(ctx, "exit=" .. tostring(result.exit_code) .. "\n```\n" .. tostring(result.output) .. "\n```")
    end
end

log.info("media-gen 插件已加载")
