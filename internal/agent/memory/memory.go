package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"JuanNiang-Neo/internal/agent/memory/longterm"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/memory/skillmem"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewModule("memory")

type ShortTermMemoryConfig = shortterm.Config
type LongTermMemoryConfig = longterm.Config

// ShortTermStore 提供 Per-ChatArea 短期记忆配置的读取接口（由 core/dao 实现）。
// MemoryGroup 通过该接口按 areaID 解析 AutoCompact / WindowSize，替代全局共享配置。
type ShortTermStore interface {
	GetOrCreate(ctx context.Context, chatAreaID string) (*models.ShortTermMemory, error)
}

type MemoryGroup struct {
	ShortTerm   *shortterm.ShortTermMemory
	LongTerm    *longterm.LongTermMemory
	SkillMemory *skillmem.SkillMemory
	LLMProvider provider.Provider // 用于 Compact 中的技能记忆更新

	// per-ChatArea 短期记忆配置缓存（cache → DB → 全局默认）
	shortTermStore  ShortTermStore
	shortTermConfMu sync.Mutex
	shortTermConf   map[string]ShortTermMemoryConfig
}

func NewMemoryGroup(st *shortterm.ShortTermMemory, lt *longterm.LongTermMemory, sm *skillmem.SkillMemory) *MemoryGroup {
	return &MemoryGroup{
		ShortTerm:     st,
		LongTerm:      lt,
		SkillMemory:   sm,
		shortTermConf: make(map[string]ShortTermMemoryConfig),
	}
}

// SetShortTermStore 注入 Per-ChatArea 配置读取源（由 agent.Init 调用）。
func (m *MemoryGroup) SetShortTermStore(store ShortTermStore) {
	m.shortTermStore = store
}

// shortTermConfigFor 解析指定 ChatArea 的短期记忆配置：优先读缓存，未命中则查 DB，
// 最后回退到全局实例默认值。解析结果缓存到内存 map，避免每次消息都查库。
func (m *MemoryGroup) shortTermConfigFor(ctx context.Context, areaID string) ShortTermMemoryConfig {
	m.shortTermConfMu.Lock()
	defer m.shortTermConfMu.Unlock()
	if c, ok := m.shortTermConf[areaID]; ok {
		return c
	}
	c := ShortTermMemoryConfig{
		WindowSize:  m.ShortTerm.WindowSize(),
		AutoCompact: m.ShortTerm.AutoCompact(),
	}
	if m.shortTermStore != nil {
		if got, err := m.shortTermStore.GetOrCreate(ctx, areaID); err == nil {
			c.WindowSize = int64(got.WindowSize)
			c.AutoCompact = got.AutoCompact
		}
	}
	m.shortTermConf[areaID] = c
	return c
}

// GetShortTermMessages 返回指定 ChatArea 当前短期记忆窗口内的消息。
func (m *MemoryGroup) GetShortTermMessages(ctx context.Context, areaID string) ([]shortterm.ChatMessage, error) {
	return m.ShortTerm.GetAll(ctx, areaID)
}

// AddShortTermMessage 向指定 ChatArea 的短期记忆追加一条消息。
// Per-ChatArea 配置解析后按该 area 的窗口维护滑动窗口；
// 如果该 area 开启了 AutoCompact 且窗口已满，异步触发 Compact。
func (m *MemoryGroup) AddShortTermMessage(ctx context.Context, areaID string, msg shortterm.ChatMessage) error {
	conf := m.shortTermConfigFor(ctx, areaID)
	if err := m.ShortTerm.AddWithWindow(ctx, areaID, msg, conf.WindowSize); err != nil {
		return err
	}

	// AutoCompact: 窗口已满时异步触发 Compact（不阻塞消息处理）
	if conf.AutoCompact && m.LLMProvider != nil {
		msgs, err := m.ShortTerm.GetAll(ctx, areaID)
		if err == nil && int64(len(msgs)) >= conf.WindowSize {
			go func() {
				compactCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if err := m.CompactShortTermMemory(compactCtx, areaID, m.LLMProvider); err != nil {
					log.Error("AutoCompact 失败", "area_id", areaID, "err", err)
				} else {
					log.Info("AutoCompact 完成", "area_id", areaID)
				}
			}()
		}
	}

	return nil
}

