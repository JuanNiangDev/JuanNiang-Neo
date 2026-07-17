# 插件架构

## 概述

JuanNiang-Neo 的 Lua 插件系统基于 `gopher-lua` (Go-Lua 绑定), 允许用户通过 Lua 脚本扩展机器人功能。插件可以拦截 OneBot11 消息事件、调用机器人 API、访问日志系统等。

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    PluginEngine                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ Plugin A │  │ Plugin B │  │ Plugin C │  (独立 LState)   │
│  │ LState₁  │  │ LState₂  │  │ LState₃  │                 │
│  │          │  │          │  │          │                 │
│  │ manifest │  │ manifest │  │ manifest │                 │
│  │ API ✅   │  │ API ✅   │  │ API ✅   │  (权限控制)     │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                 │
│       │              │              │                       │
│       ▼              ▼              ▼                       │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Shared Go API Layer                     │   │
│  │  ┌──────────┬──────────┬──────────┬──────────────┐  │   │
│  │  │ log.*    │ json.*   │ onebot11 │ http.*       │  │   │
│  │  │ info     │ encode   │  .send_* │ get/post     │  │   │
│  │  │ warn     │ decode   │          │              │  │   │
│  │  │ error    │          │          │              │  │   │
│  │  └──────────┴──────────┴──────────┴──────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                         │                                   │
│                         ▼                                   │
│              SendAdapter (消息发送)                          │
│              adapter.Provider                               │
└─────────────────────────────────────────────────────────────┘

事件流中的插件位置:

  OneBot11 Event → PluginEngine.OnMessage(event)
                      │
                      ├─ for each plugin:
                      │    └─ lua.call("on_message", event)
                      │         ├─ consumed=true  → 跳过 Agent
                      │         └─ consumed=false → 继续
                      │
                      ▼
                   Agent 处理
```

## 插件目录结构

```
data/pluggins/
├── ping/                  # 插件名 = 目录名
│   ├── pluggin.yaml       # 插件清单 (必需)
│   └── main.lua           # 入口脚本 (默认)
├── weather/               # 另一个插件
│   ├── pluggin.yaml
│   ├── main.lua
│   └── utils.lua          # 插件内部依赖
└── ...
```

## pluggin.yaml 格式

```yaml
name: my-plugin            # 插件名称 (唯一)
version: "1.0.0"           # 语义化版本
author: YourName           # 作者
description: "插件描述"     # 简短描述
entry: main.lua            # 入口 Lua 文件 (默认 main.lua)
permissions:               # 权限列表, 控制 API 暴露
  - onebot11               #  OneBot11 消息发送
  - http                   #  HTTP 请求
  - database               #  数据库访问 (预留)
  - cache                  #  缓存访问 (预留)
  - t2i                    #  文生图 (预留)
  - sandbox                #  沙箱 (预留)
  - agent                  #  Agent 配置 (预留)
```

权限说明:
- `"*"` — 所有权限
- `"onebot11"` — 可调用 `onebot11.send_*` 函数
- `"http"` — 可调用 `http.get/post`
- 其他权限预留中, 当前仅 `onebot11` 和 `http` 有效

## 插件生命周期

```
加载 (Load):
  1. 读取 pluggin.yaml → 解析 Manifest
  2. 创建 LState (lua.NewState)
  3. injectBaseAPI: 根据 permissions 注入 Go 函数到 Lua 全局表
  4. L.DoFile(entry.lua): 执行插件脚本
  5. 注册到 Plugins map

运行:
  - on_message(event) 被事件循环调用 (如果存在该函数)
  - 插件可通过全局 onebot11/log/json 表调用 API

热加载 (Reload):
  1. Unload: LState.Close() → 从 map 删除
  2. Load: 重新执行上述流程

卸载 (Unload):
  1. LState.Close() → 释放 Lua VM
  2. 从 Plugins map 删除
```

## 内存模型

每个插件拥有独立的 `lua.LState`, 这意味着:
- 插件间的全局变量互不干扰
- 每个插件的 Lua VM 完全隔离
- 插件崩溃不影响其他插件或主进程 (gopher-lua 的 panic 被 recover 捕获)

Go ←→ Lua 类型映射:

| Go 类型 | Lua 类型 |
|---------|---------|
| `string` | `string` |
| `int / int64 / float64` | `number` |
| `bool` | `boolean` |
| `map[string]any` | `table` (key-value) |
| `[]any` | `table` (array, 1-indexed) |
| `nil` | `nil` |
