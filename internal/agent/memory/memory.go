package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ragcaller "JuanNiang-Neo/infrastructure/rag/handler"
	"JuanNiang-Neo/internal/agent/memory/longterm"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/memory/skillmem"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/core/ragtag"
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
	LLMProvider provider.Provider // 用于 Compact 中的技能记忆更新（兼容旧赋值，动态获取函数优先）

	// RAGClient 向量检索客户端（Compact 双写记忆向量用）；Load()=nil 时静默跳过。
	// 原子指针：Web 配置热更新（HTTP goroutine）与 Compact 双写（agent goroutine）并发读写无竞争。
	RAGClient atomic.Pointer[ragcaller.Client]

	// llmProviderFn 动态获取 Text LLM Provider：Compact 触发时实时取最新模型，
	// 避免启动时序（Init 时 ProviderGroup 尚未加载）与 Provider 热更新导致的 nil/过期问题。
	llmProviderFn func() provider.Provider

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

// SetLLMProviderFn 注入 Text LLM Provider 动态获取函数（由 agent.Init 调用）。
func (m *MemoryGroup) SetLLMProviderFn(fn func() provider.Provider) {
	m.llmProviderFn = fn
}

// SetRAGClient 注入 RAG 向量检索客户端（双写记忆向量用；nil=未启用）。
func (m *MemoryGroup) SetRAGClient(c *ragcaller.Client) {
	m.RAGClient.Store(c)
}

// getLLMProvider 返回当前可用的 Text LLM Provider：优先动态获取函数，回退旧字段。
func (m *MemoryGroup) getLLMProvider() provider.Provider {
	if m.llmProviderFn != nil {
		return m.llmProviderFn()
	}
	return m.LLMProvider
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
	if conf.AutoCompact {
		llm := m.getLLMProvider()
		if llm != nil {
			msgs, err := m.ShortTerm.GetAll(ctx, areaID)
			if err == nil && int64(len(msgs)) >= conf.WindowSize {
				go func() {
					compactCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					if err := m.CompactShortTermMemory(compactCtx, areaID, llm); err != nil {
						// 同一 area 已有 Compact 在运行是预期的并发防护，非致命错误，降级为 Debug
						if errors.Is(err, shortterm.ErrCompactInProgress) {
							log.Debug("AutoCompact 已在运行，跳过本次触发", "area_id", areaID)
						} else {
							log.Error("AutoCompact 失败", "area_id", areaID, "err", err)
						}
					} else {
						log.Info("AutoCompact 完成", "area_id", areaID)
					}
				}()
			}
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

// RemoveHotItems 从长期记忆热区缓存移除条目（GC 删除 PG/RAG 后调用，避免热区残留）。
func (m *MemoryGroup) RemoveHotItems(ids map[string]bool) {
	if m.LongTerm != nil {
		m.LongTerm.Remove(ids)
	}
}

// ragMemSyncTimeout 记忆向量双写超时：RAG 属于可降级能力，短超时快速失败不拖主链路。
const ragMemSyncTimeout = 5 * time.Second

// AddLongTermMemory 写入长期记忆并异步双写 RAG 向量。
// 双写异步化：Compact 落在 agent 路径内，同步 30s Upsert 会把 Agent 循环拖死；
// RAG 未配置/不可用静默跳过，残留可由「记忆页手动同步向量库」补齐。
func (m *MemoryGroup) AddLongTermMemory(ctx context.Context, areaID, content string) error {
	item, err := m.LongTerm.Add(ctx, areaID, content)
	if err != nil {
		return err
	}
	if item != nil {
		m.syncMemoryVector(item.ID, content)
	}
	return nil
}

// syncMemoryVector 异步写记忆向量到 RAG-Service（goroutine + 短超时快速失败，不阻塞调用方）。
func (m *MemoryGroup) syncMemoryVector(id, content string) {
	go func() {
		cli := m.RAGClient.Load()
		if cli == nil {
			return
		}
		ragCtx, cancel := context.WithTimeout(context.Background(), ragMemSyncTimeout)
		defer cancel()
		if _, err := cli.Upsert(ragCtx, ragtag.Memory(id), content); err != nil {
			log.Warn("记忆向量同步失败", "id", id, "err", err)
		}
	}()
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

// RecallLongTermMemory 对话主链路召回：按当前消息语义召回长期记忆
// （消息 gram → pg_trgm 倒排候选 → similarity 排序；空候选/异常回退最近）。
func (m *MemoryGroup) RecallLongTermMemory(ctx context.Context, areaID, msg string, limit int) ([]string, error) {
	query, grams := longterm.RecallTerms(msg)
	if query == "" {
		// 无有效文本（纯表情/纯 CQ 码）：回退最近条目
		return m.GetLongTermMemory(ctx, areaID, "", limit)
	}
	items, err := m.LongTerm.Recall(ctx, areaID, grams, query, limit)
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
	if m.SkillMemory == nil {
		return nil
	}
	llm := m.getLLMProvider()
	if llm == nil {
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

	resp, err := llm.Chat(ctx, provider.ChatRequest{
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
