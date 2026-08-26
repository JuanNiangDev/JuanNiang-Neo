package dao

import (
	"context"
	"strings"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

type LongTermMemoryItemDAO struct{ db *gorm.DB }

// NewLongTermMemoryItemDAO 构造长期记忆条目 DAO。
func NewLongTermMemoryItemDAO(db *gorm.DB) *LongTermMemoryItemDAO {
	return &LongTermMemoryItemDAO{db: db}
}

// Create 写入一条长期记忆条目（ID 为空时自动生成 UUID）。
func (d *LongTermMemoryItemDAO) Create(ctx context.Context, item *models.LongTermMemoryItem) error {
	if item.ID == "" {
		item.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(item).Error
}

// ListByChatArea 按 ChatArea 列出条目（创建时间倒序，取 limit 条）。
func (d *LongTermMemoryItemDAO) ListByChatArea(ctx context.Context, chatAreaID string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// GetByIDs 批量取条目（供 RAG 语义召回按命中 tag 反查内容）。
func (d *LongTermMemoryItemDAO) GetByIDs(ctx context.Context, ids []string) ([]models.LongTermMemoryItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).Where("id IN (?)", ids).Find(&list).Error
	return list, err
}

// ListAllIDs 返回全部条目 ID（供 RAG 记忆候选集构建，仅主键列）。
func (d *LongTermMemoryItemDAO) ListAllIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Pluck("id", &ids).Error
	return ids, err
}

// likePred 返回子串匹配谓词：PostgreSQL 用 ILIKE（大小写不敏感），
// 其它方言（SQLite 测试环境）用 LIKE，避免语法错误。
func (d *LongTermMemoryItemDAO) likePred() string {
	if d.db.Dialector.Name() == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

// SearchByContent 关键词子串搜索（PG 用 ILIKE，其余方言 LIKE）。
func (d *LongTermMemoryItemDAO) SearchByContent(ctx context.Context, chatAreaID string, keyword string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ? AND content "+d.likePred()+" ?", chatAreaID, "%"+keyword+"%").
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// SemanticSearch 语义候选召回（pg_trgm GIN 倒排）：
//   - 候选产生：消息 gram 作为常量模式 LIKE OR 展开（每个 gram 独立走 GIN 索引，BitmapOr 合并），
//     trgm 命中候选是超集，之后精确过滤排序保证结果语义不变
//   - 精确排序：候选集内按 similarity(content, query) 降序（相似度=共享 trigram/并集），
//     再按时间倒序，截取 limit
//
// 兼容性：仅 PostgreSQL 支持 GIN trgm 与 similarity()；SQLite（单元测试）自动回退为
// 单关键词 ILIKE（旧行为），grams 为空时同样回退——保证两种环境下结果至少不劣于现状。
func (d *LongTermMemoryItemDAO) SemanticSearch(ctx context.Context, chatAreaID string, grams []string, query string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem

	// 回退：非 PG 方言或 gram 不足时，用整段 query 做 ILIKE（等价旧行为）
	if d.db.Dialector.Name() != "postgres" || len(grams) == 0 {
		return d.SearchByContent(ctx, chatAreaID, query, limit)
	}

	var sb strings.Builder
	sb.WriteString("SELECT * FROM long_term_memory_items")
	sb.WriteString(" WHERE chat_area_id = ? AND (")
	pred := d.likePred()
	args := make([]any, 0, len(grams)+3)
	args = append(args, chatAreaID)
	for i, g := range grams {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		sb.WriteString("content " + pred + " '%' || ? || '%'")
		args = append(args, g)
	}
	sb.WriteString(")")
	// 候选内按相似度降序（与消息整体最相关的记忆优先），再按时间倒序
	sb.WriteString(" ORDER BY similarity(content, ?) DESC, created_at DESC LIMIT ?")
	args = append(args, query, limit)

	err := d.db.WithContext(ctx).Raw(sb.String(), args...).Scan(&list).Error
	return list, err
}

// Delete 删除条目（GC 清理用）。
func (d *LongTermMemoryItemDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.LongTermMemoryItem{}).Error
}

// Touch 更新条目最近召回时间（对话召回命中时调用；GC 判定未使用记忆用）。
func (d *LongTermMemoryItemDAO) Touch(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).Where("id = ?", id).
		UpdateColumn("last_recalled_at", time.Now()).Error
}

// TouchMany 批量更新最近召回时间（召回命中多条时一次 SQL）。
func (d *LongTermMemoryItemDAO) TouchMany(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).Where("id IN ?", ids).
		UpdateColumn("last_recalled_at", time.Now()).Error
}

// MarkRAGSynced 标记条目 RAG 同步状态（GC 删除向量失败时置 false，下次重试防孤儿向量）。
func (d *LongTermMemoryItemDAO) MarkRAGSynced(ctx context.Context, id string, synced bool) error {
	return d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).Where("id = ?", id).
		UpdateColumn("rag_synced", synced).Error
}

// ListUnused 列出最近窗口内未被召回的条目（GC 用），按最近召回时间升序取 limit 条。
func (d *LongTermMemoryItemDAO) ListUnused(ctx context.Context, since time.Time, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).Where("last_recalled_at IS NULL OR last_recalled_at < ?", since).
		Order("last_recalled_at ASC").Limit(limit).Find(&list).Error
	return list, err
}

// CountByChatArea 统计某 ChatArea 的条目数。
func (d *LongTermMemoryItemDAO) CountByChatArea(ctx context.Context, chatAreaID string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Where("chat_area_id = ?", chatAreaID).Count(&count).Error
	return count, err
}

// DeleteOldest 删除某 ChatArea 最旧的条目，保留最近 keep 条（容量控制用）。
func (d *LongTermMemoryItemDAO) DeleteOldest(ctx context.Context, chatAreaID string, keep int) error {
	sub := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Select("id").
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(999999).
		Offset(keep)
	return d.db.WithContext(ctx).Where("id IN (?)", sub).Delete(&models.LongTermMemoryItem{}).Error
}
