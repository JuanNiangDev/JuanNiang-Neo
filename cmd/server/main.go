package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"JuanNiang-Neo/infrastructure/postgres"
	"JuanNiang-Neo/infrastructure/rag"
	ragcaller "JuanNiang-Neo/infrastructure/rag/handler"
	"JuanNiang-Neo/infrastructure/redis"
	sandbox "JuanNiang-Neo/infrastructure/sandbox"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2i "JuanNiang-Neo/infrastructure/t2i"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent"
	"JuanNiang-Neo/internal/agent/fishcal"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/scheduledmsg"
	"JuanNiang-Neo/internal/api/engine"
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/service"
	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/imgstore"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
	"JuanNiang-Neo/internal/pluggin"
	"JuanNiang-Neo/internal/web"

	"github.com/cloudwego/hertz/pkg/app/server"
)

var log = logging.NewModule("main")

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ---------- 命令行参数 ----------
	debug := flag.Bool("debug", false, "启用 debug 模式：pprof + 详细日志")
	pprofAddr := flag.String("pprof-addr", ":6060", "pprof HTTP 监听地址（仅 debug 模式有效）")
	devConfigPath := flag.String("dev-config", "dev.yaml", "开发配置文件路径（不存在则静默跳过）")
	flag.Parse()

	// ---------- 加载 dev.yaml ----------
	devCfg := loadDevConfig(*devConfigPath)

	// 预处理 OneBot11 配置（loadAdapterConfig 通过 env() 读取）
	setEnvIfUnset("OB_PORT", devCfg.OneBot11.Port)
	setEnvIfUnset("OB_TOKEN", devCfg.OneBot11.Token)
	if len(devCfg.OneBot11.Admins) > 0 && os.Getenv("OB_ADMINS") == "" {
		os.Setenv("OB_ADMINS", strings.Join(devCfg.OneBot11.Admins, ","))
	}

	// ---------- Banner ----------
	printBanner()

	// ---------- 日志 ----------
	// 初始化新日志系统（彩色输出 + JSON 格式化 + 调用栈 + Hub 推送）
	logging.Init(logging.Config{
		Debug:       *debug,
		Output:      os.Stdout,
		Hub:         logging.DefaultHub,
		LLMMaxChars: 300,
	})

	// 保留 slog 桥接：现有 slog.Info/Warn/Error 调用自动走新系统
	var logLevel slog.Leveler = slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(logging.NewHandler(os.Stdout, logging.DefaultHub, &slog.HandlerOptions{
		Level: logLevel,
	})))

	log.Info("JuanNiang-Neo 启动中...")

	// ---------- Debug 模式 ----------
	if *debug {
		log.Info("🐛 Debug 模式已启用",
			"pprof_addr", *pprofAddr,
			"go_version", runtime.Version(),
			"cpu_num", runtime.NumCPU(),
			"goroot", runtime.GOROOT(),
		)
		go func() {
			log.Info("pprof HTTP 已启动", "addr", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				log.Error("pprof 服务异常退出", "err", err)
			}
		}()
	}

	// ---------- 1. 基础设施 ----------
	db, err := postgres.NewPostgresClient(
		postgres.WithHost(devEnv("DB_HOST", devCfg.Database.Host, "localhost")),
		postgres.WithPort(devEnv("DB_PORT", devCfg.Database.Port, "5432")),
		postgres.WithUser(devEnv("DB_USER", devCfg.Database.User, "postgres")),
		postgres.WithPassword(devEnv("DB_PASSWORD", devCfg.Database.Password, "postgres")),
		postgres.WithDefaultDB(devEnv("DB_NAME", devCfg.Database.Name, "juan")),
	)
	if err != nil {
		log.Error("Postgres 连接失败", "err", err)
		os.Exit(1)
	}
	log.Info("Postgres 已连接")

	redisClient, err := redis.NewRedisSentinelClient(
		redis.WithAddr(devEnv("REDIS_ADDR", devCfg.Redis.Addr, "localhost:6379")),
		redis.WithPassword(devEnv("REDIS_PASSWORD", devCfg.Redis.Password, "root")),
		redis.WithDB(mustAtoi(devEnv("REDIS_DB", devCfg.Redis.DB, "0"))),
	)
	if err != nil {
		log.Error("Redis 连接失败", "err", err)
		os.Exit(1)
	}
	log.Info("Redis 已连接")

	coreInst, err := core.Init(ctx, db, redisClient)
	if err != nil {
		log.Error("Core 初始化失败", "err", err)
		os.Exit(1)
	}
	log.Info("Core 已初始化")

	if s := devEnv("JWT_SECRET", devCfg.JWT.Secret, ""); s != "" {
		middleware.JWTSecret = []byte(s)
	}

	adapterCfg := loadAdapterConfig(ctx, coreInst.DAO)
	adapterProv := adapter.New(adapterCfg)

	// ---------- 图床 ----------
	// 图片二进制存 data/imgs（IMG_DIR 可覆盖）；元数据在 Postgres。
	imgDir := devEnv("IMG_DIR", devCfg.Images.Dir, "data/imgs")
	imgStore := imgstore.New(imgDir)
	if err := imgStore.EnsureDir(); err != nil {
		log.Warn("图床目录创建失败", "dir", imgDir, "err", err)
	}
	// 发送消息时把 imgs://<id> 图床引用解析为 base64（Onebot11 与机器人网络可能不互通）。
	adapterProv.SetImageResolver(func(raw string) (string, bool) {
		if !strings.HasPrefix(raw, "imgs://") {
			return "", false
		}
		id := strings.TrimPrefix(raw, "imgs://")
		b64, err := imgStore.LoadBase64(id)
		if err != nil {
			log.Warn("图床图片加载失败", "id", id, "err", err)
			return "", false
		}
		return "base64://" + b64, true
	})
	// 发送表情时把 stk://<短UUID> 解析为 base64：短 UUID → 表情 → 图床长 UUID → base64。
	adapterProv.SetStickerResolver(func(stickerID string) (string, bool) {
		st, err := coreInst.DAO.Sticker.GetByID(ctx, stickerID)
		if err != nil {
			log.Warn("表情不存在", "id", stickerID, "err", err)
			return "", false
		}
		b64, err := imgStore.LoadBase64(st.ImageID)
		if err != nil {
			log.Warn("表情图片加载失败", "sticker", stickerID, "img", st.ImageID, "err", err)
			return "", false
		}
		return "base64://" + b64, true
	})
	if adapterCfg.Enable {
		if err := adapterProv.Start(ctx); err != nil {
			log.Error("Adapter 启动失败", "err", err)
			os.Exit(1)
		}
	} else {
		log.Info("Adapter 已禁用（DB 配置 Enable=false），跳过启动")
	}

	// ---------- 4b. Webhook Adapter ----------
	webhookEvents := make(chan adapter.Event, 128)
	webhookCfg, err := loadWebhookConfig(ctx, coreInst.DAO)
	if err != nil {
		log.Warn("Webhook 配置加载失败", "err", err)
	}
	webhookAdapter := adapter.NewWebhookAdapter(adapter.WebhookConfig{
		Addr:   webhookCfg.Addr,
		Port:   webhookCfg.Port,
		Token:  webhookCfg.Token,
		Admins: adapterCfg.Admins,
		Enable: webhookCfg.Enabled,
	}, webhookEvents)
	if webhookCfg.Enabled {
		if err := webhookAdapter.Start(ctx); err != nil {
			log.Error("Webhook adapter 启动失败", "err", err)
			os.Exit(1)
		}
	}

	// ---------- 5. Agent ----------

	hago := agent.NewHagoCenter()
	if err := hago.Init(ctx, agent.Config{
		Adapter:        adapterProv,
		WebhookAdapter: webhookAdapter,
		Sandbox:        nil,
		T2I:            nil,
		RAG:            nil,
		Providers:      hago.Providers,
		MCPGroup:       hago.MCP,
		DAO:            coreInst.DAO,
		ACL:            coreInst.ACL,
		Cache:          coreInst.Cache,
	}); err != nil {
		log.Error("Agent 初始化失败", "err", err)
		os.Exit(1)
	}

	if err := hago.Start(ctx); err != nil {
		log.Error("Agent 启动失败", "err", err)
		os.Exit(1)
	}

	// ---------- 6. Plugin Engine ----------

	pluginEngine := pluggin.NewPluginEngine(
		"data/pluggins",
		pluggin.WrapAdapter(adapterProv),
		db,
		coreInst.Cache,
		nil,
		nil,
		coreInst.DAO,
		hago,
		hago,
	)
	if err := pluginEngine.LoadAll(); err != nil {
		log.Error("插件加载失败", "err", err)
	}
	if *debug {
		plugins := pluginEngine.List()
		log.Debug("插件加载完毕", "count", len(plugins))
		for _, p := range plugins {
			log.Debug("  → 插件", "name", p.Name, "version", p.Version, "system", p.System, "permissions", p.Permissions)
		}
	}
	hago.PluginEngine = pluginEngine

	// 将 PluginEngine 注册为 Webhook 插件路由器
	webhookAdapter.SetPluginRouter(pluginEngine)

	// ---------- 7. Web API ----------

	svc := service.New(coreInst.DAO, adapterProv, webhookAdapter, pluginEngine)
	// 插件商店客户端（拉取元数据 / 安装 / 镜像源管理），数据目录持久化配置。
	svc.StoreClient = pluggin.NewStoreClient("data")
	svc.ProviderGroup = hago.Providers
	svc.MCPGroup = hago.MCP
	svc.MemoryGroup = hago.Memory
	svc.SessionMgr = hago.Session
	svc.ToolRegistry = hago.Tools
	svc.SkillEngine = hago.Skills
	svc.ACLMgr = hago.ACL

	// T2I / Sandbox 运行时同步：从 DB 加载配置并设置回调
	loadT2IFromDB(ctx, svc, coreInst.DAO, hago)
	loadSandboxFromDB(ctx, svc, coreInst.DAO, hago)
	loadRAGFromDB(ctx, svc, coreInst.DAO, hago)
	svc.OnUpdateT2I = func(client *t2icaller.Client) { hago.T2IClient = client }
	svc.OnUpdateSandbox = func(client *sandboxcaller.Client) { hago.SandboxClient = client }
	svc.OnUpdateRAG = func(client *ragcaller.Client) { hago.RAGClient = client }
	svc.OnRebuildAgent = func() { hago.RebuildEinoAgent(ctx) }
	svc.OnUpdateToolAdminOnly = func() { hago.RefreshToolAdminOnly(ctx) }
	svc.OnReplyStrategyChanged = func() { hago.InvalidateReplySettings() }
	svc.OnKnowledgeChanged = func() { hago.InvalidateKnowledgeLRU() }
	svc.OnExtractKnowledge = func(id string) { hago.ExtractKeywordsAsync(ctx, id) }
	svc.CronJobManager = hago.CronJobManager
	svc.LoopTracker = hago.Loops
	svc.PromptMgr = hago.Prompt
	svc.ImageStore = imgStore

	// ---------- 摸鱼人日历（独立调度器，不复用 CronJob） ----------
	fishCal := fishcal.New(coreInst.DAO.FishCalendar,
		func() *t2icaller.Client { return hago.T2IClient },
		adapterProv)
	go fishCal.Run(ctx)
	svc.OnFishCalReload = func() { fishCal.Reload(context.Background()) }
	svc.OnFishCalTrigger = func(triggerCtx context.Context) error { return fishCal.TriggerNow(triggerCtx) }

	// ---------- 定时消息（独立调度器） ----------
	schedMgr := scheduledmsg.New(coreInst.DAO.ScheduledMsg,
		func() *t2icaller.Client { return hago.T2IClient },
		adapterProv)
	go schedMgr.Run(ctx)
	svc.OnSchedMsgReload = func() { schedMgr.Reload(context.Background()) }
	svc.OnSchedMsgTrigger = func(triggerCtx context.Context, id string) error { return schedMgr.TriggerNow(triggerCtx, id) }

	// 前端静态资源目录: 默认 web/dist (构建产物), 可通过 WEB_DIR 覆盖。
	//   - 开发模式: 前端走 Vite (:3000) 代理 /api 到 :8090, 后端无需服务前端。
	//   - 生产/裸跑: make web-build 后, 后端直接 serve web/dist 作为 SPA。
	//   - 目录不存在或未构建时, 后端走引导提示页, 不影响 API 与 /health。
	webDir := devEnv("WEB_DIR", devCfg.Web.Dir, "web/dist")
	if err := web.EnsureDir(webDir); err != nil {
		log.Warn("WEB_DIR 校验失败", "dir", webDir, "err", err)
	}
	apiAddr := devEnv("API_ADDR", devCfg.API.Addr, ":8090")
	webEngine := engine.New(apiAddr, webDir, svc)

	// 用 Run 而非 Spin: Spin 会自注册 SIGINT/SIGTERM handler 并在我们已注册
	// signal.NotifyContext 的同时另起一套, 导致 Ctrl-C 时 Spin 内部的
	// Shutdown(context.Background()) 卡在 SSE 长连接上, 与主流程 defer 互锁。
	// 这里我们只复用 Hertz 的 Run, 用主 ctx 显式控制生命周期。
	webErrCh := make(chan error, 1)
	go func() {
		log.Info("Web API 已启动", "addr", apiAddr)
		webErrCh <- webEngine.Run()
	}()

	log.Info("JuanNiang-Neo 已就绪",
		"adapter_addr", adapterProv.Status().ListenAddr,
		"api_addr", apiAddr,
		"plugins", len(pluginEngine.List()),
		"goroutines", runtime.NumGoroutine(),
	)

	// ---------- 8. 等待退出 ----------
	// 主 ctx 在收到 Ctrl-C / SIGTERM 时被取消。我们以反向顺序停掉各组件,
	// 全部用带 deadline 的 shutdownCtx, 避免任何子组件挂死拖垮整体退出。
	<-ctx.Done()
	log.Info("收到退出信号，正在关闭...")

	// watchdog: 若 15s 内未完成优雅退出则强制结束, 避免任何 Stop 调用挂死。
	shutdownBudget := 15 * time.Second
	done := make(chan struct{})
	go func() {
		shutdown(adapterProv, webhookAdapter, hago, webEngine, pluginEngine)
		close(done)
	}()
	select {
	case <-done:
		log.Info("已优雅退出")
	case <-time.After(shutdownBudget):
		log.Error("优雅关闭超时, 强制退出", "budget", shutdownBudget)
		os.Exit(1)
	}
}

