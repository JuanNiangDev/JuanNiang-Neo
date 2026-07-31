# Webhook 与 CronJob 使用文档

本文档说明 JuanNiang-Neo 的两种"主动消息"机制。

- **Webhook**：接收外部 HTTP 请求，转成 `webhook` 事件喂给 Lua 插件（不走 LLM Agent），用于外部系统集成。
- **CronJob**：基于 cron 表达式定时构造合成 `cronjob` 事件，走 Agent 标准处理路径（跳过策略/ACL），让 Agent 在指定时间"主动接收用户消息"并回复。

## Webhook

### 用途

让外部服务（GitHub、GitLab、监控告警、表单 webhook 等）通过 HTTP 请求触发机器人动作。Webhook **不走 LLM Agent**，只把 Payload 交给有 `webhook` 权限的 Lua 插件处理——这是给插件做外部集成的专用钩子。

### 架构与数据流

```
外部服务 (GitHub / 监控 / 自定义)
  │  POST http://your-server:8091/github
  │  Body: {"action":"opened","pull_request":{...}}
  ▼
WebhookAdapter (独立端口 8091, 默认关闭)
  │  1. Token 校验 → 不匹配返回 401
  │  2. 读取 Body → JSON 解析成功则直接使用, 失败则包装为
  │     {"raw":"原文","type":"non-json"}
  │  3. 构造 Event{
  │       PostType:"webhook",
  │       Webhook:{Path:"/github", Method:"POST", Payload:{...}},
  │       Admins: OB 管理员列表
  │     }
  │  4. 非阻塞写入 events channel (cap=128)
  │     · 满 → 丢弃事件, 返回 HTTP 503 → 外部服务应重试
  ▼
HagoCenter.runEventLoop (事件循环 goroutine)
  │  select 收到 webhook 事件
  │  → processEvent(): PostType=="webhook" 分支, 构造
  │    pluggin.EventData{
  │      PostType:"webhook",
  │      Webhook:{path, method, payload},
  │      Admins: OB 管理员
  │    }
  │  → PluginEngine.OnWebhook(event) → 遍历所有已加载插件
  ▼
PluginEngine.OnWebhook
  │  遍历所有已加载插件
  │  ├— 插件无 webhook 权限 → 跳过
  │  └— 插件有 webhook 权限 → 调 on_webhook(event)
  ▼
Lua 插件 on_webhook(event)
  │  event.webhook.path    = "/github"
  │  event.webhook.method  = "POST"
  │  event.webhook.payload = {action:"opened", ...}
  │  event.admins          = {"管理员QQ号"}
  │
  │  插件自行判断是否处理该事件:
  │    · 检查 payload 字段 → 不相关则 return false
  │    · 相关 → 执行逻辑 → return true (已消费)
  ▼
  处理完毕, 事件循环继续等待下一个事件
```

### 核心特性

| 特性 | 说明 |
|------|------|
| 独立端口 | `:8091`，与 API `:8090`、OB `:8081` 完全隔离 |
| Token 鉴权 | Bearer token，配了才校验，不配则任何请求都通过 |
| 不走 Agent | `PostType=="webhook"` 在事件循环中直接短路，永远不会进 LLM |
| 广播模式 | 每个 webhook 事件会调用**所有**有 `webhook` 权限的插件 |
| 插件自决 | 插件自己在 `on_webhook` 里判断 payload 并决定是否处理 |
| 队列丢弃 | events channel cap=128，满了返回 HTTP 503，外部服务应重试 |
| 热更新 | Web 面板点"启用"即生效，无需重启 |

### 配置入口

Web 面板"Webhook"页 → `PUT /api/v1/webhook/config`：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `addr` | string | `0.0.0.0` | 监听地址 |
| `port` | int | `8091` | 监听端口 |
| `token` | string | (空) | Bearer 鉴权令牌；空=不校验 |
| `enabled` | bool | `false` | 启用开关 |

配置是 DB 单行 `id=1`，不读 env。docker compose 默认未映射 `:8091`，需自行添加：

```yaml
# docker-compose.yaml
services:
  juan-niang-neo:
    ports:
      - "8091:8091"   # Webhook
```

### HTTP 协议

| 项目 | 说明 |
|------|------|
| 监听路径 | `/` 和 `/webhook` 都受理，路径透传到 event.webhook.path |
| 方法 | 任意（常用 POST） |
| Header | `Authorization: Bearer <token>`（配了 token 才校验） |
| Body | 任意。先尝试 JSON unmarshal；失败则包装为 `{"raw":"原文","type":"non-json"}` |
| 成功 | `200 OK`，body 为空 |
| 队列满 | `503 Service Unavailable`，事件被丢弃 |
| 鉴权失败 | `401 Unauthorized` |

