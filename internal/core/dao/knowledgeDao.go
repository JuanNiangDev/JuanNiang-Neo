package dao

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
// keywords 必须转成 models.JSONSlice，否则 GORM 直接把 []string 交给 PG jsonb 列会报
// “column \"keywords\" is of type jsonb but expression is of type record”(SQLSTATE 42804)。
func (d *KnowledgeDAO) SetKeywords(ctx context.Context, id string, keywords []string, status string) error {
	return d.db.WithContext(ctx).Model(&models.KnowledgeItem{}).Where("id = ?", id).
		Updates(map[string]any{"keywords": models.JSONSlice(keywords), "keyword_status": status}).Error
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
// 额外返回每条命中知识实际命中的关键词（HitKeywords），供命中统计使用。
// 使用 Postgres 专有语法（jsonb_array_elements_text），不兼容 SQLite。
// 注意：keywords 是 jsonb 列，不能用 unnest（它只接受数组），必须用 jsonb_array_elements_text。
func (d *KnowledgeDAO) Match(ctx context.Context, msg string, limit int) ([]models.KnowledgeItem, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var items []models.KnowledgeItem
	err := d.db.WithContext(ctx).Raw(`
		SELECT k.*,
		  COALESCE(
			(SELECT json_agg(kw) FROM jsonb_array_elements_text(k.keywords) kw WHERE position(kw IN ?) > 0),
			'[]'::json
		  ) AS hit_keywords
		FROM knowledge_items k
		WHERE k.deleted_at IS NULL AND k.keyword_status = ?
		  AND (
			EXISTS (SELECT 1 FROM jsonb_array_elements_text(k.keywords) kw WHERE position(kw IN ?) > 0)
			OR ? ILIKE '%' || SUBSTRING(k.content, 1, 20) || '%'
		  )
		ORDER BY (SELECT count(*) FROM jsonb_array_elements_text(k.keywords) kw WHERE position(kw IN ?) > 0) DESC, k.updated_at DESC
		LIMIT ?`,
		msg, models.KeywordStatusReady, msg, msg, msg, limit,
	).Scan(&items).Error
	return items, err
}

// RecordKeywordHits 批量累加关键词命中次数（upsert，不存在则插入、存在则 +1）。
func (d *KnowledgeDAO) RecordKeywordHits(ctx context.Context, keywords []string) error {
	if len(keywords) == 0 {
		return nil
	}
	now := time.Now()
	rows := make([]models.KeywordHit, 0, len(keywords))
	for _, kw := range keywords {
		rows = append(rows, models.KeywordHit{Keyword: kw, HitCount: 1, UpdatedAt: now})
	}
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "keyword"}},
		DoUpdates: clause.Assignments(map[string]any{
			"hit_count":  gorm.Expr("keyword_hits.hit_count + EXCLUDED.hit_count"),
			"updated_at": now,
		}),
	}).Create(&rows).Error
}

// KeywordCloud 词云：所有 ready 条目关键词的词频统计（TOP N，按出现条数降序）。
func (d *KnowledgeDAO) KeywordCloud(ctx context.Context, limit int) ([]models.KeywordCount, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []models.KeywordCount
	err := d.db.WithContext(ctx).Raw(`
		SELECT kw AS keyword, count(*) AS count
		FROM knowledge_items k, jsonb_array_elements_text(k.keywords) kw
		WHERE k.deleted_at IS NULL AND k.keyword_status = ?
		GROUP BY kw
		ORDER BY count DESC, kw
		LIMIT ?`, models.KeywordStatusReady, limit).Scan(&out).Error
	return out, err
}

// KeywordHitRank 关键词命中次数排行（TOP N）。
func (d *KnowledgeDAO) KeywordHitRank(ctx context.Context, limit int) ([]models.KeywordHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var out []models.KeywordHit
	err := d.db.WithContext(ctx).Raw(`
		SELECT keyword, hit_count
		FROM keyword_hits
		ORDER BY hit_count DESC, keyword
		LIMIT ?`, limit).Scan(&out).Error
	return out, err
}
