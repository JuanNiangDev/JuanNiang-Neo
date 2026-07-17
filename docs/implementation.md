# 实现细节

## 核心模块实现

### 1. Adapter (`internal/adapter/`)

**WebSocket 服务器 (`server.go`)**
- 使用 `RomiChan/websocket` 实现 WS 升级
- `wsServer` 管理多个 `wsConn` (每个 OneBot11 客户端一个连接)
- `callAPI()`: 同步等待, echo 匹配响应, 10s 超时
- `readLoop()`: 循环读取 → gjson 解析 post_type → 分发到 events chan (buffer 128)
- 鉴权: `Authorization: Bearer <token>` header 或 `?access_token=<token>` query

**消息段构造 (`segment.go`)**
- 函数式: `Text()`, `Image()`, `At()`, `Face()`, `Reply()`, ...
- 链式: `NewMsg().At("123").Text(" hello").Image("url").Build()`
- CQ 码自动解析: `[CQ:image,file=xxx]` → Segment

**API 封装 (`api.go`)**
- 全部 OneBot11 API 方法
- `callAndParse[T]` 泛型辅助: 调用 → 解析 Data 字段 → 返回 T
- `normalizeMessage`: 统一 string/Segment/MessageBuilder → OneBot11 格式

### 2. Core (`internal/core/`)

**数据模型 (`models/models.go`)**
- 15 个 GORM 模型, 全部带软删除
- 自定义 JSON 类型: `JSONMap` (map[string]any), `JSONSlice` ([]string)
- UUID 主键 (Provider/MCPServer/Skill/Tool/Prompt/ChatArea/Session/Memory/BgTask)
- 自增 ID (AdminUser/ChatRecord/ACLRule)

**DAO (`dao/dao.go`)**
- 每个模型一个 DAO 结构体, 构造时注入 `*gorm.DB`
- `Bundle` 聚合所有 DAO, 方便一次性注入
- `GetOrCreate` 模式 (ChatArea/Session/Memory): 先查后建

**缓存 (`cache/cache.go`)**
- Redis key 前缀隔离 (`juan:`)
- 基础 KV: Get/Set/Del/Exists/SetNX
- List (短期记忆): LPush/RPush/LRange/LTrim/LLen
- PubSub (任务通知): Publish/Subscribe
- Hash: HGet/HSet/HGetAll/HDel

**ACL (`acl/acl.go`)**
- 默认策略: 无规则 = 允许所有
- `Check(userID, chatAreaID, action)`: deny → false; allow+actions → 白名单匹配
- 支持通配符 `*`

### 3. Agent (`internal/agent/`)

**Provider (`provider/`)**
- OpenAI 兼容客户端: POST `/v1/chat/completions`
- Chat: 同步请求, 解析 JSON → `ChatResponse`
- ChatStream: SSE 流式, 通过 channel 推送 `ChatStreamChunk`
- Vision: multimodal 请求, base64 编码图片
- `ProviderGroup.SelectModel(type)`: 按类型选择可用模型

**MCP (`mcp/`)**
- 使用 `mark3labs/mcp-go` SDK
- `NewSSEMCPClient` → `Initialize` → ready
- `ListTools` → 转换 `mcp.Tool` → `ToolDefinition`
- `CallTool` → `mcp.CallToolResult` → `GetTextFromContent`

**Memory (`memory/`)**
- 短期记忆: 查询 `ChatRecord` 表最近 N 条 → `shortterm.ChatMessage`
- 长期记忆: 写入 `LongTermMemoryItem` 表, HotArea (LRU) 内存缓存
- Compact: 调用 LLM 摘要窗口内容 → 写入长期记忆
- 后台任务记忆: 内存 map + 结果缓冲 channel (256)

**Prompt (`prompt/`)**
- `text/template` 模板渲染, 支持 `{{.变量}}`
- 默认变量: `Time`, `UserName`, `GroupName`
- `BuildFullContext`: System + Personality + 长期记忆 + 工具描述

