# 插件开发指南

## 快速开始

### 1. 创建插件目录

```bash
mkdir -p data/pluggins/my-plugin
```

### 2. 编写 pluggin.yaml

```yaml
name: my-plugin
version: "1.0.0"
author: YourName
description: "我的第一个插件"
entry: main.lua
permissions:
  - onebot11
```

### 3. 编写 main.lua

```lua
-- 插件初始化代码 (加载时执行一次)
log.info("my-plugin 已加载")

function on_message(event)
    local msg = event.raw_message

    -- 复读功能
    if msg == "复读" then
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, msg)
        end
        return true, event
    end

    return false, event
end
```

### 4. 启动服务

```bash
go run ./cmd/server/
# 或 Docker:
docker compose -f deployments/docker-compose.yaml up
```

服务启动后会自动加载 `data/pluggins/` 下的所有插件。

---

## 开发规范

### 目录结构约定

```
data/pluggins/<plugin-name>/
├── pluggin.yaml    # 插件清单 (必需)
├── main.lua        # 入口脚本 (默认)
└── *.lua           # 其他 Lua 文件
```

### 命名约定

- 插件目录名使用小写字母和连字符: `my-plugin`
- `name` 字段与目录名一致
- 版本号遵循 SemVer: `"1.0.0"`, `"2.1.3"`

### 错误处理

Lua 中的运行时错误会被 `PluginEngine` 捕获并记录到日志, 不会导致进程崩溃:

```
// 日志输出:
// ERROR [plugin:my-plugin] on_message 错误 err=<string>:4: attempt to ...
```

建议在插件中使用 `pcall` 进行关键操作的错误处理:

```lua
local ok, err = pcall(function()
    -- 可能出错的操作
    onebot11.send_group_msg(group_id, complex_message)
end)
if not ok then
    log.error("发送失败: " .. tostring(err))
end
```

### 性能注意事项

- `on_message` 函数会阻塞事件循环: 每条消息串行处理, 不要在 on_message 中执行耗时操作
- 避免在 `on_message` 中进行网络请求或复杂计算
- 插件加载时 (`main.lua` 顶层) 执行的代码只在加载时运行一次

---

## 完整示例

### 示例 1: Ping 插件

```lua
-- pluggin.yaml:
-- permissions: [onebot11]

function on_message(event)
    if event.raw_message == "/ping" then
        local reply = "pong!"
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, reply)
        else
            onebot11.send_private_msg(event.user_id, reply)
        end
        return true, event
    end
    return false, event
end
```

### 示例 2: 关键词回复

```lua
-- pluggin.yaml:
-- permissions: [onebot11]

-- 预定义回复表
local replies = {
    ["你好"] = "你好呀!",
    ["晚安"] = "晚安, 好梦~",
    ["早上好"] = "早上好! 新的一天开始了!",
}

function on_message(event)
    local msg = event.raw_message
    local reply = replies[msg]

    if reply then
        if event.message_type == "group" then
            onebot11.send_group_msg(event.group_id, reply)
        else
            onebot11.send_private_msg(event.user_id, reply)
        end
        return true, event
    end

    return false, event
end
```

### 示例 3: 入群欢迎

```lua
-- pluggin.yaml:
-- permissions: [onebot11]

-- 注意: 当前 on_message 仅接收 message 事件
-- 后续版本将支持 on_notice 事件用于入群通知
```

### 示例 4: 使用 JSON

```lua
-- 保存和读取配置
local config = {
    prefix = "/",
    admins = {12345, 67890}
}

-- 序列化
local cfg_str = json.encode(config)
log.info("配置: " .. cfg_str)

-- 反序列化
local loaded = json.decode(cfg_str)
log.info("prefix: " .. loaded.prefix)
```

---

## 调试

### 查看日志

插件日志输出到服务器标准输出:

```bash
# 直接运行
go run ./cmd/server/

# Docker
docker compose -f deployments/docker-compose.yaml logs -f juan-niang-neo
```

### 热加载

```bash
# 修改插件后热加载 (通过 Web API)
curl -X POST http://localhost:8090/api/v1/plugins/reload
```

或在代码中:

```go
pluginEngine.Reload("my-plugin")
```
