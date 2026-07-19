package memory

import (
	"context"

	"JuanNiang-Neo/internal/agent/memory/bgtask"
	"JuanNiang-Neo/internal/agent/memory/longterm"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/provider"
)

type ShortTermMemoryConfig = shortterm.Config
type LongTermMemoryConfig = longterm.Config

type MemoryGroup struct {
	ShortTerm            *shortterm.ShortTermMemory
	LongTerm             *longterm.LongTermMemory
	BackGroundTaskMemory *bgtask.BackGroundTaskMemory
}

func NewMemoryGroup(st *shortterm.ShortTermMemory, lt *longterm.LongTermMemory, bgt *bgtask.BackGroundTaskMemory) *MemoryGroup {
	return &MemoryGroup{
		ShortTerm:            st,
		LongTerm:             lt,
		BackGroundTaskMemory: bgt,
	}
}

// GetShortTermMessages 返回指定 ChatArea 当前短期记忆窗口内的消息。
func (m *MemoryGroup) GetShortTermMessages(ctx context.Context, areaID string) ([]shortterm.ChatMessage, error) {
	return m.ShortTerm.GetAll(ctx, areaID)
}

// AddShortTermMessage 向指定 ChatArea 的短期记忆追加一条消息。
func (m *MemoryGroup) AddShortTermMessage(ctx context.Context, areaID string, msg shortterm.ChatMessage) error {
	return m.ShortTerm.Add(ctx, areaID, msg)
}

func (m *MemoryGroup) OverwriteShortTermMemory(ctx context.Context, areaID string, msgs []shortterm.ChatMessage) error {
	return m.ShortTerm.Overwrite(ctx, areaID, msgs)
}

func (m *MemoryGroup) CompactShortTermMemory(ctx context.Context, areaID string, llm provider.Provider) error {
	return m.ShortTerm.Compact(ctx, areaID, llm, m)
}

func (m *MemoryGroup) UpdateShortTermConfig(conf ShortTermMemoryConfig) {
	m.ShortTerm.SetWindowSize(conf.WindowSize)
	m.ShortTerm.SetAutoCompact(conf.AutoCompact)
}

func (m *MemoryGroup) AddLongTermMemory(ctx context.Context, areaID, content string) error {
	return m.LongTerm.Add(ctx, areaID, content)
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

func (m *MemoryGroup) AddBackGroundTask(meta bgtask.BackGroundTaskMetaInfo) string {
	return m.BackGroundTaskMemory.Add(meta)
}

func (m *MemoryGroup) DelBackGroundTask(taskID string) {
	m.BackGroundTaskMemory.Del(taskID)
}

func (m *MemoryGroup) GetBackGroundTask(taskID string) (bgtask.BackGroundTaskMetaInfo, bool) {
	return m.BackGroundTaskMemory.Get(taskID)
}

func (m *MemoryGroup) ListBackGroundTasks() map[string]bgtask.BackGroundTaskMetaInfo {
	return m.BackGroundTaskMemory.List()
}
