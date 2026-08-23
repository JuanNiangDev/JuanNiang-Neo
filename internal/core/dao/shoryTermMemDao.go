package dao

import (
	"context"
	"strings"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

type LongTermMemoryItemDAO struct{ db *gorm.DB }

func NewLongTermMemoryItemDAO(db *gorm.DB) *LongTermMemoryItemDAO {
	return &LongTermMemoryItemDAO{db: db}
}

func (d *LongTermMemoryItemDAO) Create(ctx context.Context, item *models.LongTermMemoryItem) error {
	if item.ID == "" {
		item.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(item).Error
}

func (d *LongTermMemoryItemDAO) ListByChatArea(ctx context.Context, chatAreaID string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *LongTermMemoryItemDAO) SearchByContent(ctx context.Context, chatAreaID string, keyword string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ? AND content ILIKE ?", chatAreaID, "%"+keyword+"%").
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
	args := make([]any, 0, len(grams)+3)
	args = append(args, chatAreaID)
	for i, g := range grams {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		sb.WriteString("content ILIKE '%' || ? || '%'")
		args = append(args, g)
	}
	sb.WriteString(")")
	// 候选内按相似度降序（与消息整体最相关的记忆优先），再按时间倒序
	sb.WriteString(" ORDER BY similarity(content, ?) DESC, created_at DESC LIMIT ?")
	args = append(args, query, limit)

	err := d.db.WithContext(ctx).Raw(sb.String(), args...).Scan(&list).Error
	return list, err
}

func (d *LongTermMemoryItemDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.LongTermMemoryItem{}).Error
}

func (d *LongTermMemoryItemDAO) CountByChatArea(ctx context.Context, chatAreaID string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Where("chat_area_id = ?", chatAreaID).Count(&count).Error
	return count, err
}

func (d *LongTermMemoryItemDAO) DeleteOldest(ctx context.Context, chatAreaID string, keep int) error {
	sub := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Select("id").
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(999999).
		Offset(keep)
	return d.db.WithContext(ctx).Where("id IN (?)", sub).Delete(&models.LongTermMemoryItem{}).Error
}
