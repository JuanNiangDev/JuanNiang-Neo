# JuanNiang-Neo 实现计划

## 项目当前状态

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/adapter/` | **已完成** | OneBot11 反向WS服务端、消息段、全部API封装 |
| `infrastructure/` | **已完成** | postgres/redis/sandbox/t2i 客户端 |
| `internal/agent/*/root.go` | **结构体已定义** | provider/mcp/memory/tool/skill 数据模型就绪 |
| `internal/agent/*/xxx.go` | **空桩** | 所有 CRUD 方法为空函数体 |
| `internal/agent/prompt/` | **仅注释** | 完全未实现 |
| `internal/agent/session/` | **空目录** | 完全未实现 |
| `internal/core/` | **全部空** | models/dao/cache/acl/handler 全空 |
| `internal/api/` | **全部空** | engine/middleware/router/service 全空 |
| `internal/pluggin/` | **仅包声明** | 完全未实现 |
| `cmd/server/main.go` | **TODO 桩** | 完全未实现 |

---

## 总体分层架构

```
cmd/server/main.go  (组装 & 启动)
    │
    ├── internal/adapter/      (OneBot11 WS → 事件)
    ├── internal/pluggin/      (Lua插件拦截/改写事件)
    ├── internal/agent/        (Agent核心: 会话/记忆/工具/技能)
    ├── internal/core/         (ACL | 缓存 | 数据库 | 模型)
    ├── internal/api/          (Web管理面板API)
    └── infrastructure/        (Postgres/Redis/Sandbox/T2I)
```

**事件流：**
```
OneBot11 → adapter(事件) → pluggin(拦截/改写) → agent(处理)
    → 调用Tool(含OneBot11 API / T2I / Sandbox) → 长任务→后台执行
    → bgtask Memory(缓冲区) → 排水Agent(消费) → 发送QQ消息
