# JuanNiang-Neo Web API 文档

**Base URL:** `http://localhost:8090/api/v1`

**Content-Type:** `application/json` (除上传文件外)

**统一响应格式:** 所有接口都返回 `FinalResponse`:

```json
{
  "status": 0,
  "info": "OK",
  "data": <任意类型或 null>
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | uint | 0=成功，非 0=错误码 |
| `info` | string | 状态描述（成功为 "OK"，失败为错误信息） |
| `data` | any | 业务数据，失败时可为 `null` 或 `{"error_detail": "..."}` |

**错误码表:**

| Code | 说明 |
|------|------|
| 0 | 成功 |
| 40001 | 参数格式错误 |
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
| 50000 | 服务器内部错误 |

**认证:** 除 `POST /login` 和 `GET /health` 外所有接口需 `Authorization: Bearer <token>` header，通过 `POST /login` 获取。

---

## 通用数据类型

| 类型 | Go 类型 | JSON 形式 | 说明 |
|------|---------|-----------|------|
| `JSONMap` | `map[string]any` | `{"k":"v"}` | 任意键值对 |
| `JSONSlice` | `[]string` | `["a","b"]` | 字符串数组 |
| `time.Time` | RFC3339 | `"2026-07-20T12:00:00Z"` | 时间戳 |

**枚举类型:**

- `ModelType`: `text_model` | `image_model` | `embedding_model`
- `PromptType`: `system` | `personality` | `custom`
- `AreaType`: `private` | `group`
- `ACLScope`: `chat` | `tool` | `mcp`
- `ACLPermission`: `allow` | `deny`
- `ACLTargetType`: `all` | `list` （`list` 时 `user_ids` 才有效）

**ACL 规则语义:**

- 无规则 = 允许所有（默认）
- `deny` + `all` = 拒绝所有用户
- `deny` + `list` = 拒绝指定用户
- `allow` + `all` = 允许所有用户
- `allow` + `list` = 仅允许指定用户（白名单）

检查优先级: `deny` > `allow`；存在 `allow` 规则时未命中即拒绝。Admins 列表中的用户绕过所有 ACL 检查。

---

## 目录

1. [认证](#1-认证)
2. [Adapter](#2-adapter)
3. [Provider 管理](#3-provider-管理)
4. [MCP 管理](#4-mcp-管理)
5. [记忆管理](#5-记忆管理)
6. [Prompt 管理](#6-prompt-管理)
7. [Session 管理](#7-session-管理)
8. [Skill 管理](#8-skill-管理)
9. [Tool 管理](#9-tool-管理)
10. [Plugin 管理](#10-plugin-管理)
11. [ACL 管理](#11-acl-管理)
12. [聊天记录](#12-聊天记录)
13. [Chat Areas](#13-chat-areas)
14. [全局概览](#14-全局概览)
15. [T2I](#15-t2i)
16. [Sandbox](#16-sandbox)
17. [Webhook](#17-webhook)
18. [日志](#18-日志)
19. [健康检查](#19-健康检查)

---

## 1. 认证

### 1.1 POST /login

**功能:** 管理员登录，获取 JWT token。系统初始化时默认账号 `admin / Admin123`。

**请求体** `LoginReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `username` | string | 是 | 用户名 |
| `password` | string | 是 | 明文密码 |

**成功响应** `data: TokenResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `token` | string | JWT token，后续请求放入 `Authorization: Bearer <token>` |

**示例:**

```bash
curl -X POST http://localhost:8090/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin123"}'
```

```json
{"status":0,"info":"OK","data":{"token":"eyJhbGciOiJIUzI1NiIs..."}}
```

### 1.2 POST /change-password

**功能:** 修改当前登录用户密码（实际查 `admin` 用户）。

**请求体** `ChangePasswordReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `old_password` | string | 是 | 原密码 |
| `new_password` | string | 是 | 新密码 |

**响应** `data: null`

**示例:**

```bash
curl -X POST http://localhost:8090/api/v1/change-password \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"Admin123","new_password":"NewPass456"}'
```

```json
{"status":0,"info":"OK","data":null}
```

---

## 2. Adapter

OneBot11 适配器状态查询与配置更新。

### 2.1 GET /adapter

**功能:** 获取 OneBot11 适配器当前运行状态（不含配置）。

