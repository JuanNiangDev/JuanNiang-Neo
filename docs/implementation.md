# 实现细节

## 核心模块实现

### 1. Adapter (`internal/adapter/`)

**WebSocket 服务器 (`server.go`)**
- 使用 `RomiChan/websocket` 实现 WS 升级
- `wsServer` 管理多个 `wsConn` (每个 OneBot11 客户端一个连接)
- `callAPI()`: 同步等待, echo 匹配响应, 10s 超时
- `readLoop()`: 循环读取 → gjson 解析 post_type → 分发到 events chan (buffer 128)
- 鉴权: `Authorization: Bearer <token>` header 或 `?access_token=<token>` query
- **`newWSServer` 上下文隔离**: 内部使用 `context.WithCancel(context.Background())` 而非传入的 `ctx`，避免上层 `SyncConfig` 的 5s 超时 ctx cancel 后级联取消 ws server 导致服务立即停止；ws server 生命周期完全由 `wsServer.stop()` 控制
- **`wsConn.remoteAddr` 字段**: 在 `handleWS` 握手时记录 `r.RemoteAddr`，供后续 `connDetails()` 返回连接 IP
- **`ConnDetail` 结构**: `{ID int64, IP string, Self int64}`，由 `connDetails()` 返回所有连接的展示信息（供前端"查看详情"按钮）
- **`ProviderStatus.Conns`**: 新增字段，类型 `[]ConnDetail`，`Status()` 调用 `server.connDetails()` 填充

**监听地址规范化 (`adapter.go`)**
- `listenAddr()` 兼容三种 `cfg.Addr` 形态，始终返回 `net.Listen` 可直接使用的 `host:port` 串：
  - `""` 空串 → `":<port>"`（仅端口）
  - `"host:port"` 标准形式 → 原样返回
  - `":port"` 仅端口 → 原样返回
  - `"host"` 仅 host → 追加端口 `"<host>:<port>"`（WebUI 更新时常见）
  - 含冒号但尾部非数字（如 IPv6 边界）→ 追加端口
- `Status().ListenAddr` 也通过此函数计算，保证前端展示与实际监听地址一致

**生命周期管理 (`adapter.go`)**
- `Start()`: 若 `events` channel 已被 `Stop` 关闭（nil），重建新 channel，避免后续推送 panic
- `Stop()`: 整个操作放入 goroutine + `context` 超时控制，避免死锁；关闭 events 后置 nil，避免二次 close panic（修复了曾经的 "close of closed channel" bug）；`Start` 时重新创建 events channel
- `Restart()`: 5s 超时下顺序调用 `Stop` → `Start`
- **`SyncConfig` 简化**: 启用 → 先 Stop 再 Start 重启加载新配置；停用 → 仅 Stop。删除了原有的"若配置未变则跳过"分支，逻辑更直接
- `Admins()` 方法: 返回当前管理员列表，供事件透传到插件
- `AdminQQNumbers` 字段持久化到 `Onebot11Adapter` 表（GORM `serializer:json`），由 `UpdateAdapterConfig` 同步 DB 与运行时

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
- 内容直接拼接，不再使用 `text/template` 渲染（已删除 `RenderTemplate` / `GetDefaultVars` / `Variables` 字段）
- **`IsSystem bool` 字段**: `models.Prompt` 中新增字段，标记系统锁定提示词，AutoMigrate 自动添加该列
- **`SystemLockedPromptName = "__system_locked__"`** 常量: 系统锁定提示词的固定 name，用于幂等查询
- **`SystemLockedPromptContent`** 常量: 全局行为约束文本（消息格式 / 回复策略 / @与权限层级 / 安全与稳健），随二进制分发
- **`EnsureSystemPrompt(ctx)` 幂等播种**: `HagoCenter.Init` 启动时调用一次
  - 若 DB 中已存在 `name = __system_locked__` 的记录，且内容/`IsSystem` 与代码版本不一致 → 覆盖更新（保持提示词与二进制同步）
  - 若不存在 → 创建一条 `IsSystem=true, Type=system, IsActive=true` 的记录
  - `not found` / `no rows` 错误走创建分支，其它错误才告警
