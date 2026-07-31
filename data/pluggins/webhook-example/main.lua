-- ====================================================================
-- Webhook 示例插件
-- ====================================================================
-- 处理外部服务推送的 HTTP 请求，转发通知到指定 QQ 群。
--
-- 用法：
--   1. 在 Web 面板开启 Webhook（默认 8091 端口）
--   2. 外部服务 POST 到 http://<host>:8091/（路径不限）
--   3. 插件自动识别 payload 格式并转发
--
-- 支持的格式：
--   · GitHub Webhook (push/issues/pull_request/ping)
--   · 通用告警 JSON: {"message":"...", "group_id":123, "level":"warn"}
--   · 钉钉/飞书文本: {"msgtype":"text","text":{"content":"..."}}
-- ====================================================================

local jn = require("jn")

local DEFAULT_GROUP = 123456789  -- 默认通知群号

-- ====================================================================
-- on_webhook 回调
-- ====================================================================
function on_webhook(event)
    local wb = event.webhook or {}
    local path = wb.path or ""
    local method = wb.method or "POST"
    local payload = wb.payload or {}
    local group = payload.group_id or DEFAULT_GROUP

    jn.log.info(string.format("[webhook] %s %s", method, path))

    -- ---- GitHub Webhook ----
    if payload.repository and payload.sender then
        local repo = payload.repository.full_name or "?"
        local user = payload.sender.login or "?"

        -- Push
        if payload.ref and payload.pusher then
            local branch = payload.ref:gsub("refs/heads/", "")
            local commits = payload.commits or {}
            local msg = string.format("📤 %s pushed to %s/%s (%d commits)", user, repo, branch, #commits)
            for _, c in ipairs(commits) do
                msg = msg .. string.format("\n  · %s: %s",
                    string.sub(c.id, 1, 7), c.message)
            end
            jn.onebot11.send_group_msg(group, msg)
            return true
        end

        -- Pull Request
        if payload.pull_request then
            local action = payload.action or ""
            local title = payload.pull_request.title or ""
            local url = payload.pull_request.html_url or ""
            if action == "opened" then
                jn.onebot11.send_group_msg(group,
                    string.format("🔀 %s 提了新 PR:\n[%s](%s)", user, title, url))
            elseif action == "closed" and payload.pull_request.merged then
                local merger = payload.pull_request.merged_by.login or user
                jn.onebot11.send_group_msg(group,
                    string.format("✅ %s 合并了 PR:\n[%s](%s)", merger, title, url))
            end
            return true
        end

        -- Issue
        if payload.issue and not payload.pull_request then
            local action = payload.action or "updated"
            local title = payload.issue.title or ""
            local url = payload.issue.html_url or ""
            if action == "opened" then
                jn.onebot11.send_group_msg(group,
                    string.format("🐛 %s 提了新 Issue:\n[%s](%s)", user, title, url))
            end
            return true
        end

        -- Ping
        if payload.hook_id and payload.zen then
            jn.onebot11.send_group_msg(group, "🟢 GitHub Webhook 连接测试成功! " .. repo)
            return true
        end
    end

    -- ---- 通用告警 ----
    -- {"message":"CPU 超过 90%", "group_id":123, "level":"warn"}
    if payload.message then
        local level = payload.level or "info"
        local emoji = { warn = "⚠️", error = "🚨", info = "ℹ️", critical = "🔥" }
        local prefix = emoji[level] or "📢"
        jn.onebot11.send_group_msg(group, prefix .. " " .. payload.message)
        return true
    end

    -- ---- 钉钉/飞书 ----
    if payload.msgtype == "text" and payload.text and payload.text.content then
        jn.onebot11.send_group_msg(group, payload.text.content)
        return true
    end

    -- ---- 未识别 ----
    jn.log.warn("[webhook] 未识别的 payload: " .. jn.json.encode(payload))
    return false
end

jn.log.info("[webhook] 插件已加载")
