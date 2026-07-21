# 调用栈 (Call Stack)

## 启动流程

```
main()
├─ postgres.NewPostgresClient(...)
├─ redis.NewRedisSentinelClient(...)
├─ sandbox.NewClient(...)       // 可选, 失败则 nil
├─ t2i.NewClient(...)           // 可选, 失败则 nil
│
├─ core.Init(ctx, db, redis)
│   ├─ AutoMigrate(db)          // 15 张表
│   ├─ cache.NewCache(redis)
│   ├─ dao.NewBundle(db)        // 15 个 DAO
│   ├─ acl.NewACL(bundle.ACL)
│   └─ InitAdminUser(bundle.User) // 首次: admin/Admin123
│
├─ adapter.New(cfg)
│   └─ adapter.Start(ctx)
│       ├─ newWSServer(ctx, addr, token, events)
│       └─ go wsServer.serve() → handleWS → readLoop → parseEvent
│
├─ agent.NewHagoCenter()
│   └─ hago.Init(ctx, cfg)
│       ├─ session.NewSessionManager(dao, cache)
│       ├─ prompt.NewPromptManager(dao)
│       │   └─ EnsureSystemPrompt(ctx)   // 幂等种子系统锁定提示词 __system_locked__
│       │       └─ DAO.GetByName → 不存在则 Create, 存在则同步内容到最新版本
│       ├─ skills.NewSkillEngine()
│       ├─ tool.RegisterBuiltinTools(...)  // 16 个内置工具
│       ├─ loadProviders(ctx)              // DB → ProviderGroup
│       ├─ loadMCPs(ctx)                   // DB → MCPGroup (MCP SDK)
│       ├─ loadSkills(ctx)                 // DB → SkillEngine
│       ├─ NewBackgroundTaskExecutor(...)
│       ├─ NewDrainerAgent(...)
│       └─ cronjob.New(h.DAO.CronJob, h.CronJobEvents)  // CronJob 调度器初始化
│
├─ hago.Start(ctx)
│   ├─ go BgTaskExecutor.Run()
│   ├─ go DrainerAgent.Run()
│   ├─ go h.CronJobManager.Run(ctx)
│   └─ go runEventLoop()
│
├─ pluggin.NewPluginEngine(path, adapter, db, cache, t2i, sandbox, dao, agentOp)
│   ├─ registerBuiltinCommands()    // 注册 /help 系统命令到 CommandRegistry
│   └─ LoadAll()
│       ├─ ensureEmbeddedAssets()   // 把 //go:embed 的 jn.lua 与 system 插件落盘
│       │   ├─ data/pluggins/sdk/jn.lua   (SDK, 总是覆盖)
│       │   └─ data/pluggins/system/{pluggin.yaml, main.lua}  (仅不存在时写)
│       └─ for each plugin dir → Load()
│           ├─ readManifest("pluggin.yaml")
│           ├─ lua.NewState()
│           ├─ injectSDK(L, name)         // 把 sdk 目录加入 package.path
│           ├─ injectBaseAPI(L, name, perms)  // log, json, onebot11, http, ...
│           ├─ injectCommandAPI(L, name)  // 注册 __jn_internal.register_command
│           └─ L.DoFile(entry.lua)        // 插件 require("jn") + jn.command.register
│
├─ api/engine.New(addr, svc)
│   ├─ server.Default(addr)
│   ├─ h.Use(Recovery, CORS)
│   └─ router.RegisterRoutes(h, svc)
│       └─ 22 个路由组
│
├─ go webEngine.Spin()       // Hertz 启动
│
└─ <-ctx.Done()              // 等待信号, 优雅退出
```

## 消息处理调用栈

