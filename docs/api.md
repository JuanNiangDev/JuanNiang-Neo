# Web API 文档

**Base URL:** `http://localhost:8090/api/v1`

**认证:** 除 `/login` 外所有端点需 `Authorization: Bearer <token>` header

---

## 认证

### POST /login

```bash
curl -X POST http://localhost:8090/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin123"}'
```

```json
// response 200
{
  "code": 0,
  "data": {"token": "eyJhbGciOiJIUzI1NiIs..."}
}

// response 401
{
  "code": 401,
  "msg": "用户名或密码错误"
}
```

### POST /change-password

```bash
curl -X POST http://localhost:8090/api/v1/change-password \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"Admin123","new_password":"NewPass456"}'
```

```json
{"code": 0, "data": null}
```

---

## 管理员 QQ 列表

动态管理拥有管理员权限的 QQ 号, 无需写死配置文件。

### GET /admins

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/admins
```

```json
{
  "code": 0,
  "data": [
    {"ID": 123456789, "CreatedAt": "2026-01-01T00:00:00Z"},
    {"ID": 987654321, "CreatedAt": "2026-06-01T00:00:00Z"}
  ]
}
```

### POST /admins

```bash
curl -X POST http://localhost:8090/api/v1/admins \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"qq": 111222333}'
```

```json
{"code": 0, "data": null}
```

### DELETE /admins/:id

```bash
curl -X DELETE http://localhost:8090/api/v1/admins/111222333 \
  -H "Authorization: Bearer <token>"
```

---

## Adapter

### GET /adapter

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/adapter
```

```json
{
  "code": 0,
  "data": {
    "running": true,
    "listen_addr": "0.0.0.0:8081",
    "self_id": 123456789,
    "conn_count": 1,
    "conn_ids": [123456789]
  }
}
```

### PUT /adapter

```bash
curl -X PUT http://localhost:8090/api/v1/adapter \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"token":"new-secret-token","admins":[123,456,789]}'
```

```json
{
  "code": 0,
  "data": {
    "running": true,
    "listen_addr": "0.0.0.0:8081",
    "conn_count": 1,
    ...
  }
}
```

### POST /adapter/restart

```bash
curl -X POST http://localhost:8090/api/v1/adapter/restart \
  -H "Authorization: Bearer <token>"
```

```json
{"code": 0, "data": {"running": true, ...}}
```

---

## Provider 管理

