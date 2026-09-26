package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RomiChan/websocket"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/layasticker"
	"JuanNiang-Neo/internal/core/models"
)

type privacyChatModel struct{ messages []*schema.Message }

func (m *privacyChatModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.messages = messages
	return schema.AssistantMessage("收到啦", nil), nil
}
func (m *privacyChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("unexpected streaming")
}
func (m *privacyChatModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// Exercises handleMessage through the real async worker and outgoing HTTP request.
// Replacing the primary raw text with combinedUserMsg leaks the other user's data.
func TestHandleMessageLayaOnlySendsPrimaryUserMessage(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		ordinary        bool
	}{
		{"text", "  CURRENT_TEXT  ", "CURRENT_TEXT", false},
		{"cq", "[CQ:image,file=current.png]", "[CQ:image,file=current.png]", false},
		{"empty", "  ", "", false},
		{"ordinary_text", "  CURRENT_TEXT  ", "CURRENT_TEXT", true},
		{"ordinary_cq", "[CQ:image,file=current.png]", "[CQ:image,file=current.png]", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h, db := newDedupTestHago(t)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { sqlDB.Close() })
			h.DAO.Knowledge = nil
			h.DAO.Sticker = nil
			// An in-process OneBot peer acknowledges actual reply sends; no external service.
			// The adapter cannot accept a pre-bound listener or expose its
			// port-0 address. Retry if another process claims the selected port.
			var addr string
			var startErr error
			for attempt := 0; attempt < 5; attempt++ {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				addr = listener.Addr().String()
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
				h.Adapter = adapter.New(adapter.Config{Addr: addr, Enable: true})
				startErr = h.Adapter.Start(ctx)
				if startErr == nil {
					break
				}
			}
			if startErr != nil {
				t.Fatalf("adapter start failed after 5 port attempts: %v", startErr)
			}
			t.Cleanup(func() { h.Adapter.Stop(ctx) })
			conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.Close() })
			if err := conn.WriteJSON(map[string]any{"self_id": 999}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for h.Adapter.SelfID() == 0 {
				if time.Now().After(deadline) {
					t.Fatal("OneBot handshake timed out")
				}
				time.Sleep(time.Millisecond)
			}
			go func() {
				for {
					var req map[string]any
					if err := conn.ReadJSON(&req); err != nil {
						return
					}
					if err := conn.WriteJSON(map[string]any{"status": "ok", "retcode": 0, "echo": req["echo"], "data": map[string]any{"message_id": 1, "messages": []any{}}}); err != nil {
						return
					}
				}
			}()
			cm := &privacyChatModel{}
			h.EinoAgent, err = adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "privacy_test", Model: cm})
			if err != nil {
				t.Fatal(err)
			}
			requests := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					UserMessage string `json:"user_message"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body.UserMessage
				w.Write([]byte(`{"category":"NONE"}`))
			}))
			defer srv.Close()
			h.layaPost = newLayaPostProcessor(ctx, h, 1)
			defer h.layaPost.Stop()
			current := &adapter.MessageEvent{MessageType: "group", GroupID: 456, UserID: 222, RawMessage: tc.raw, Message: []adapter.Segment{adapter.Text("SEGMENT_FALLBACK")}}
			current.Sender.Nickname = "CURRENT_NAME"
			other := &adapter.MessageEvent{MessageType: "group", GroupID: 456, UserID: 111, RawMessage: "OTHER_SECRET"}
			other.Sender.Nickname = "OTHER_NAME"
			batch := []string{memoryMsg(other)}
			if mu := memoryMsg(current); mu != "" {
				batch = append(batch, mu)
			}
			cfg := layasticker.Config{Enabled: true, Endpoint: srv.URL, ProtocolMode: layasticker.ProtocolJSON, HTTPMethod: http.MethodPost, Timeout: time.Second, RequestTemplate: `{"user_message":"{{user_message}}"}`, CategoryPointer: "/category", Categories: []layasticker.Category{{ID: "NONE", Description: "不发送", Enabled: true, NoSend: true}}}
			if !tc.ordinary {
				ctx = WithBatchUserMsgs(ctx, batch)
			}
			h.handleMessage(ctx, []adapter.Event{{Message: current}}, &models.ChatArea{ID: "privacy-test"}, ReplySettings{LayaSticker: cfg})
			select {
			case got := <-requests:
				if got != tc.want {
					t.Errorf("Laya user_message = %q, want primary content %q", got, tc.want)
				}
				for _, secret := range []string{"OTHER_SECRET", "OTHER_NAME", "111"} {
					if strings.Contains(got, secret) {
						t.Errorf("Laya leaked %q", secret)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Laya request not received")
			}
			var prompt strings.Builder
			for _, msg := range cm.messages {
				prompt.WriteString(msg.Content)
			}
			for _, preserved := range []string{"OTHER_SECRET", "OTHER_NAME", "111"} {
				if !tc.ordinary && !strings.Contains(prompt.String(), preserved) {
					t.Errorf("Agent lost batch context %q", preserved)
				}
			}
			if tc.want != "" && !strings.Contains(prompt.String(), tc.want) {
				t.Error("Agent lost current message")
			}
		})
	}
}
