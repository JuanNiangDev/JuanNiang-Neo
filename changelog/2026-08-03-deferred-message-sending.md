# Changelog 2026-08-03

## 新功能

### Agent 消息延迟发送（执行完再发）
- Agent 在任务执行（ReAct 循环）中途不再发送任何消息，发送请求统一入队，任务执行完成后按序统一发送
- 涉及工具：`send_private_msg`、`send_group_msg`、`send_face`、`text_to_image`
- 实现：新增 `tool.DeferredSendQueue`，通过 context 注入工具；`handleMessage` 在循环结束后 `Flush` 队列，再发送最终回复
### 工具在无队列上下文（非 Agent 路径调用）时回退为立即发送，保持向后兼容

## Bug 修复

### 任务完成后重复发送（工具投递 + 最终回复复述）
- **问题**：Agent 通过 send_group_msg / send_private_msg 向当前会话发送消息后，最终回复仍会复述操作过程（如"我先看下…找到了…搞定咯"），一次任务连发多条消息
- **修复**：`DeferredSend` 增加 `Delivery` 标记，`send_*_msg` 投递到当前会话后，`handleMessage` 跳过最终回复的发送；同时强化系统锁定提示词，要求"已通过工具发送则最终回复只输出 __NO_REPLY__，不描述操作过程"

### send_group_msg 发送失败（target=0）
- **问题**：LLM 将 group_id 输出为字符串或省略该参数时，executor 的 `json.Unmarshal` / `BuildMessageFromJSON` 错误被忽略，导致 `GroupID=0`、`Message=nil`，Flush 时 `SendGroupMsg(0, nil)` 发送失败（retcode=1200）
- **修复**：新增 `FlexInt64` 类型兼容数字/字符串两种 JSON 格式；`send_group_msg` / `send_private_msg` 补上参数与消息解析错误检查；目标缺失时从当前会话上下文兜底推断，仍无法确定则返回明确错误提示
- **保护**：`DeferredSendQueue.Flush` 跳过 `TargetID<=0` 的非法目标并告警

### text_to_image 不再自动发送图片
- **问题**：text_to_image 自动投图 + LLM 又用 CQ 码拼接富文本，导致同一张图重复发送
- **修复**：text_to_image 只返回图片 URL，由 LLM 按系统提示词流程用 `[CQ:image,file=URL]` 拼接（可与文字组成一条富文本消息），与系统锁定提示词的说明保持一致

### send_group_msg 参数非标准 JSON 报错
- **问题**：LLM 把消息内容直接当作工具参数传入（如裸的 `[CQ:image,file=...] 文字`，未包成 JSON 对象），`json.Unmarshal` 报 `invalid character 'C' looking for beginning of value`，富文本发送失败
- **修复**：新增 `BuildMessageLoose` 容错解析——标准 JSON（字符串/消息段数组）解析失败时，将原始参数整体视为消息内容；目标仍从当前会话兜底推断

### 投递后跳过最终回复未生效（Flush 清空队列）
- **问题**：`DeferredSendQueue.Flush` 会清空队列，但 `handleMessage` 在 Flush 之后才调用 `DeliveredTo` 判断是否已向当前会话投递，导致永远返回 false，静默抑制逻辑从未生效
- **修复**：`DeliveredTo` 判断移到 `Flush` 之前

## Token 统计

### 修复 Token 统计不生效
- **问题**：Eino 输出循环只收集文本，未读取 `ResponseMeta.Usage.TotalTokens`；`Session.UpdateTokenUsage` 无任何调用方；`recordChat` 的 token 参数写死 0，导致 `sessions.token_usage` 与 `chat_records.token_count`（Overview 总用量来源）恒为 0
- **修复**：`handleMessage` 在收集输出时累加每次 LLM 调用的 TotalTokens（ReAct 每轮都计入）；调用 `Session.RecordTokenUsage` 写入会话总账；assistant 聊天记录的 `token_count` 写入真实值

### 新增：每日 Token 用量统计
- 新增 `token_usage_daily` 表（`TokenUsageDaily` 模型）：`date`（YYYY-MM-DD，主键）+ `token_count`，按自然日 UPSERT 累加，跨全部会话
- `SessionManager.RecordTokenUsage` 同时写入会话总账（Session.TokenUsage）与每日统计，`UpdateTokenUsage` 由它取代
- `TokenUsageDailyDAO`：`AddTokenUsage`（UPSERT）/ `ListByRange`（按日期区间查询）/ `Total`（历史累计）
- 已注册 GORM AutoMigrate，启动自动建表

### 新增：每日 Token 用量 Web API
- `GET /api/v1/overview/daily-token-usage?days=7|15|30`（默认 7，上限 30），返回连续日期序列，缺失日期补 0
- `service.GetDailyTokenUsage` + `dto.DailyTokenUsageResp`

### 新增：仪表盘 Token 用量折线图
- DashboardPage 新增「Token 用量趋势」卡片，支持近 7/15/30 天切换
- 引入 echarts（按需引入 LineChart + Grid/Tooltip + CanvasRenderer），按暗色主题配色

### 新增：Webhook /webhook 插件列表路由
- Webhook 服务 `GET /webhook`（或 `/webhook/`）返回所有启用 webhook 的插件及其 URL 路径，JSON 格式：`{code, message, metadata: [{name, path, enabled}]}`
- `adapter.PluginWebhookRouter` 接口新增 `ListWebhookPlugins`，由 `PluginEngine` 实现（判定：已加载 + webhook 权限 + 定义 on_webhook）
- 其他方法（如 POST /webhook）仍走原有广播逻辑，不受影响

### 修复：每日 Token 统计 UPSERT 报 ambiguous
- **问题**：`ON CONFLICT DO UPDATE SET token_count = token_count + X` 的 RHS 未限定表名，Postgres 中与 INSERT 列清单同名列冲突，报 `column reference "token_count" is ambiguous (SQLSTATE 42702)`，每日累加全部失败
- **修复**：表达式限定为 `token_usage_dailies.token_count + ?`

### 修复：对话历史记忆断层导致旧任务被重复执行
- **问题**：延迟队列投递的消息未写回短期记忆/ChatRecord；当最终回复被静默或投递抑制丢弃时，记忆里留下"用户请求无人回复"的空档，下次用户发言时 LLM 误以为旧任务（如查询天气）仍待执行
- **修复**：`DeferredSendQueue.Flush` 返回本次发送列表；`handleMessage` 将投递给当前会话的交付消息（`Delivery=true`）写回短期记忆与聊天记录
