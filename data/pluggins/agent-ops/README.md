# agent-ops

Agent 运行时管理示例：**多级命令**（`/agent xxx`）+ **agent API 全家桶**（配置查询 + 运行时切换）。

## 文件结构

```
data/pluggins/agent-ops/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明
├── avatar.png     # 插件图标
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/agent providers` | 运行时已加载的 LLM Provider 列表 |
| `/agent provider switch <id>` | 切换主 Provider（**仅管理员**） |
| `/agent tools` | 运行时已注册的 Tool 列表 |
| `/agent skills` | 已配置的 Skill |
| `/agent sessions` | Session 列表 |
| `/agent prompts` | Prompt 模板 |
| `/agent mcps` | 运行时 MCP 服务器 |
| `/agent plugins` | 已安装插件 |
| `/agent memory compact` | 压缩当前会话短期记忆（**仅管理员**） |

## 覆盖的 API（agent 全局表，共 17 个函数）

**配置查询**（DB 读取）：
`get_providers()` `get_mcp_servers()` `get_skills()` `get_sessions()` `get_prompts()` `get_tools()` `get_plugins()`

**Provider 管理**：`set_provider_active(id, active)` `list_runtime_providers()` `switch_provider(id)`

**MCP 管理**：`set_mcp_active(id, active)` `list_mcps()` `toggle_mcp(id, active)`

**Tool 管理**：`list_tools()` `toggle_tool(name, active)`

**上下文与记忆**：`get_current_chat_area()` `compact_memory()`

## 多级命令写法

```lua
-- 顶层分组节点（无 handler，命中后自动列出子命令）
jn.command.register("agent", nil, { description = "Agent 管理命令分组", usage = "/agent <子命令>" })

-- 二级/三级命令
jn.command.register("agent providers", function(args, event) ... end, { description = "...", usage = "/agent providers" })
jn.command.register("agent provider switch", function(args, event) ... end, { ... })
```

## 权限

`permissions: [agent, onebot11]`

## 试用

- `/agent providers` → 查看当前加载的 LLM Provider
- `/agent tools` → 查看 Agent 拥有的全部工具（含 list_images / send_sticker 等）
- 切换 Provider、压缩记忆需要你的 QQ 在管理员列表