// shutdown 按反向顺序停掉各组件, 每步独立带 deadline, 任一卡死不影响后续。
// 注意: 先停 adapter 再停 web 引擎, 避免 web 请求持 adapter 锁导致 Stop 死锁。
func shutdown(adapterProv *adapter.Adapter, webhookAdapter *adapter.WebhookAdapter, hago *agent.HagoCenter, webEngine *server.Hertz, pluginEngine *pluggin.PluginEngine) {
	// 8.1 先停 Agent (关闭事件循环, 避免后续 adapter 关闭时事件循环还在消费)。
	hago.Stop()

	// 8.2 停 Webhook adapter。
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := webhookAdapter.Stop(stopCtx); err != nil {
		log.Warn("Webhook adapter 关闭出错", "err", err)
	}
	cancel()

	// 8.3 停 OneBot11 反向 WS adapter。
	stopCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	if err := adapterProv.Stop(stopCtx); err != nil {
		log.Warn("Adapter 关闭出错", "err", err)
	}
	cancel()

	// 8.4 停 Web 引擎 (adapter 已停, 不再有请求竞争 adapter 锁)。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := webEngine.Shutdown(shutdownCtx); err != nil {
		log.Warn("Web 引擎关闭出错", "err", err)
	}
	cancel()

	// 8.5 关闭 plugin engine。
	_ = pluginEngine
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// setEnvIfUnset 当环境变量未设置且 val 非空时，设置环境变量。
func setEnvIfUnset(key, val string) {
	if val != "" && os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseAdmins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var admins []string
	for _, id := range parts {
		admins = append(admins, id)
	}
	return admins
}

