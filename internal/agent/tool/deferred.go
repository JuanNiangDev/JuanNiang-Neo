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
// 静默内容（isSilenceToolContent）与无效目标在 Flush 中不会真正发送，
// 因此这里同样跳过，避免未实际发出的条目抑制最终回复。
func (q *DeferredSendQueue) DeliveredTo(messageType string, targetID int64) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, s := range q.sends {
		if !s.Delivery || s.MessageType != messageType || s.TargetID != targetID {
			continue
		}
		if s.TargetID <= 0 || isSilenceToolContent(s.Text()) {
			continue
		}
		return true
	}
	return false
}

// Text 返回消息的纯文本内容（供写入记忆/聊天记录用）。
// string 原样返回；[]Segment 拼接 text 段；其他类型返回空串。
func (s DeferredSend) Text() string {
	return messagePlainText(s.Message)
}

// Flush 按入队顺序发送所有排队消息，发送后清空队列。
// 仅返回实际发送成功的条目（供调用方写入记忆/记录）：
// 静默内容、无效目标与发送失败的条目不会出现在返回值中，
// 避免未真正发出的消息被当作已投递写进记忆或抑制最终回复。
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

	delivered := make([]DeferredSend, 0, len(sends))
	for _, s := range sends {
		if s.TargetID <= 0 {
			log.Warn("延迟发送跳过无效目标", "type", s.MessageType, "target", s.TargetID)
			continue
		}
		// 静默内容（LLM 把 __NO_REPLY__ 或纯静默短语当作工具消息发出）不发送，防止占位标记泄漏到群里。
		if isSilenceToolContent(s.Text()) {
			log.Info("延迟发送跳过静默内容", "content", s.Text(), "type", s.MessageType, "target", s.TargetID)
			continue
		}
		sent := false
		switch s.MessageType {
		case "private":
			if _, err := a.SendPrivateMsg(s.TargetID, s.Message); err != nil {
				log.Error("延迟发送私聊消息失败", "target", s.TargetID, "err", err)
			} else {
				log.Info("延迟发送私聊消息成功", "target", s.TargetID)
				sent = true
			}
		case "group":
			if _, err := a.SendGroupMsg(s.TargetID, s.Message); err != nil {
				log.Error("延迟发送群消息失败", "target", s.TargetID, "err", err)
			} else {
				log.Info("延迟发送群消息成功", "target", s.TargetID)
				sent = true
			}
		default:
			log.Warn("延迟发送忽略未知消息类型", "type", s.MessageType)
		}
		if sent {
			delivered = append(delivered, s)
		}
	}

	return delivered
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

// silenceToken 与 agent 包 SilenceToken 保持一致：LLM 判定不回复时输出的固定标记。
const silenceToken = "__NO_REPLY__"

// messagePlainText 提取任意消息类型的纯文本：string 原样返回；
// []adapter.Segment 拼接所有 text 段；其他类型返回空串。
// 发送前静默判定与记忆写回共用同一提取逻辑，避免消息段数组绕过过滤。
func messagePlainText(message any) string {
	switch v := message.(type) {
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

// isSilenceToolContent 判断工具发送的纯文本是否为静默内容（__NO_REPLY__ 标记
// 或纯静默声明短语）。LLM 偶尔会把"不回复"判定输出成 send_*_msg 的消息内容，
// 导致占位标记泄漏到聊天里；这类消息应在发送前丢弃。
func isSilenceToolContent(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, silenceToken) {
		return true
	}
	if len([]rune(trimmed)) > 15 {
		return false
	}
	lower := strings.ToLower(trimmed)
	silencePhrases := []string{
		"保持静默", "保持沉默", "静默观察", "静默",
		"不回复", "我不回复", "我不回",
		"不插话", "我不插话",
		"不说话", "我不说话",
		"不发言", "我不发言",
		"不参与", "我不参与",
		"与我无关", "不关我的事",
		"我不说",
		"不响", "不响，做空气", "不响，做空气。",
		"做空气", "装死", "当没看到", "没看到",
		"路过", "潜水", "暗中观察",
		"😶", "🤐", "🙈", "🫥",
	}
	for _, m := range silencePhrases {
		if lower == m {
			return true
		}
	}
	return strings.Contains(lower, "静默") || strings.Contains(lower, "不回复") || strings.Contains(lower, "不插话") ||
		strings.Contains(lower, "不响") || strings.Contains(lower, "做空气") || strings.Contains(lower, "装死")
}
