# JuanNiang-Neo Web API 文档

**Base URL:** `http://localhost:8090/api/v1`
**Content-Type:** `application/json`（上传文件为 `multipart/form-data`）

## 统一响应格式

所有接口返回 `FinalResponse`：

```json
{ "status": 0, "info": "OK", "data": <任意类型或 null> }
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | uint | 0=成功，非 0=错误码 |
| `info` | string | 状态描述（成功为 `"OK"`，失败为错误信息） |
| `data` | any | 业务数据；失败时可为 `null` 或 `{"error_detail": "..."}` |

> 注意：逻辑错误也使用 HTTP 200，全部以信封中的 `status` 判定结果。

### 错误码表

| Code | 说明 |
|------|------|
| 0 | 成功 |
| 40001 | 参数格式错误（BindJSONErr） |
| 40002 | 用户名或密码错误 |
| 40003 | token 生成失败 |
| 40004 | 用户不存在 |
| 40005 | 原密码错误 |
| 40006 | 密码更新失败 |
| 40007 | 无效的 QQ 号 |
| 40008 | adapter 未初始化 |
| 40009 | provider 不存在 |
| 40010 | MCP 服务器不存在 |
| 40011 | Session 不存在 |
| 40012 | 缺少上传文件 |
| 40013 | 临时文件创建失败 |
| 40014 | 文件写入失败 |
| 40015 | 无效的 ZIP 文件 |
| 40016 | 无效的 ACL ID |
| 40017 | onebot11 适配器配置更新失败 |
| 40018 | Skill 不存在 |
| 40019 | Prompt 不存在 |
| 40020 | Tool 不存在 |
| 40021 | Plugin 不存在 |
| 40022 | 插件加载失败 |
| 40023 | ChatArea 不存在 |
| 40024 | Memory 配置不存在 |
| 40025 | adapter 配置不存在 |
| 40026 | T2I 配置不存在 |
| 40027 | Sandbox 配置不存在 |
| 40028 | 系统插件不允许删除或停用 |
| 40029 | 系统提示词不允许修改或删除 |
| 40030 | 内置工具运行时常驻，不支持启停 |
| 40031 | CronJob 不存在 |
| 40032 | 回复策略配置不存在 |
| 50000 | 服务器内部错误 |

## 认证

除 `POST /login` 和根路径下的 `GET /health` 外，**所有接口需要** `Authorization: Bearer <token>` 头。Token 由 `POST /login` 获取。
系统初始化时默认账号 `admin / Admin123`，首次启动后请尽快通过 `POST /change-password` 修改。

`JWT_SECRET` 用于 HMAC 签名；Token 有效期 72 小时（`internal/api/middleware/auth.go`）。

---

## 通用数据类型

| 类型 | JSON 形式 | 说明 |
|------|-----------|------|
| `JSONMap` | `{"k":"v"}` | `map[string]any`（GORM jsonb） |
| `JSONSlice` | `["a","b"]` | `[]string`（GORM jsonb） |
| `time.Time` | RFC3339 字符串 | `"2026-07-20T12:00:00Z"` |

**枚举类型**

- `ModelType`: `text_model` | `image_model` | `embedding_model`
- `PromptType`: `system` | `personality` | `custom`（`system` 保留给系统锁定提示词，禁止新建）
- `AreaType`: `private` | `group`
- `ACLScope`: `chat` | `tool` | `mcp`（**当前仅 `chat` 生效**，`tool`/`mcp` 为历史保留）
- `ACLPermission`: `allow` | `deny`
- `ACLTargetType`: `all` | `list`（`list` 时 `user_ids` 才有效）
- `ReplyStrategy`: `never_reply` | `at_only` | `always` | `relevance`

**ACL 语义**（当前仅聊天黑名单）：无规则=允许所有；仅 `deny` 规则生效（`all`=禁止所有人、`list`=禁止指定 `user_ids`）；`allow` 规则不再生效；Admins 列表中的用户绕过 ACL。

---

## 目录

1. [认证](#1-认证) · [健康检查](#2-健康检查)
2. [Adapter](#3-adapter) · [Webhook](#17-webhook)
3. [Providers](#4-providers) · [MCP](#5-mcp) · [Tools](#10-tools) · [Skills](#9-skills)
4. [Prompts](#7-prompts) · [Sessions](#8-sessions)
5. [Memory](#6-memory) · [聊天记录](#12-聊天记录) · [Chat Areas](#13-chat-areas) · [Overview](#14-overview)
6. [Plugins](#11-plugins) · [ACL](#15-acl)
7. [T2I](#18-t2i) · [Sandbox](#19-sandbox)
8. [Logs](#16-日志) · [Agent 活跃循环](#20-agent-活跃循环) · [CronJob](#21-cronjob) · [回复策略](#22-回复策略)

---

## 1. 认证

### POST /login
管理员登录，返回 JWT。

**Body** `LoginReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `username` | string | 是 | 用户名 |
| `password` | string | 是 | 明文密码 |

