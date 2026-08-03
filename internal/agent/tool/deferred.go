package tool

import (
	"context"
	"strings"
	"sync"

	"JuanNiang-Neo/internal/adapter"
)

// DeferredSend 任务执行期间排队、任务执行完成后统一发送的消息。
// Message 与 adapter.Send*Msg 保持一致：可以是 string（含 CQ 码）或 []adapter.Segment。
type DeferredSend struct {
	MessageType string // "private" / "group"
	TargetID    int64  // 私聊: user_id; 群聊: group_id
	Message     any
	Delivery    bool // 主要交付消息（send_*_msg）：投递到当前会话后应抑制最终回复，避免复述
}

// DeferredSendQueue 收集 Agent 任务执行期间工具发起的发送请求，
// 任务完成后由事件循环统一 Flush，保证"中途不发、执行完再发"。
type DeferredSendQueue struct {
	mu    sync.Mutex
	sends []DeferredSend
}

func NewDeferredSendQueue() *DeferredSendQueue {
	return &DeferredSendQueue{}
}

// Add 将一条待发送消息加入队列。
func (q *DeferredSendQueue) Add(s DeferredSend) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sends = append(q.sends, s)
}

// Len 返回队列长度。
func (q *DeferredSendQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.sends)
}

// DeliveredTo 判断是否已向指定会话投递过主要交付消息（Delivery=true）。
// 若已投递，最终回复通常只是复述/操作过程描述，应跳过发送。
func (q *DeferredSendQueue) DeliveredTo(messageType string, targetID int64) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, s := range q.sends {
		if s.Delivery && s.MessageType == messageType && s.TargetID == targetID {
			return true
		}
	}
	return false
}

// Text 返回消息的纯文本内容（供写入记忆/聊天记录用）。
// string 原样返回；[]Segment 拼接 text 段；其他类型返回空串。
func (s DeferredSend) Text() string {
	switch v := s.Message.(type) {
	case string:
		return v
	case []adapter.Segment:
		var b strings.Builder
		for _, seg := range v {
			if seg.Type == "text" {
				if t, ok := seg.Data["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

// Flush 按入队顺序发送所有排队消息，发送后清空队列，并返回本次发送的列表（供调用方写入记忆/记录）。
func (q *DeferredSendQueue) Flush(ctx context.Context, a AdapterProvider) []DeferredSend {
	if q == nil || a == nil {
		return nil
	}
	q.mu.Lock()
	sends := q.sends
	q.sends = nil
	q.mu.Unlock()

	if len(sends) == 0 {
		return nil
	}
	log.Info("任务执行完成，统一发送排队消息", "count", len(sends))

	for _, s := range sends {
		if s.TargetID <= 0 {
			log.Warn("延迟发送跳过无效目标", "type", s.MessageType, "target", s.TargetID)
			continue
		}
		switch s.MessageType {
		case "private":
			if _, err := a.SendPrivateMsg(s.TargetID, s.Message); err != nil {
				log.Error("延迟发送私聊消息失败", "target", s.TargetID, "err", err)
			} else {
				log.Info("延迟发送私聊消息成功", "target", s.TargetID)
			}
		case "group":
			if _, err := a.SendGroupMsg(s.TargetID, s.Message); err != nil {
				log.Error("延迟发送群消息失败", "target", s.TargetID, "err", err)
			} else {
				log.Info("延迟发送群消息成功", "target", s.TargetID)
			}
		default:
			log.Warn("延迟发送忽略未知消息类型", "type", s.MessageType)
		}
	}

	return sends
}

// ---------- context 传递 ----------

type deferredQueueKey struct{}

// WithDeferredSendQueue 将延迟发送队列注入 context，供工具在执行期间入队。
func WithDeferredSendQueue(ctx context.Context, q *DeferredSendQueue) context.Context {
	return context.WithValue(ctx, deferredQueueKey{}, q)
}

// GetDeferredSendQueue 从 context 读取延迟发送队列；不存在（如插件直接调用工具）返回 nil。
func GetDeferredSendQueue(ctx context.Context) *DeferredSendQueue {
	if ctx == nil {
		return nil
	}
	q, _ := ctx.Value(deferredQueueKey{}).(*DeferredSendQueue)
	return q
}
