package session

import (
	"context"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// Session 会话管理。
type SessionManager struct {
	dao   *dao.SessionDAO
	cache *cache.Cache
}

func NewSessionManager(dao *dao.SessionDAO, c *cache.Cache) *SessionManager {
	return &SessionManager{dao: dao, cache: c}
}

func (sm *SessionManager) GetOrCreate(ctx context.Context, chatAreaID string) (*models.Session, error) {
	return sm.dao.GetOrCreate(ctx, chatAreaID)
}

func (sm *SessionManager) GetByID(ctx context.Context, id string) (*models.Session, error) {
	return sm.dao.GetByID(ctx, id)
}

// MessageHistory 返回会话的历史消息 (从 Redis 读取)。
func (sm *SessionManager) MessageHistory(ctx context.Context, sessionID string) ([]provider.ChatMessage, error) {
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

// AddMessage 追加消息到会话历史。
func (sm *SessionManager) AddMessage(ctx context.Context, sessionID string, msg provider.ChatMessage) error {
	key := "session:msgs:" + sessionID
	return sm.cache.LPush(ctx, key, msg)
}

// ClearHistory 清空会话历史。
func (sm *SessionManager) ClearHistory(ctx context.Context, sessionID string) error {
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
	sm.cache.Del(ctx, "session:msgs:"+id)
	return sm.dao.Delete(ctx, id)
}