**data** `TokenResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `token` | string | JWT，放入后续 `Authorization: Bearer <token>` |

```bash
curl -X POST http://localhost:8090/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin123"}'
# {"status":0,"info":"OK","data":{"token":"eyJhbGciOi..."}}
```

### POST /change-password
修改当前登录用户密码。

**Body** `ChangePasswordReq`: `old_password` string、`new_password` string（均必填）。
**data** `null`。

---

## 2. 健康检查

### GET /health
（**不在 `/api/v1` 前缀下**，无需认证）服务存活检查。

```json
{"status":"ok"}
```

---

## 3. Adapter

OneBot11 反向 WebSocket 适配器状态查询与配置更新。

> 说明：`listen_addr` 由 `Adapter.listenAddr()` 规范化为 `host:port`；管理员 QQ 列表持久化在 DB 的 `AdminQQNumbers` 字段；`SyncConfig` 在启用时 Stop+Start 重启，禁用时仅 Stop。

### GET /adapter
返回适配器运行状态（不含配置）。

**data** `AdapterStatus`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `running` | bool | 是否在运行 |
| `listen_addr` | string | 规范化后的 `host:port` |
| `self_id` | int64 | 机器人 QQ |
| `conn_count` | int | WS 连接数 |
| `conn_ids` | int64[] | 已连接客户端 QQ 列表 |
| `conns` | ConnDetail[] | 每条连接详情 `{id, ip, self_id}` |

### GET /adapter/config
读取持久化的适配器配置。

**data** `AdapterConfigResp`: `addr` string、`port` int、`token` string、`admin_qq_numbers` string[]、`enabled` bool。

### PUT /adapter
更新适配器配置并同步到运行时。

**Body** `UpdateAdapterConfigReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `addr` | string | 是 | 监听地址（支持 host / `:port` / `host:port`） |
| `port` | int | 是 | 监听端口 |
| `token` | string | 是 | OneBot access token |
| `admin_qq_numbers` | string[] | 是 | 管理员 QQ 列表 |
| `enabled` | bool | 是 | 是否启用 |

**data** `null`。

### POST /adapter/restart
重启 OneBot11 适配器。**data** `null`。

---

## 4. Providers

LLM Provider (text/image/embedding) CRUD。同类型只能一个 Active，激活时自动停用同类型其他 Provider。

### GET /providers
列出所有 Provider。

