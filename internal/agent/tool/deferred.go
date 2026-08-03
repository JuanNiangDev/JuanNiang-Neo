package tool

import (
	"context"
	"sync"
)

// DeferredSend 任务执行期间排队、任务执行完成后统一发送的消息。
// Message 与 adapter.Send*Msg 保持一致：可以是 string（含 CQ 码）或 []adapter.Segment。
type DeferredSend struct {
	MessageType string // "private" / "group"
	TargetID    int64  // 私聊: user_id; 群聊: group_id
	Message     any
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

// Flush 按入队顺序发送所有排队消息，发送后清空队列。
func (q *DeferredSendQueue) Flush(ctx context.Context, a AdapterProvider) {
	if q == nil || a == nil {
		return
	}
	q.mu.Lock()
	sends := q.sends
	q.sends = nil
	q.mu.Unlock()

	if len(sends) == 0 {
		return
	}
	log.Info("任务执行完成，统一发送排队消息", "count", len(sends))

	for _, s := range sends {
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
