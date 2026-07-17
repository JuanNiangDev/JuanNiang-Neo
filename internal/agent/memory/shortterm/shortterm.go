package shortterm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

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

// ShortTermMemory 基于 Postgres 的短期记忆，通过 ChatRecord 实现滑动窗口。
type ShortTermMemory struct {
	conf   Config
	dao    *dao.ChatRecordDAO
	areaID string
}

func New(conf Config, chatRecordDAO *dao.ChatRecordDAO, areaID string) *ShortTermMemory {
	if conf.WindowSize <= 0 {
		conf.WindowSize = 20
	}
	return &ShortTermMemory{conf: conf, dao: chatRecordDAO, areaID: areaID}
}

func (m *ShortTermMemory) WindowSize() int64    { return m.conf.WindowSize }
func (m *ShortTermMemory) SetWindowSize(n int64) { m.conf.WindowSize = n }
func (m *ShortTermMemory) AutoCompact() bool     { return m.conf.AutoCompact }
func (m *ShortTermMemory) SetAutoCompact(v bool) { m.conf.AutoCompact = v }

func (m *ShortTermMemory) Add(ctx context.Context, msg ChatMessage) error {
	record := &models.ChatRecord{
		ChatAreaID: m.areaID,
		UserID:     0,
		Role:       msg.Role,
		Content:    msg.Content,
	}
	return m.dao.Create(ctx, record)
}

func (m *ShortTermMemory) GetAll(ctx context.Context) ([]ChatMessage, error) {
	records, _, err := m.dao.ListByChatArea(ctx, m.areaID, int(m.conf.WindowSize), 0)
	if err != nil {
		return nil, fmt.Errorf("shortterm getall: %w", err)
	}

	msgs := make([]ChatMessage, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		msgs = append(msgs, ChatMessage{
			Role:    r.Role,
			Content: r.Content,
		})
	}
	return msgs, nil
}

func (m *ShortTermMemory) Overwrite(ctx context.Context, msgs []ChatMessage) error {
	// 简单实现：追加新消息，旧消息自然被窗口大小限制
	for i := len(msgs) - 1; i >= 0; i-- {
		if err := m.Add(ctx, msgs[i]); err != nil {
			return err
		}
	}
	return nil
}

// Compact 压缩短期记忆: 调用 LLM 摘要后写入长期记忆。
func (m *ShortTermMemory) Compact(ctx context.Context, llm provider.Provider, store LongTermStore) error {
	msgs, err := m.GetAll(ctx)
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
		slog.Error("Compact LLM 调用失败", "err", err)
		return err
	}

	summary := resp.Message.Content
	if err := store.AddLongTermMemory(ctx, m.areaID, summary); err != nil {
		return fmt.Errorf("compact 写入长期记忆失败: %w", err)
	}

	slog.Info("短期记忆 Compact 完成", "area_id", m.areaID, "summary_len", len(summary))
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

func (m *ShortTermMemory) Export(ctx context.Context) ([]provider.ChatMessage, error) {
	msgs, err := m.GetAll(ctx)
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

func (m *ShortTermMemory) ExportJSON(ctx context.Context) ([]byte, error) {
	msgs, err := m.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(msgs)
}