```

---

## Phase 1: 核心数据模型与基础设施 `core/`

> **目标：** 建立完整的 GORM 数据模型、DAO 封装、缓存封装、ACL 模块。
> 这是所有上层模块的基石，必须最先完成。

### 1.1 `core/models/` — 数据模型定义

定义所有 GORM 模型。所有模型带 `CreatedAt/UpdatedAt/DeletedAt` 时间戳。

| 模型 | 字段 | 说明 |
|------|------|------|
| `AdminUser` | ID, Username, PasswordHash, Role, CreatedAt, UpdatedAt | 管理员用户 (单用户, JWT登录) |
| `Provider` | ID(UUID), Type(ModelType), Name, Endpoint, Token, Model, Temperature, IsActive, CreatedAt, UpdatedAt | LLM Provider 配置 |
| `MCPServer` | ID(UUID), Name, ServerURL, Headers(JSON), Timeout, RetryCount, ToolFilter(JSON), AutoReconnect, IsActive, CreatedAt, UpdatedAt | MCP SSE 服务器配置 |
| `Skill` | ID(UUID), Name, Description, Keywords(JSON), RegexPattern, PromptRef, ToolRefs(JSON), McpRefs(JSON), IsActive, Priority, IsSystem, CreatedAt, UpdatedAt | 技能配置 |
| `ToolConfig` | ID(UUID), Name, Description, Parameters(JSON), Timeout, IsActive, IsBuiltin, CreatedAt, UpdatedAt | 工具配置 (内置工具也入库) |
| `Prompt` | ID(UUID), Name, Content, Type(system/personality/custom), IsActive, CreatedAt, UpdatedAt | 提示词模板 |
| `ChatArea` | ID(UUID), AreaType(private/group), TargetID(QQ号或群号), CreatedAt, UpdatedAt | 聊天区域 = Session+Memory的集合 |
| `Session` | ID(UUID), ChatAreaID(FK), Model, MessageHistory(JSON), TokenUsage(int), CreatedAt, UpdatedAt | 会话状态 |
| `ShortTermMemory` | ID(UUID), ChatAreaID(FK), WindowSize, AutoCompact(bool), Messages(JSON), CreatedAt, UpdatedAt | 短期记忆(滑动窗口, 可自动Compact) |
| `LongTermMemory` | ID(UUID), ChatAreaID(FK), HotAreaSize, HotMemoryTTL, Memories(JSON), CreatedAt, UpdatedAt | 长期记忆(热区存储) |
| `BackgroundTask` | ID(UUID), ChatAreaID(FK), Status(running/done/failed), Steps(JSON), Results(JSON), CreatedAt, UpdatedAt | 后台任务 |
| `ChatRecord` | ID, ChatAreaID(FK), UserID, Role(user/assistant/tool), Content, TokenCount, ToolCalls(JSON), CreatedAt | 聊天记录 |
| `Plugin` | ID(UUID), Name, Version, Path, Config(JSON), IsActive, CreatedAt, UpdatedAt | Lua插件元数据 |
| `ACLRule` | ID, UserID, ChatAreaID(FK), Permission(allowed/denied), Actions(JSON), CreatedAt, UpdatedAt | ACL规则 |

### 1.2 `core/dao/` — 数据库操作封装

封装 GORM 操作，每个模型对应的 DAO 函数。统一使用 `gorm.DB` 实例，通过构造函数注入。

```go
// 示例模式
type ProviderDAO struct { db *gorm.DB }
func NewProviderDAO(db *gorm.DB) *ProviderDAO
func (d *ProviderDAO) Create(p *models.Provider) error
func (d *ProviderDAO) GetByID(id string) (*models.Provider, error)
func (d *ProviderDAO) Update(p *models.Provider) error
func (d *ProviderDAO) Delete(id string) error
func (d *ProviderDAO) List(typ models.ModelType) ([]models.Provider, error)
```

需要实现的 DAO：
- `UserDAO` — 管理员用户 CRUD, 密码验证, 密码修改
- `ProviderDAO` — LLM Provider CRUD + 按类型查询
- `MCPServerDAO` — MCP 服务器 CRUD
- `SkillDAO` — Skill CRUD + 按关键词/正则查询
- `ToolConfigDAO` — Tool 配置 CRUD
- `PromptDAO` — Prompt CRUD + 按类型查询
- `ChatAreaDAO` — ChatArea CRUD
- `SessionDAO` — Session CRUD + 关联 ChatArea
- `MemoryDAO` — 短期/长期记忆 CRUD
- `ChatRecordDAO` — 聊天记录 CRUD, 分页查询, 按 ChatArea 查询
- `PluginDAO` — 插件元数据 CRUD
- `ACLDAO` — ACL 规则 CRUD + 按用户/ChatArea 查询

### 1.3 `core/cache/` — 缓存功能封装

封装 Redis 操作，提供统一的缓存接口：

```go
type Cache interface {
    Get(ctx context.Context, key string, dest any) error
    Set(ctx context.Context, key string, val any, ttl time.Duration) error
    Del(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, keys ...string) (int64, error)
    // 短期记忆专用: 列表操作
    LPush(ctx context.Context, key string, vals ...any) error
    LRange(ctx context.Context, key string, start, stop int64, dest any) error
    LTrim(ctx context.Context, key string, start, stop int64) error
    // 后台任务缓冲
    PubSub(ctx context.Context) *redis.PubSub
    Publish(ctx context.Context, channel string, msg any) error
}
```

### 1.4 `core/acl/` — 访问控制列表

```go
type ACL struct {
    dao  *dao.ACLDAO
    cache *cache.Cache
}

