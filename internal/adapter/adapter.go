package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Config struct {
	Addr   string // 监听地址, 格式 host:port
	Port   int
	Token  string
	Admins []string
	Enable bool
}

type Adapter struct {
	cfg    Config
	server *wsServer
	events chan Event
	mu     sync.RWMutex
	closed bool
}

func New(cfg Config) *Adapter {
	return &Adapter{
		cfg:    cfg,
		events: make(chan Event, 128),
	}
}

func (p *Adapter) Start(ctx context.Context) error {
	srv, err := newWSServer(ctx, p.cfg.Addr, p.cfg.Token, p.events)
	if err != nil {
		return fmt.Errorf("adapter start: %w", err)
	}
	p.mu.Lock()
	if !p.closed {
		return nil
	} else {
		p.server = srv
		p.closed = false
	}
	p.mu.Unlock()
	slog.Info("adapter 已启动", "addr", p.cfg.Addr)
	return nil
}

func (p *Adapter) Stop(ctx context.Context) error {
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

	slog.Info("adapter 已停止")
	return nil
}

func (p *Adapter) Events() <-chan Event {
	return p.events
}

func (p *Adapter) SelfID() int64 {
	if p.server == nil {
		return 0
	}
	return p.server.selfID()
}

func (p *Adapter) Restart(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := p.Stop(ctx); err != nil {
		slog.Error("adapter 重启出错 (Stop)", "err", err.Error())
		return err
	}

	if err := p.Start(ctx); err != nil {
		slog.Error("adapter 重启出错 (Start)", "err", err.Error())
		return err
	}

	return nil
}

func (p *Adapter) Status() ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s := ProviderStatus{ListenAddr: p.cfg.Addr}
	if p.server == nil || p.closed {
		return s
	}

	s.Running = true
	s.SelfID = p.server.selfID()
	s.ConnCount = p.server.connCount()
	s.ConnIDs = p.server.connIDs()
	s
	return s
}

func (p *Adapter) SyncConfig(ctx context.Context, conf Config) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	p.mu.Lock()
	p.cfg = conf
	p.mu.Unlock()

	if conf.Enable {
		if err := p.Start(ctx); err != nil {
			slog.Error("adapter 启动Adapter出错", "err", err.Error())
			return err
		}
	}

	if !conf.Enable {
		if err := p.Stop(ctx); err != nil {
			slog.Error("adapter 停止Adapter出错", "err", err.Error())
			return err
		}
	}

	slog.Info("adapter 重启中")

	if err := p.Stop(ctx); err != nil {
		slog.Error("adapter 配置更新出错 (Stop)", "err", err.Error())
		return err
	}

	if err := p.Start(ctx); err != nil {
		slog.Error("adapter 配置更新出错 (Start)", "err", err.Error())
		return err
	}

	slog.Info("adapter 配置已更新")

	return nil
}

func (p *Adapter) call(action string, params map[string]any) (*APIResponse, error) {
	if p.server == nil {
		return nil, fmt.Errorf("adapter 未启动")
	}
	return p.server.callAPI(action, params)
}