**响应** `data: AdapterStatus`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `running` | bool | 适配器是否在运行 |
| `listen_addr` | string | 监听地址 `host:port` |
| `self_id` | int64 | 机器人 QQ 号 |
| `conn_count` | int | 当前 WebSocket 连接数 |
| `conn_ids` | int64[] | 已连接客户端 QQ 号列表 |

### 2.2 PUT /adapter

**功能:** 更新 OneBot11 适配器配置（地址、Token、管理员列表、启用状态），同步到运行时。

**请求体** `UpdateAdapterConfigReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `addr` | string | 是 | 监听地址 |
| `port` | int | 是 | 监听端口 |
| `token` | string | 是 | OneBot access token |
| `admin_qq_numbers` | string[] | 是 | 管理员 QQ 号列表 |
| `enabled` | bool | 是 | 是否启用 |

**响应** `data: null`

### 2.3 POST /adapter/restart

**功能:** 重启 OneBot11 适配器。

**响应** `data: null`

---

## 3. Provider 管理

LLM Provider (text/image/embedding) CRUD。同类型只能有一个 Active，激活时自动停用同类型其他 Provider。

### 3.1 GET /providers

**功能:** 列出所有 Provider。

**响应** `data: ProviderResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `created_at` | time | 创建时间 |
| `name` | string | 名称 |
| `type` | ModelType | `text_model`/`image_model`/`embedding_model` |
| `endpoint` | string | API 地址 |
| `token` | string | API token |
| `model` | string | 模型名 |
| `temperature` | float32 | 温度 |
| `is_active` | bool | 是否激活 |

### 3.2 GET /providers/:id

**路径参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | string | Provider UUID |

**响应** `data: ProviderResp`（同上）

### 3.3 POST /providers

**功能:** 新增 Provider。若 `is_active=true`，自动停用同类型其他 Provider。

**请求体** `AddProviderReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `type` | ModelType | 是 | 类型 |
| `endpoint` | string | 是 | API 地址 |
| `token` | string | 是 | API token |
| `model` | string | 是 | 模型名 |
| `temperature` | float32 | 否 | 温度（默认 0.7） |
| `isActive` | bool | 是 | 是否激活 |

**响应** `data: AddProviderReq`（回显请求体）

### 3.4 PUT /providers/:id

**路径参数:** `id` (Provider UUID)

**请求体** `UpdateProviderReq`（同 AddProviderReq 字段，覆盖更新）

**响应** `data: null`

### 3.5 DELETE /providers/:id

**路径参数:** `id` (Provider UUID)

**功能:** 删除 Provider，同时从运行时 ProviderGroup 移除。

**响应** `data: null`

### 3.6 PUT /providers/:id/toggle

**功能:** 启用/停用 Provider。启用时自动停用同类型其他 Provider。

**路径参数:** `id` (Provider UUID)

**请求体** `ToggleProviderReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_active` | bool | 是 | 目标状态 |

**响应** `data: null`

---

## 4. MCP 管理

MCP (Model Context Protocol) 服务器配置 CRUD，支持运行时连接/断开。

### 4.1 GET /mcp

**功能:** 列出所有 MCP 服务器。

**响应** `data: MCPServerResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `server_url` | string | SSE 端点 URL |
| `headers` | JSONMap | 自定义请求头 |
| `timeout` | int | 超时毫秒 |
| `retry_count` | int | 重试次数 |
| `tool_filter` | string[] | 工具白名单（空表示全量） |
| `auto_reconnect` | bool | 是否自动重连 |
| `is_active` | bool | 是否激活 |
| `created_at` | time | 创建时间 |

### 4.2 GET /mcp/:id

**路径参数:** `id` (MCP UUID)

**响应** `data: MCPServerResp`

### 4.3 POST /mcp

**功能:** 新增 MCP 配置，若 `is_active=true` 立即建立 SSE 连接。

