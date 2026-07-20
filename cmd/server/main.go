package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"JuanNiang-Neo/infrastructure/postgres"
	"JuanNiang-Neo/infrastructure/redis"
	sandbox "JuanNiang-Neo/infrastructure/sandbox"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2i "JuanNiang-Neo/infrastructure/t2i"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent"
	"JuanNiang-Neo/internal/api/engine"
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/service"
	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
	"JuanNiang-Neo/internal/pluggin"
	"JuanNiang-Neo/internal/web"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 日志：同时输出到 stdio 与前端（Hub），Hub 维护最近 250 条 + SSE 实时推送。
	slog.SetDefault(slog.New(logging.NewHandler(os.Stdout, logging.Default, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("JuanNiang-Neo 启动中...")

	db, err := postgres.NewPostgresClient(
		postgres.WithHost(env("DB_HOST", "localhost")),
		postgres.WithPort(env("DB_PORT", "5432")),
		postgres.WithUser(env("DB_USER", "postgres")),
		postgres.WithPassword(env("DB_PASSWORD", "postgres")),
		postgres.WithDefaultDB(env("DB_NAME", "juan")),
	)
	if err != nil {
		slog.Error("Postgres 连接失败", "err", err)
		os.Exit(1)
	}
	slog.Info("Postgres 已连接")

	redisClient, err := redis.NewRedisSentinelClient(
		redis.WithAddr(env("REDIS_ADDR", "localhost:6379")),
		redis.WithPassword(env("REDIS_PASSWORD", "root")),
		redis.WithDB(mustAtoi(env("REDIS_DB", "0"))),
	)
	if err != nil {
		slog.Error("Redis 连接失败", "err", err)
		os.Exit(1)
	}
	slog.Info("Redis 已连接")

	coreInst, err := core.Init(ctx, db, redisClient)
	if err != nil {
		slog.Error("Core 初始化失败", "err", err)
		os.Exit(1)
	}
	slog.Info("Core 已初始化")

	if s := os.Getenv("JWT_SECRET"); s != "" {
		middleware.JWTSecret = []byte(s)
	}

	adapterCfg := adapter.Config{
		// Addr 是 net.Listen 直接接收的 "host:port" 串, 必须填充否则会随机选端口。
		Addr:   fmt.Sprintf(":%d", mustAtoi(env("OB_PORT", "8081"))),
		Port:   mustAtoi(env("OB_PORT", "8081")),
		Token:  env("OB_TOKEN", ""),
		Admins: parseAdmins(env("OB_ADMINS", "")),
	}
	adapterProv := adapter.New(adapterCfg)
	if err := adapterProv.Start(ctx); err != nil {
		slog.Error("Adapter 启动失败", "err", err)
		os.Exit(1)
	}

	// ---------- 4b. Webhook Adapter ----------
	webhookEvents := make(chan adapter.Event, 128)
	webhookCfg, err := loadWebhookConfig(ctx, coreInst.DAO)
	if err != nil {
		slog.Warn("Webhook 配置加载失败", "err", err)
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
			slog.Error("Webhook adapter 启动失败", "err", err)
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
		Providers:      hago.Providers,
		MCPGroup:       hago.MCP,
		DAO:            coreInst.DAO,
		ACL:            coreInst.ACL,
		Cache:          coreInst.Cache,
	}); err != nil {
		slog.Error("Agent 初始化失败", "err", err)
		os.Exit(1)
	}

	if err := hago.Start(ctx); err != nil {
		slog.Error("Agent 启动失败", "err", err)
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
	)
	if err := pluginEngine.LoadAll(); err != nil {
		slog.Error("插件加载失败", "err", err)
	}
	hago.PluginEngine = pluginEngine

	// ---------- 7. Web API ----------

	svc := service.New(coreInst.DAO, adapterProv, webhookAdapter, pluginEngine)
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
	svc.OnUpdateT2I = func(client *t2icaller.Client) { hago.T2IClient = client }
	svc.OnUpdateSandbox = func(client *sandboxcaller.Client) { hago.SandboxClient = client }

	// 前端静态资源目录: 默认 web/dist (构建产物), 可通过 WEB_DIR 覆盖。
	//   - 开发模式: 前端走 Vite (:3000) 代理 /api 到 :8090, 后端无需服务前端。
	//   - 生产/裸跑: make web-build 后, 后端直接 serve web/dist 作为 SPA。
	//   - 目录不存在或未构建时, 后端走引导提示页, 不影响 API 与 /health。
	webDir := env("WEB_DIR", "web/dist")
	if err := web.EnsureDir(webDir); err != nil {
		slog.Warn("WEB_DIR 校验失败", "dir", webDir, "err", err)
	}
	webEngine := engine.New(env("API_ADDR", ":8090"), webDir, svc)

	// 用 Run 而非 Spin: Spin 会自注册 SIGINT/SIGTERM handler 并在我们已注册
	// signal.NotifyContext 的同时另起一套, 导致 Ctrl-C 时 Spin 内部的
	// Shutdown(context.Background()) 卡在 SSE 长连接上, 与主流程 defer 互锁。
	// 这里我们只复用 Hertz 的 Run, 用主 ctx 显式控制生命周期。
	webErrCh := make(chan error, 1)
	go func() {
		slog.Info("Web API 已启动", "addr", env("API_ADDR", ":8090"))
		webErrCh <- webEngine.Run()
	}()

	slog.Info("JuanNiang-Neo 已就绪",
		"adapter_addr", adapterProv.Status().ListenAddr,
		"api_addr", env("API_ADDR", ":8090"),
		"plugins", len(pluginEngine.List()),
	)

	// ---------- 8. 等待退出 ----------
	// 主 ctx 在收到 Ctrl-C / SIGTERM 时被取消。我们以反向顺序停掉各组件,
	// 全部用带 deadline 的 shutdownCtx, 避免任何子组件挂死拖垮整体退出。
	<-ctx.Done()
	slog.Info("收到退出信号，正在关闭...")

	// watchdog: 若 15s 内未完成优雅退出则强制结束, 避免任何 Stop 调用挂死。
	shutdownBudget := 15 * time.Second
	done := make(chan struct{})
	go func() {
		shutdown(adapterProv, webhookAdapter, hago, webEngine, pluginEngine)
		close(done)
	}()
	select {
	case <-done:
		slog.Info("已优雅退出")
	case <-time.After(shutdownBudget):
		slog.Error("优雅关闭超时, 强制退出", "budget", shutdownBudget)
		os.Exit(1)
	}
}

// shutdown 按反向顺序停掉各组件, 每步独立带 deadline, 任一卡死不影响后续。
// 注意: 先停 adapter 再停 web 引擎, 避免 web 请求持 adapter 锁导致 Stop 死锁。
func shutdown(adapterProv *adapter.Adapter, webhookAdapter *adapter.WebhookAdapter, hago *agent.HagoCenter, webEngine *server.Hertz, pluginEngine *pluggin.PluginEngine) {
	// 8.1 先停 Agent (关闭事件循环 / Drainer 输入, 避免后续 adapter 关闭时事件循环还在消费)。
	hago.Stop()

	// 8.2 停 Webhook adapter。
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := webhookAdapter.Stop(stopCtx); err != nil {
		slog.Warn("Webhook adapter 关闭出错", "err", err)
	}
	cancel()

	// 8.3 停 OneBot11 反向 WS adapter。
	stopCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	if err := adapterProv.Stop(stopCtx); err != nil {
		slog.Warn("Adapter 关闭出错", "err", err)
	}
	cancel()

	// 8.4 停 Web 引擎 (adapter 已停, 不再有请求竞争 adapter 锁)。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := webEngine.Shutdown(shutdownCtx); err != nil {
		slog.Warn("Web 引擎关闭出错", "err", err)
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
		slog.Warn("T2I 配置加载失败，使用默认", "err", err)
		return
	}
	if !cfg.IsActive {
		slog.Info("T2I 未启用")
		return
	}
	client, err := t2i.NewClient(
		t2i.WithBaseURL(cfg.BaseURL),
		t2i.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
	)
	if err != nil {
		slog.Warn("T2I 客户端创建失败", "err", err)
		return
	}
	svc.T2IClient = client
	hago.T2IClient = client
	slog.Info("T2I 客户端已就绪", "base_url", cfg.BaseURL)
}

func loadSandboxFromDB(ctx context.Context, svc *service.Service, daos *dao.Bundle, hago *agent.HagoCenter) {
	cfg, err := daos.Sandbox.GetConfig(ctx)
	if err != nil {
		slog.Warn("Sandbox 配置加载失败，使用默认", "err", err)
		return
	}
	if !cfg.IsActive {
		slog.Info("Sandbox 未启用")
		return
	}
	client, err := sandbox.NewClient(
		sandbox.WithBaseURL(cfg.BaseURL),
		sandbox.WithAPIKey(cfg.APIKey),
		sandbox.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
	)
	if err != nil {
		slog.Warn("Sandbox 客户端创建失败", "err", err)
		return
	}
	svc.SandboxClient = client
	hago.SandboxClient = client
	slog.Info("Sandbox 客户端已就绪", "base_url", cfg.BaseURL)
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