**data** `ProviderResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `type` | ModelType | 类型 |
| `endpoint` | string | API 地址 |
| `token` | string | API token |
| `model` | string | 模型名 |
| `temperature` | float32 | 温度（默认 0.7） |
| `is_active` | bool | 是否激活 |
| `created_at` | time | 创建时间 |

### GET /providers/:id
获取单个 Provider。`data` `ProviderResp`。

### POST /providers
新增 Provider。若 `is_active=true` 自动停用同类型其他 Provider。

**Body** `AddProviderReq`: `name`、`type`、`endpoint`、`token`、`model`（均必填），`temperature` float32（可选），`isActive` bool（必填）。

**data** `ProviderResp`（含生成的 UUID）。

### PUT /providers/:id
覆盖更新。**Body** `UpdateProviderReq`（同 Add）。**data** `null`。

### DELETE /providers/:id
删除 Provider 并从运行时 ProviderGroup 移除。**data** `null`。

### PUT /providers/:id/toggle
启停 Provider。

**Body** `ToggleProviderReq`: `is_active` bool（必填）。**data** `null`。

---

## 5. MCP

MCP（Model Context Protocol，SSE 传输）服务器配置 CRUD，支持运行时连接/断开。

### GET /mcp
列出所有 MCP 服务器。注意：返回会合并运行时连接状态。

**data** `MCPServerResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `server_url` | string | SSE 端点 |
| `headers` | JSONMap | 自定义请求头 |
| `timeout` | int | 超时毫秒 |
| `retry_count` | int | 重试次数 |
| `tool_filter` | string[] | 工具白名单（空=全量） |
| `auto_reconnect` | bool | 自动重连 |
| `is_active` | bool | 是否激活 |
| `created_at` | time | 创建时间 |

### GET /mcp/:id
获取单个。**data** `MCPServerResp`。

### POST /mcp
新增 MCP。若 `is_active=true` 立即建立 SSE 连接。

**Body** `AddMCPServerReq`：`name`、`server_url`（必填）；`headers` JSONMap、`timeout` int、`retry_count` int、`tool_filter` string[]、`auto_reconnect` bool（可选）；`is_active` bool（必填）。

**data** `MCPServerResp`。

### PUT /mcp/:id
覆盖更新。断开旧连接，若 `is_active=true` 重新建立 SSE。**data** `MCPServerResp`。

### DELETE /mcp/:id
断开连接并删除。**data** `null`。

### GET /mcp/:id/check
实时检测指定 MCP SSE 连接状态。**data** `{"connected": bool}`。

> 注：`GET /mcp/:id/check` 与 `PUT /mcp/:id/toggle` 配合使用，前者探活，后者启停。

### PUT /mcp/:id/toggle
启停 MCP，对应建立/断开 SSE 连接。
**Body** `ToggleMCPServerReq`: `is_active` bool。**data** `null`。

---

## 6. Memory

短期/长期记忆**配置**管理（按 ChatArea）。短期消息实际存 Redis，长期条目存 Postgres，本组接口只管理配置元数据。

### GET /memory/:chatAreaID/short-term
获取短期记忆配置，不存在则自动创建（`window_size=20, auto_compact=false`）。

**data** `ShortTermMemoryResp`: `id`、`chat_area_id`、`window_size` int、`auto_compact` bool、`created_at`。

### PUT /memory/:chatAreaID/short-term
更新短期记忆配置，同步运行时 MemoryGroup。

**Body**: `window_size` int、`auto_compact` bool（均必填）。**data** `ShortTermMemoryResp`。

### GET /memory/:chatAreaID/long-term
获取长期记忆配置，不存在自动创建（`hot_area_size=10, hot_memory_ttl=86400`）。

**data** `LongTermMemoryResp`: `id`、`chat_area_id`、`hot_area_size` int、`hot_memory_ttl` int（秒）、`created_at`。

### PUT /memory/:chatAreaID/long-term
更新长期记忆配置。

**Body**: `hot_area_size` int、`hot_memory_ttl` int（均必填）。**data** `LongTermMemoryResp`。

---

## 7. Prompts

Prompt 模板 CRUD。

> SystemLocked 提示词：启动时 `EnsureSystemPrompt` 幂等播种名为 `__system_locked__`、`IsSystem=true` 的种子，不受 `IsActive` 影响强制拼接。Service 层 Update/Delete/Toggle 拒绝 `IsSystem` 行；新建 Prompt 禁止使用 `type=system`（返回 40029）。拼接顺序：**SystemLocked → system → personality → custom**。

### GET /prompts
列出所有 Prompt（含系统锁定，前端只读）。