func loadT2IFromDB(ctx context.Context, svc *service.Service, daos *dao.Bundle, hago *agent.HagoCenter) {
	cfg, err := daos.T2I.GetConfig(ctx)
	if err != nil {
		// 数据库无配置 → 初始化默认配置，保证前端读取不报错
		if initErr := daos.T2I.InitConfig(ctx); initErr != nil {
			log.Warn("T2I 默认配置初始化失败", "err", initErr)
			return
		}
		cfg, err = daos.T2I.GetConfig(ctx)
		if err != nil {
			log.Warn("T2I 配置加载失败，使用默认", "err", err)
			return
		}
	}
	// 同步渲染风格选择到提示词注入（空 = 随机），独立于 T2I 服务启用状态
	prompt.SetSelectedT2IStyle(cfg.SelectedStyle)
	if !cfg.IsActive {
		log.Info("T2I 未启用")
		return
	}
	client, err := t2i.NewClient(
		t2i.WithBaseURL(cfg.BaseURL),
		t2i.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
	)
	if err != nil {
		log.Warn("T2I 客户端创建失败", "err", err)
		return
	}
	svc.T2IClient = client
	hago.T2IClient = client
	log.Info("T2I 客户端已就绪", "base_url", cfg.BaseURL)

	// T2I 客户端晚于 buildEinoAgent 就绪，重建 Agent 注册 text_to_image 工具
	hago.RebuildEinoAgent(ctx)
}

