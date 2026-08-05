package adapter

import (
	"context"
	"fmt"
	"strings"
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

	// imageResolver 图床图片解析器：发送消息时遇到 image 段 file 以 "imgs://" 开头，
	// 调用它把图片 ID 解析为 OneBot11 的 base64 图片串（"base64://<data>"）。
	// 由 main.go 注入图床存储实现，Plugin 与 Agent 对解析过程无感。
	imageResolver func(rawFile string) (base64Image string, ok bool)

	// stickerResolver 表情解析器：发送表情段（subType=1）时，用短 UUID 查图床长 UUID
	// 并返回 base64 图片串。由 main.go 注入表情库存储实现，Plugin 与 Agent 只接触短 UUID。
	stickerResolver func(stickerID string) (base64Image string, ok bool)
}

func New(cfg Config) *Adapter {
	return &Adapter{
		cfg:    cfg,
		events: make(chan Event, 128),
		closed: true, // 初始为"已停止"状态, 允许 Start
	}
}

// SetImageResolver 注入图床图片解析器（见 imageResolver 注释）。
func (p *Adapter) SetImageResolver(fn func(rawFile string) (string, bool)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.imageResolver = fn
}

// SetStickerResolver 注入表情解析器（见 stickerResolver 注释）。
func (p *Adapter) SetStickerResolver(fn func(stickerID string) (string, bool)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stickerResolver = fn
}

func (p *Adapter) Start(ctx context.Context) error {
	listenAddr := p.listenAddr()
	p.mu.Lock()
	// 若 events 已被 Stop 关闭, 重建一个新的 channel, 否则后续推送会 panic
	// (向已关闭 channel 发送 panic)。
	if p.events == nil {
		p.events = make(chan Event, 128)
	}
	p.mu.Unlock()

	srv, err := newWSServer(ctx, listenAddr, p.cfg.Token, p.events)
	if err != nil {
		return fmt.Errorf("adapter start: %w", err)
	}
	p.mu.Lock()
	if !p.closed {
		// 已经在运行, 把多余启动的 srv 关掉, 不替换现有 server。
		p.mu.Unlock()
		srv.stop()
		return nil
	}
	p.server = srv
	p.closed = false
	p.mu.Unlock()
	log.Info("adapter 已启动", "addr", listenAddr)
	return nil
}

// listenAddr 返回 net.Listen 直接可用的 "host:port" 串。
// 兼容三种 cfg.Addr 形态：
//   - "host:port"（标准）
//   - "host"（仅有 host, 缺端口, WebUI 更新时常见）
//   - ":port"（仅端口, 省略 host）
func (p *Adapter) listenAddr() string {
	addr := strings.TrimSpace(p.cfg.Addr)
	if addr == "" {
		return fmt.Sprintf(":%d", p.cfg.Port)
	}
	// 含冒号 → 视为 host:port 或 :port
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		tail := addr[i+1:]
		// tail 非数字 → 说明冒号后不是端口（如 IPv6 边界），追加 Port
		if _, err := fmt.Sscanf(tail, "%d", new(int)); err != nil {
			return fmt.Sprintf("%s:%d", addr, p.cfg.Port)
		}
		return addr
	}
	// 不含冒号 → 仅 host, 追加端口
	return fmt.Sprintf("%s:%d", addr, p.cfg.Port)
}

func (p *Adapter) Stop(ctx context.Context) error {
	// 整个 Stop 操作放入 goroutine，用 context 控制超时，避免死锁。
	done := make(chan error, 1)
	go func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.closed {
			done <- nil
			return
		}
		p.closed = true

		if p.server != nil {
			p.server.stop()
			p.server = nil
		}
		// 关闭 events channel 通知消费者停止, 并置 nil 避免二次 close panic。
		// Start 时会重新创建 events channel。
		if p.events != nil {
			close(p.events)
			p.events = nil
		}

		log.Info("adapter 已停止")
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		log.Warn("adapter Stop 超时, 强制退出", "err", ctx.Err())
		return ctx.Err()
	}
}

func (p *Adapter) Events() <-chan Event {
	return p.events
}

// Admins 返回当前配置的管理员列表（供事件透传使用）。
func (p *Adapter) Admins() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.Admins
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
		log.Error("adapter 重启出错 (Stop)", "err", err.Error())
		return err
	}

	if err := p.Start(ctx); err != nil {
		log.Error("adapter 重启出错 (Start)", "err", err.Error())
		return err
	}

	return nil
}

func (p *Adapter) Status() ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s := ProviderStatus{ListenAddr: p.listenAddr()}
	if p.server == nil || p.closed {
		return s
	}

	s.Running = true
	s.SelfID = p.server.selfID()
	s.ConnCount = p.server.connCount()
	s.ConnIDs = p.server.connIDs()
	s.Conns = p.server.connDetails()
	return s
}

func (p *Adapter) SyncConfig(ctx context.Context, conf Config) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	p.mu.Lock()
	p.cfg = conf
	p.mu.Unlock()

	// 简化逻辑:
	//   - 启用: 若已在运行, 先 Stop 再 Start 重启加载新配置; 否则直接 Start
	//   - 停用: 调用 Stop
	if conf.Enable {
		log.Info("adapter 重启中")
		if err := p.Stop(ctx); err != nil {
			log.Error("adapter 配置更新出错 (Stop)", "err", err.Error())
			return err
		}
		if err := p.Start(ctx); err != nil {
			log.Error("adapter 配置更新出错 (Start)", "err", err.Error())
			return err
		}
	} else {
		if err := p.Stop(ctx); err != nil {
			log.Error("adapter 停止Adapter出错", "err", err.Error())
			return err
		}
	}

	log.Info("adapter 配置已更新")
	return nil
}

func (p *Adapter) call(action string, params map[string]any) (*APIResponse, error) {
	if p.server == nil {
		return nil, fmt.Errorf("adapter 未启动")
	}
	return p.server.callAPI(action, params)
}