**请求体** `AddMCPServerReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `server_url` | string | 是 | SSE 端点 URL |
| `headers` | JSONMap | 否 | 自定义请求头 |
| `timeout` | int | 否 | 超时毫秒 |
| `retry_count` | int | 否 | 重试次数 |
| `tool_filter` | string[] | 否 | 工具白名单 |
| `auto_reconnect` | bool | 否 | 是否自动重连 |
| `is_active` | bool | 是 | 是否激活 |

**响应** `data: MCPServerResp`（含生成的 UUID）

### 4.4 PUT /mcp/:id

**路径参数:** `id` (MCP UUID)

**请求体** `UpdateMCPServerReq`（同 AddMCPServerReq 字段）

**功能:** 覆盖更新，断开旧连接，若 `is_active=true` 重新建立 SSE 连接。

**响应** `data: MCPServerResp`

### 4.5 DELETE /mcp/:id

**路径参数:** `id` (MCP UUID)

**功能:** 断开连接并删除配置。

**响应** `data: null`

### 4.6 PUT /mcp/:id/toggle

**功能:** 启用/停用 MCP，对应建立/断开 SSE 连接。

**路径参数:** `id` (MCP UUID)

**请求体** `ToggleMCPServerReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_active` | bool | 是 | 目标状态 |

**响应** `data: null`

---

## 5. 记忆管理

短期/长期记忆配置管理（按 ChatArea）。短期记忆的实际消息存储在 Redis，长期记忆条目存储在 Postgres。本组接口只管理**配置**。

### 5.1 GET /memory/:chatAreaID/short-term

**功能:** 获取指定 ChatArea 的短期记忆配置，不存在则自动创建（默认 `window_size=20, auto_compact=false`）。

**路径参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `chatAreaID` | string | ChatArea UUID |

**响应** `data: ShortTermMemoryResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 配置 UUID |
| `chat_area_id` | string | 所属 ChatArea |
| `window_size` | int | 滑动窗口大小（保留最近 N 条） |
| `auto_compact` | bool | 是否自动压缩 |
| `created_at` | time | 创建时间 |

### 5.2 PUT /memory/:chatAreaID/short-term

**功能:** 更新短期记忆配置，同步到运行时 MemoryGroup。

**路径参数:** `chatAreaID`

**请求体** `UpdateShortTermMemoryReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `window_size` | int | 是 | 滑动窗口大小 |
| `auto_compact` | bool | 是 | 是否自动压缩 |

**响应** `data: ShortTermMemoryResp`（更新后的配置）

### 5.3 GET /memory/:chatAreaID/long-term

**功能:** 获取长期记忆配置，不存在则自动创建（默认 `hot_area_size=10, hot_memory_ttl=86400`）。

**路径参数:** `chatAreaID`

**响应** `data: LongTermMemoryResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 配置 UUID |
| `chat_area_id` | string | 所属 ChatArea |
| `hot_area_size` | int | 热区大小 |
| `hot_memory_ttl` | int | 热区 TTL（秒） |
| `created_at` | time | 创建时间 |

### 5.4 PUT /memory/:chatAreaID/long-term

**功能:** 更新长期记忆配置，同步到运行时 MemoryGroup。

**路径参数:** `chatAreaID`

**请求体** `UpdateLongTermMemoryReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `hot_area_size` | int | 是 | 热区大小 |
| `hot_memory_ttl` | int | 是 | 热区 TTL（秒） |

**响应** `data: LongTermMemoryResp`（更新后的配置）

---

## 6. Prompt 管理

Prompt 模板 CRUD，支持变量插值。

### 6.1 GET /prompts

**功能:** 列出所有 Prompt。

**响应** `data: PromptResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `content` | string | 模板内容 |
| `type` | PromptType | `system`/`personality`/`custom` |
| `is_active` | bool | 是否激活 |
| `variables` | string[] | 变量列表 |
| `created_at` | time | 创建时间 |

### 6.2 POST /prompts

**请求体** `AddPromptReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `content` | string | 是 | 模板内容 |
| `type` | PromptType | 是 | 类型 |
| `is_active` | bool | 是 | 是否激活 |
| `variables` | string[] | 否 | 变量列表 |

**响应** `data: PromptResp`

### 6.3 PUT /prompts/:id

**路径参数:** `id` (Prompt UUID)

**请求体** `UpdatePromptReq`（同 AddPromptReq 字段）

**响应** `data: PromptResp`

### 6.4 DELETE /prompts/:id

**路径参数:** `id` (Prompt UUID)

**响应** `data: null`

### 6.5 PUT /prompts/:id/toggle

**功能:** 启用/停用 Prompt。

**路径参数:** `id` (Prompt UUID)

**请求体** `TogglePromptReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_active` | bool | 是 | 目标状态 |

**响应** `data: null`

---

## 7. Session 管理

会话状态查询。每个 ChatArea 对应一个 Session。

### 7.1 GET /sessions

**功能:** 列出所有 Session。

**响应** `data: SessionResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | Session UUID |
| `chat_area_id` | string | 所属 ChatArea |
| `model` | string | 当前模型名 |
| `token_usage` | int64 | 累计 Token 用量 |
| `meta_data` | JSONMap | 自定义元数据 |
| `created_at` | time | 创建时间 |

