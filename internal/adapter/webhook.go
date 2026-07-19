package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebhookConfig Webhook 适配器配置。
type WebhookConfig struct {
	Addr    string
	Port    int
	Token   string
	Admins  []string
	Enable  bool
}

// WebhookAdapter 是特殊的 adapter：监听独立端口接收 HTTP 请求，
// 把请求转换为 webhook 事件发送给事件循环。
// 插件层通过 on_webhook 回调处理此类事件，与 on_message 区分。
type WebhookAdapter struct {
	cfg     WebhookConfig
	server  *http.Server
	events  chan Event
	mu      sync.RWMutex
	closed  bool
	listener net.Listener
}

// NewWebhookAdapter 创建 Webhook adapter。events 为事件输出 channel。
func NewWebhookAdapter(cfg WebhookConfig, events chan Event) *WebhookAdapter {
	return &WebhookAdapter{
		cfg:    cfg,
		events: events,
	}
}

// Start 启动 Webhook adapter。
func (w *WebhookAdapter) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cfg.Port == 0 {
		w.cfg.Port = 8091
	}
	if w.cfg.Addr == "" {
		w.cfg.Addr = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%d", w.cfg.Addr, w.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("webhook listen %s: %w", addr, err)
	}
	w.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleRequest)
	mux.HandleFunc("/webhook", w.handleRequest)

	w.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	w.closed = false
	go func() {
		slog.Info("webhook adapter 已启动", "addr", addr)
		if err := w.server.Serve(listener); err != nil && !isServerClosed(err) {
			slog.Error("webhook serve 异常", "err", err)
		}
		slog.Info("webhook adapter 已停止", "addr", addr)
	}()

	return nil
}

// Stop 停止 Webhook adapter。
func (w *WebhookAdapter) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	if w.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		w.server.Shutdown(shutdownCtx)
		w.server = nil
	}
	return nil
}

// Events 返回事件 channel。
func (w *WebhookAdapter) Events() <-chan Event {
	return w.events
}

// IsRunning 返回 adapter 是否正在运行。
func (w *WebhookAdapter) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return !w.closed && w.server != nil
}

// Admins 返回当前管理员列表。
func (w *WebhookAdapter) Admins() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg.Admins
}

// SyncConfig 更新配置并重启服务。
func (w *WebhookAdapter) SyncConfig(ctx context.Context, conf WebhookConfig) error {
	w.mu.Lock()
	w.cfg = conf
	w.mu.Unlock()

	if !conf.Enable {
		return w.Stop(ctx)
	}

	// 重启
	_ = w.Stop(ctx)
	return w.Start(ctx)
}

// handleRequest 处理进入的 HTTP 请求。
func (w *WebhookAdapter) handleRequest(rw http.ResponseWriter, r *http.Request) {
	// Token 校验
	if w.cfg.Token != "" && !checkWebhookAuth(r, w.cfg.Token) {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}

	// 读取 body
	var payload map[string]any
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "read body failed", http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			// 尝试解析 JSON；非 JSON 则用 raw 字段包装
			if err := json.Unmarshal(body, &payload); err != nil {
				payload = map[string]any{
					"raw":  string(body),
					"type": "non-json",
				}
			}
		}
	}

	ev := Event{
		PostType: "webhook",
		Time:     time.Now().Unix(),
		Webhook: &WebhookEvent{
			Path:    r.URL.Path,
			Method:  r.Method,
			Payload: payload,
		},
		Admins: w.Admins(),
	}

	select {
	case w.events <- ev:
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(`{"status":"ok"}`))
	default:
		slog.Warn("webhook events channel 满，丢弃事件")
		http.Error(rw, "events channel full", http.StatusServiceUnavailable)
	}
}

// checkWebhookAuth 校验 token：Authorization: Bearer <token> 或 query ?access_token=
func checkWebhookAuth(r *http.Request, token string) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		auth = r.URL.Query().Get("access_token")
	} else {
		_, after, ok := strings.Cut(auth, " ")
		if ok {
			auth = after
		}
	}
	return auth == token
}
