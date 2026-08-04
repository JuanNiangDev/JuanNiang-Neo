package dao

import (
	"context"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

type KnowledgeDAO struct{ db *gorm.DB }

func NewKnowledgeDAO(db *gorm.DB) *KnowledgeDAO { return &KnowledgeDAO{db: db} }

func (d *KnowledgeDAO) Create(ctx context.Context, item *models.KnowledgeItem) error {
	if item.ID == "" {
		item.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(item).Error
}

func (d *KnowledgeDAO) GetByID(ctx context.Context, id string) (*models.KnowledgeItem, error) {
	var item models.KnowledgeItem
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *KnowledgeDAO) Update(ctx context.Context, item *models.KnowledgeItem) error {
	return d.db.WithContext(ctx).Save(item).Error
}

func (d *KnowledgeDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.KnowledgeItem{}).Error
}

// List 分页列出（最新在前）。
func (d *KnowledgeDAO) List(ctx context.Context, limit, offset int) ([]models.KnowledgeItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var list []models.KnowledgeItem
	err := d.db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(limit).Offset(offset).
		Find(&list).Error
	return list, err
}

func (d *KnowledgeDAO) Count(ctx context.Context) (int64, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&models.KnowledgeItem{}).Count(&n).Error
	return n, err
}

// SetKeywords 写回提取的关键词与状态。
func (d *KnowledgeDAO) SetKeywords(ctx context.Context, id string, keywords []string, status string) error {
	return d.db.WithContext(ctx).Model(&models.KnowledgeItem{}).Where("id = ?", id).
		Updates(map[string]any{"keywords": keywords, "keyword_status": status}).Error
}

// SetKeywordStatus 更新关键词提取状态。
func (d *KnowledgeDAO) SetKeywordStatus(ctx context.Context, id, status string) error {
	return d.db.WithContext(ctx).Model(&models.KnowledgeItem{}).Where("id = ?", id).
		Update("keyword_status", status).Error
}

// Match 对话前模糊匹配：命中条件（满足其一）：
//   - 消息文本包含某条知识的某个关键词（keywords 数组）
//   - 消息与知识内容前缀（前 20 字）ILIKE 模糊匹配（兜底）
//
// 只匹配 keyword_status='ready' 的条目；结果按关键词命中数降序。
// 使用 Postgres 专有语法（unnest），不兼容 SQLite。
func (d *KnowledgeDAO) Match(ctx context.Context, msg string, limit int) ([]models.KnowledgeItem, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var items []models.KnowledgeItem
	err := d.db.WithContext(ctx).Raw(`
		SELECT * FROM knowledge_items
		WHERE deleted_at IS NULL AND keyword_status = ?
		  AND (
			EXISTS (SELECT 1 FROM unnest(keywords) k WHERE position(k IN ?) > 0)
			OR ? ILIKE '%' || SUBSTRING(content, 1, 20) || '%'
		  )
		ORDER BY (SELECT count(*) FROM unnest(keywords) k WHERE position(k IN ?) > 0) DESC, updated_at DESC
		LIMIT ?`,
		models.KeywordStatusReady, msg, msg, msg, limit,
	).Scan(&items).Error
	return items, err
}
