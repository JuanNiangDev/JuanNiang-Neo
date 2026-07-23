# 相关性检查与回复策略优化

> 日期: 2026-07-23

## 变更概述

优化系统回复策略中的相关性检查机制，解决插件命令被误判丢弃的问题，并新增机器人名字配置用于辅助相关性判断。

## 详细改动

### 1. 插件命令跳过相关性检查

**问题**: 插件注册的命令（如 `/qd` 签到）被 LLM 相关性检查误判为不相关（得分 0.1），导致消息在策略层被丢弃，插件命令无法生效。

**修复**: 在 `StrategyRelevance` 模式下，检测到已注册的插件命令时直接跳过相关性评估，与 `@` 机器人同等处理。

涉及文件:

| 文件 | 变更 |
|------|------|
| `internal/pluggin/command.go` | 新增 `HasCommand(raw)` 方法，检测消息是否匹配已注册命令（不执行） |
| `internal/pluggin/pluggin.go` | 新增 `HasPluginCommand(raw)` 暴露给 agent 层 |
| `internal/agent/reply_strategy.go` | 新增 `isPluginCommand()` 辅助方法 |
| `internal/agent/event.go` | `StrategyRelevance` 分支增加 `!h.isPluginCommand(msg.RawMessage)` 条件 |

逻辑流程:
```
消息到达 → StrategyRelevance
  ├─ @我?         → 跳过相关性检查
  ├─ 插件命令?     → 跳过相关性检查  ← 新增
  └─ 其他          → 调用 LLM 相关性评估
```

### 2. 新增机器人名字配置

**问题**: 相关性检查的 LLM prompt 仅使用 QQ 昵称作为身份标识，群聊中用户可能用自定义名字称呼机器人（如"小卷"），导致相关性判断不准确。

**修复**: 在回复策略配置中新增 `bot_name` 字段，相关性 prompt 同时参考昵称和自定义名字。

涉及文件:

| 文件 | 变更 |
|------|------|
| `internal/core/models/reply_strategy.go` | `ReplyStrategyConfig` 新增 `BotName` 字段 |
| `internal/agent/reply_strategy.go` | `relevanceAgentEvaluate` 新增 `botName` 参数，prompt 增加 `你的名字` 字段 |
| `internal/api/dto/request.go` | `UpdateReplyStrategyReq` 新增 `bot_name` |
| `internal/api/dto/response.go` | `ReplyStrategyResp` 新增 `bot_name` |
| `internal/api/service/service.go` | Get/Update 接口透传 `BotName` |
| `web/src/api/index.ts` | 接口类型新增 `bot_name` |
| `web/src/views/ReplyStrategyPage.vue` | 相关性模式下新增"机器人名字"输入框 |

### 3. 修复相关性结果解析 BUG

**问题**: LLM 返回的 JSON 中 `reason` 字段偶尔包含未转义的双引号（如 `"消息称呼我的名字"卷娘""`），导致 `json.Unmarshal` 失败，fallback 到 `extractRelevanceJSON` 也因同样原因失败，最终返回 `relevance=0`。

**修复**: `extractRelevanceJSON` 改用正则表达式分别提取 `relevance` 数值和 `reason` 文本，不依赖标准 JSON 解析，自然容忍 reason 中的嵌套引号。

涉及文件:

| 文件 | 变更 |
|------|------|
| `internal/agent/reply_strategy.go` | `extractRelevanceJSON` 重写为正则提取方式 |

## API 变更

### GET/PUT `/api/v1/reply-strategy`

响应新增字段:

```json
{
  "strategy": "relevance",
  "relevance_threshold": 0.5,
  "bot_name": "小卷"
}
```

## 数据库变更

`reply_strategy_config` 表新增列 `bot_name` (TEXT, 默认空字符串)，由 GORM AutoMigrate 自动处理。