**data** `PromptResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `content` | string | 内容 |
| `type` | PromptType | 类型 |
| `is_active` | bool | 是否激活 |
| `is_system` | bool | 系统锁定（禁改/删/停用） |
| `created_at` | time | 创建时间 |

### POST /prompts
新增 Prompt。**禁止 `type=system`**。

**Body** `AddPromptReq`: `name`、`content`、`type`（personality/custom）、`is_active`（均必填）。

**data** `PromptResp`。

### PUT /prompts/:id
更新。若目标行 `IsSystem=true` 或请求 `type=system` 返回 40029。**Body** `UpdatePromptReq`（同 Add）。**data** `PromptResp`。

### DELETE /prompts/:id
删除。`IsSystem=true` 返回 40029。**data** `null`。

### PUT /prompts/:id/toggle
启停。系统锁定提示词**允许启用、不允许停用**（停用返回 40029）。
**Body** `TogglePromptReq`: `is_active` bool。**data** `null`。

---

## 8. Sessions

每个 ChatArea 对应一个 Session。

### GET /sessions
列出所有 Session（Preload ChatArea）。

**data** `SessionResp[]`: `id`、`chat_area_id`、`model`、`token_usage` int64、`meta_data` JSONMap、`created_at`。

### GET /sessions/:id
**data** `SessionResp`。

### DELETE /sessions/:id
删除 Session，同时清除 Redis 短期消息缓存。**data** `null`。

---

## 9. Skills

Skill = 关键词/正则触发的 Prompt+Tool 组合配置。`priority` 越大越优先。`prompt_refs` 支持引用多个 Prompt。

### GET /skills
**data** `SkillResp[]`: `id`、`name`、`description`、`keywords` string[]、`regex_pattern`、`prompt_refs` string[]、`tool_refs` string[]、`mcp_refs` string[]、`is_active`、`is_system`、`priority` int、`created_at`。

### POST /skills
**Body** `AddSkillReq`: `name`（必填）；`description`、`keywords`、`regex_pattern`、`prompt_refs`、`tool_refs`、`mcp_refs`（可选）；`is_active`（必填）；`is_system`、`priority`（可选）。

**data** `SkillResp`。

### PUT /skills/:id
覆盖更新。**Body** `UpdateSkillReq`（同 Add）。**data** `SkillResp`。

### DELETE /skills/:id
**data** `null`。

---

## 10. Tools

工具配置查看与启停。

> `GET /tools` 合并两份数据源：运行时 `ToolRegistry.List()` 中的内置工具（ID 形如 `builtin:<name>`，`is_builtin=true`，常驻不可启停）+ DB 中 `ToolConfig` 表（自定义工具与历史条目）。同名条目用 DB 的 ID/`is_active`/`created_at`，但 `is_builtin` 与 `parameters` 以运行时注册表为准。

### GET /tools
**data** `ToolConfigResp[]`: `id`、`name`、`description`、`parameters` JSONMap、`timeout` int、`is_active`、`is_builtin`、`created_at`。

### PUT /tools/:id/toggle
启停 Tool。**内置工具（`id` 以 `builtin:` 开头）运行时常驻，不支持启停**，返回 40030。

**Body** `ToggleToolReq`: `is_active` bool。**data** `null`。

---

## 11. Plugins

Lua 插件管理。插件通过 ZIP 上传，自动解压到 `data/pluggins/<name>/`。

> `GET /plugins` 改用 `PluginEngine.ListMaps()`：不再有 `path`/`config`/`created_at`，新增 `permissions`/`commands`/`is_system`/`author`/`description`。系统插件（`is_system=true`）三层保护（Manifest.System + `PluginEngine.IsSystem()` + Service 守卫）禁止删除与停用，违规返回 40028。`POST /plugins/upload` 用 `multipart/form-data`。CronJob/Provider/MCP 新建后自动 reload 对应调度器/运行时。

### GET /plugins
列出所有插件配置。

**data** `PluginListMap[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 插件名（=目录名，作为 `id`） |
| `ppid` | string | 稳定 UUID |
| `version` | string | 版本 |
| `author` | string | 作者 |
| `description` | string | 描述 |
| `permissions` | string[] | 权限列表 |
| `is_system` | bool | 系统插件（禁删/停） |
| `is_active` | bool | 是否激活 |
| `commands` | PluginCommandInfo[] | 注册命令列表 |

`PluginCommandInfo`: `path` string[]、`description`、`usage`、`is_leaf` bool。

### POST /plugins/upload
**Body**: `multipart/form-data`，字段 `file` 为 ZIP。
**data** `PluginUploadResp`: `name`、`status`（`loaded`）。

```bash
curl -X POST http://localhost:8090/api/v1/plugins/upload \
  -H "Authorization: Bearer <token>" -F "file=@my-plugin.zip"
```

### PUT /plugins/:id/toggle
启停插件。启用 `Load`，停用 `Unload`。系统插件禁停用（40028）。
**Body** `TogglePluginReq`: `is_active` bool。**data** `null`。

### DELETE /plugins/:id
卸载并删除插件配置（**不删磁盘文件**）。系统插件禁删（40028）。**data** `null`。

### POST /plugins/reload
**热重载所有非系统插件**。先卸载全部非系统插件，再调用 `LoadAll()` 重新扫描并加载。
适用于：新增/修改 `on_cronjob` 或注册了新命令后无需重启进程即可生效。
**Body** 无。**data** `null`。

---

## 12. 聊天记录

按 ChatArea 分页查询持久化聊天记录（Postgres）。

### GET /chat-records/:chatAreaID
| Query | 类型 | 默认 | 说明 |
|-------|------|------|------|
| `limit` | int | 20 | 每页数量 |
| `offset` | int | 0 | 偏移 |
| `role` | string | (空) | 过滤角色 `user`/`assistant`/`tool` |

**data** `ChatRecordListResp`: `total` int64、`list` ChatRecordResp[]。

`ChatRecordResp`: `id` int64、`chat_area_id`、`user_id` int64、`role`、`content`、`token_count` int、`tool_calls` JSONMap、`created_at` time。

### GET /chat-records/:chatAreaID/token-usage
返回该 ChatArea 的会话 Token 用量（实际是 `Session.GetOrCreate`）。**data** `SessionResp`。

---

## 13. Chat Areas

聊天区域自动由消息驱动创建（私聊/群聊各一个）。

### GET /chat-areas
**data** `ChatAreaResp[]`: `id`、`area_type`、`target_id` int64、`created_at`。

---

## 14. Overview

### GET /overview
返回系统全局概览（资源计数 + 系统状态 + T2I/Sandbox 健康）。

**data** `OverviewResp`: `chat_area_count`、`mcp_count`、`adapter_count`（固定 1）、`plugin_count`、`provider_count`、`skill_count`、`session_count`、`total_token_usage` int64、`cpu_count`、`goroutine_num`、`mem_alloc_bytes`、`mem_sys_bytes`、`mem_heap_inuse_bytes` uint64、`go_version`、`t2i_active`、`t2i_healthy`、`sandbox_active`、`sandbox_healthy` bool。

### GET /overview/daily-token-usage
近 N 天每日 Token 用量（折线图数据点）。

| Query | 类型 | 默认 | 说明 |
|-------|------|------|------|
| `days` | int | 7 | 天数，范围 1–30 |

**data** `DailyTokenUsageResp[]`: `date` string（`YYYY-MM-DD`）、`token_count` int64。

---

## 15. ACL

访问控制规则管理。规则以 ChatArea 为单位组织。

### GET /acl
**data** `ACLRuleResp[]`: `id` int64、`chat_area_id`、`scope`、`permission`、`target_type`、`user_ids` string[]、`created_at`。

### POST /acl
新增或覆盖规则（同 ChatArea + Scope 已存在则覆盖）。

**Body** `AddACLRuleReq`: `chat_area_id`、`scope`、`permission`、`target_type`（均必填）；`user_ids` string[]（`target_type=list` 时有效）。**data** `ACLRuleResp`。

### DELETE /acl/:id
删除规则并同步运行时 ACL 管理器。**data** `null`。

---

## 16. 日志

日志由 `internal/logging` Hub 维护，环形缓冲区保留最近 250 条。

### GET /logs
返回最近 250 条，**最新排在最前**。

**data** `LogEntryResp[]`: `time` time、`level` string、`message` string、`attrs` map。

### GET /logs/stream
SSE 实时日志流。`text/event-stream`。

- 先按时间顺序发送最近 250 条历史
- 再订阅 Hub 实时推送；每 15 秒发送一次 keepalive 心跳
- 客户端断开或服务停止时退出

事件：

```
event: log
data: {"time":"2026-07-20T12:00:00Z","level":"INFO","message":"...","attrs":{}}
```

```javascript
const es = new EventSource('/api/v1/logs/stream', {
  headers: { Authorization: 'Bearer ' + token }
});
es.addEventListener('log', (e) => {
  const entry = JSON.parse(e.data);
  console.log(entry.time, entry.level, entry.message);
});
```

---

## 17. Webhook

Webhook 适配器配置（监听独立端口接收外部 HTTP 事件）。详见 [webhook-cronjob.md](webhook-cronjob.md)。

### GET /webhook/config
**data** `WebhookConfigResp`: `addr`、`port` int、`token`、`enabled` bool、`running` bool。

### PUT /webhook/config
**Body** `UpdateWebhookConfigReq`: `addr`、`port`、`token`、`enabled`（均必填）。**data** `WebhookConfigResp`（含最新 `running`）。

---

## 18. T2I

Text-to-Image 配置与健康管理。单行配置（ID=1）。详见 [external-services.md](external-services.md#t2i)。

### GET /t2i/config
**data** `T2IConfigResp`: `base_url`、`timeout` int、`is_active` bool、`healthy` bool。

### PUT /t2i/config
更新配置。运行时若启用则重建客户端并注入 HagoCenter，停用则置空。
**Body** `UpdateT2IConfigReq`: `base_url`（必填）、`timeout` int（可选）、`is_active` bool（必填）。**data** `T2IConfigResp`。

### GET /t2i/health
实时健康检查。**data** `{"healthy": bool}`。

---

## 19. Sandbox

代码沙箱配置与健康管理。单行配置（ID=1）。详见 [external-services.md](external-services.md#sandbox)。

### GET /sandbox/config
**data** `SandboxConfigResp`: `base_url`、`api_key`、`timeout` int、`is_active` bool、`healthy` bool。

### PUT /sandbox/config
**Body** `UpdateSandboxConfigReq`: `base_url`、`api_key`、`is_active`（必填），`timeout`（可选）。**data** `SandboxConfigResp`。

### GET /sandbox/health
**data** `{"healthy": bool}`。

---

## 20. Agent 活跃循环

当前正在执行的 Agent ReAct 循环（监控展示，原后台任务页改造）。对应前端页面 `web/src/views/AgentLoopsPage.vue`，实现见 `internal/agent/loop_tracker.go`。

### GET /agent/loops
返回当前所有活跃的 Agent ReAct 循环。

**data** `AgentLoopResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 循环 ID |
| `chat_area_id` | string | 所属 ChatArea |
| `message_type` | string | `private` / `group` |
| `target_id` | int64 | 私聊: user_id；群聊: group_id |
| `user_id` | int64 | 发起者 QQ |
| `user_msg` | string | 触发消息（批内合并） |
| `current_tool` | string | 当前正在执行的工具；空=思考/生成中 |
| `started_at` | time | 开始时间 |

---

## 21. CronJob

定时任务管理。详见 [webhook-cronjob.md](webhook-cronjob.md)。

CronJob 增删改/toggle 后**自动 reload** 调度器（`robfig/cron`，6 字段：秒 分 时 日 月 周）。

### GET /cronjobs
**data** `CronJobResp[]`。

### GET /cronjobs/:id
**data** `CronJobResp`。

### POST /cronjobs
新增，自动同步调度器。

**Body** `AddCronJobReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `cron_expr` | string | 是 | 6 字段 cron，如 `0 0 9 * * *` 每天 9:00 |
| `is_active` | bool | 是 | 是否立即启用 |
| `message` | string | 否 | 合成消息内容（透传给插件 `event.raw_message`） |
| `message_type` | string | 否 | `private`（默认）/ `group` |
| `target_id` | int64 | 否 | 消息目标：私聊=QQ 号，群聊=群号 |
| `plugin_ids` | string[] | 否 | 触发插件列表（插件目录名），到点时调用其 `on_cronjob` 回调 |
| `payload` | string | 否 | JSON 字符串，传递给插件 `on_cronjob(event)` 的 `event.payload` |

**data** `CronJobResp`。

`CronJobResp`: `id`、`name`、`cron_expr`、`plugin_ids` JSONSlice、`payload` JSONMap、`is_active`、`last_run_at` *time、`last_error`、`created_at`、`updated_at`。

### PUT /cronjobs/:id
覆盖更新，自动 reload。**Body** `UpdateCronJobReq`（同 Add）。**data** `CronJobResp`。

### DELETE /cronjobs/:id
删除，自动 reload。**data** `null`。

### PUT /cronjobs/:id/toggle
启停，自动 reload。**Body** `ToggleCronJobReq`: `is_active` bool。**data** `null`。

---

## 22. 回复策略

系统回复策略（单例，仅一行）。控制群聊中 Agent 对消息的回复行为。

**ReplyStrategy 枚举**

| 值 | 含义 |
|----|------|
| `never_reply` | 完全不回复 |
| `at_only` | 仅被 @ 时回复 |
| `always` | 始终回复（默认） |
| `relevance` | 按相关性回复：@/命令/提及名字必回；噪音消息规则过滤；其余候选批量合并为一次 LLM 判断（受 `relevance_threshold` 影响），带结果缓存/冷却与刷屏降级 |

### GET /reply-strategy
获取配置。首次 GET 不存在时自动创建（`strategy=always, relevance_threshold=0.5`）。

**data** `ReplyStrategyResp`: `strategy`、`relevance_threshold` float64、`bot_name`、`strip_markdown` bool、`agent_lite` bool、`relevance_prompt` string、`relevance_model` string、`judge_fail_policy` string（`drop`=判断失败不回复（默认）/ `reply`=照常回复）。

### PUT /reply-strategy
更新。

**Body** `UpdateReplyStrategyReq`: `strategy`、`relevance_threshold`（必填）；`bot_name`、`strip_markdown`、`agent_lite`（可选）；`relevance_prompt`（相关性检测自定义提示词，空=默认）、`relevance_model`（相关性检测 Text Provider ID，空=默认）、`judge_fail_policy`（`drop`/`reply`，空=默认 `drop`）。

**data** `ReplyStrategyResp`。

```bash
curl -X PUT http://localhost:8090/api/v1/reply-strategy \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"strategy":"relevance","relevance_threshold":0.6,"bot_name":"小卷"}'
```

---

## 附：前端 SPA 静态服务

后端复用 Hertz 引擎同端口（`:8090`）服务前端 SPA：

| 请求路径模式 | 行为 |
|--------------|------|
| `/api/v1/<已注册路由>` | Hertz 路由，JWT 鉴权（除 `/login`） |
| `/health` | 内联健康检查（root，无需鉴权） |
| `/api/*`（未命中） | 标准信封 404：`{"status":40400,"info":"资源不存在","data":null}` |
| 其它任何路径 | 文件存在→serve 文件；不存在→回退 `index.html` |
| 前端未构建（`index.html` 缺失） | 200 + 引导提示页（"请先构建前端"） |

实现：`internal/web/web.go::SPAHandler(webDir)`，在 `engine.New` 中通过 `h.NoRoute(...)` 注册；不嵌入二进制，磁盘上 `WEB_DIR`（默认 `web/dist`）为准；开发期 Vite `:3000` 代理 `/api`→`:8090`，Go 的 fallback 不会被触发。