- **`BuildSystemPrompt` 拼接优先级**（高 → 低，内容直接 `strings.Join("\n\n")`）：
  1. `dao.Prompt.ListSystemLocked()` — 所有 `IsSystem=true` 的记录（强制拼接，不受 `IsActive` 影响）
  2. `dao.Prompt.ListByType(System)` — 常规 system 提示词，跳过 `IsSystem=true` 避免重复
  3. `dao.Prompt.ListByType(Personality)` — 人格设定
  4. `dao.Prompt.ListByType(Custom)` — 用户自定义补充
- **API 守卫**: `UpdatePrompt` / `DeletePrompt` / `TogglePrompt` 在 Service 层检查 `existing.IsSystem`，命中则返回 `40029 PromptIsSystem`；`AddPrompt` / `UpdatePrompt` 拒绝用户将 `Type` 设为 `system`
- `BuildFullContext`: SystemPrompt + 长期记忆 + 工具描述，内部调用 `BuildSystemPrompt`
- **群聊回复策略**（在 `SystemLockedPromptContent` 中定义）:
  - **被@铁律**: 当消息中包含@本机器人时，无条件回复，覆盖所有静默规则
  - **相关性判断**: 不满足被@铁律时，综合以下条件决定是否回复：
    - **条件 B** — 消息内容与机器人知识/能力相关（可直接回答或提供价值）
    - **条件 C** — 当前对话上下文中机器人刚参与过相关话题（延续讨论）
    - **条件 D** — 群聊处于热聊状态且内容适合自然参与（融入对话）
  - **静默标记 `__NO_REPLY__`**: 当 LLM 判断无需回复时，系统提示词要求其输出 `__NO_REPLY__` 标记；系统层检测到该标记后丢弃响应（不发消息、不记聊天记录、不存短期记忆）；仅限群聊场景，私聊始终回复

**Session (`session/`)**
- MessageHistory 存 Redis List (LPush/LRange)
- TokenUsage 累加 (gorm.Expr("token_usage + ?"))
- 与 ChatArea 1:1 绑定

**Tool (`tool/`)**
- `Tool` 接口: ID/Name/Description/Parameters/Execute/IsBuiltin/IsLongRunning
- `ToolRegistry`: map 存储, GetOpenAITools 转换
- 内置工具 (18 个):
  - OneBot11 (12): send_private_msg, send_group_msg, delete_msg, get_group_info,
    get_group_member_list, kick_group_member, ban_group_member, set_group_whole_ban,
    set_group_card, handle_friend_request, handle_group_request, send_face
  - 查询 (1): list_super_faces
  - 基础 (1): get_time
  - 沙箱 (3): browser_search, command_exec, code_exec (长耗时)
  - T2I (1): text_to_image (长耗时，返回图片 URL 由主 Agent 组装发送)
  - Vision (1): 识图 (仅 Image Model 配置时可用)
- 富文本消息: `BuildMessageFromJSON` 解析 string/[]Segment
- **内置工具 ID 前缀 `builtin:`**: `Service.ListTools` 合并 `ToolRegistry.List()` 内置工具与 DB `ToolConfig`：
  - 内置工具响应 `ID = "builtin:" + name`，`IsBuiltin = true`，`IsActive = true`（运行时常驻）
  - 若 DB 中存在同名 `ToolConfig` 记录，则合并其 `ID / IsActive / CreatedAt`（让前端可识别 DB 持久化状态）
  - DB 中非内置条目（用户自定义工具）追加在列表末尾
- **`ToggleTool` 守卫**: 收到 `builtin:` 前缀的 ID 时直接返回 `40030 ToolIsBuiltin`，拒绝切换内置工具状态（内置工具运行时始终在注册表中，不支持启停）；非内置工具停用时从 `ToolRegistry` 移除，启用时不重复注册（已在 init 时注册）

