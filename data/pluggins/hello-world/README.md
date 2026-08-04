# hello-world

JuanNiang-Neo 插件入门示例，覆盖插件最基础的三件事：**命令注册、事件监听、日志/JSON**。

## 文件结构

```
data/pluggins/hello-world/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
└── README.md
```

## 功能

| 命令 / 触发 | 说明 |
|------------|------|
| `/hello [名字]` | 按当前时间返回问候语（早上好/下午好/晚上好） |
| 发送 `ping` | 回复 `pong 🏓`（演示 `on_message` 事件监听） |
| `/json {"k":"v"}` | 演示 `json.encode / json.decode`，回显解析结果 |

## 覆盖的 API

- **`jn.command.register(path, handler, opts)`**：命令注册，handler 返回 `(consumed, reply)`，reply 非空时由系统自动回复
- **`on_message(event)`**：消息事件回调，返回 `(consumed, modified_event, skip_reply)`；`consumed=true` 时跳过 Agent
- **`log.info / warn / error`**：结构化日志（进 stdout 与前端 SSE 日志流）
- **`json.encode / decode`**：Lua 值 ↔ JSON 互转

## 权限说明

`pluggin.yaml` 里 `permissions: [onebot11]` —— 只有申请了 `onebot11` 权限才会注入 `onebot11` 全局表。
`log` / `json` 始终可用，无需申请。

## 试玩

1. 把 `hello-world` 目录放入 `data/pluggins/`，重启服务（或 Web 面板插件页 toggle 启用）
2. 群里发 `/hello 卷娘` → 返回问候
3. 发 `ping` → 回复 `pong 🏓`
