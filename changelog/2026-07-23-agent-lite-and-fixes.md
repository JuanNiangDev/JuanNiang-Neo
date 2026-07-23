# Changelog 2026-07-23

## 新功能

### AgentLite 模式
- 精简模式：不向 LLM 暴露工具/MCP，跳过 Agent 循环，LLM 直接回复
- 记忆、提示词、Skill 行为不受影响，回复仍写回记忆
- 注入 system 提示词告知 LLM 当前处于精简模式
- 此模式下关闭分条回复功能，整条消息发送
- 开关位于 Web 面板「回复设置 → 其他设置」

### Markdown 去除工具
- Agent 发送消息前去除 `**加粗**`、`*斜体*`、`` `代码` ``、`[链接](url)`、`# 标题`、`> 引用`、列表、表格等格式
- 修复了 `|` 替换破坏 `<|msg|>` 分隔符导致分条回复失效的问题（改为先拆分再去格式）
- 开关位于 Web 面板「回复设置 → 其他设置」

### 回复设置页面重构
- 原「核心配置 - 回复策略」改名为「回复设置」
- 页面改为左右双列卡片布局：回复策略（7）+ 其他设置（5）
- 其他设置卡片含 AgentLite 开关和 Markdown 去除开关

### 机器人名字配置
- 相关性检查时参考机器人名字辅助判断
- 配置位于「回复设置 → 回复策略 → 相关性阈值」卡片内

### 仅应用部署
- 新增 `deployments/docker-compose.app-only.yaml`，连接外部 Postgres + Redis
- 支持 `PLUGGINS_DIR` 环境变量自定义宿主侧插件目录

### Dockerfile.cn 国内镜像
- Alpine 源替换为 aliyun 镜像加速

## Bug 修复

### DAO 零值字段不更新
- **问题**：Web 界面关闭沙箱后 DB 未同步，重启后状态丢失
- **原因**：GORM `Updates(struct)` 跳过零值字段，`IsActive: false`（bool 零值）无法写入
- **修复**：所有 `Updates` 调用加 `Select("*")` 强制更新全部字段
- **影响范围**：sandboxDao、onebotDao、webhookDao、t2iDao

### 插件命令被路由给 Agent
- **问题**：相关性回复模式和仅 Plugin 模式下，插件命令仍被交给 Agent 执行
- **原因**：Lua 插件 handler 未返回 `consumed` 时默认 `false`，`OnMessage` 因 `c==false` 跳过回复且不标记消费
- **修复**：改为 `c || reply != ""`，只要命令产生了回复内容即视为已处理

### 插件分组命令触发相关性检查
- **问题**：发送 `/system` 等分组节点命令时仍触发相关性检查
- **原因**：`HasCommand` 只检查 `Handler != nil`，分组节点无 Handler 被判定为非插件命令
- **修复**：条件扩展为 `Handler != nil || len(Children) > 0`

### 相关性结果解析失败
- **问题**：LLM 返回的 JSON 中 reason 含未转义引号导致解析失败，fallback 到 `relevance=0`
- **修复**：`extractRelevanceJSON` 改用正则提取 `relevance` 和 `reason`，不依赖标准 JSON 解析