```
OneBot11 WS → adapter.readLoop()
├─ gjson 解析 → parseEvent()
├─ events chan ← Event{PostType, Message, Notice, ...}
│
└─ agent.runEventLoop()  // for + select 多路复用
    │
    ├─ pluggin.OnMessage(event)
    │   ├─ 存储 currentEv (供 agent.get_current_chat_area() 查询)
    │   ├─ [NEW] if raw_message 以 "/" 开头:
    │   │   └─ CommandRegistry.Dispatch(raw, event)
    │   │       ├─ 最长前缀匹配命令树
    │   │       ├─ 命中 handler → handler(args, event) → (consumed, reply, err)
    │   │       │                └─ sendReply(event, reply)  (reply 非空时)
    │   │       └─ 未命中但有子命令 → reply 子命令列表
    │   │   if consumed: continue
    │   └─ for each plugin:
    │       └─ lua.Call("on_message", eventTable)
    │       └─ if consumed: continue
    │
    └─ handleMessage(ctx, ev)
        │
        ├─ dao.ChatArea.GetOrCreate(type, targetID)
        ├─ acl.Check(userID, chatAreaID, "chat")
        ├─ session.GetOrCreate(chatAreaID)
        ├─ skills.Match(rawMessage)
        ├─ prompt.BuildFullContext(ctx, longTermMems, toolDescs)
        │   └─ BuildSystemPrompt(ctx):
        │       ├─ dao.Prompt.ListSystemLocked()        // 1. SystemLocked (强制拼接)
        │       ├─ dao.Prompt.ListByType(System)        // 2. system (跳过 IsSystem)
        │       ├─ dao.Prompt.ListByType(Personality)   // 3. personality
        │       └─ dao.Prompt.ListByType(Custom)        // 4. custom
        │       → 内容直接拼接, 不再调用 template.Execute
        ├─ memory.GetShortTermMessages()  // ChatRecord 最近 N 条
        ├─ memory.GetLongTermMemory("", limit)
        │
        ├─ provider.Chat(ctx, ChatRequest{...})
        │   ├─ POST /v1/chat/completions
        │   └─ parseChatResponse → ChatResponse{Message, TokenUsage}
        │
        ├─ [文本响应]:
        │   ├─ [群聊] isSilenceResponse(content)?
        │   │   ├─ 是 → 丢弃(不记录), 跳过 handleToolCalls
        │   │   └─ 否 → sendReply(msg, content)
        │   │       ├─ [private] adapter.SendPrivateMsg
        │   │       └─ [group]   adapter.SendGroupMsg
        │   │           └─ wsServer.callAPI("send_group_msg", params)
        │   │               └─ WS write → OneBot11 客户端 → QQ API
        │   │       ├─ dao.ChatRecord.Create(assistant)
        │   │       └─ memory.AddShortTermMessage(assistant)
        │   │
        │   └─ [私聊] → sendReply → 记录 (同上)
        │
        └─ [Tool Call 响应]:
            └─ handleToolCalls(ctx, msg, chatAreaID, ...)
                │
                ├─ [短工具]:
                │   ├─ tool.Execute(ctx, name, args)
                │   ├─ 结果回传 → provider.Chat (followUp)
                │   └─ 递归 (重复上述流程)
                │
                └─ [长工具]:
                    ├─ BgTaskExecutor.Submit(chatAreaID, steps)
                    │   ├─ dao.BackgroundTask.Create (pending)
                    │   ├─ go executeAsync(task, steps)
                    │   │   ├─ dao.UpdateStatus(running)
                    │   │   ├─ errgroup.Go → tool.Execute (并发)
                    │   │   ├─ pushResult(OutputChan)
                    │   │   └─ dao.UpdateStatus(done)
                    │   └─ return taskID
                    │
                    ├─ sendReply("任务已提交后台执行")
                    │
                    └─ DrainerAgent.Run (独立 goroutine)
                        ├─ for output := range OutputChan
                        │   ├─ pending[chatAreaID] ← append(output)
                        │   ├─ [步骤完成]: sendProgress (每 3 步)
                        │   └─ [任务完成]: LLM 整合 → sendReply
                        └─ loop
    │
    └─ [CronJob 事件]:
        └─ case ev, ok := <-h.CronJobEvents:  // 注入队列后的 CronJob 事件
            └─ processEvent(ctx, ev) → handleMessage (跳过 Plugin 拦截)
```

## MCP 工具调用栈

```
MCP Server (SSE) ← MCP SDK (mark3labs/mcp-go)
│
├─ Connect(ctx)
│   ├─ client.NewSSEMCPClient(serverURL, headers)
│   ├─ client.Initialize(ctx, InitializeRequest)
│   └─ slog.Info("MCP 连接成功")
│
├─ ListTools(ctx)
│   ├─ client.ListTools(ctx, ListToolsRequest)
│   └─ return []ToolDefinition
│
└─ CallTool(ctx, name, args)
    ├─ client.CallTool(ctx, CallToolRequest{Name, Arguments})
    └─ mcp.GetTextFromContent(result.Content)
```

## Web API 调用栈

```
HTTP Request
│
├─ middleware.Recovery()
├─ middleware.CORS()
│
├─ [需认证]:
│   └─ middleware.JWTAuth()
│       ├─ 解析 Authorization: Bearer <token>
│       ├─ jwt.ParseWithClaims → Claims{UserID, Username, Role}
│       └─ [失败] → 401
│
└─ service.Service
    ├─ Login → bcrypt.CompareHashAndPassword → jwt.GenerateToken
    ├─ ListProviders → dao.Provider.List → JSON
    ├─ AddProvider → c.BindJSON → dao.Provider.Create → JSON
    ├─ ... (所有 CRUD 统一模式)
    └─ GetOverview → 多表 Count → JSON
```