func (a *ACL) CheckPermission(userID int64, chatAreaID string, action string) (allowed bool)
func (a *ACL) AddRule(rule *models.ACLRule) error
func (a *ACL) RemoveRule(ruleID string) error
func (a *ACL) ListRules(userID int64) ([]models.ACLRule, error)
```

### 1.5 `core/` — 自动迁移 & 核心启动

在 `internal/core/core.go` 中提供：
- `AutoMigrate(db *gorm.DB) error` — 自动迁移所有模型
- `InitAdminUser(dao *dao.UserDAO) error` — 初始化管理员 (初始密码 `Admin123`)

---

## Phase 2: Agent 核心实现

> **目标：** 将所有 agent 子包的空桩填充完整，实现 Agent 的完整功能链路。

### 2.1 `agent/provider/` — LLM Provider 管理

扩展 `Provider` 接口，实现基于 OpenAI-compatible API 的客户端：

```go
type Provider interface {
    ID() string
    Name() string
    Type() ModelType
    Model() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatStreamChunk, error)
    // Image model 专用
    Vision(ctx context.Context, imageData []byte, prompt string) (string, error)
}
```

实现步骤：
1. 扩展 `Provider` 接口，定义 `ChatRequest`/`ChatResponse`/`ChatStreamChunk` 类型
2. 实现 `openAIProvider` 结构体，封装 HTTP 调用
3. 实现 `ProviderGroup` 的 CRUD 方法 (填充 provider.go 空桩)
4. 添加 `SelectModel(typ ModelType) (Provider, error)` 选择可用模型
5. Provider 状态与数据库同步

### 2.2 `agent/mcp/` — MCP SSE 客户端

实现 MCP (Model Context Protocol) SSE 客户端：

```go
type MCP interface {
    ID() string
    Name() string
    Connect(ctx context.Context) error
    Disconnect() error
    ListTools(ctx context.Context) ([]ToolDefinition, error)
    CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
    IsConnected() bool
}
```

实现步骤：
1. 定义 `ToolDefinition` 类型 (对应 MCP tool schema)
2. 实现 SSE 连接、心跳保活、自动重连
3. 实现 `tools/list` 和 `tools/call` MCP 协议方法
4. 实现 `MCPGroup` 的 CRUD 方法
5. MCP 工具注册为 Agent Tool 的逻辑

### 2.3 `agent/memory/` — 记忆系统

#### 2.3.1 短期记忆 (`shortterm/`)
- 基于 Redis List 的滑动窗口 (默认模式)
- `WindowSize` 条最近消息
- 每次新消息 push → 超出窗口则 trim (FIFO 淘汰)
- 支持 **手动 Compact** (压缩模式):
  - 调用 `Compact(ctx, chatAreaID)` → 将当前窗口中所有消息提交给 LLM 做摘要压缩
  - 压缩结果作为一条"记忆片段"写入长期记忆
  - 压缩完成后清空短期记忆窗口
  - Compact 是一个同步操作 (耗时可能较长, 由调用方决定时机)
  - 典型调用时机: 用户手动触发 / 窗口接近满时后台自动触发 / 会话切换话题时
- 通过 `core/cache` 操作 Redis

#### 2.3.2 长期记忆 (`longterm/`)
- 存储在 Postgres 中
- `HotAreaSize` 条热记忆在内存中 (LRU)
- `HotMemoryTTL` 过期淘汰
- 通过 `core/dao` 操作 Postgres
- 支持向量检索 (后续可选，先做关键词匹配)

**长期记忆写入条件 (何时向长期记忆中存入新记忆):**

| 触发条件 | 说明 |
|----------|------|
| **手动 Compact** | 用户或系统显式调用 `Compact`, 短期记忆压缩后摘要写入长期记忆 |
| **滑动窗口溢出** | 短期记忆窗口满时, 旧消息被踢出前可选择自动 Compact (可配置开关) |
| **用户显式要求** | 用户发 "/remember xxx" 或自然语言 "记住xxx" 时, Agent 识别意图后调用记忆写入 Tool |
| **重要事件触发** | 特定事件发生时写入: 用户自我介绍(名字/偏好)、Agent 完成重要任务、会话中提取到的结构化信息 |
| **跨会话持久化** | 同一个 ChatArea 的 Session 关闭/清空时, 可选择将短期记忆 Compact 后持久化

#### 2.3.3 后台任务记忆 (`bgtask/`)
- 当前在内存的 `map[string]BackGroundTaskMetaInfo` 需改造
- 添加缓冲区 (channel) 用于接收任务结果
- 持久化到数据库 (通过 `core/dao`)
- 暴露 `ResultChan() <-chan TaskResult` 供 Draine Agent 消费

#### 2.3.4 `MemoryGroup` 方法实现
填充 memory.go 中的空桩：
- `GetShortTermMemory(ctx, chatAreaID) -> []ChatMessage`
- `AddShortTermMemory(ctx, chatAreaID, msg ChatMessage)`
- `OverWriteShortTermMemory(ctx, chatAreaID, messages []ChatMessage)`
- `CompactShortTermMemory(ctx, chatAreaID) (summary string, error)` — **新增**: 调用 LLM 压缩窗口内容, 摘要写入长期记忆, 清空窗口
- `UpdateShortTermMemoryConfig(ctx, chatAreaID, conf)`
- `GetLongTermMemory(ctx, chatAreaID, query) -> []MemoryItem`
- `AddLongTermMemory(ctx, chatAreaID, item MemoryItem)`
- `UpdateLongTermMemoryConfig(ctx, chatAreaID, conf)`
- `AddBackGroundTask(ctx, chatAreaID, task)`
- `DelBackGroundTask(ctx, taskID)`
- `GetBackGroundTask(ctx, taskID) -> BackgroundTask`
- `ListBackGroundTasks(ctx, chatAreaID) -> []BackgroundTask`

### 2.4 `agent/prompt/` — 提示词系统

```go
type PromptTemplate struct {
    ID      string
    Name    string
    Type    PromptType  // system / personality / custom
    Content string      // 支持 {{.variable}} 模板变量
    IsActive bool
}

