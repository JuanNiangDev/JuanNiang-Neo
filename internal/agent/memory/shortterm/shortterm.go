package shortterm

import (
	"context"
	"encoding/json"
	"fmt"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewModule("shortterm")

// ChatMessage 聊天消息模型。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// Config 短期记忆配置。
type Config struct {
	WindowSize  int64
	AutoCompact bool
}

// ShortTermMemory 基于 Redis 的短期记忆，使用 List 实现滑动窗口。
// 所有方法按 areaID 隔离，实例本身无状态，可跨 ChatArea 共享。
type ShortTermMemory struct {
	conf  Config
	cache *cache.Cache
}

func New(conf Config, c *cache.Cache) *ShortTermMemory {
	if conf.WindowSize <= 0 {
		conf.WindowSize = 20
	}
	return &ShortTermMemory{conf: conf, cache: c}
}

func (m *ShortTermMemory) WindowSize() int64     { return m.conf.WindowSize }
func (m *ShortTermMemory) SetWindowSize(n int64) { m.conf.WindowSize = n }
func (m *ShortTermMemory) AutoCompact() bool     { return m.conf.AutoCompact }
func (m *ShortTermMemory) SetAutoCompact(v bool) { m.conf.AutoCompact = v }

func (m *ShortTermMemory) key(areaID string) string {
	return "shortterm:msgs:" + areaID
}

// Add 追加一条消息并维护滑动窗口（保留最近 WindowSize 条，最早在前）。
func (m *ShortTermMemory) Add(ctx context.Context, areaID string, msg ChatMessage) error {
	if m.cache == nil {
		return fmt.Errorf("shortterm cache 未初始化")
	}
	key := m.key(areaID)
	if err := m.cache.RPush(ctx, key, msg); err != nil {
		return err
	}
	// 仅保留最近 WindowSize 条
	return m.cache.LTrim(ctx, key, -m.conf.WindowSize, -1)
}

// GetAll 返回该 ChatArea 当前窗口内的全部消息（按时间最早→最新）。
func (m *ShortTermMemory) GetAll(ctx context.Context, areaID string) ([]ChatMessage, error) {
	if m.cache == nil {
		return nil, fmt.Errorf("shortterm cache 未初始化")
	}
	var msgs []ChatMessage
	if err := m.cache.LRange(ctx, m.key(areaID), 0, -1, &msgs); err != nil {
		return nil, fmt.Errorf("shortterm getall: %w", err)
	}
	return msgs, nil
}

// Overwrite 用新列表覆盖窗口（先清空后追加）。
func (m *ShortTermMemory) Overwrite(ctx context.Context, areaID string, msgs []ChatMessage) error {
	if m.cache == nil {
		return fmt.Errorf("shortterm cache 未初始化")
	}
	key := m.key(areaID)
	if err := m.cache.Del(ctx, key); err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	args := make([]any, len(msgs))
	for i, mm := range msgs {
		args[i] = mm
	}
	if err := m.cache.RPush(ctx, key, args...); err != nil {
		return err
	}
	return m.cache.LTrim(ctx, key, -m.conf.WindowSize, -1)
}

// Clear 清空窗口。
func (m *ShortTermMemory) Clear(ctx context.Context, areaID string) error {
	if m.cache == nil {
		return fmt.Errorf("shortterm cache 未初始化")
	}
	return m.cache.Del(ctx, m.key(areaID))
}

// Compact 压缩短期记忆: 调用 LLM 摘要后写入长期记忆。
func (m *ShortTermMemory) Compact(ctx context.Context, areaID string, llm provider.Provider, store LongTermStore) error {
	msgs, err := m.GetAll(ctx, areaID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	content := buildCompactContent(msgs)
	prompt := "请将以下对话历史压缩为简洁摘要，保留关键信息和上下文:\n\n" + content

	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是一个对话摘要助手，请用中文输出简洁摘要。"},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		log.Error("Compact LLM 调用失败", "err", err)
		return err
	}

	summary := resp.Message.Content
	if err := store.AddLongTermMemory(ctx, areaID, summary); err != nil {
		return fmt.Errorf("compact 写入长期记忆失败: %w", err)
	}

	log.Info("短期记忆 Compact 完成", "area_id", areaID, "summary_len", len(summary))
	return nil
}

type LongTermStore interface {
	AddLongTermMemory(ctx context.Context, areaID string, content string) error
}

func buildCompactContent(msgs []ChatMessage) string {
	var out string
	for _, m := range msgs {
		out += fmt.Sprintf("[%s]: %s\n", m.Role, m.Content)
	}
	return out
}

func (m *ShortTermMemory) Export(ctx context.Context, areaID string) ([]provider.ChatMessage, error) {
	msgs, err := m.GetAll(ctx, areaID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.ChatMessage, len(msgs))
	for i, msg := range msgs {
		out[i] = provider.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Name:    msg.Name,
		}
	}
	return out, nil
}

func (m *ShortTermMemory) ExportJSON(ctx context.Context, areaID string) ([]byte, error) {
	msgs, err := m.GetAll(ctx, areaID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(msgs)
}
