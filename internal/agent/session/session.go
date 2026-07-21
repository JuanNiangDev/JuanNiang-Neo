package session

import (
	"context"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// SessionManager 会话管理。
// 同时维护两份数据：
//   - Postgres: Session 表 (会话元数据 + token 计数)
//   - Postgres: ChatRecord 表 (原始聊天记录，作为持久化存档)
//   - Redis: session:msgs:<sessionID> (历史方案，暂保留)
type SessionManager struct {
	dao       *dao.SessionDAO
	recordDAO *dao.ChatRecordDAO
	cache     *cache.Cache
}

func NewSessionManager(dao *dao.SessionDAO, recordDAO *dao.ChatRecordDAO, c *cache.Cache) *SessionManager {
	return &SessionManager{dao: dao, recordDAO: recordDAO, cache: c}
}

func (sm *SessionManager) GetOrCreate(ctx context.Context, chatAreaID string) (*models.Session, error) {
	return sm.dao.GetOrCreate(ctx, chatAreaID)
}

func (sm *SessionManager) GetByID(ctx context.Context, id string) (*models.Session, error) {
	return sm.dao.GetByID(ctx, id)
}

// MessageHistory 返回会话历史消息 (从 Redis 读取)。
// 注意: 短期记忆现在由 MemoryGroup 维护，本方法保留用于历史调用。
func (sm *SessionManager) MessageHistory(ctx context.Context, sessionID string) ([]provider.ChatMessage, error) {
	if sm.cache == nil {
		return nil, nil
	}
	key := "session:msgs:" + sessionID
	var msgs []provider.ChatMessage
	err := sm.cache.LRange(ctx, key, 0, -1, &msgs)
	if err != nil {
		return nil, err
	}
	// LRange 返回最早→最新，需要反转
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// AddMessage 追加消息到会话历史 (Redis)。
func (sm *SessionManager) AddMessage(ctx context.Context, sessionID string, msg provider.ChatMessage) error {
	if sm.cache == nil {
		return nil
	}
	key := "session:msgs:" + sessionID
	return sm.cache.LPush(ctx, key, msg)
}

// ClearHistory 清空会话历史 (Redis)。
func (sm *SessionManager) ClearHistory(ctx context.Context, sessionID string) error {
	if sm.cache == nil {
		return nil
	}
	key := "session:msgs:" + sessionID
	return sm.cache.Del(ctx, key)
}

// UpdateTokenUsage 累加 Token 用量。
func (sm *SessionManager) UpdateTokenUsage(ctx context.Context, sessionID string, tokens int64) error {
	return sm.dao.AddTokenUsage(ctx, sessionID, tokens)
}

// ListSessions 列出所有会话。
func (sm *SessionManager) ListSessions(ctx context.Context) ([]models.Session, error) {
	return sm.dao.List(ctx)
}

// DeleteSession 删除会话。
func (sm *SessionManager) DeleteSession(ctx context.Context, id string) error {
	if sm.cache != nil {
		sm.cache.Del(ctx, "session:msgs:"+id)
	}
	return sm.dao.Delete(ctx, id)
}

// AppendRecord 将原始聊天记录持久化到 DB (chat_records 表)。
// 与 MemoryGroup 的短期记忆解耦: 短期记忆走 Redis (易失, LLM 上下文窗口)，
// 本方法写入 Postgres 作为持久化存档, 即便 Redis 重启或 Compact 摘要后, 仍可回溯原始对话。
func (sm *SessionManager) AppendRecord(ctx context.Context, chatAreaID string, userID int64, role, content string, tokenCount int, toolCalls models.JSONMap) error {
	if sm.recordDAO == nil {
		return nil
	}
	record := &models.ChatRecord{
		ChatAreaID: chatAreaID,
		UserID:     userID,
		Role:       role,
		Content:    content,
		TokenCount: tokenCount,
		ToolCalls:  toolCalls,
	}
	return sm.recordDAO.Create(ctx, record)
}
