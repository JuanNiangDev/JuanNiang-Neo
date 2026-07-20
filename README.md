# JuanNiang-Neo

> 复活吧卷娘 — 基于 OneBot11 协议的 LLM QQ 聊天 Agent

***JuanNiang-Neo 是一个以 Go 1.25 构建的 QQ 机器人项目，红岩网校的吉祥物卷娘。***

核心由 LLM 驱动的对话 Agent（`HagoCenter` 聚合 Provider / MCP / Memory / Prompt / Session / Skill / Tool）与 OneBot11 反向 WebSocket 适配器组成，长任务以 errgroup 风格在后台执行，再由独立 Drainer Agent 排空缓冲并发送 QQ 消息。项目同时包含 Lua 插件引擎、Vue 3 管理面板，以及 Postgres + Redis + Sandbox + T2I 等可插拔基础设施。所有持久化状态落 Postgres + Redis，配置与运行时状态均可在 Web 面板热切换。

## 主要特性

- **Agent 系统**：基于 LLM（OpenAI 兼容）的对话 Agent，支持 Provider / MCP / Tool / Skill / Prompt / Plugin 多模块组合
- **OneBot11 反向 WebSocket 适配器**：与 QQ 机器人框架对接，OneBot11 API 作为 Agent 工具注册
- **Lua 插件系统**：gopher-lua 驱动，支持多级命令、Lua SDK（带 LuaCATS 注解）、系统插件保护
- **Web 管理后台**：Vue 3 + Vuetify 3，JWT 鉴权（可选 OIDC SSO），管理全部配置与运行时状态
- **基础设施**：Postgres 持久化 + Redis 缓存 + Sandbox 代码沙箱 + T2I 文生图，未配置时自动返回未启用提示
- **系统锁定提示词**：每次对话强制拼接，引导 LLM 使用 T2I 富文本、分消息段回复、权限层级等

## 文档导航

| 文档名称 | 说明 |
|----------|------|
| [architecture.md](docs/architecture.md) | 项目架构 |
| [event-flow.md](docs/event-flow.md) | 事件流 & Agent 处理流程 |
| [call-stack.md](docs/call-stack.md) | 调用栈 |
| [implementation.md](docs/implementation.md) | 实现细节 |
| [api.md](docs/api.md) | Web API 文档 |
| [provider.md](docs/provider.md) | Provider 设计 |
| [deployment.md](docs/deployment.md) | 部署与调试指南 |
| [pluggin/architecture.md](docs/pluggin/architecture.md) | 插件系统架构 |
| [pluggin/api-reference.md](docs/pluggin/api-reference.md) | 插件 API 参考 |
| [pluggin/development.md](docs/pluggin/development.md) | 插件开发指南 |
| [pluggin/implementation.md](docs/pluggin/implementation.md) | 插件系统实现 |

## 许可证

本项目采用 [MIT License](LICENSE)。