### Event 数据结构（Lua 侧）

```lua
-- on_webhook(event) 收到的 event 结构

event.post_type  = "webhook"        -- 固定

event.webhook = {
    path    = "/github",             -- string: 请求路径
    method  = "POST",                -- string: HTTP 方法
    payload = {                      -- table: Body JSON 解析结果
        action       = "opened",     --   (示例: GitHub PR opened)
        pull_request = { ... },
        repository   = { ... },
        sender       = { ... },
    }
}

event.admins = { "10001", "10002" } -- 系统管理员 QQ 列表
```

### 多插件共存：如何区分事件归属

webhook 是**广播模式**——每个请求会调用所有有 `webhook` 权限的插件。插件通过以下三种方式自行判断是否处理。

#### 方式一：payload 字段自识别（推荐）

每个插件检查自己关心的字段，不相关则立即 `return false`：

```lua
-- GitHub 插件
function on_webhook(event)
    local p = event.webhook.payload
    if not p.repository or not p.sender then return false end
    -- 处理 GitHub 事件...
end

-- 告警插件
function on_webhook(event)
    local p = event.webhook.payload
    if not p.alert_name then return false end
    -- 处理告警事件...
end
```

> **优点是解耦**：外部服务不需要知道"该调哪个 URL"，只要发一个 JSON 就行。插件靠字段自我识别。

#### 方式二：路径区分

外部服务请求不同路径，插件检查 `event.webhook.path`：

```bash
# GitHub 配置这个 URL
http://host:8091/github

# 监控配置这个 URL
http://host:8091/alert
```

```lua
-- GitHub 插件
function on_webhook(event)
    if event.webhook.path ~= "/github" then return false end
    -- ...
end
```

#### 方式三：约定 action 字段

在 payload 里约定一个标识字段：

```json
{"_source": "github", "pull_request": {...}}
{"_source": "alert", "message": "CPU 超了"}
```

```lua
function on_webhook(event)
    if event.webhook.payload._source ~= "alert" then return false end
    -- ...
end
```

### 配置与测试

#### 开启 webhook

```bash
curl -X PUT http://localhost:8090/api/v1/webhook/config \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "addr": "0.0.0.0",
    "port": 8091,
    "token": "my-secret-token",
    "enabled": true
  }'
```

#### curl 测试

```bash
# 测试 1：简单 JSON
curl -X POST http://localhost:8091/ \
  -H "Authorization: Bearer my-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"message": "服务器负载过高", "level": "error", "group_id": 1076723599}'

# 测试 2：非 JSON body（会被包装）
curl -X POST http://localhost:8091/ \
  -H "Authorization: Bearer my-secret-token" \
  -d "raw text body"
# 插件收到: event.webhook.payload = {raw="raw text body", type="non-json"}

# 测试 3：不配 token 时不需要 Authorization 头
curl -X POST http://localhost:8091/ \
  -H "Content-Type: application/json" \
  -d '{"hi":"there"}'
```

### 完整示例

**pluggin.yaml**：

```yaml
name: github-notify
version: "1.0.0"
author: me
description: "GitHub Webhook 通知"
entry: main.lua
permissions:
  - webhook
  - onebot11
enabled: true
```

**main.lua**：

```lua
local jn = require("jn")
local GROUP = 987654321

function on_webhook(event)
    local p = event.webhook and event.webhook.payload or {}

    -- GitHub PR opened
    if p.action == "opened" and p.pull_request then
        local repo = (p.repository and p.repository.full_name) or "?"
        local title = p.pull_request.title or "?"
        local url  = p.pull_request.html_url or ""
        local user = p.sender.login or "?"
        jn.onebot11.send_group_msg(GROUP,
            string.format("🔀 %s 提了新 PR\n[%s] %s\n%s", user, repo, title, url))
        return true
    end

    -- GitHub Issue opened
    if p.action == "opened" and p.issue and not p.pull_request then
        local title = p.issue.title or "?"
        local url   = p.issue.html_url or ""
        jn.onebot11.send_group_msg(GROUP,
            string.format("🐛 新 Issue: %s\n%s", title, url))
        return true
    end

    -- GitHub Push
    if p.commits and p.ref then
        local branch = p.ref:gsub("refs/heads/", "")
        local n = #(p.commits or {})
        jn.onebot11.send_group_msg(GROUP,
            string.format("📤 %s pushed to %s (%d commits)",
                p.pusher.name, branch, n))
        return true
    end

    return false  -- 不是 GitHub 事件，放行给其他插件
end
```

