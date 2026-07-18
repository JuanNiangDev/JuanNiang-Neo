package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"JuanNiang-Neo/infrastructure/postgres"
	"JuanNiang-Neo/infrastructure/redis"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent"
	"JuanNiang-Neo/internal/api/engine"
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/service"
	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/pluggin"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
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
		Port:   mustAtoi(env("OB_PORT", "8081")),
		Token:  env("OB_TOKEN", ""),
		Admins: parseAdmins(env("OB_ADMINS", "")),
	}
	adapterProv := adapter.New(adapterCfg)
	if err := adapterProv.Start(ctx); err != nil {
		slog.Error("Adapter 启动失败", "err", err)
		os.Exit(1)
	}
	defer adapterProv.Stop(context.Background())

	// ---------- 5. Agent ----------

	hago := agent.NewHagoCenter()
	if err := hago.Init(ctx, agent.Config{
		Adapter:   adapterProv,
		Sandbox:   nil,
		T2I:       nil,
		Providers: hago.Providers,
		MCPGroup:  hago.MCP,
		DAO:       coreInst.DAO,
		ACL:       coreInst.ACL,
	}); err != nil {
		slog.Error("Agent 初始化失败", "err", err)
		os.Exit(1)
	}

	if err := hago.Start(ctx); err != nil {
		slog.Error("Agent 启动失败", "err", err)
		os.Exit(1)
	}
	defer hago.Stop()

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

	svc := service.New(coreInst.DAO, adapterProv, pluginEngine)
	webEngine := engine.New(env("API_ADDR", ":8090"), svc)

	go func() {
		slog.Info("Web API 已启动", "addr", env("API_ADDR", ":8090"))
		webEngine.Spin()
	}()

	slog.Info("JuanNiang-Neo 已就绪",
		"adapter_addr", adapterProv.Status().ListenAddr,
		"api_addr", env("API_ADDR", ":8090"),
		"plugins", len(pluginEngine.List()),
	)

	// ---------- 8. 等待退出 ----------

	<-ctx.Done()
	slog.Info("收到退出信号，正在关闭...")
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

func parseAdmins(s string) []int64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var admins []int64
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err == nil {
			admins = append(admins, id)
		}
	}
	return admins
}

// pluginCB 将 pluggin.PluginEngine 包装为 service.PluginEngineCb。
type pluginCB struct{ pe *pluggin.PluginEngine }

func (p *pluginCB) Load(name string) error   { return p.pe.Load(name) }
func (p *pluginCB) Unload(name string) error { return p.pe.Unload(name) }
func (p *pluginCB) Reload(name string) error { return p.pe.Reload(name) }
func (p *pluginCB) List() []map[string]any   { return p.pe.ListMaps() }
