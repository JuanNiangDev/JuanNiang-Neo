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

	// 群成员信息缓存（正缓存 10min / 负缓存 60s），避免高频群消息每条都调 OneBot11
	memberInfoMu    sync.RWMutex
	memberInfoCache map[string]memberInfoCacheEntry

	// imageResolver 图床图片解析器：发送消息时遇到 image 段 file 以 "imgs://" 开头，
	// 调用它把图片 ID 解析为 OneBot11 的 base64 图片串（"base64://<data>"）。
	// 由 main.go 注入图床存储实现，Plugin 与 Agent 对解析过程无感。
	imageResolver func(rawFile string) (base64Image string, ok bool)

	// stickerResolver 表情解析器：发送表情段（subType=1）时，用短 UUID 查图床长 UUID
	// 并返回 base64 图片串。由 main.go 注入表情库存储实现，Plugin 与 Agent 只接触短 UUID。
	stickerResolver func(stickerID string) (base64Image string, ok bool)
}

// 群成员信息缓存参数。
const (
	memberInfoCacheTTL    = 10 * time.Minute // 正缓存：角色变更不频繁
	memberInfoNegativeTTL = 60 * time.Second // 负缓存：查询失败/成员不存在，防每条消息重试打 API
	memberInfoCacheLimit  = 2048             // 上限，超出整体清空（简单策略）
)

// memberInfoCacheEntry 群成员信息缓存条目。
// err 一并缓存：负缓存命中时还原查询错误，避免调用方把"查询失败"误判为"成员存在"。
type memberInfoCacheEntry struct {
	info      *GroupMemberInfo
	err       error
	expiresAt time.Time
}

func New(cfg Config) *Adapter {
	return &Adapter{
		cfg:             cfg,
		events:          make(chan Event, 128),
		closed:          true, // 初始为"已停止"状态, 允许 Start
		memberInfoCache: make(map[string]memberInfoCacheEntry),
	}
}

// GetGroupMemberInfoCached 带缓存的群成员信息查询：命中缓存直接返回；
// 未命中调 OneBot11 API 并缓存。查询失败或成员不存在时写入短负缓存，
// 避免退群/异常成员在每条群消息上都重复触发 get_group_member_info。
func (p *Adapter) GetGroupMemberInfoCached(groupID, userID int64) (*GroupMemberInfo, error) {
	key := fmt.Sprintf("%d:%d", groupID, userID)
	now := time.Now()

	p.memberInfoMu.RLock()
	if e, ok := p.memberInfoCache[key]; ok && now.Before(e.expiresAt) {
		p.memberInfoMu.RUnlock()
		return e.info, e.err
	}
	p.memberInfoMu.RUnlock()

	info, err := p.GetGroupMemberInfo(groupID, userID)
	ttl := memberInfoCacheTTL
	if err != nil || info == nil {
		ttl = memberInfoNegativeTTL // 失败/成员不存在：短负缓存
	}

	p.memberInfoMu.Lock()
	p.memberInfoCache[key] = memberInfoCacheEntry{info: info, err: err, expiresAt: now.Add(ttl)}
	// 防无界增长：超过上限时整体清空（简单策略）
	if len(p.memberInfoCache) > memberInfoCacheLimit {
		p.memberInfoCache = make(map[string]memberInfoCacheEntry)
	}
	p.memberInfoMu.Unlock()
	return info, err
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
	p.mu.RLock()
	running := !p.closed
	p.mu.RUnlock()
	if running {
		return nil // 已在运行
	}

	listenAddr := p.listenAddr()

	p.mu.Lock()
	// 若 events 已被 Stop 关闭, 重建一个新的 channel, 否则后续推送会 panic
	// (向已关闭 channel 发送 panic)。
	if p.events == nil {
		p.events = make(chan Event, 128)
	}
	events := p.events // 局部变量贯穿创建过程，避免 Stop 并发置 nil 后拿到 nil channel
	p.mu.Unlock()

	srv, err := newWSServer(ctx, listenAddr, p.cfg.Token, events)
	if err != nil {
		return fmt.Errorf("adapter start: %w", err)
	}

	p.mu.Lock()
	// newWSServer 创建期间可能被 Stop：Stop 会把 events 关闭并置 nil（或重建），
	// 用 p.events != events 即可可靠检测。不能用 p.closed 判断——首次启动时
	// closed 本来就是 true（New 初始为"已停止"），会导致刚创建的 server 被误关。
	if p.events != events {
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
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.events
}

// Admins 返回当前配置的管理员列表（供事件透传使用）。
func (p *Adapter) Admins() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg.Admins
}

func (p *Adapter) SelfID() int64 {
	p.mu.RLock()
	server := p.server
	p.mu.RUnlock()
	if server == nil {
		return 0
	}
	return server.selfID()
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
	p.mu.RLock()
	server := p.server
	p.mu.RUnlock()
	if server == nil {
		return nil, fmt.Errorf("adapter 未启动")
	}
	return server.callAPI(action, params)
}