### 7.2 GET /sessions/:id

**路径参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | string | Session UUID |

**响应** `data: SessionResp`

### 7.3 DELETE /sessions/:id

**功能:** 删除 Session，同时清除 Redis 中的消息缓存。

**路径参数:** `id` (Session UUID)

**响应** `data: null`

---

## 8. Skill 管理

Skill 是关键词/正则触发的工具/Prompt 组合配置。

### 8.1 GET /skills

**功能:** 列出所有 Skill。

**响应** `data: SkillResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `description` | string | 描述 |
| `keywords` | string[] | 触发关键词 |
| `regex_pattern` | string | 触发正则 |
| `prompt_ref` | string | 关联 Prompt UUID |
| `tool_refs` | string[] | 关联 Tool ID 列表 |
| `mcp_refs` | string[] | 关联 MCP ID 列表 |
| `is_active` | bool | 是否激活 |
| `is_system` | bool | 是否系统内置 |
| `priority` | int | 优先级（数字越大越优先） |
| `created_at` | time | 创建时间 |

### 8.2 POST /skills

**请求体** `AddSkillReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 名称 |
| `description` | string | 否 | 描述 |
| `keywords` | string[] | 否 | 触发关键词 |
| `regex_pattern` | string | 否 | 触发正则 |
| `prompt_ref` | string | 否 | 关联 Prompt UUID |
| `tool_refs` | string[] | 否 | 关联 Tool ID |
| `mcp_refs` | string[] | 否 | 关联 MCP ID |
| `is_active` | bool | 是 | 是否激活 |
| `is_system` | bool | 否 | 是否系统内置 |
| `priority` | int | 否 | 优先级 |

**响应** `data: SkillResp`

### 8.3 PUT /skills/:id

**路径参数:** `id` (Skill UUID)

**请求体** `UpdateSkillReq`（同 AddSkillReq）

**响应** `data: SkillResp`

### 8.4 DELETE /skills/:id

**路径参数:** `id` (Skill UUID)

**响应** `data: null`

---

## 9. Tool 管理

工具配置查看与启用/停用。Tool 本身通过代码内置注册，前端只能切换状态。

### 9.1 GET /tools

**功能:** 列出所有 Tool 配置。

**响应** `data: ToolConfigResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 工具名 |
| `description` | string | 描述 |
| `parameters` | JSONMap | JSON Schema 参数定义 |
| `timeout` | int | 超时毫秒 |
| `is_active` | bool | 是否激活 |
| `is_builtin` | bool | 是否内置工具 |
| `created_at` | time | 创建时间 |

### 9.2 PUT /tools/:id/toggle

**功能:** 启用/停用 Tool。停用时从注册表移除；启用时内置工具已在 init 时注册，无需重复。

**路径参数:** `id` (Tool UUID)

**请求体** `ToggleToolReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_active` | bool | 是 | 目标状态 |

**响应** `data: null`

---

## 10. Plugin 管理

Lua 插件管理。插件通过 ZIP 上传，自动解压到 `data/pluggins/<name>/`。

### 10.1 GET /plugins

**功能:** 列出所有插件配置。

**响应** `data: PluginResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `name` | string | 名称 |
| `version` | string | 版本 |
| `path` | string | 文件路径 |
| `config` | JSONMap | 插件配置 |
| `is_active` | bool | 是否激活 |
| `created_at` | time | 创建时间 |

### 10.2 POST /plugins/upload

**功能:** 上传 ZIP 插件包。解压到 `data/pluggins/<name>/`，自动调用 `PluginEngine.Load`。

**请求体:** `multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `file` | file | 是 | ZIP 文件 |

**响应** `data: PluginUploadResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 插件名 |
| `status` | string | 状态（`loaded`） |

**示例:**

```bash
curl -X POST http://localhost:8090/api/v1/plugins/upload \
  -H "Authorization: Bearer <token>" \
  -F "file=@my-plugin.zip"
```

### 10.3 PUT /plugins/:id/toggle

**功能:** 启用/停用插件。启用时调用 `PluginEngine.Load`，停用时 `Unload`。

**路径参数:** `id` (插件名)

**请求体** `TogglePluginReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_active` | bool | 是 | 目标状态 |

