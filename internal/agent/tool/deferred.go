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
	MessageType  string // "private" / "group"
	TargetID     int64  // 私聊: user_id; 群聊: group_id
	Message      any
	Delivery     bool   // 主要交付消息（send_*_msg）：投递到当前会话后应抑制最终回复，避免复述
	Origin       string // 来源：空值保持原版行为；laya_auto 表示 Laya 自动追加项
	DropOnReview bool   // 群审核拒绝时移除；仅新增自动项使用
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

// HasExpressionTo 判断指定会话的排队消息是否包含表情表达式。
// 该检查独立于 Delivery：send_face 等原版非主要交付消息也必须阻止 Laya 重复追加。
// 只解析实际消息结构中的 image/stk 或 face 段，不把普通文本里的字面量当成已发送表情。
func (q *DeferredSendQueue) HasExpressionTo(messageType string, targetID int64) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, s := range q.sends {
		if s.MessageType != messageType || s.TargetID != targetID {
			continue
		}
		if messageHasExpression(s.Message) {
			return true
		}
	}
	return false
}

// Text 返回消息的纯文本内容（供写入记忆/聊天记录用）。
// string 原样返回；[]Segment 拼接 text 段；其他类型返回空串。
func (s DeferredSend) Text() string {
	return messagePlainText(s.Message)
}

// SendNow 立即发送单条消息，返回是否确认送达。
// 与 Flush 不同，它不触碰队列中的其他条目——自动追加的表情不能顺带重放
// 已排队/已发送的内容。成功语义与 Flush 一致：适配器返回 nil error
// （OneBot 返回 message_id）才算送达，没有"已提交"中间态。
func (q *DeferredSendQueue) SendNow(ctx context.Context, a AdapterProvider, s DeferredSend) bool {
	return sendDeferred(a, s)
}

// sendDeferred 共用单条消息的校验、静默过滤及确认送达逻辑。
// AdapterProvider 的发送接口不接收 context；队列管理由调用方负责。
func sendDeferred(a AdapterProvider, s DeferredSend) bool {
	if a == nil {
		return false
	}
	if s.TargetID <= 0 {
		log.Warn("延迟发送跳过无效目标", "type", s.MessageType, "target", s.TargetID)
		return false
	}
	if isSilenceToolContent(s.Text()) {
		log.Info("延迟发送跳过静默内容", "content", s.Text(), "type", s.MessageType, "target", s.TargetID)
		return false
	}
	switch s.MessageType {
	case "private":
		if _, err := a.SendPrivateMsg(s.TargetID, s.Message); err != nil {
			log.Error("延迟发送私聊消息失败", "target", s.TargetID, "err", err)
			return false
		}
		log.Info("延迟发送私聊消息成功", "target", s.TargetID)
		return true
	case "group":
		if _, err := a.SendGroupMsg(s.TargetID, s.Message); err != nil {
			log.Error("延迟发送群消息失败", "target", s.TargetID, "err", err)
			return false
		}
		log.Info("延迟发送群消息成功", "target", s.TargetID)
		return true
	default:
		log.Warn("延迟发送忽略未知消息类型", "type", s.MessageType)
		return false
	}
}

// Flush 按入队顺序发送所有排队消息，发送后清空队列。
// 仅返回**确认送达**的条目（供调用方写入记忆/记录）：适配器返回 nil error
// （OneBot 已返回 message_id）才算送达，不存在"已提交但未确认"的中间态。
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
		if sendDeferred(a, s) {
			delivered = append(delivered, s)
		}
	}

	return delivered
}

// DropDelivery 移除投递到指定会话的交付消息（群管理审核闸门判定违规时丢弃
// Agent 回复用）：只过滤 Delivery=true 且目标匹配的项，私聊/其他群消息保留。
func (q *DeferredSendQueue) DropDelivery(messageType string, targetID int64) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.sends[:0]
	for _, s := range q.sends {
		if s.Delivery && s.MessageType == messageType && s.TargetID == targetID {
			continue
		}
		kept = append(kept, s)
	}
	q.sends = kept
}

// DropOnReview 移除审核拒绝时需要丢弃的自动消息。
// 原版队列项的 DropOnReview 默认为 false，因此不会改变官方工具行为。
func (q *DeferredSendQueue) DropOnReview(messageType string, targetID int64) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.sends[:0]
	for _, s := range q.sends {
		if s.DropOnReview && s.MessageType == messageType && s.TargetID == targetID {
			continue
		}
		kept = append(kept, s)
	}
	q.sends = kept
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

func messageHasExpression(message any) bool {
	switch v := message.(type) {
	case string:
		return segmentsHaveExpression(adapter.ParseCQCodes(adapter.NormalizeCQCodes(v)))
	case adapter.Segment:
		return segmentHasExpression(v)
	case []adapter.Segment:
		return segmentsHaveExpression(v)
	case []*adapter.Segment:
		for _, seg := range v {
			if seg != nil && segmentHasExpression(*seg) {
				return true
			}
		}
	}
	return false
}

// MessageHasExpression exposes the same structural expression check for final
// assistant content, which is not stored in DeferredSendQueue.
func MessageHasExpression(message any) bool {
	return messageHasExpression(message)
}

// MessageHasGallerySticker 只检测图库表情包（stk:// 图片段），不把普通
// QQ 内联小表情（face 段）算作表情包。供 Laya 自动表情做细粒度去重：
// 文字里的 QQ 小表情不应阻止 Laya 决策，已有图库表情仍阻止重复追加。
func MessageHasGallerySticker(message any) bool {
	switch v := message.(type) {
	case string:
		return segmentsHaveGallerySticker(adapter.ParseCQCodes(adapter.NormalizeCQCodes(v)))
	case adapter.Segment:
		return segmentHasGallerySticker(v)
	case []adapter.Segment:
		return segmentsHaveGallerySticker(v)
	case []*adapter.Segment:
		for _, seg := range v {
			if seg != nil && segmentHasGallerySticker(*seg) {
				return true
			}
		}
	}
	return false
}

func segmentsHaveGallerySticker(segments []adapter.Segment) bool {
	for _, seg := range segments {
		if segmentHasGallerySticker(seg) {
			return true
		}
	}
	return false
}

func segmentHasGallerySticker(seg adapter.Segment) bool {
	if !strings.EqualFold(strings.TrimSpace(seg.Type), "image") {
		return false
	}
	file, ok := seg.Data["file"].(string)
	return ok && strings.HasPrefix(strings.TrimSpace(file), "stk://")
}

func segmentsHaveExpression(segments []adapter.Segment) bool {
	for _, seg := range segments {
		if segmentHasExpression(seg) {
			return true
		}
	}
	return false
}

func segmentHasExpression(seg adapter.Segment) bool {
	if strings.EqualFold(strings.TrimSpace(seg.Type), "face") {
		return true
	}
	return segmentHasGallerySticker(seg)
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