配套 `data/pluggins/webhook-example/` 插件提供了完整的多格式支持（GitHub / 通用告警 / 钉钉飞书），可直接使用或参考。

### 注意

- Webhook 默认关闭；docker compose 默认未映射 `:8091`，需自行加 `ports:`
- 事件队列满（128）会丢并返回 503，外部服务应实现重试
- 无 SPA 兜底，仅 webhook + `/` 路由
- Webhook 配置是 DB 单行 `id=1`，不读 env
- 插件需声明 `webhook` 权限才会被调用；想在回调里发消息需同时声明 `onebot11`
- 多个 webhook 插件共存时，每个插件在 `on_webhook` 开头做字段判断快速 `return false` 避免冲突

## CronJob

### 用途

CronJob 支持**两种分发模式**，可独立或同时使用：

| 模式 | 字段 | 说明 |
|------|------|------|
| **Agent 分发** | `message` + `message_type` + `target_id` | 定时向 Agent 注入一条消息，Agent 将其作为用户输入处理并回复（原有行为） |
| **Plugin 分发** | `plugin_ids` + `payload` | 定时触发指定插件的 `on_timer_call(event)` 回调，传递 JSON payload |

当 `message` 为空时不发给 Agent，当 `plugin_ids` 为空时不触发插件。两者可同时配置。

#### Agent 分发

到点时 `CronJobManager.makeJobFunc` 构造一个合成 `Event{PostType:"cronjob", IsCronJob:true}`，注入主 EventLoop，走 `handleMessage` 标准路径——Agent 把 `message` 当用户输入处理。

合成事件会**跳过回复策略与 ACL**（`event.go:150`），因为这是系统主动任务，必须回复。

#### Plugin 分发

到点时 `CronJobManager.makeJobFunc` 解析 `plugin_ids`（插件目录名列表），调用 `PluginEngine.OnTimerCall()`。只有**已加载且定义了 `on_timer_call` 全局函数**的插件会被调用。`payload` 作为 JSON 对象传入 `event.payload`。

前端"定时任务"页面在新建/编辑时通过 Tab 切换 Agent/Plugin 模式，Plugin 模式下多选下拉框自动过滤显示 `supports_timer=true` 的已启用插件。

检测方式：运行时检查插件 Lua 全局 `on_timer_call` 是否为函数 → `ListMaps()` 返回 `supports_timer: bool`。

**示例插件**：`data/pluggins/cron-example/` 提供了一个完整的定时触发示例——向 payload 中指定的 QQ 号或群发送消息。

示例 Payload（私聊）：
```json
{
  "target_qq": 123456789,
  "message": "⏰ 定时提醒：该喝水啦！"
}
```

示例 Payload（群聊）：
```json
{
  "message_type": "group",
  "group_id": 123456,
  "message": "⏰ 群每日播报：今日天气..."
}
```

### Cron 表达式

⚠ **6 字段，支持秒级**（`robfig/cron` `WithSeconds()`）：

```
秒 分 时 日 月 周
0   0  9  *  *  *   # 每天 9:00
0   */5 *  *  *  *  # 每 5 分钟
0   0   0  1  *  *  # 每月 1 日 0:00
0   30  8  *  *  1-5 # 工作日 8:30
```

时区：`time.Local`（容器里 `TZ=Asia/Shanghai`）。

### API

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/api/v1/cronjobs` | 列出所有 |
| `GET` | `/api/v1/cronjobs/:id` | 单个详情 |
| `POST` | `/api/v1/cronjobs` | 新增（自动 reload 调度器） |
| `PUT` | `/api/v1/cronjobs/:id` | 覆盖更新（自动 reload） |
| `DELETE` | `/api/v1/cronjobs/:id` | 删除（自动 reload） |
| `PUT` | `/api/v1/cronjobs/:id/toggle` | 启停（自动 reload） |

`AddCronJobReq` body 字段：`name`、`cron_expr`、`message`（可选，空则不发给 Agent）、`message_type`（默认 `private`）、`target_id`、`is_active`、`plugin_ids`（`string[]`，可选）、`payload`（JSON 字符串，可选）。

### 示例：每天早上 9 点私聊提醒（Agent 分发）

```bash
curl -X POST http://localhost:8090/api/v1/cronjobs \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{
    "name": "早安提醒",
    "cron_expr": "0 0 9 * * *",
    "message": "早上好！记得喝水。",
    "message_type": "private",
    "target_id": 123456789,
    "is_active": true
  }'