func loadSandboxFromDB(ctx context.Context, svc *service.Service, daos *dao.Bundle, hago *agent.HagoCenter) {
	cfg, err := daos.Sandbox.GetConfig(ctx)
	if err != nil {
		// 数据库无配置 → 初始化默认配置，保证前端读取不报错
		if initErr := daos.Sandbox.InitConfig(ctx); initErr != nil {
			log.Warn("Sandbox 默认配置初始化失败", "err", initErr)
			return
		}
		cfg, err = daos.Sandbox.GetConfig(ctx)
		if err != nil {
			log.Warn("Sandbox 配置加载失败，使用默认", "err", err)
			return
		}
	}
	if !cfg.IsActive {
		log.Info("Sandbox 未启用")
		return
	}
	client, err := sandbox.NewClient(
		sandbox.WithBaseURL(cfg.BaseURL),
		sandbox.WithAPIKey(cfg.APIKey),
		sandbox.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
	)
	if err != nil {
		log.Warn("Sandbox 客户端创建失败", "err", err)
		return
	}
	svc.SandboxClient = client
	hago.SandboxClient = client
	log.Info("Sandbox 客户端已就绪", "base_url", cfg.BaseURL)

	// Sandbox 客户端晚于 buildEinoAgent 就绪，重建 Agent 注册 sandbox 系列工具
	hago.RebuildEinoAgent(ctx)
}

