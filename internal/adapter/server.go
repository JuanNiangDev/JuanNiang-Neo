package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RomiChan/websocket"
	"github.com/tidwall/gjson"
)

// wsServer 管理 OneBot11 反向 WebSocket 连接。
type wsServer struct {
	listener net.Listener
	conns    map[int64]*wsConn
	mu       sync.RWMutex
	events   chan Event
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

type wsConn struct {
	conn       *websocket.Conn
	selfID     int64
	remoteAddr string
	mu         sync.Mutex
	seq        uint64
	responses  map[string]chan *APIResponse
}

func newWSServer(ctx context.Context, addr, token string, events chan Event) (*wsServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	ctx, cancel := context.WithCancel(ctx)

	s := &wsServer{
		listener: listener,
		conns:    make(map[int64]*wsConn),
		events:   events,
		ctx:      ctx,
		cancel:   cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.handleWS(w, r, token)
	})

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		slog.Info("ws server 启动", "addr", addr)
		if err := http.Serve(s.listener, mux); err != nil && !isServerClosed(err) {
			slog.Error("http serve 异常", "err", err)
		}
		slog.Info("ws server 已停止", "addr", addr)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()
		s.listener.Close()
	}()

	return s, nil
}

func (s *wsServer) stop() {
	s.cancel()
	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.close()
	}
	s.conns = nil
}

func (s *wsServer) selfID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id := range s.conns {
		return id
	}
	return 0
}

func (s *wsServer) connCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conns)
}

func (s *wsServer) connIDs() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]int64, 0, len(s.conns))
	for id := range s.conns {
		ids = append(ids, id)
	}
	return ids
}

// ConnDetail 表示一条 WS 连接的展示信息。
type ConnDetail struct {
	ID   int64  `json:"id"`
	IP   string `json:"ip"`
	Self int64  `json:"self_id"`
}

// connDetails 返回所有连接的 ID + RemoteAddr 信息（供前端展示）。
func (s *wsServer) connDetails() []ConnDetail {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ConnDetail, 0, len(s.conns))
	for id, c := range s.conns {
		out = append(out, ConnDetail{ID: id, IP: c.remoteAddr, Self: c.selfID})
	}
	return out
}

func (s *wsServer) callAPI(action string, params map[string]any) (*APIResponse, error) {
	selfID := s.selfID()
	if selfID == 0 {
		return nil, fmt.Errorf("no active connection")
	}

	s.mu.RLock()
	conn, ok := s.conns[selfID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no connection for selfID=%d", selfID)
	}

	echo := fmt.Sprint(atomic.AddUint64(&conn.seq, 1))
	ch := make(chan *APIResponse, 1)

	conn.mu.Lock()
	conn.responses[echo] = ch
	conn.mu.Unlock()

	defer func() {
		conn.mu.Lock()
		delete(conn.responses, echo)
		conn.mu.Unlock()
	}()

	req := APIRequest{
		Action: action,
		Params: params,
		Echo:   echo,
	}

	conn.mu.Lock()
	err := conn.conn.WriteJSON(req)
	conn.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write api request: %w", err)
	}

	select {
	case rsp := <-ch:
		if rsp.Status == "failed" {
			return rsp, fmt.Errorf("api %s failed: retcode=%d msg=%s", action, rsp.RetCode, rsp.Msg)
		}
		return rsp, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("api %s timeout", action)
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *wsServer) handleWS(w http.ResponseWriter, r *http.Request, token string) {
	if token != "" && !checkAuth(r, token) {
		slog.Warn("token 不匹配", "remote_addr", r.RemoteAddr)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade 失败", "remote_addr", r.RemoteAddr, "err", err)
		return
	}

	var handshake struct {
		SelfID int64 `json:"self_id"`
	}
	if err := conn.ReadJSON(&handshake); err != nil {
		slog.Warn("握手失败", "remote_addr", r.RemoteAddr, "err", err)
		conn.Close()
		return
	}

	wsc := &wsConn{
		conn:       conn,
		selfID:     handshake.SelfID,
		remoteAddr: r.RemoteAddr,
		responses:  make(map[string]chan *APIResponse),
	}

	s.mu.Lock()
	if old, ok := s.conns[handshake.SelfID]; ok {
		old.close()
	}
	s.conns[handshake.SelfID] = wsc
	s.mu.Unlock()

	slog.Info("客户端连接", "self_id", handshake.SelfID, "remote_addr", r.RemoteAddr)
	s.readLoop(wsc)
}

func (s *wsServer) readLoop(wsc *wsConn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, wsc.selfID)
		s.mu.Unlock()
		wsc.close()
		slog.Info("客户端断开", "self_id", wsc.selfID)
	}()

	for {
		_, payload, err := wsc.conn.ReadMessage()
		if err != nil {
			if !isConnClosed(err) {
				slog.Warn("读取消息失败", "self_id", wsc.selfID, "err", err)
			}
			return
		}

		rsp := gjson.ParseBytes(payload)

		// API 调用响应
		if echo := rsp.Get("echo"); echo.Exists() {
			wsc.mu.Lock()
			ch, ok := wsc.responses[echo.String()]
			wsc.mu.Unlock()
			if ok {
				ch <- &APIResponse{
					Status:  rsp.Get("status").String(),
					RetCode: rsp.Get("retcode").Int(),
					Data:    rsp.Get("data").Raw,
					Msg:     rsp.Get("msg").String(),
				}
			}
			continue
		}

		// 心跳忽略
		if rsp.Get("meta_event_type").String() == "heartbeat" {
			continue
		}

		// 解析事件
		ev := parseEvent(payload)
		if ev.PostType != "" {
			select {
			case s.events <- ev:
			default:
				slog.Warn("events channel 满, 丢弃事件")
			}
		}
	}
}

func parseEvent(raw []byte) Event {
	ev := Event{Raw: raw}
	json.Unmarshal(raw, &ev)

	switch ev.PostType {
	case "message":
		var msg MessageEvent
		json.Unmarshal(raw, &msg)
		ev.Message = &msg
		slog.Info("收到消息", "type", msg.MessageType, "user_id", msg.UserID, "content", msg.RawMessage)
	case "notice":
		var n NoticeEvent
		json.Unmarshal(raw, &n)
		ev.Notice = &n
		slog.Info("收到通知", "type", n.NoticeType, "user_id", n.UserID)
	case "request":
		var r RequestEvent
		json.Unmarshal(raw, &r)
		ev.Request = &r
		slog.Info("收到请求", "type", r.RequestType, "user_id", r.UserID)
	case "meta_event":
		var m MetaEvent
		json.Unmarshal(raw, &m)
		ev.Meta = &m
		slog.Info("元事件", "type", m.MetaEventType)
	}

	return ev
}

func (c *wsConn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.Close()
	for _, ch := range c.responses {
		close(ch)
	}
	c.responses = nil
}

func checkAuth(r *http.Request, token string) bool {
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

func isServerClosed(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "Server closed")
}

func isConnClosed(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) ||
		strings.Contains(err.Error(), "closed network connection")
}