type PromptManager struct {
    dao *dao.PromptDAO
}

func (pm *PromptManager) RenderSystemPrompt(ctx context.Context, session *Session) (string, error)
func (pm *PromptManager) RenderPersonalityPrompt(ctx context.Context) (string, error)
func (pm *PromptManager) BuildFullPrompt(ctx context.Context, session *Session, userMsg string) ([]ChatMessage, error)
```

实现：
1. 定义 `PromptType` 枚举: `system`, `personality`, `custom`
2. 实现模板渲染 (使用 `text/template`)
3. `BuildFullPrompt` 组装: system prompt + personality prompt + 短期记忆 + 长期记忆 + 工具/技能描述 + 用户消息
4. 变量: `{{.UserName}}`, `{{.GroupName}}`, `{{.Time}}`, `{{.Tools}}`, `{{.LongTermMemories}}`, 等
5. 长期记忆上下文注入方式: 从长期记忆中检索与当前对话相关的条目, 以 `<long_term_memory>` 标记块注入 prompt

### 2.5 `agent/session/` — 会话管理

```go
type Session struct {
    ID            string
    ChatAreaID    string
    Model         string
    MessageHistory []ChatMessage // 当前会话的消息历史
    TokenUsage    int            // Token 统计
    // 会话状态
    CurrentSkill    *SkillConfig   // 当前激活的技能
    PendingActions  []PendingAction // 待处理的工具调用
}

type ChatMessage struct {
    Role       string      // system / user / assistant / tool
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string
    Name       string
}

type SessionManager struct {
    dao   *dao.SessionDAO
    cache *cache.Cache
}

