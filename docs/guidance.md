# JuanNiang-Neo

## 1. 项目简介

****

**JuanNiang-Neo** 是一个类似 **Astrbot** 的基于 **Onebot11** 的QQ聊天Agent

### 1.1 主要功能简介：

- Web管理面板：
  - 管理员用户登录
  - 可以调整Adapter的相关设置，启用，禁用Adapter
  - 查看，添加，删除，修改 Agent 的MCP服务器配置
  - 查看，修改Agent的Memory设置
  - 管理Agent的Prompt
  - 管理Agent的Provider
  - 管理Agent的Session
  - 管理Agent的Skill
  -  查看，启用，禁用Agent的Tool
  - 管理Agent的lua插件
  - 设置用户的ACL（访问控制列表）
  - 查看Agent在每一个Chat-Area内的聊天记录，工具，MCP调用记录，Skill触发记录，Token用量（一个单聊或者群聊就是一个Chat-Area，一个Chat-Area是Session和Memory的集合）
  - 查看全局概览：Chat-Area数，MCP，Adapter，插件数，Token总用量

- QQ聊天和QQ群聊管理
  - 可以在QQ内与这个Agent聊天
  - Agent的账号在管理员状态下可以进行QQ群管理
- Agent相关功能
  - 可以使用MCP，Tool等Agent功能
- lua插件功能：
  - 可以管理lua插件
  - 在聊天中使用lua插件
  - lua插件热加载
  - 指令lua插件化

## 2. 项目架构

| 模块             | 作用               | 说明                                                         |
| ---------------- | ------------------ | ------------------------------------------------------------ |
| `inferstructure` | **基础设施**       | postgres数据库，redis缓存，sandbox沙箱，t2i文转图            |
| `adapter`        | **Onebot11适配器** | 实现Onebot11协议的Websocket Server                           |
| `agent`          | **Agent模块**      | Agent的功能实现                                              |
| `api`            | **Web服务的API**   | Web界面使用的API                                             |
| `core`           | **机器人核心库**   | 实现ACL，数据库功能封装，缓存功能封装，其他功能封装，数据模型 |
| `pluggin`        | **lua插件系统**    | 实现Lua引擎，Lua插件的相关操作                               |

- `agent`模块

| 模块       | 作用                  |
| ---------- | --------------------- |
| `mcp`      | **MCP相关功能**       |
| `memory`   | **Agent的记忆实现**   |
| `prompt`   | **Agent的提示词实现** |
| `provider` | **大模型提供商**      |
| `session`  | **大模型Session实现** |
| `skill`    | **Skill功能**         |
| `tool`     | **内置工具**          |

- `api`模块

| 模块       | 作用               |
| ---------- | ------------------ |
| engine     | **Hertz web 引擎** |
| middleware | **中间件**         |
| router     | **路由注册**       |
| service    | **api功能实现**    |

- `core` 模块

| 模块    | 作用             |
| ------- | ---------------- |
| acl     | **访问控制列表** |
| cache   | 缓存功能封装     |
| dao     | 数据库功能封装   |
| handler | 其他高级功能     |
| models  | 数据模型         |

## 3. Onebot11事件数据流

```
Onebot11 --> pluggin(插件拦截，重写) --> Agent --> 调用用Onebot11Api函数封装的Tool
```

## 4. 实现细则：

### 4.1 概述：

这个项目的状态完全由数据库和缓存提供（除lua插件，lua插件的配置由目录下的pluggin.yaml提供），例如即使Agent模块是有状态的，它最终状态还是要和数据库同步。

### 4.2 Web控制台

- 鉴权使用JWT
- 支持OIDC单点登录
- 只有一个admin用户，初始密码Admin123

### 4.3 QQ群聊管理

- 把Onebot11提供的Api函数注册成Agent的Tool

### 4,4 文生图和沙箱

- 也是把API封装成Agent的Tool给Agent自行调用

### 4.5 Skill

- 存数据库里面
- 有系统Skill，每次对话强制加载

### 4.6 后台任务与记忆

> 某些任务可能会执行很长时间，为保证Agent不阻塞，这个使用后台任务执行

- 后台任务即把一个任务放在后台执行，比如MCP调用，Tool调用
- 如果一个任务的某个步骤跑了太长时间，就把这个任务挂到后台继续执行，并写入bgtask Memory
- 如果一个任务有很多个耗时的步骤且这些步骤相互解耦，那就放在后台，如果某些步骤需要其他步骤的结果，那就等待其他所有需要的步骤执行好后再启动
- 每个步骤执行好后，把执行好结果放一个缓冲区里，由大模型统一处理（同一时间只能由一个Agent处理这些，如果当前由Agent处理中的话新的执行结果就在管道里等待）
- 负责处理的Agent和处理聊天的Agent不是同一个
- 当所有任务执行好后整合好结果并发送QQ消息
- 可以参考errgroup

### 4.7 异步Agent

- 如果当前由多个Agent任务，就要用异步Agent保证任务不阻塞
- 注意并发记忆的问题

### 4.8 内置Tool

- 浏览器搜索（沙箱）
- 命令执行（沙箱）
- 代码执行（沙箱）
- Onebot11相关
- 时间
- 文转图

> Onebot11要可以发送富文本QQ消息：LLM调用工具时返回一个json数组，Tool执行时解析这个json并构建消息

### 4.9 Pluigin

- 要支持Web界面上传插件压缩包添加插件
- 外层要暴露足够的API以满足Lua插件的开发，包括但不限于：Onebot11操作，HTTP访问，Agent操作（包括Agent的相关设置），数据库，缓存（这两个注意与系统使用的数据分开），文转图，沙箱操作等
- 要提供日志功能
- 插件存储在data\pluggins下