**响应** `data: null`

### 10.4 DELETE /plugins/:id

**功能:** 卸载并删除插件配置（不删除文件）。

**路径参数:** `id` (插件名)

**响应** `data: null`

---

## 11. ACL 管理

访问控制规则管理。规则以 ChatArea 为单位组织。

### 11.1 GET /acl

**功能:** 列出所有 ACL 规则。

**响应** `data: ACLRuleResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 自增 ID |
| `chat_area_id` | string | 所属 ChatArea |
| `scope` | ACLScope | `chat`/`tool`/`mcp` |
| `permission` | ACLPermission | `allow`/`deny` |
| `target_type` | ACLTargetType | `all`/`list` |
| `user_ids` | string[] | 目标用户列表（`target_type=list` 时有效） |
| `created_at` | time | 创建时间 |

### 11.2 POST /acl

**功能:** 新增或更新 ACL 规则（同 ChatArea + Scope 已存在则覆盖）。

**请求体** `AddACLRuleReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `chat_area_id` | string | 是 | 所属 ChatArea |
| `scope` | ACLScope | 是 | 管理范围 |
| `permission` | ACLPermission | 是 | 允许或拒绝 |
| `target_type` | ACLTargetType | 是 | 目标类型 |
| `user_ids` | string[] | 否 | 用户列表（`target_type=list` 时有效） |

**响应** `data: ACLRuleResp`

### 11.3 DELETE /acl/:id

**功能:** 删除 ACL 规则，同步到运行时 ACL 管理器。

**路径参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | ACL 规则 ID |

**响应** `data: null`

---

## 12. 聊天记录

按 ChatArea 分页查询持久化聊天记录（Postgres）。

### 12.1 GET /chat-records/:chatAreaID

**功能:** 分页查询指定 ChatArea 的聊天记录，支持按 role 过滤。

**路径参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| `chatAreaID` | string | ChatArea UUID |

**Query 参数:**

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `limit` | int | 20 | 每页数量 |
| `offset` | int | 0 | 偏移量 |
| `role` | string | (空) | 可选过滤角色: `user`/`assistant`/`tool` |

**响应** `data: ChatRecordListResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | int64 | 总记录数 |
| `list` | ChatRecordResp[] | 当前页记录列表 |

`ChatRecordResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 记录 ID |
| `chat_area_id` | string | 所属 ChatArea |
| `user_id` | int64 | 发送者 QQ（assistant/tool 为 0） |
| `role` | string | `user`/`assistant`/`tool` |
| `content` | string | 消息内容 |
| `token_count` | int | Token 数 |
| `tool_calls` | JSONMap | 工具调用详情（role=tool 时有效） |
| `created_at` | time | 创建时间 |

**示例:**

```bash
curl "http://localhost:8090/api/v1/chat-records/area-001?limit=20&offset=0&role=user" \
  -H "Authorization: Bearer <token>"
```

### 12.2 GET /chat-records/:chatAreaID/token-usage

**功能:** 获取指定 ChatArea 的会话 Token 用量（实际是 Session.GetOrCreate 返回）。

**路径参数:** `chatAreaID`

**响应** `data: SessionResp`

---

## 13. Chat Areas

聊天区域查询。ChatArea 由消息驱动自动创建（私聊/群聊各一个）。

### 13.1 GET /chat-areas

**功能:** 列出所有 ChatArea。

**响应** `data: ChatAreaResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `area_type` | AreaType | `private`/`group` |
| `target_id` | int64 | 私聊=用户QQ，群聊=群号 |
| `created_at` | time | 创建时间 |

---

## 14. 全局概览

### 14.1 GET /overview

**功能:** 返回系统全局概览，包括资源计数、系统状态、T2I/Sandbox 健康状态。

**响应** `data: OverviewResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `chat_area_count` | int64 | ChatArea 总数 |
| `mcp_count` | int64 | MCP 数量 |
| `adapter_count` | int64 | Adapter 数量（固定 1） |
| `plugin_count` | int64 | Plugin 数量 |
| `provider_count` | int | Provider 数量 |
| `skill_count` | int | Skill 数量 |
| `session_count` | int | Session 数量 |
| `total_token_usage` | int64 | 累计 Token 用量 |
| `cpu_count` | int | 逻辑 CPU 核数 |
| `goroutine_num` | int | 当前 goroutine 数 |
| `mem_alloc_bytes` | uint64 | 堆已分配（活跃对象） |
| `mem_sys_bytes` | uint64 | 从 OS 获取的内存总量 |
| `mem_heap_inuse_bytes` | uint64 | 堆中正在使用 |
| `go_version` | string | Go 版本 |
| `t2i_active` | bool | T2I 客户端已加载 |
| `t2i_healthy` | bool | T2I HealthCheck 通过 |
| `sandbox_active` | bool | Sandbox 客户端已加载 |
| `sandbox_healthy` | bool | Sandbox HealthCheck 通过 |

