# webhook-cron

外部集成示例：**Webhook 接收外部 HTTP 触发**（如 GitHub 通知）+ **CronJob 定时触发**。

## 文件结构

```
data/pluggins/webhook-cron/
├── pluggin.yaml
├── main.lua
└── README.md
```

## 覆盖的回调

### `on_webhook(event)` —— 外部 HTTP 触发

- **定向模式**：`POST /webhook/webhook-cron`（需要 `WebhookToken`，在 Web「Webhook」页查看端口/Token）→ 只有本插件收到
- **广播模式**：`POST /webhook` → 广播给所有定义了 `on_webhook` 的插件
- 返回 `(consumed, reply)`；定向模式下 `reply` 会作为 HTTP 响应 `metadata` 返回给调用方

**curl 测试**（假设 Webhook 监听 `:8091`、Token 为 `whsecret`）：

```bash
# 定向模式（发群通知）
curl -X POST http://localhost:8091/webhook/webhook-cron \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer whsecret" \
  -d '{"action":"opened","title":"feat: 新功能","sender":{"login":"juan"}}'
```

**GitHub 推送通知**的 payload 兼容 `{action, title/ref, sender.login}`。

### `on_cronjob(event)` —— 定时触发

在 Web「CronJob」页新建任务：

| 配置项 | 值 |
|--------|-----|
| `message_type` | `cronjob` |
| `cron_expr` | 如 `0 0 9 * * *`（每天 9:00） |
| 插件选择 | 勾选 `webhook-cron` |
| `payload` | `{"target_group": 1076723599, "message": "早上好，新的一天开始啦"}` |

支持 `payload.target_group`（发群）或 `payload.target_qq`（发私聊），都不填默认发到示例群号。

## 权限

`permissions: [onebot11]`

> `on_webhook` / `on_cronjob` 由调用层按是否定义该函数过滤，无需额外权限声明。

## 试用

1. Web「Webhook」页确认已启用（记下端口与 Token）
2. 用上面的 curl 发一条测试 → 群/私聊收到通知
3. Web「CronJob」页建一个 cronjob 任务选本插件 → 到点自动发消息
