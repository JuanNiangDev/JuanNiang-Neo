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