**Skill (`skill/`)**
- 匹配策略: 关键词 (strings.Contains) + 正则 (regexp.Compile)
- 优先级排序: Priority 降序
- 系统技能 (`IsSystem=true`): 每次对话强制加载

**CronJob 定时任务 (`agent/cronjob/`)**
- `Manager` 结构: 封装 `robfig/cron/v3` 调度器，管理定时任务生命周期
- `Run(ctx)`: 启动 cron 调度器，监听 `ctx.Done()` 实现优雅退出
- `Reload(ctx)`: 从 DB 重新加载所有启用的任务 (`IsActive=true`)，清空并重建 cron 条目
- `makeJobFunc(task)`: 构造 `MessageEvent`（触发者为空）→ 封装为 `Event{PostType:"cronjob", IsCronJob:true}` → 非阻塞写入 `CronJobEvents` channel
- **事件处理**: `processEvent` 中对 `IsCronJob=true` 的事件跳过 Plugin 拦截和 ACL 检查，直接进入 Agent 处理链

**群聊静默过滤器**
- `SilenceToken = "__NO_REPLY__"` 常量（`internal/agent/event.go`）
- `isSilenceResponse(content)`: 双路径检测
  - 主路径: `strings.Contains(content, SilenceToken)` 精确匹配静默标记
  - 兜底路径: 匹配静默短语/emoji 列表（LLM 未按要求输出标记时的容错）
- `handleMessage` 中: 群聊场景 + 静默响应 → 丢弃该响应（不发 QQ 消息、不记录 `ChatRecord`、不写入短期记忆），同时跳过 ToolCalls 结果处理；`StrategyAlways` 模式下跳过静默检测
- 兜底短语列表: `保持静默` / `不回复` / `不响应` / `做空气` / `装死` / `😶` / `🤐` 等

**回复策略 (`reply_strategy.go`)**
- `ReplyStrategyConfig` GORM 单例行，持久化于 `reply_strategy_config` 表
- 五种策略模式: `never_reply` / `at_only` / `always` / `plugin_only` / `relevance`
- `relevanceAgentEvaluate()`: `StrategyRelevance` 的核心，调用独立 LLM（不影响主对话）评估消息相关性：
  - 文本消息: 取最近 10 条短期记忆 + 发送者信息，构造`你是群聊判断者` prompt，调用 Text Provider（temperature=0.3）
  - 图片消息: 有 Vision Provider 时走图片分析 prompt；无则返回 score=0
  - 解析 LLM 返回的 `{"relevance": float, "reason": string}` JSON
  - `extractRelevanceJSON()` 容错: 正则分别提取 `relevance` 和 `reason`，容忍 reason 中的未转义引号
- `isAtSelf(rawMsg)`: 检测 `[CQ:at,qq=<selfQQ>]`，selfQQ 优先运行时 `Adapter.SelfID()`，fallback 到启动时缓存的 `SelfQQ`
- `isPluginCommand(rawMsg)`: 委托到 `PluginEngine.HasPluginCommand`（trie 存在性检查，不执行命令）
- 策略配置通过 `GET/PUT /api/v1/reply-strategy` 热更新，`loadReplyStrategy()` 在 `HagoCenter.Init` 时初始化
- `BotName` 字段: 用户自定义机器人名字（如 "小卷"），注入相关性和系统 prompt 用于辅助判断

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
- 清单: `pluggin.yaml` (YAML)，新增 `system: true` 字段标记系统插件
- **Lua SDK (`internal/pluggin/sdk/jn.lua`)**: 由 `//go:embed` 内嵌的二进制内置 SDK
  - `ensureEmbeddedAssets()` 在 `LoadAll()` 启动时落盘到 `data/pluggins/sdk/jn.lua`（每次启动总是覆盖，保持与二进制版本同步）
  - `injectSDK(L, pluginName)` 在每条 LState 中将 SDK 目录追加到 `package.path`，使插件可通过 `local jn = require("jn")` 引入
  - SDK 仅捕获 Go 注入的全局表（`log/json/onebot11/http/database/cache/t2i/sandbox/agent`）并重新暴露为模块字段（`jn.log` / `jn.json` / `jn.onebot11` 等），不引入额外行为；附带 sumneko lua-language-server 的 `---@class` / `---@field` 类型注解，IDE 可获得完整代码提示
  - `jn.command.register(path, handler, opts?)` 是命令注册的入口，内部委托到 Go 侧 `__jn_internal.register_command` 全局函数