```

### 示例：群每日播报（Agent 分发）

```bash
curl -X POST http://localhost:8090/api/v1/cronjobs \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{
    "name":"群每日播报",
    "cron_expr":"0 0 8 * * *",
    "message":"今天有什么新闻？",
    "message_type":"group",
    "target_id":987654321,
    "is_active":true
  }'
```

### 示例：每 10 秒触发插件发消息（Plugin 分发）

```bash
curl -X POST http://localhost:8090/api/v1/cronjobs \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{
    "name": "定时通知",
    "cron_expr": "*/10 * * * * *",
    "plugin_ids": ["cron-example"],
    "payload": "{\"target_qq\":123456789,\"message\":\"每10秒的提醒\"}",
    "is_active": true
  }'
```

到点后 Agent 会接到一条"群 987654321 发来消息 '今天有什么新闻？'"，按它能力回复。`message` 等同用户输入，会被 LLM 处理并可调工具（如 `browser_search`）。

### 运行时流（细节）

**Agent 分发路径**（`message` 非空）：
```
robfig/cron 到期 → CronJobManager.makeJobFunc(job)
  ├─ DAO.CronJob.UpdateLastRun(now, err)
  ├─ 构造 MessageEvent{MessageType=job.MessageType,
  │                   RawMessage=job.Message,
  │                   GroupID or UserID=job.TargetID}
  └─ 构造 Event{PostType:"cronjob", IsCronJob:true, Message, Time:now}
     └─ 非阻塞 send → HagoCenter.CronJobEvents (满则丢)

EventLoop 分支5 → processEvent (PostType=="cronjob") → handleMessage
  跳过策略与 ACL，正常 SendMessage 给 LLM，LLM 可调工具再 sendReply
```

**Plugin 分发路径**（`plugin_ids` 非空）：
```
robfig/cron 到期 → CronJobManager.makeJobFunc(job)
  ├─ DAO.CronJob.UpdateLastRun(now, err)
  ├─ 解析 plugin_ids JSON → []string
  ├─ 解析 payload JSON → map[string]any
  └─ PluginTimerDispatcher.OnTimerCall(pluginIDs, payload, admins)
     └─ PluginEngine.OnTimerCall → 遍历指定插件
        └─ 调用 Lua on_timer_call(event) （event.payload = payload）
```

两种路径**相互独立**：若同时配置则都会执行。

`LastRunAt`/`LastError` 回写 DB，前端"定时任务"页可看历史。删改/toggle 后由 `Service.AddCronJob/...` 调 `CronJobManager.Reload()` 同步调度器，无需重启进程。

### 注意

- **满则丢**：`CronJobEvents` cap=64，并发突发触发 queued 会被丢，不阻塞调度器
- **AgentSkill 触发**：`message` 走 `Skills.Match`——可让"早安"skill 在 `message=早安` 时注入特定 prompt
- **Token 计费**：CronJob 触发的消息也走 Session，按账记 `TokenUsage`
- **跨容器时区**：`TZ=Asia/Shanghai` 容器内是北京时间；裸机部署注意主机时区
- **target 必须 ChatArea 已存在**：`GetOrCreate` 会自动按 (type, targetID) 创建新 ChatArea，所以首次触发 Session 是新建的（无短期历史）
- **Plugin 调用的检测**：只有定义了 `on_timer_call` 全局函数且已加载的插件才会被调用；前端多选下拉框自动过滤
- **Plugin Payload**：必须是合法 JSON 字符串，保存时前端 CodeMirror 编辑器会实时校验格式
- **插件重载**：`POST /api/v1/plugins/reload` 可热重载全部非系统插件，新增/修改 `on_timer_call` 后需重载才生效

## 二者结合场景

- **监控报警 → Webhook → 插件 → 立即推送 + 创建临时 CronJob 多波次提醒**
  - 插件在 `on_webhook` 处理时调 `POST /api/v1/cronjobs` 建一个每隔 10 分钟触发的任务，告诉 Agent"提醒用户报警还在"
  - 用 CronJob 的 `message` 把上一轮报警内容装进去让 Agent 轮询处理

这利用了 Webhook 不走 LLM、CronJob 走 LLM 的差异，分工精确省 token。