**示例:**

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/overview
```

```json
{
  "status": 0,
  "info": "OK",
  "data": {
    "chat_area_count": 42,
    "mcp_count": 3,
    "adapter_count": 1,
    "plugin_count": 5,
    "provider_count": 2,
    "skill_count": 8,
    "session_count": 42,
    "total_token_usage": 1234567,
    "cpu_count": 8,
    "goroutine_num": 47,
    "mem_alloc_bytes": 33554432,
    "mem_sys_bytes": 67108864,
    "mem_heap_inuse_bytes": 16777216,
    "go_version": "go1.25",
    "t2i_active": true,
    "t2i_healthy": true,
    "sandbox_active": false,
    "sandbox_healthy": false
  }
}
```

---

## 15. T2I

Text-to-Image 配置与健康管理。单行配置（ID=1）。

### 15.1 GET /t2i/config

**功能:** 获取 T2I 配置与当前健康状态。

**响应** `data: T2IConfigResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `base_url` | string | T2I 服务地址 |
| `timeout` | int | 超时毫秒 |
| `is_active` | bool | 是否启用 |
| `healthy` | bool | 当前健康状态 |

### 15.2 PUT /t2i/config

**功能:** 更新 T2I 配置，运行时同步客户端（启用则创建新客户端并注入 HagoCenter，停用则置空）。

**请求体** `UpdateT2IConfigReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `base_url` | string | 是 | 服务地址 |
| `timeout` | int | 否 | 超时毫秒 |
| `is_active` | bool | 是 | 是否启用 |

**响应** `data: T2IConfigResp`（含最新 healthy 状态）

### 15.3 GET /t2i/health

**功能:** 实时检查 T2I 服务健康状态（调用客户端 `HealthCheck`）。

**响应** `data: {"healthy": bool}`

---

## 16. Sandbox

代码沙箱配置与健康管理。单行配置（ID=1）。

### 16.1 GET /sandbox/config

**功能:** 获取 Sandbox 配置与当前健康状态。

**响应** `data: SandboxConfigResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `base_url` | string | Sandbox 服务地址 |
| `api_key` | string | API key |
| `timeout` | int | 超时毫秒 |
| `is_active` | bool | 是否启用 |
| `healthy` | bool | 当前健康状态 |

### 16.2 PUT /sandbox/config

**功能:** 更新 Sandbox 配置，运行时同步客户端。

**请求体** `UpdateSandboxConfigReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `base_url` | string | 是 | 服务地址 |
| `api_key` | string | 是 | API key |
| `timeout` | int | 否 | 超时毫秒 |
| `is_active` | bool | 是 | 是否启用 |

**响应** `data: SandboxConfigResp`（含最新 healthy 状态）

### 16.3 GET /sandbox/health

**功能:** 实时检查 Sandbox 健康状态。

**响应** `data: {"healthy": bool}`

---

## 17. Webhook

Webhook 适配器配置（监听独立端口接收外部事件）。

### 17.1 GET /webhook/config

**功能:** 获取 Webhook 配置与运行状态。

**响应** `data: WebhookConfigResp`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `addr` | string | 监听地址 |
| `port` | int | 监听端口 |
| `token` | string | 鉴权 token |
| `enabled` | bool | 是否启用 |
| `running` | bool | 当前是否在运行 |

### 17.2 PUT /webhook/config

**功能:** 更新 Webhook 配置，同步到运行时 WebhookAdapter。

**请求体** `UpdateWebhookConfigReq`:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `addr` | string | 是 | 监听地址 |
| `port` | int | 是 | 监听端口 |
| `token` | string | 是 | 鉴权 token |
| `enabled` | bool | 是 | 是否启用 |

**响应** `data: WebhookConfigResp`（含最新 running 状态）

---

## 18. 日志

日志查询与实时 SSE 流推送。日志由 `internal/logging` Hub 维护，环形缓冲区保留最近 250 条。

### 18.1 GET /logs

**功能:** 返回最近 250 条日志（按时间顺序，最早→最新）。

**响应** `data: LogEntryResp[]`:

| 字段 | 类型 | 说明 |
|------|------|------|
| `time` | time | 日志时间 |
| `level` | string | 日志级别（`INFO`/`WARN`/`ERROR` 等） |
| `message` | string | 日志消息 |
| `attrs` | map[string]any | 结构化字段（可选） |

### 18.2 GET /logs/stream

**功能:** 通过 SSE 推送实时日志。

**响应类型:** `text/event-stream`

**事件流:**

1. 连接建立后先发送最近 250 条历史日志（每个一条 `log` 事件）
2. 然后订阅 Hub，实时推送新日志
3. 每 15 秒发送一次 keepalive 心跳
4. 客户端断开或服务停止时退出

**事件格式:**

```
event: log
data: {"time":"2026-07-20T12:00:00Z","level":"INFO","message":"...","attrs":{}}
```

**客户端示例 (JavaScript):**

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

## 19. 健康检查

### 19.1 GET /health

**功能:** 服务存活检查，无需认证。

**路径:** `http://localhost:8090/health` (注意不在 `/api/v1` 前缀下)

