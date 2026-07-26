# Webhook 与 CronJob 使用文档

本文档说明 JuanNiang-Neo 的两种"主动消息"机制。

- **Webhook**：接收外部 HTTP 请求，转成 `webhook` 事件喂给 Lua 插件（不走 LLM Agent），用于外部系统集成。
- **CronJob**：基于 cron 表达式定时构造合成 `cronjob` 事件，走 Agent 标准处理路径（跳过策略/ACL），让 Agent 在指定时间"主动接收用户消息"并回复。

## Webhook

### 用途

让外部服务（GitHub、GitLab、监控告警、表单 webhook 等）触发机器人动作。Webhook **不走 LLM Agent**，只把 Payload 交给有 `webhook` 权限的 Lua 插件处理——这是给插件做外部集成的钩子。

### 配置入口

Web 面板"Webhook"页：`PUT /api/v1/webhook/config` 启用：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `addr` | string | `0.0.0.0` | 监听地址 |
| `port` | int | `8091` | 监听端口（独立于 API `:8090` 与 OB `:8081`，编排上需放行） |
| `token` | string | (空) | Bearer 鉴权令牌；空=不验证 |
| `enabled` | bool | `false` | 启用 |

`POST?enabled=true` 后由 `WebhookAdapter.Start` 起 HTTP server。**默认关闭**。

### HTTP 协议

- 监听路径：`/` 与 `/webhook` 都接受
- 方法：任意（常用 `POST`）
- Header：`Authorization: Bearer <token>`（若配 `token`）；不匹配返回 401
- Body：任意。`WebhookAdapter.handleRequest` 先尝试 JSON unmarshal 若失败则包装为 `{"raw": "<原文>", "type": "non-json"}`
- 成功返回 `200 No-Body`/`503`（事件队列满时丢）

合成事件结构（喂给 Lua）：

```lua
event.post_type       == "webhook"
event.webhook.path    = "/webhook"
event.webhook.method  = "POST"
event.webhook.payload = { raw="...", action="opened", ... }
event.admins          = { "QQ号" }   -- 透传 OB AdminQQNumbers; 无连入则为 nil
```

### 示例：GitHub PR 通知群消息

`pluggin.yaml`（注意 `permissions: [webhook, onebot11]`）:

```yaml
name: github-pr
version: "1.0.0"
permissions:
  - webhook
  - onebot11
enabled: true
entry: main.lua
```

`main.lua`:

```lua
local jn = require("jn")
local GROUP = 987654321

function on_webhook(event)
    local p = event.webhook and event.webhook.payload or {}
    if p.action == "opened" then
        local repo = (p.repository and p.repository.full_name) or "?"
        local title = (p.pull_request and p.pull_request.title) or "?"
        local url = (p.pull_request and p.pull_request.html_url) or ""
        jn.onebot11.send_group_msg(GROUP,
            "GitHub PR 新开 [" .. repo .. "]\n" ..
            title .. "\n" .. url)
    end
    return false
end
```

外部服务配置 GitHub webhook → `https://your-bot:8091/webhook`，Header `Authorization: Bearer <token>`，content-type `application/json`。

### 示例：任意 HTTP 触发私聊提醒（curl 测试）

```bash
# 启用 webhook + 设 token=secret123
curl -X PUT http://localhost:8090/api/v1/webhook/config \
  -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" \
  -d '{"addr":"0.0.0.0","port":8091,"token":"secret123","enabled":true}'

# 插件 on_webhook 收到 payload: {"hi":"there"}
curl -X POST http://localhost:8091/webhook \
  -H "Authorization: Bearer secret123" \
  -H "Content-Type: application/json" \
  -d '{"hi":"there"}'
```

### 注意

- Webhook 默认关闭；记得放行 `:8091`（docker compose 默认未映射此端口，需自行加 `ports:`）
- 事件队列满（128）会丢并返回 503，外部服务应重试
- 无 SPA 兜底，仅 webhook + `/` 路由
- Webhook 配置是 DB 单行 `id=1`，不读 env

## CronJob

### 用途

让 Agent 定时"被提醒发消息"。每个 CronJob 存 `cron_expr` + `message` + `MessageType`（`private`/`group`）+ `TargetID`（QQ 号或群号）。到点时 `CronJobManager.makeJobFunc` 构造一个合成 `Event{PostType:"cronjob", IsCronJob:true}`，注入主 EventLoop，走 `handleMessage` 标准路径——Agent 把 `message` 当用户输入处理。

合成事件会**跳过回复策略与 ACL**（`event.go:150`），因为这是系统主动任务，必须回复。

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

`AddCronJobReq` body 字段：`name`、`cron_expr`、`message`、`message_type`（默认 `private`）、`target_id`、`is_active`。

### 示例：每天早上 9 点私聊提醒

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

### 示例：群每日播报

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

到点后 Agent 会接到一条"群 987654321 发来消息 '今天有什么新闻？'"，按它能力回复。`message` 等同用户输入，会被 LLM 处理并可调工具（如 `browser_search`）。

### 运行时流（细节）

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

`LastRunAt`/`LastError` 回写 DB，前端"定时任务"页可看历史。删改/toggle 后由 `Service.AddCronJob/...` 调 `CronJobManager.Reload()` 同步调度器，无需重启进程。

### 注意

- **满则丢**：`CronJobEvents` cap=64，并发突发触发 queued 会被丢，不阻塞调度器
- **AgentSkill 触发**：`message` 走 `Skills.Match`——可让"早安"skill 在 `message=早安` 时注入特定 prompt
- **Token 计费**：CronJob 触发的消息也走 Session，按账记 `TokenUsage`
- **跨容器时区**：`TZ=Asia/Shanghai` 容器内是北京时间；裸机部署注意主机时区
- **target 必须 ChatArea 已存在**：`GetOrCreate` 会自动按 (type, targetID) 创建新 ChatArea，所以首次触发 Session 是新建的（无短期历史）

## 二者结合场景

- **监控报警 → Webhook → 插件 → 立即推送 + 创建临时 CronJob 多波次提醒**
  - 插件在 `on_webhook` 处理时调 `POST /api/v1/cronjobs` 建一个每隔 10 分钟触发的任务，告诉 Agent"提醒用户报警还在"
  - 用 CronJob 的 `message` 把上一轮报警内容装进去让 Agent 轮询处理

这利用了 Webhook 不走 LLM、CronJob 走 LLM 的差异，分工精确省 token。