// loadRAGFromDB 从 DB 加载 RAG-Service 配置并创建客户端（未启用/失败时保持 nil，
// 记忆与知识检索自动降级到非 RAG 路径；nil 客户端是降级开关，不是错误）。
func loadRAGFromDB(ctx context.Context, svc *service.Service, daos *dao.Bundle, hago *agent.HagoCenter) {
	cfg, err := daos.RAG.GetConfig(ctx)
	if err != nil {
		// 数据库无配置 → 初始化默认配置，保证前端读取不报错
		if initErr := daos.RAG.InitConfig(ctx); initErr != nil {
			log.Warn("RAG 默认配置初始化失败", "err", initErr)
			return
		}
		cfg, err = daos.RAG.GetConfig(ctx)
		if err != nil {
			log.Warn("RAG 配置加载失败，使用默认", "err", err)
			return
		}
	}
	if !cfg.IsActive {
		log.Info("RAG 未启用（记忆/知识检索走降级路径）")
		return
	}
	client, err := rag.NewClient(
		rag.WithBaseURL(cfg.BaseURL),
		rag.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
	)
	if err != nil {
		log.Warn("RAG 客户端创建失败，降级到非 RAG 路径", "err", err)
		return
	}
	svc.RAGClient = client
	hago.RAGClient = client
	log.Info("RAG 客户端已就绪", "base_url", cfg.BaseURL)
}

// loadWebhookConfig 从 DB 加载 Webhook 配置；若不存在则使用默认值并初始化 DB。
func loadWebhookConfig(ctx context.Context, daos *dao.Bundle) (models.WebhookConfig, error) {
	cfg, err := daos.Webhook.GetConfig(ctx)
	if err != nil {
		// 初始化默认配置
		defaultCfg := models.WebhookConfig{
			ID:      1,
			Addr:    "0.0.0.0",
			Port:    8091,
			Enabled: false,
		}
		_ = daos.Webhook.InitConfig(ctx)
		return defaultCfg, nil
	}
	return *cfg, nil
}

// loadAdapterConfig 从 DB 加载 OneBot11 Adapter 配置；若 DB 无记录则初始化默认配置并重新读取，
// 若 DB 加载失败则回退到 env 默认值。
func loadAdapterConfig(ctx context.Context, daos *dao.Bundle) adapter.Config {
	// env 默认值作为 fallback
	defaultCfg := adapter.Config{
		Addr:   fmt.Sprintf(":%d", mustAtoi(env("OB_PORT", "8081"))),
		Port:   mustAtoi(env("OB_PORT", "8081")),
		Token:  env("OB_TOKEN", ""),
		Admins: parseAdmins(env("OB_ADMINS", "")),
		Enable: true,
	}

	cfg, err := daos.Onebot11Adapter.GetAdapterConfig(ctx)
	if err != nil {
		// DB 中无记录 → 初始化默认配置
		if initErr := daos.Onebot11Adapter.InitAdapterConfig(ctx); initErr != nil {
			log.Warn("Adapter 配置初始化失败，使用 env 默认值", "err", initErr)
			return defaultCfg
		}
		cfg, err = daos.Onebot11Adapter.GetAdapterConfig(ctx)
		if err != nil {
			log.Warn("Adapter 配置加载失败，使用 env 默认值", "err", err)
			return defaultCfg
		}
	}

	// DB 加载成功，用 DB 配置覆盖 env 默认值
	return adapter.Config{
		Addr:   fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
		Port:   cfg.Port,
		Token:  cfg.Token,
		Admins: cfg.AdminQQNumbers,
		Enable: cfg.Enabled,
	}
}