func (sm *SessionManager) GetOrCreateSession(ctx context.Context, chatAreaID string) (*Session, error)
func (sm *SessionManager) AddMessage(ctx context.Context, sessionID string, msg ChatMessage) error
func (sm *SessionManager) GetHistory(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
func (sm *SessionManager) ClearSession(ctx context.Context, sessionID string) error
func (sm *SessionManager) UpdateTokenUsage(ctx context.Context, sessionID string, tokens int) error
```

实现：
1. 定义 `ChatMessage`, `ToolCall`, `PendingAction` 等类型
2. Session 与 ChatArea 一一对应
3. MessageHistory 存 Redis (快速读写), 定期同步到 Postgres
4. TokenUsage 累计统计

### 2.6 `agent/tool/` — 工具系统

#### 2.6.1 工具引擎

```go
type Tool interface {
    ID() string
    Name() string
    Description() string
    Parameters() openai.FunctionParameters
    Execute(ctx context.Context, args json.RawMessage) (string, error)
    IsBuiltin() bool
}

type ToolRegistry struct {
    mu    sync.RWMutex
    tools map[string]Tool
    dao   *dao.ToolConfigDAO
}

func (tr *ToolRegistry) Register(t Tool) error
func (tr *ToolRegistry) Unregister(toolID string) error
func (tr *ToolRegistry) GetOpenAITools() []openai.ChatCompletionToolParam
func (tr *ToolRegistry) Execute(ctx context.Context, call ToolCall) (string, error)
func (tr *ToolRegistry) ExecuteBackground(ctx context.Context, call ToolCall) (string, error) // 后台执行
```

#### 2.6.2 内置工具列表

| 工具名 | 实现 | 后台? |
|--------|------|-------|
| `send_private_msg` | 封装 `adapter.Provider.SendPrivateMsg` | 否 |
| `send_group_msg` | 封装 `adapter.Provider.SendGroupMsg` | 否 |
| `delete_msg` | 封装 `adapter.Provider.DeleteMsg` | 否 |
| `get_group_info` | 封装 `adapter.Provider.GetGroupInfo` | 否 |
| `get_group_member_list` | 封装 `adapter.Provider.GetGroupMemberList` | 否 |
| `kick_group_member` | 封装 `adapter.Provider.KickGroupMember` | 否 |
| `ban_group_member` | 封装 `adapter.Provider.BanGroupMember` | 否 |
| `set_group_whole_ban` | 封装 `adapter.Provider.SetGroupWholeBan` | 否 |
| `set_group_card` | 封装 `adapter.Provider.SetGroupCard` | 否 |
| `handle_friend_request` | 封装 `adapter.Provider.HandleFriendRequest` | 否 |
| `handle_group_request` | 封装 `adapter.Provider.HandleGroupRequest` | 否 |
| `get_time` | 返回当前时间 | 否 |
| `browser_search` | 调用 sandbox 执行浏览器搜索 | **是** |
| `command_exec` | 调用 sandbox 执行命令 | **是** |
| `code_exec` | 调用 sandbox 执行代码 | **是** |
| `text_to_image` | 调用 T2I 服务 | **是** |
| `vision` | 调用 Image Model 识图 (仅当Image模型配置时注册) | 否 |

#### 2.6.3 富文本消息发送

LLM 调用 `send_private_msg`/`send_group_msg` 时，参数支持 JSON 数组格式的消息段：
```json
{
  "user_id": 123456,
  "message": [
    {"type": "text", "data": {"text": "你好"}},
    {"type": "image", "data": {"file": "https://..."}},
    {"type": "at", "data": {"qq": "123456"}}
  ]
}
```
Tool 执行时解析此 JSON 并调用 `BuildMessage` 构建富文本。

### 2.7 `agent/skill/` — 技能系统

```go
type SkillEngine struct {
    dao    *dao.SkillDAO
    skills map[string]*SkillConfig  // 内存缓存
}

func (se *SkillEngine) Match(input string) (*SkillConfig, bool)  // 关键词+正则匹配
func (se *SkillEngine) Activate(ctx context.Context, skill *SkillConfig, session *Session) error
func (se *SkillEngine) Deactivate(ctx context.Context, session *Session) error
func (se *SkillEngine) GetSystemSkills() []*SkillConfig
```

实现：
1. 关键词匹配 (trie 树或简单遍历)
2. 正则匹配
3. 激活 Skill 时：将 Skill 的 PromptRef 注入 session, ToolRefs/McpRefs 的 tool 临时启用
4. 优先级排序 (Priority 越高越优先)
5. 系统 Skill 每次对话强制加载
6. 状态与数据库同步

### 2.8 `agent/agent.go` — HagoCenter 整合

扩展 `HagoCenter` 结构体，整合所有子模块：

```go
type HagoCenter struct {
    Pvd          *provider.ProviderGroup    // LLM Provider 群
    Mcp          *mcp.MCPGroup             // MCP 客户端群
    Memory       *memory.MemoryGroup       // 记忆系统
    Prompt       *prompt.PromptManager     // 提示词系统
    Session      *session.SessionManager   // 会话管理
    Tools        *tool.ToolRegistry        // 工具注册表
    Skills       *skill.SkillEngine        // 技能引擎
    ACL          *acl.ACL                  // 访问控制
    DrainerAgent *DrainerAgent             // 排水 Agent
    
    // 适配器引用（从 adapter layer 注入）
    Adapter      *adapter.Provider
}
```

---

## Phase 3: 后台任务与异步 Agent

> **目标：** 实现文档中的后台任务执行、排水 Agent、errgroup 并发模型。

### 3.1 后台任务执行器

```go
type TaskStep struct {
    ID       string
    ToolName string
    Args     json.RawMessage
    Depends  []string  // 依赖的步骤ID
    Status   StepStatus // pending/running/done/failed
    Result   string
    Error    string
}

type BackgroundTaskExecutor struct {
    tools     *tool.ToolRegistry
    bgtaskMem *bgtask.BackGroundTaskMemory
    resultCh  chan TaskStepResult  // 结果缓冲管道
    maxWorkers int
}

func (bte *BackgroundTaskExecutor) Submit(ctx context.Context, chatAreaID string, steps []TaskStep) (taskID string, err error)
```

实现：
1. 解析步骤依赖 → 构建 DAG
2. 无依赖步骤并发执行 (errgroup 模式)
3. 有依赖步骤等待前置步骤完成后执行
4. 每步骤完成 → 结果写入 `resultCh` 缓冲管道
5. 全部完成 → 标记任务完成
6. 状态持久化到数据库

### 3.2 排水 Agent (DrainerAgent)

```go
type DrainerAgent struct {
    resultCh    <-chan TaskStepResult
    llmProvider provider.Provider  // 专用 LLM Provider
    adapter     *adapter.Provider
    sessionMgr  *session.SessionManager
    promptMgr   *prompt.PromptManager
}

func (da *DrainerAgent) Run(ctx context.Context)  // 常驻 goroutine
```

实现：
1. 监听 `resultCh` (一个 ChatArea 一个 Agent, 串行处理)
2. 收到步骤结果 → 追加到会话上下文
3. 判断任务是否全部完成
4. 全部完成 → 调用 LLM 整合所有结果 → 发送 QQ 消息
5. 非全部完成 → 等待更多结果 (有 timeout)
6. 并发安全: 同一时间一个 ChatArea 只有一个 DrainerAgent 在处理

### 3.3 异步 Agent 模型

- 一个 Goroutine 处理聊天 (接收事件 → Agent 推理 → 触发工具调用)
- 长工具调用触发 → 挂载到 BackgroundTaskExecutor → 立即返回
- DrainerAgent 异步消费结果 → 整合 → 发送消息
- 多个 Agent 任务 (不同 ChatArea) 并发执行 (每个 ChatArea 独立的 goroutine)

---

## Phase 4: Web 管理面板 API `api/`

> **目标：** 实现基于 Hertz 的 Web 管理面板 API。

### 4.1 `api/engine/` — Hertz Web 引擎

初始化 Hertz 服务器，挂载中间件和路由：

```go
func NewEngine(cfg *EngineConfig) *route.Engine
```

配置项：监听地址、JWT Secret、OIDC 配置、跨域设置

### 4.2 `api/middleware/` — 中间件

| 中间件 | 说明 |
|--------|------|
| `JWTAuth` | JWT 签发与验证，保护管理 API |
| `OIDCProxy` | OIDC SSO 回调处理 |
| `RequestLog` | 请求日志记录 |
| `Recovery` | Panic 恢复 |
| `CORS` | 跨域支持 |

### 4.3 `api/router/` — 路由注册

```go
func RegisterRoutes(engine *route.Engine, services *ServiceBundle)
```

路由分组：
```
/api/v1
├── /auth
│   ├── POST /login          # 用户名+密码登录
│   ├── POST /refresh         # 刷新JWT
│   └── POST /change-password # 修改密码
├── /oidc
│   ├── GET  /login           # OIDC登录跳转
│   └── GET  /callback        # OIDC回调
├── /adapter
│   ├── GET  /                 # 获取适配器状态
│   ├── PUT  /                 # 更新适配器配置
│   └── POST /restart          # 重启适配器
├── /providers
│   ├── GET  /                 # 列出所有Provider
│   ├── POST /                 # 添加Provider
│   ├── PUT  /:id              # 修改Provider
│   └── DELETE /:id            # 删除Provider
├── /mcp
│   ├── GET  /                 # 列出所有MCP服务器
│   ├── POST /                 # 添加MCP
│   ├── PUT  /:id              # 修改MCP
│   └── DELETE /:id            # 删除MCP
├── /memory
│   ├── GET  /:chatAreaID/short-term  # 获取短期记忆配置
│   ├── PUT  /:chatAreaID/short-term  # 修改短期记忆配置
│   ├── GET  /:chatAreaID/long-term   # 获取长期记忆配置
│   └── PUT  /:chatAreaID/long-term   # 修改长期记忆配置
├── /prompts
│   ├── GET  /                 # 列出所有Prompt
│   ├── POST /                 # 添加Prompt
│   ├── PUT  /:id              # 修改Prompt
│   └── DELETE /:id            # 删除Prompt
├── /sessions
│   ├── GET  /                 # 列出所有Session
│   ├── GET  /:id              # 查看Session详情
│   └── DELETE /:id            # 清除Session
├── /skills
│   ├── GET  /                 # 列出所有Skill
│   ├── POST /                 # 添加Skill
│   ├── PUT  /:id              # 修改Skill
│   └── DELETE /:id            # 删除Skill
├── /tools
│   ├── GET  /                 # 列出所有Tool
│   └── PUT  /:id/toggle       # 启用/禁用Tool
├── /plugins
│   ├── GET  /                 # 列出所有插件
│   ├── POST /upload           # 上传插件zip
│   ├── PUT  /:id/toggle       # 启用/禁用插件
│   ├── DELETE /:id            # 删除插件
│   └── GET  /:id/config       # 获取插件配置
├── /acl
│   ├── GET  /                 # 列出所有ACL规则
│   ├── POST /                 # 添加ACL规则
│   └── DELETE /:id            # 删除ACL规则
├── /chat-records
│   ├── GET  /:chatAreaID      # 获取聊天记录(分页)
│   └── GET  /:chatAreaID/tools # 获取工具调用记录
├── /overview
│   └── GET  /                 # 全局概览(各模块统计)
└── /chat-areas
    └── GET  /                 # 列出所有ChatArea
```

### 4.4 `api/service/` — API 功能实现

每个 service 文件对应一个路由组，调用 DAO 层完成业务逻辑：

| Service | 对应功能 |
|---------|---------|
| `auth_service.go` | 登录、刷新、改密 (JWT + bcrypt) |
| `oidc_service.go` | OIDC SSO 处理 |
| `adapter_service.go` | 适配器状态管理 |
| `provider_service.go` | LLM Provider CRUD |
| `mcp_service.go` | MCP CRUD |
| `memory_service.go` | 记忆配置管理 |
| `prompt_service.go` | Prompt CRUD |
| `session_service.go` | 会话管理 |
| `skill_service.go` | Skill CRUD |
| `tool_service.go` | 工具启用/禁用 |
| `plugin_service.go` | 插件上传/管理 |
| `acl_service.go` | ACL 规则管理 |
| `chat_record_service.go` | 聊天记录查询 |
| `overview_service.go` | 全局概览统计 |
| `chat_area_service.go` | ChatArea 查询 |

---

## Phase 5: Lua 插件系统 `pluggin/`

> **目标：** 实现 Lua 插件引擎，支持热加载和 API 暴露。

### 5.1 插件引擎

```go
type PluginEngine struct {
    mu       sync.RWMutex
    plugins  map[string]*LoadedPlugin
    basePath string  // data/pluggins/
}

type LoadedPlugin struct {
    Config PluginManifest
    State  *lua.LState
    APIs   PluginAPI
}
```

### 5.2 插件清单 (`pluggin.yaml`)

```yaml
name: my-plugin
version: 1.0.0
author: author
description: "插件描述"
entry: main.lua
permissions:
  - onebot11
  - http
  - database
  - cache
  - agent
  - t2i
  - sandbox
```

### 5.3 Lua API 暴露

| API 分组 | 暴露的函数 |
|----------|-----------|
| `onebot11` | send_private_msg, send_group_msg, get_group_info, ... (全部 adapter API) |
| `http` | http_get, http_post, http_put, http_delete |
| `agent` | get_current_session, get_providers, get_tools, ... |
| `database` | db_query, db_exec (独立命名空间) |
| `cache` | cache_get, cache_set, cache_del (独立命名空间) |
| `t2i` | t2i_generate, t2i_get_image |
| `sandbox` | sandbox_exec, sandbox_search, sandbox_code |
| `log` | log_info, log_warn, log_error |

### 5.4 插件生命周期

1. Web 上传 zip → 解压到 `data/pluggins/<name>/`
2. 读取 `pluggin.yaml` → 解析清单
3. 加载 `entry` Lua 文件
4. 注册事件处理器: `on_message`, `on_notice`, `on_request`
5. 注入 API 函数
6. 热加载: 卸载旧 state → 重新加载
7. 插件拦截流程: 事件 → 匹配的插件 handlers → 可改写/拦截事件 → 传递给 Agent

---

## Phase 6: 主入口 & 整体串联 `cmd/server/`

> **目标：** 实现 `cmd/server/main.go`，将所有模块组装并启动。

### 6.1 启动流程

```go
func main() {
    // 1. 加载配置 (文件/环境变量)
    // 2. 初始化 Infrastructure
    //    - Postgres: 连接 + AutoMigrate
    //    - Redis: 连接 + Ping
    //    - Sandbox/T2I: 健康检查
    // 3. 初始化 Core
    //    - 创建所有 DAO
    //    - 创建 Cache
    //    - 创建 ACL
    //    - 初始化 Admin 用户 (首次启动)
    // 4. 初始化 Agent
    //    - 从 DB 加载 Providers → ProviderGroup
    //    - 从 DB 加载 MCP 配置 → MCPGroup
    //    - 创建 MemoryGroup
    //    - 创建 PromptManager
    //    - 创建 SessionManager
    //    - 注册内置 Tools → ToolRegistry
    //    - 从 DB 加载 Skills → SkillEngine
    //    - 创建 DrainerAgent
    //    - 组装 HagoCenter
    // 5. 初始化 Adapter (OneBot11)
    // 6. 初始化 Plugin Engine → 加载已安装插件
    // 7. 初始化 Web API (Hertz)
    // 8. 启动事件循环 (监听 OneBot11 事件 → 处理)
    // 9. 优雅退出 (signal handling)
}
```

### 6.2 事件处理流程

```go
func handleEvent(ctx context.Context, h *agent.HagoCenter, ev adapter.Event) {
    // 1. ACL 检查
    // 2. Plugin 拦截 (on_message / on_notice / on_request)
    //    如果插件返回 consumed=true, 跳过 Agent 处理
    // 3. 获取/创建 ChatArea
    // 4. 获取/创建 Session
    // 5. Skill 匹配 (关键词 + 正则)
    // 6. 组装上下文: 
    //    - System Prompts + Personality Prompt
    //    - 匹配的 Skill Prompt
    //    - 短期记忆 (滑动窗口)
    //    - 长期记忆 (相关检索)
    //    - 当前消息
    // 7. 构建 OpenAI Chat Completion 请求
    //    - 注入 Tools 列表 (当前启用的 tools + session skills 的 tools)
    // 8. 调用 LLM
    // 9. 处理响应:
    //    - 如果是文本: 记录到 ChatRecord → 发送 QQ 消息
    //    - 如果是 Tool Call:
    //        - 调用工具执行
    //        - 短工具: 同步执行 → 结果返回 LLM → 继续
    //        - 长工具: 提交到 BackgroundTaskExecutor → 返回
    // 10. 更新 Session (MessageHistory, TokenUsage)
    // 11. 持久化聊天记录
}
```

---

## Phase 7: 测试、文档与收尾

- 单元测试 (dao/cache/acl 层)
- 集成测试 (API 路由)
- 配置文件模板 `config/config.yaml`
- Docker 部署 `deployments/`
- 数据库初始化脚本 `sql/`

---

## 需要确认的问题

在开始实现前，以下几个技术选型和设计细节需要确认：

1. **Lua 引擎:** 推荐使用 `github.com/yuin/gopher-lua` (最流行的 Go-Lua 绑定)，是否合适？

2. **MCP 客户端:** 是否需要使用现成的 Go MCP SDK (如 `mark3labs/mcp-go`)，还是自己实现 MCP SSE 协议？考虑到控制力，建议自己实现或使用官方 `github.com/modelcontextprotocol/go-sdk`。

3. **Hertz 版本:** 使用 `github.com/cloudwego/hertz` 对吗？当前 go.mod 还没有这个依赖。

4. **Image Model 作为 Tool 的细节:**
   - 工具名 `vision`, 参数 `image_url` + `prompt`
   - Text模型需要识图时 → 调用 `vision` tool → Image Model 处理 → 返回文本
   - Image Model 未配置时 → `vision` tool 返回 "未配置识图模型"
   - 这个理解对吗？

5. **Session 与 Memory 的关系:** Session 存当前对话历史(类似 OpenAI messages 数组)，Memory 存跨会话的长期信息。Session 历史的 token 上限是多少？超出时如何处理(截断/摘要)？

6. **长期记忆的检索方式:** 先做基于关键词+向量的简单余弦相似度，还是后续再上向量数据库？目前计划用 Postgres 存储。

7. **后台任务的重启恢复:** 如果服务重启，正在执行的后台任务怎么处理？标记为 failed 并通知用户，还是重新执行？

8. **配置文件格式:** 使用 YAML (config.yaml) 还是环境变量为主？建议两者都支持。