### GET /providers

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/providers
```

```json
{
  "code": 0,
  "data": [
    {
      "id": "a1b2c3d4...",
      "name": "GPT-4",
      "type": "text_model",
      "endpoint": "https://api.openai.com",
      "token": "sk-****",
      "model": "gpt-4",
      "temperature": 0.7,
      "is_active": true,
      "created_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

### GET /providers/:id

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/providers/a1b2c3d4...
```

### POST /providers

```bash
curl -X POST http://localhost:8090/api/v1/providers \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"GPT-4",
    "type":"text_model",
    "endpoint":"https://api.openai.com",
    "token":"sk-xxx",
    "model":"gpt-4",
    "temperature":0.7,
    "is_active":true
  }'
```

```json
{"code": 0, "data": {"id": "a1b2c3d4...", "name": "GPT-4", ...}}
```

### PUT /providers/:id

```bash
curl -X PUT http://localhost:8090/api/v1/providers/a1b2c3d4... \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"GPT-4","type":"text_model","endpoint":"https://api.openai.com","token":"sk-new","model":"gpt-4","temperature":0.5}'
```

### DELETE /providers/:id

```bash
curl -X DELETE http://localhost:8090/api/v1/providers/a1b2c3d4... \
  -H "Authorization: Bearer <token>"
```

### PUT /providers/:id/toggle

```bash
curl -X PUT http://localhost:8090/api/v1/providers/a1b2c3d4.../toggle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_active":false}'
```

```json
{"code": 0, "data": null}
```

---

## MCP 管理

### GET /mcp

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/mcp
```

```json
{
  "code": 0,
  "data": [
    {
      "id": "mcp-001...",
      "name": "My MCP Server",
      "server_url": "http://localhost:3001",
      "headers": {"X-Key": "value"},
      "timeout": 30000,
      "retry_count": 3,
      "tool_filter": [],
      "auto_reconnect": true,
      "is_active": true
    }
  ]
}
```

### GET /mcp/:id

### POST /mcp

```bash
curl -X POST http://localhost:8090/api/v1/mcp \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My MCP Server",
    "server_url": "http://localhost:3001",
    "headers": {},
    "timeout": 30000,
    "retry_count": 3,
    "auto_reconnect": true,
    "is_active": true
  }'
```

### PUT /mcp/:id

### DELETE /mcp/:id

### PUT /mcp/:id/toggle

```bash
curl -X PUT http://localhost:8090/api/v1/mcp/mcp-001/toggle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_active":false}'
```

---

## 记忆管理

### GET /memory/:chatAreaID/short-term

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/memory/<chat-area-id>/short-term
```

```json
{
  "code": 0,
  "data": {
    "id": "mem-001...",
    "chat_area_id": "area-001...",
    "window_size": 20,
    "auto_compact": false
  }
}
```

### PUT /memory/:chatAreaID/short-term

```bash
curl -X PUT http://localhost:8090/api/v1/memory/<chat-area-id>/short-term \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"window_size":30,"auto_compact":true}'
```

### GET /memory/:chatAreaID/long-term

```json
{
  "code": 0,
  "data": {
    "id": "lmem-001...",
    "chat_area_id": "area-001...",
    "hot_area_size": 10,
    "hot_memory_ttl": 86400
  }
}
```

### PUT /memory/:chatAreaID/long-term

```bash
curl -X PUT http://localhost:8090/api/v1/memory/<chat-area-id>/long-term \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"hot_area_size":15,"hot_memory_ttl":3600}'
```

---

## Prompt 管理

### GET /prompts

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/prompts
```

```json
{
  "code": 0,
  "data": [
    {
      "id": "p-001...",
      "name": "system-default",
      "type": "system",
      "content": "你是一个友好的 QQ 机器人助手...",
      "is_active": true,
      "variables": ["UserName", "GroupName"]
    }
  ]
}
```

### POST /prompts

```bash
curl -X POST http://localhost:8090/api/v1/prompts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"system-default",
    "type":"system",
    "content":"你是一个友好的 QQ 机器人助手，名字叫娟娘。",
    "is_active":true,
    "variables":["UserName","GroupName","Time"]
  }'
```

### PUT /prompts/:id

### DELETE /prompts/:id

---

## Session 管理

### GET /sessions

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/sessions
```

```json
{
  "code": 0,
  "data": [
    {
      "id": "sess-001...",
      "chat_area_id": "area-001...",
      "model": "gpt-4",
      "token_usage": 12345,
      "meta_data": {},
      "chat_area": {
        "id": "area-001...",
        "area_type": "group",
        "target_id": 123456789
      }
    }
  ]
}
```

### GET /sessions/:id

### DELETE /sessions/:id

```bash
curl -X DELETE http://localhost:8090/api/v1/sessions/sess-001... \
  -H "Authorization: Bearer <token>"
```

---

## Skill 管理

### GET /skills

```json
{
  "code": 0,
  "data": [
    {
      "id": "sk-001...",
      "name": "weather",
      "description": "天气查询",
      "keywords": ["天气", "weather"],
      "regex_pattern": "",
      "prompt_ref": "",
      "tool_refs": ["browser_search"],
      "mcp_refs": [],
      "is_active": true,
      "is_system": false,
      "priority": 10
    }
  ]
}
```

### POST /skills

```bash
curl -X POST http://localhost:8090/api/v1/skills \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"weather","description":"天气查询","keywords":["天气","weather"],"is_active":true,"priority":10}'
```

### PUT /skills/:id

### DELETE /skills/:id

---

## Tool 管理

### GET /tools

```json
{
  "code": 0,
  "data": [
    {"id": "t-001", "name": "send_group_msg", "description": "发送群聊消息", "is_active": true, "is_builtin": true},
    {"id": "t-002", "name": "get_time", "description": "获取当前时间", "is_active": true, "is_builtin": true}
  ]
}
```

### PUT /tools/:id/toggle

```bash
curl -X PUT http://localhost:8090/api/v1/tools/t-001/toggle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_active":false}'
```

---

## Plugin 管理

### GET /plugins

```json
{
  "code": 0,
  "data": [
    {"name": "ping", "version": "1.0.0", "author": "JuanNiang", "description": "Ping 插件"}
  ]
}
```

### POST /plugins/upload

```bash
curl -X POST http://localhost:8090/api/v1/plugins/upload \
  -H "Authorization: Bearer <token>" \
  -F "file=@my-plugin.zip"
```

```json
{"code": 0, "data": {"name": "my-plugin", "status": "loaded"}}
```

### PUT /plugins/:id/toggle

```bash
curl -X PUT http://localhost:8090/api/v1/plugins/my-plugin/toggle \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_active":false}'
```

### DELETE /plugins/:id

```bash
curl -X DELETE http://localhost:8090/api/v1/plugins/my-plugin \
  -H "Authorization: Bearer <token>"
```

---

## ACL 管理

### GET /acl

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/acl
```

```json
{
  "code": 0,
  "data": [
    {
      "id": 1,
      "user_id": 123456789,
      "chat_area_id": "area-001...",
      "permission": "denied",
      "actions": []
    }
  ]
}
```

### POST /acl

```bash
curl -X POST http://localhost:8090/api/v1/acl \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"user_id":123456789,"chat_area_id":"area-001...","permission":"denied","actions":[]}'
```

### DELETE /acl/:id

```bash
curl -X DELETE http://localhost:8090/api/v1/acl/1 \
  -H "Authorization: Bearer <token>"
```

---

## 聊天记录

### GET /chat-records/:chatAreaID

全部记录 (分页)。支持 `role` query 参数过滤:

```bash
# 全部记录
curl "http://localhost:8090/api/v1/chat-records/area-001?limit=20&offset=0" \
  -H "Authorization: Bearer <token>"

# 仅用户消息
curl "http://localhost:8090/api/v1/chat-records/area-001?role=user&limit=20" \
  -H "Authorization: Bearer <token>"

# 仅助手回复
curl "http://localhost:8090/api/v1/chat-records/area-001?role=assistant" \
  -H "Authorization: Bearer <token>"

# 仅工具调用记录 (含 MCP 调用)
curl "http://localhost:8090/api/v1/chat-records/area-001?role=tool" \
  -H "Authorization: Bearer <token>"
```

```json
{
  "code": 0,
  "data": {
    "total": 150,
    "list": [
      {
        "id": 1001,
        "chat_area_id": "area-001...",
        "user_id": 123456789,
        "role": "user",
        "content": "今天天气怎么样",
        "token_count": 5,
        "tool_calls": {},
        "created_at": "2026-07-18T12:00:00Z"
      },
      {
        "id": 1002,
        "chat_area_id": "area-001...",
        "user_id": 0,
        "role": "assistant",
        "content": "今天天气晴朗...",
        "token_count": 50,
        "tool_calls": {},
        "created_at": "2026-07-18T12:00:01Z"
      },
      {
        "id": 1003,
        "chat_area_id": "area-001...",
        "user_id": 0,
        "role": "tool",
        "content": "browser_search: 搜索结果...",
        "token_count": 100,
        "tool_calls": {"tool_name": "browser_search", "status": "done"},
        "created_at": "2026-07-18T12:00:02Z"
      }
    ]
  }
}
```

### GET /chat-records/:chatAreaID/token-usage

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8090/api/v1/chat-records/area-001/token-usage
```

```json
{
  "code": 0,
  "data": {
    "chat_area_id": "area-001...",
    "token_usage": 123456
  }
}
```

---

## Chat Areas

### GET /chat-areas

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/chat-areas
```

```json
{
  "code": 0,
  "data": [
    {
      "id": "area-001...",
      "area_type": "group",
      "target_id": 123456789,
      "created_at": "2026-01-01T00:00:00Z"
    },
    {
      "id": "area-002...",
      "area_type": "private",
      "target_id": 987654321,
      "created_at": "2026-06-01T00:00:00Z"
    }
  ]
}
```

---

## 全局概览

### GET /overview

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8090/api/v1/overview
```

```json
{
  "code": 0,
  "data": {
    "chat_area_count": 42,
    "mcp_count": 3,
    "adapter_count": 1,
    "plugin_count": 5,
    "provider_count": 2,
    "skill_count": 8,
    "session_count": 42,
    "total_token_usage": 1234567
  }
}
```

---

## 健康检查

### GET /health

```bash
curl http://localhost:8090/health
```

```json
{"status": "ok"}
```

---

## 错误响应

所有错误统一格式:

```json
{
  "code": 400,
  "msg": "参数格式错误"
}
```

| Code | 说明 |
|------|------|
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 过期 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
