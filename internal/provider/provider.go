package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Config OneBot11 适配器配置。
type Config struct {
	Addr   string // 监听地址, 格式 host:port
	Port   int
	Token  string
	Admins []int64
}

// Provider 是 OneBot11 协议适配器, 管理 WebSocket 连接并提供 OneBot11 API。
// Agent 通过 Events() 获取事件流, 通过 API 方法调用 OneBot11 接口。
type Provider struct {
	cfg    Config
	server *wsServer
	events chan Event
	mu     sync.RWMutex
	closed bool
}

// New 创建 Provider 实例。
func New(cfg Config) *Provider {
	return &Provider{
		cfg:    cfg,
		events: make(chan Event, 128),
	}
}

// Start 启动 WebSocket 服务器, 开始接收事件。
func (p *Provider) Start(ctx context.Context) error {
	addr := p.resolveAddr()

	srv, err := newWSServer(ctx, addr, p.cfg.Token, p.events)
	if err != nil {
		return fmt.Errorf("provider start: %w", err)
	}
	p.server = srv
	slog.Info("provider 已启动", "addr", addr)
	return nil
}

// Stop 关闭 WebSocket 服务器和事件通道。
func (p *Provider) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	if p.server != nil {
		p.server.stop()
		p.server = nil
	}
	close(p.events)

	slog.Info("provider 已停止")
	return nil
}

// Events 返回只读事件通道, Agent 通过此通道接收所有 OneBot11 事件。
func (p *Provider) Events() <-chan Event {
	return p.events
}

// SelfID 返回当前连接的机器人 QQ 号。
func (p *Provider) SelfID() int64 {
	if p.server == nil {
		return 0
	}
	return p.server.selfID()
}

// UpdateConfig 热更新配置。仅 Token 和 Admins 可在运行时更新, Addr/Port 需重启。
func (p *Provider) UpdateConfig(cfg Config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg.Token = cfg.Token
	if cfg.Admins != nil {
		p.cfg.Admins = append([]int64{}, cfg.Admins...)
	}
	slog.Info("provider 配置已更新")
}

// Status 返回适配器当前运行状态。
func (p *Provider) Status() ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s := ProviderStatus{ListenAddr: p.resolveAddr()}
	if p.server == nil || p.closed {
		return s
	}

	s.Running = true
	s.SelfID = p.server.selfID()
	s.ConnCount = p.server.connCount()
	s.ConnIDs = p.server.connIDs()
	return s
}

func (p *Provider) resolveAddr() string {
	if p.cfg.Addr != "" {
		return p.cfg.Addr
	}
	return fmt.Sprintf("0.0.0.0:%d", p.cfg.Port)
}

// call 向 OneBot11 客户端发送 API 调用并返回原始响应。
func (p *Provider) call(action string, params map[string]any) (*APIResponse, error) {
	if p.server == nil {
		return nil, fmt.Errorf("provider 未启动")
	}
	return p.server.callAPI(action, params)
}
