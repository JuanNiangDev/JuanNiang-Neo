# Web API 文档

**Base URL:** `http://localhost:8090/api/v1`

**认证:** JWT Bearer Token (有效期 24h)

---

## 认证

### POST /login

登录获取 JWT Token。

```
请求体:
{
  "username": "admin",
  "password": "Admin123"
}

响应:
{
  "code": 0,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs..."
  }
}
```

### POST /change-password (需认证)

修改管理员密码。

```
请求头: Authorization: Bearer <token>
请求体:
{
  "old_password": "Admin123",
  "new_password": "new-password"
}
```

---

## 适配器

### GET /adapter (需认证)

获取 OneBot11 适配器运行状态。

```
响应:
{
  "code": 0,
  "data": {
    "status": "running"
  }
}
```

---

## Provider 管理

### GET /providers (需认证)

列出所有 LLM Provider。

### POST /providers (需认证)

添加 LLM Provider。

```
请求体:
{
  "name": "my-gpt",
  "type": "text_model",
  "endpoint": "https://api.openai.com",
  "token": "sk-xxx",
  "model": "gpt-4",
  "temperature": 0.7,
  "is_active": true
}
```

### PUT /providers/:id (需认证)

修改 Provider 配置。

### DELETE /providers/:id (需认证)

删除 Provider。

---

## MCP 管理

### GET /mcp (需认证)

列出所有 MCP 服务器。

### POST /mcp (需认证)

添加 MCP 服务器。

```
请求体:
{
  "name": "my-mcp",
  "server_url": "http://localhost:3001",
  "headers": {"X-Key": "value"},
  "timeout": 30000,
  "retry_count": 3,
  "auto_reconnect": true,
  "is_active": true
}
```

### PUT /mcp/:id (需认证)

修改 MCP 配置。

### DELETE /mcp/:id (需认证)

删除 MCP 服务器。

---

## 记忆管理

### GET /memory/:chatAreaID/short-term (需认证)

获取某 ChatArea 的短期记忆配置。

```
响应:
{
  "code": 0,
  "data": {
    "id": "uuid",
    "chat_area_id": "uuid",
    "window_size": 20,
    "auto_compact": false
  }
}
```

### PUT /memory/:chatAreaID/short-term (需认证)

修改短期记忆配置。

```
请求体:
{
  "window_size": 30,
  "auto_compact": true
}
```

### GET /memory/:chatAreaID/long-term (需认证)

获取长期记忆配置。

### PUT /memory/:chatAreaID/long-term (需认证)

修改长期记忆配置。

```
请求体:
{
  "hot_area_size": 10,
  "hot_memory_ttl": 86400
}
```

---

## Prompt 管理

### GET /prompts (需认证)

列出所有 Prompt。

### POST /prompts (需认证)

添加 Prompt。

```
请求体:
{
  "name": "system-default",
  "type": "system",
  "content": "你是一个友好的 QQ 机器人助手...",
  "is_active": true,
  "variables": ["UserName", "GroupName"]
}
```

### PUT /prompts/:id (需认证)

修改 Prompt。

### DELETE /prompts/:id (需认证)

删除 Prompt。

---

## Session 管理

### GET /sessions (需认证)

列出所有 Session (含关联 ChatArea)。

### DELETE /sessions/:id (需认证)

清除 Session (含 Redis 消息历史)。

---

## Skill 管理

### GET /skills (需认证)

列出所有 Skill。

### POST /skills (需认证)

添加 Skill。

```
请求体:
{
  "name": "weather",
  "description": "天气查询",
  "keywords": ["天气", "weather"],
  "regex_pattern": "",
  "prompt_ref": "weather-prompt-id",
  "tool_refs": ["browser_search"],
  "mcp_refs": [],
  "is_active": true,
  "is_system": false,
  "priority": 10
}
```

### PUT /skills/:id (需认证)

修改 Skill。

### DELETE /skills/:id (需认证)

删除 Skill。

---

## Tool 管理

### GET /tools (需认证)

列出所有 Tool (含内置)。

### PUT /tools/:id/toggle (需认证)

启用/禁用 Tool。

```
请求体:
{
  "is_active": false
}
```

---

## Plugin 管理

### GET /plugins (需认证)

列出所有已安装插件。

### PUT /plugins/:id/toggle (需认证)

启用/禁用插件。

### DELETE /plugins/:id (需认证)

删除插件。

---

## ACL 管理

### GET /acl (需认证)

列出所有 ACL 规则。

### POST /acl (需认证)

添加 ACL 规则。

```
请求体:
{
  "user_id": 123456789,
  "chat_area_id": "uuid",
  "permission": "denied",
  "actions": []
}
```

### DELETE /acl/:id (需认证)

删除 ACL 规则。

---

## 聊天记录

### GET /chat-records/:chatAreaID (需认证)

获取指定 ChatArea 的聊天记录 (分页)。

```
Query:
  limit: 20 (default)
  offset: 0 (default)

响应:
{
  "code": 0,
  "data": {
    "total": 150,
    "list": [...]
  }
}
```

---

## Chat Area

### GET /chat-areas (需认证)

列出所有 ChatArea。

---

## 全局概览

### GET /overview (需认证)

获取系统全局统计。

```
响应:
{
  "code": 0,
  "data": {
    "chat_area_count": 42,
    "mcp_count": 3,
    "adapter_count": 1,
    "plugin_count": 5,
    "total_token_usage": 1234567
  }
}
```

---

## 错误响应

所有错误统一格式:

```
{
  "code": 4xx/5xx,
  "msg": "错误描述"
}
```

| Code | 含义 |
|------|------|
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 过期 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