**Session (`session/`)**
- MessageHistory 存 Redis List (LPush/LRange)
- TokenUsage 累加 (gorm.Expr("token_usage + ?"))
- 与 ChatArea 1:1 绑定

**Tool (`tool/`)**
- `Tool` 接口: ID/Name/Description/Parameters/Execute/IsBuiltin/IsLongRunning
- `ToolRegistry`: map 存储, GetOpenAITools 转换
- 内置工具 (16 个):
  - OneBot11 (11): send_private_msg, send_group_msg, delete_msg, get_group_info,
    get_group_member_list, kick_group_member, ban_group_member, set_group_whole_ban,
    set_group_card, handle_friend_request, handle_group_request
  - 基础 (1): get_time
  - 沙箱 (3): browser_search, command_exec, code_exec (长耗时)
  - T2I (1): text_to_image (长耗时)
  - Vision (1): 识图 (仅 Image Model 配置时可用)
- 富文本消息: `BuildMessageFromJSON` 解析 string/[]Segment

**Skill (`skill/`)**
- 匹配策略: 关键词 (strings.Contains) + 正则 (regexp.Compile)
- 优先级排序: Priority 降序
- 系统技能 (`IsSystem=true`): 每次对话强制加载

### 4. 后台任务系统

**BackgroundTaskExecutor (`bgtask_executor.go`)**
- DAG 依赖解析: `TaskStep.Depends` 决定执行顺序
- `errgroup` 并发执行无依赖步骤
- 每步骤完成 → `OutputChan` 推送结果
- 状态持久化: pending → running → done/failed

**DrainerAgent (`drainer.go`)**
- 按 ChatArea 分组累积结果 (`pending map`)
- 进度通知: 每 3 个步骤完成 → 发送进度
- 最终整合: LLM 调用 → 友好自然语言 → 发送 QQ 消息

### 5. Plugin Engine (`internal/pluggin/`)

- gopher-lua 引擎, 每个插件独立 `lua.LState`
- 插件目录: `data/pluggins/<name>/`
- 清单: `pluggin.yaml` (YAML)
- 权限控制: `permissions` 字段 → API 注入白名单
- 事件拦截: `on_message(event)` → 返回 consumed 阻止 Agent 处理
- API 注入:
  - `log.info/warn/error` — 日志
  - `json.encode/decode` — JSON
  - `onebot11.send_private_msg/send_group_msg` — 消息发送
  - `http.get/post` — HTTP (预留)
  - 类型转换: Go ↔ Lua (map→table, slice→table, number, string, bool)

### 6. Web API (`internal/api/`)

- Hertz 框架: `server.Default(addr)`
- 中间件: Recovery + CORS + JWT Auth
- JWT: HS256, 24h 过期, `Claims{UserID, Username, Role}`
- 22 个路由组, 统一 JSON 响应: `{"code": 0, "data": ...}` / `{"code": 4xx, "msg": "..."}`
- 认证路由: `/api/v1/login`, `/api/v1/change-password` (前者无需 JWT)
- 管理路由 (需 JWT): adapter/providers/mcp/memory/prompts/sessions/skills/tools/plugins/acl/chat-records/overview/chat-areas

### 7. 基础设施 (`infrastructure/`)

**Postgres (`postgres/client.go`)**
- GORM 连接, Options 模式
- 连接池: 150 max open, 10 max idle, 1h lifetime

**Redis (`redis/client.go`)**
- go-redis v9, Options 模式
- 启动时 Ping 健康检查

**Sandbox (`sandbox/`)**
- Bay 沙箱 API 客户端
- 方法: Create/Get/List/ExtendTTL/KeepAlive/Stop/Delete
- 执行: ExecPython/ExecShell
- 文件: Read/Write/List/Delete/Upload/Download

**T2I (`t2i/`)**
- Text-to-Image API 客户端
- Generate (返回 ID) / GenerateImage (返回 bytes) / GenerateURL / GetImage