func (m *MemoryGroup) OverwriteShortTermMemory(ctx context.Context, areaID string, msgs []shortterm.ChatMessage) error {
	conf := m.shortTermConfigFor(ctx, areaID)
	return m.ShortTerm.OverwriteWithWindow(ctx, areaID, msgs, conf.WindowSize)
}

func (m *MemoryGroup) CompactShortTermMemory(ctx context.Context, areaID string, llm provider.Provider) error {
	return m.ShortTerm.Compact(ctx, areaID, llm, m)
}

// UpdateShortTermConfig 更新指定 ChatArea 的短期记忆配置缓存（运行时同步）。
func (m *MemoryGroup) UpdateShortTermConfig(areaID string, conf ShortTermMemoryConfig) {
	m.shortTermConfMu.Lock()
	if m.shortTermConf == nil {
		m.shortTermConf = make(map[string]ShortTermMemoryConfig)
	}
	m.shortTermConf[areaID] = conf
	m.shortTermConfMu.Unlock()
}

func (m *MemoryGroup) AddLongTermMemory(ctx context.Context, areaID, content string) error {
	return m.LongTerm.Add(ctx, areaID, content)
}

// GetExistingLongTermMemory 获取当前长期记忆内容（供 Compact 使用）。
func (m *MemoryGroup) GetExistingLongTermMemory(ctx context.Context, areaID string) ([]string, error) {
	items, err := m.LongTerm.Search(ctx, areaID, "", 3)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Content
	}
	return out, nil
}

func (m *MemoryGroup) GetLongTermMemory(ctx context.Context, areaID, query string, limit int) ([]string, error) {
	items, err := m.LongTerm.Search(ctx, areaID, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Content
	}
	return out, nil
}

func (m *MemoryGroup) UpdateLongTermConfig(conf LongTermMemoryConfig) {
	m.LongTerm.UpdateConfig(conf)
}

// GetSkillMemory 返回全局技能记忆内容。
func (m *MemoryGroup) GetSkillMemory() string {
	if m.SkillMemory == nil {
		return ""
	}
	return m.SkillMemory.Get()
}

// UpdateSkillMemory 使用 LLM 根据近期对话更新全局技能记忆。
// 实现 shortterm.SkillMemoryUpdater 接口，在 Compact 时自动触发。
func (m *MemoryGroup) UpdateSkillMemory(ctx context.Context, recentMsgs []shortterm.ChatMessage) error {
	if m.SkillMemory == nil || m.LLMProvider == nil {
		return nil
	}

	currentContent := m.SkillMemory.Get()

	// 构建近期对话文本
	var convText strings.Builder
	for _, msg := range recentMsgs {
		if msg.Role == "user" || msg.Role == "assistant" {
			convText.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
		}
	}

	prompt := fmt.Sprintf(`当前已有的技能记忆（黑话/热词/梗）：
%s

以下是最近的对话记录：
%s

请更新技能记忆：
1. 加入对话中新出现的黑话、网络热词、梗、缩写、社区用语
2. 如果某些已有的词在最近对话中完全没被使用，可以移除（不要强行保留）
3. 如果没有新内容要加，保持原样不变
4. 每个条目格式：词语 — 简短含义解释（10字以内）
5. 每行一个条目，不要编号，不要其他格式

直接输出更新后的完整技能记忆内容，不要加任何前缀或解释。`, currentContent, convText.String())

	resp, err := m.LLMProvider.Chat(ctx, provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是一个中文互联网文化专家，负责维护一份「黑话/热词/梗」的记忆清单。"},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return fmt.Errorf("技能记忆 LLM 调用失败: %w", err)
	}

	newContent := strings.TrimSpace(resp.Message.Content)
	if newContent == "" {
		return nil
	}

	if err := m.SkillMemory.Update(ctx, newContent); err != nil {
		return fmt.Errorf("技能记忆写入失败: %w", err)
	}

	log.Info("技能记忆已更新", "content_len", len(newContent))
	return nil
}

// Compile-time: ensure MemoryGroup implements shortterm.CompactStore and shortterm.SkillMemoryUpdater
var _ shortterm.CompactStore = (*MemoryGroup)(nil)
var _ shortterm.SkillMemoryUpdater = (*MemoryGroup)(nil)