- **多级命令系统 (`command.go`)**: `CommandRegistry` 维护一棵 `CommandNode` 树
  - `Register(plugin, path, opts, handler)`: 按路径逐级创建节点，叶节点持有 `handler` 与 `pluginName`
  - `Dispatch(raw, event)`: 解析 `/cmd subcmd args...` → 沿树最长前缀匹配 → 调用命中 handler，args 为命中路径之后的所有 token；命中后 `consumed=true` 跳过 Agent，`reply` 非空时由 `PluginEngine.sendReply` 自动回复
  - 未命中 handler 但停在某个非根节点 → 列出该节点子命令作为提示回复
  - `UnregisterPlugin(plugin)`: 卸载插件时递归清除其注册的所有命令
  - `ListByPlugin(plugin)`: 返回指定插件注册的所有叶命令路径（供 `ListMaps` 附加到插件响应）
  - `FormatHelp(path)`: 生成帮助文本，被内置 `/help` 命令调用
- **内置 `/help` 命令**: `registerBuiltinCommands()` 在 `NewPluginEngine` 时注册到 `system` 插件名下，路径 `["system", "help"]`；调用 `commands.FormatHelp(args)` 输出顶层命令列表或子命令详情
- **`system` 系统插件 (`internal/pluggin/systemplugin/`)**: 由 `//go:embed` 内嵌 `pluggin.yaml` + `main.lua`
  - 清单中 `system: true`，`ensureEmbeddedAssets()` 仅在 `data/pluggins/system/pluggin.yaml` 不存在时落盘（允许用户自定义修改）
  - `main.lua` 通过 `require("jn")` + `jn.command.register` 注册一组系统命令：`/system status`、`/system provider`、`/system mcp`、`/system tool`、`/system memory`、`/system t2i`、`/system sandbox`、`/system session` 等
- **系统插件三层守卫**:
  1. `Manifest.System` 字段（YAML 中 `system: true`）
  2. `PluginEngine.IsSystem(name)` 读取已加载 manifest 的 `System` 字段
  3. Service 层守卫：`TogglePlugin` 拒绝停用系统插件（返回 `40028 PluginIsSystem`，但允许"启用"以支持幂等场景）、`DeletePlugin` 拒绝删除系统插件（同样返回 `40028`）
  - 另外 `PluginEngine.Unload(name)` 也拒绝卸载 `Manifest.System=true` 的插件
- **`ListMaps()` 增强**: 返回的 `[]map[string]any` 每项包含
  - `name` / `version` / `author` / `description` — 来自 manifest
  - `permissions` — manifest.Permissions 数组
  - `is_system` — manifest.System 布尔
  - `is_active` — 已加载即视为 true
  - `commands` — `commands.ListByPlugin(name)` 返回的 `[]PluginCommandInfo`（path / description / usage / is_leaf）
