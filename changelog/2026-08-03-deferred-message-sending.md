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
