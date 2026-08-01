// Package replyer 提供回复消息的发送能力。
// 独立于 Agent 核心，支持文字、图片、图文混合发送，
// CQ 码自动转换，Agent 和 Plugin 均可通过 API 调用。
package replyer

import (
	"fmt"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewLogger("replyer")

// ReplyTarget 消息发送目标。
type ReplyTarget struct {
	MessageType string // "private" | "group"
	UserID      int64
	GroupID     int64
}

// ImageSource 图片来源。
type ImageSource struct {
	URL  string // HTTP(S) URL 或 base64 data URI
	Type string // "url" | "base64"
}

// MessageSegment 消息段（文字/图片/at/回复等）。
type MessageSegment struct {
	Type string         // "text" | "image" | "at" | "face" | "reply"
	Data map[string]any // 对应 OneBot11 segment.data
}

// SendResult 发送结果。
type SendResult struct {
	MessageID int64
	SentParts int // 实际发送的消息条数
}

// SendOptions 发送选项。
type SendOptions struct {
	AutoSplit     bool // 自动切分长文本（默认 true）
	MaxSegments   int  // 最大段数（默认 5）
	StripMarkdown bool // 去除 Markdown 格式
	EnableTypo    bool // 注入错别字（默认 false）
}

// Replyer 回复发送器。
type Replyer struct {
	adapter  *adapter.Adapter
	splitter Splitter // 消息切分器接口
}

// Splitter 消息切分器接口（由 splitter 包实现）。
type Splitter interface {
	Process(text string, opts SplitterOptions) []string
}

// SplitterOptions 切分选项。
type SplitterOptions struct {
	MaxSegments   int
	StripMarkdown bool
	EnableTypo    bool
}

// New 创建 Replyer 实例。
func New(a *adapter.Adapter) *Replyer {
	return &Replyer{adapter: a}
}

// SetSplitter 注入消息切分器。
func (r *Replyer) SetSplitter(s Splitter) { r.splitter = s }

// SendText 发送纯文字消息（自动切分长文本）。
func (r *Replyer) SendText(target ReplyTarget, text string, opts SendOptions) (*SendResult, error) {
	if r.adapter == nil {
		return nil, fmt.Errorf("replyer: adapter 未初始化")
	}

	var parts []string
	if r.splitter != nil && opts.AutoSplit {
		parts = r.splitter.Process(text, SplitterOptions{
			MaxSegments:   opts.MaxSegments,
			StripMarkdown: opts.StripMarkdown,
			EnableTypo:    opts.EnableTypo,
		})
	} else {
		parts = []string{text}
	}

	if len(parts) == 0 {
		return &SendResult{}, nil
	}

	var lastMsgID int64
	for _, part := range parts {
		msgID, err := r.sendRaw(target, part)
		if err != nil {
			log.Error("发送文字消息失败", "err", err, "target_type", target.MessageType)
			return nil, err
		}
		lastMsgID = msgID
	}

	return &SendResult{MessageID: lastMsgID, SentParts: len(parts)}, nil
}

// SendImage 发送图片（自动处理 URL/base64 → CQ 码）。
func (r *Replyer) SendImage(target ReplyTarget, img ImageSource, caption string) (*SendResult, error) {
	if r.adapter == nil {
		return nil, fmt.Errorf("replyer: adapter 未初始化")
	}

	content := CQImageFromSource(img)
	if caption != "" {
		content = caption + content
	}

	msgID, err := r.sendRaw(target, content)
	if err != nil {
		log.Error("发送图片失败", "err", err, "target_type", target.MessageType)
		return nil, err
	}

	return &SendResult{MessageID: msgID, SentParts: 1}, nil
}

// SendMixed 发送图文混合消息。
func (r *Replyer) SendMixed(target ReplyTarget, segments []MessageSegment) (*SendResult, error) {
	if r.adapter == nil {
		return nil, fmt.Errorf("replyer: adapter 未初始化")
	}

	content := BuildCQRaw(segments)
	msgID, err := r.sendRaw(target, content)
	if err != nil {
		log.Error("发送混合消息失败", "err", err, "target_type", target.MessageType)
		return nil, err
	}

	return &SendResult{MessageID: msgID, SentParts: 1}, nil
}

// SendCQRaw 直接发送 CQ 码文本。
func (r *Replyer) SendCQRaw(target ReplyTarget, cqRaw string) (*SendResult, error) {
	if r.adapter == nil {
		return nil, fmt.Errorf("replyer: adapter 未初始化")
	}

	msgID, err := r.sendRaw(target, cqRaw)
	if err != nil {
		return nil, err
	}

	return &SendResult{MessageID: msgID, SentParts: 1}, nil
}

// sendRaw 底层发送。
func (r *Replyer) sendRaw(target ReplyTarget, content string) (int64, error) {
	switch target.MessageType {
	case "private":
		return r.adapter.SendPrivateMsg(target.UserID, content)
	case "group":
		return r.adapter.SendGroupMsg(target.GroupID, content)
	default:
		return 0, fmt.Errorf("replyer: unknown message type %q", target.MessageType)
	}
}

// DeleteMsg 撤回消息。
func (r *Replyer) DeleteMsg(msgID int64) error {
	if r.adapter == nil {
		return fmt.Errorf("replyer: adapter 未初始化")
	}
	return r.adapter.DeleteMsg(msgID)
}

// TargetFromMessage 从 MessageEvent 构建 ReplyTarget。
func TargetFromMessage(msg *adapter.MessageEvent) ReplyTarget {
	return ReplyTarget{
		MessageType: msg.MessageType,
		UserID:      msg.UserID,
		GroupID:     msg.GroupID,
	}
}