- **9 组 Lua API (按权限注入)**:
  - `log.*` — 日志 (始终可用, 3 函数: info/warn/error)
  - `json.*` — JSON 编解码 (始终可用, 2 函数: encode/decode)
  - `onebot11.*` — 20 个函数: 消息发送、群管理、信息查询、请求处理
  - `http.*` — HTTP GET/POST (30s 超时)
  - `database.*` — 原始 SQL 执行, 表名前缀 `pluggin_<name>_` 隔离
  - `cache.*` — Redis KV, Key 前缀 `pluggin:<name>:` 隔离
  - `t2i.*` — 5 个函数: `generate` / `generate_url` / `toggle` / `is_active` / `get_config`；服务 nil → `generate` 返回 `"T2I 服务未启用"`，`toggle/is_active/get_config` 委托到 `AgentOperator`
  - `sandbox.*` — 6 个函数: `create` / `exec_shell` / `exec_python` / `toggle` / `is_active` / `get_config`；服务 nil → 返回 error
  - `agent.*` — 16 个函数: 7 个配置查询（providers/mcp_servers/skills/sessions/prompts/tools/plugins）+ 4 个 Provider/MCP/Tool 运行时管理 + 2 个 Provider 切换 + `get_current_chat_area` + `compact_memory` + 2 个 list_runtime_providers/switch_provider
- **服务开关检测**: T2I/Sandbox 为 nil 时 Lua 函数返回明确错误, 无需配置；运行时通过 `agentOp.GetT2IClient()` / `agentOp.GetSandboxClient()` 获取最新实例，支持热更新
- **`AgentOperator` 接口（增强）**: `HagoCenter` 实现，提供：
  - `SetProviderActive(ctx, id, active)` / `SetMCPActive(ctx, id, active)` / `SetToolActive(ctx, name, active)`
  - `SwitchProvider(ctx, id)` — 切换主 Provider
  - `SetT2IActive(ctx, active)` / `SetSandboxActive(ctx, active)` — 启停 T2I/Sandbox
  - `CompactMemory(ctx, chatAreaID)`
  - `GetChatAreaID(userID, groupID, messageType) string`
  - `GetProviderGroup() ProviderGroupAccess` — 暴露 `List()` / `GetActive(id)`
  - `GetMCPGroup() MCPGroupAccess` — 暴露 `ListMCPs()` / `IsConnected(id)`
  - `GetToolRegistry() ToolRegistryAccess` — 暴露 `ListTools()` / `IsActive(name)`
  - `GetT2IClient()` / `GetSandboxClient()` — 返回运行时最新客户端
- 事件拦截: `on_message(event)` → 返回 consumed 阻止 Agent 处理
- **`OnMessage` 命令优先**: 若 `event.RawMessage` 以 `/` 开头，先调用 `commands.Dispatch`，命中则自动回复并 `consumed=true`，未命中再 fallback 到插件的 `on_message` 回调
- `currentEv` 上下文: `OnMessage` 时存储当前事件, 供 `agent.get_current_chat_area()` 查询
- `AdapterWrapper`: 将 `adapter.Provider` 强类型接口包装为 `SendAdapter` (map 返回值)
- 并发安全: `RWMutex` + 独立 LState

### 6. Web API (`internal/api/`)

- Hertz 框架: `server.Default(addr)`
- 中间件: Recovery + CORS + JWT Auth
- JWT: HS256, 24h 过期, `Claims{UserID, Username, Role}`
- 22 个路由组, 统一 JSON 响应: `{"code": 0, "data": ...}` / `{"code": 4xx, "msg": "..."}`
- 认证路由: `/api/v1/login`, `/api/v1/change-password` (前者无需 JWT)
- 管理路由 (需 JWT): adapter/providers/mcp/memory/prompts/sessions/skills/tools/plugins/acl/chat-records/overview/chat-areas
- **新增错误码**:
  - `40028 PluginIsSystem` — 系统插件禁止停用/删除
  - `40029 PromptIsSystem` — 系统锁定提示词禁止修改/删除，或用户尝试创建 `Type=system` 的提示词
  - `40030 ToolIsBuiltin` — 内置工具（`builtin:` 前缀 ID）禁止启停
- **`GetLogs` 返回顺序**: 最近 250 条日志，最新写入的排最前（反转为倒序），前端无需自行排序
- **`StreamLogs` SSE**: 实时推送新增日志到前端

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