**响应** `200`:

```json
{"status": "ok"}
```

---

## 附:典型调用流程

1. `POST /login` 拿到 `token`
2. 后续所有请求都加 `Authorization: Bearer <token>` header
3. `GET /overview` 查看系统总览
4. `GET /providers` + `POST /providers` 配置 LLM
5. `GET /mcp` + `POST /mcp` 配置 MCP
6. `GET /prompts` + `POST /prompts` 配置 Prompt
7. `GET /skills` + `POST /skills` 配置 Skill
8. `GET /acl` + `POST /acl` 配置访问控制
9. `PUT /t2i/config` + `PUT /sandbox/config` 配置 T2I/Sandbox
10. `GET /chat-records/:chatAreaID` 查看历史聊天
11. `GET /logs/stream` 实时查看日志

**注意事项:**

- Provider 同类型只能一个 Active，激活时自动停用其他
- Plugin 的 `id` 是插件名（不是 UUID），通过 `POST /plugins/upload` 上传 ZIP 后自动生成
- Skill 的 `priority` 数字越大优先级越高
- Memory 接口只管理配置；实际短期消息在 Redis、长期条目在 Postgres
- ChatArea 由系统自动创建（消息驱动），无手动创建接口
- T2I/Sandbox/Webhook 都是单行配置（ID=1）

---

## 20. 前端 SPA 静态服务

后端通过 Hertz `NoRoute` 兜底, 同端口 (`:8090`) 服务 Vue 前端 SPA, 路径与 API 互不冲突:

| 请求路径模式            | 行为                                                    |
|-------------------------|---------------------------------------------------------|
| `/api/v1/<已注册路由>`   | 走 Hertz 路由, JWT 鉴权 (除 `/login`)                   |
| `/health`                | 内联健康检查 (root, 不需鉴权)                            |
| `/api/*` (未命中)        | 标准信封 404: `{status:40400, info:"资源不存在", data:null}` |
| 其它任何路径             | 文件存在 → serve 文件; 不存在 → 回退 `index.html` (Vue Router history 模式) |
| 前端未构建 (`index.html` 缺失) | 返回 200 + 文本引导页 ("请先构建前端")                |

**入口与配置:**

- 启动: `cmd/server/main.go` 读取环境变量 `WEB_DIR` (默认 `web/dist`), 传入 `engine.New(addr, webDir, svc)`。
- 实现: `internal/web/web.go` 的 `SPAHandler(webDir)`, 在 `internal/api/engine/engine.go` 中通过 `h.NoRoute(...)` 注册。
- 路径穿越防护: 通过 `filepath.Rel` 校验, 文件必须落在 `webDir` 之内。
- **不嵌入二进制**: 前端是磁盘文件, 便于只换前端不重编 Go 的部署节奏。
- 开发模式: Vite `:3000` 热更新, `vite.config.ts` 代理 `/api` → `:8090`, 因此开发期 Go 的 SPA fallback 不会被触发。
- 生产模式: 容器内 `WEB_DIR=/app/web/dist`, 单端口暴露 Web 面板 + API + 前端。

**404 信封示例** (未命中的 `/api/*`):

```json
{
  "status": 40400,
  "info": "资源不存在",
  "data": null
}
```
