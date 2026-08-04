package dao

import (
	"context"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// StickerDAO 表情包（图床二次封装）+ 标签。
type StickerDAO struct{ db *gorm.DB }

func NewStickerDAO(db *gorm.DB) *StickerDAO { return &StickerDAO{db: db} }

// ---------- 表情 ----------

// newStickerID 生成短 UUID（取完整 uuid 前 8 位 hex）。
func newStickerID() string {
	return newUUID()[:8]
}

// Create 创建表情。短 UUID 冲突时自动重试。
func (d *StickerDAO) Create(ctx context.Context, s *models.Sticker) error {
	for i := 0; i < 5; i++ {
		if s.ID == "" {
			s.ID = newStickerID()
		}
		if err := d.db.WithContext(ctx).Create(s).Error; err != nil {
			// 主键冲突（短 UUID 碰撞）则重生成重试
			if isDupKeyErr(err) {
				s.ID = ""
				continue
			}
			return err
		}
		return nil
	}
	return gorm.ErrDuplicatedKey
}

func (d *StickerDAO) GetByID(ctx context.Context, id string) (*models.Sticker, error) {
	var s models.Sticker
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// List 分页列出表情，支持按标签过滤（tags jsonb ? 操作符）与名称/简介模糊匹配。
func (d *StickerDAO) List(ctx context.Context, tag, keyword string, limit, offset int) ([]models.Sticker, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var list []models.Sticker
	q := d.db.WithContext(ctx).Model(&models.Sticker{})
	if tag != "" {
		q = q.Where("tags ? ?", tag)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR desc ILIKE ?", like, like)
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, err
}

// Count 统计表情数（与 List 相同过滤条件）。
func (d *StickerDAO) Count(ctx context.Context, tag, keyword string) (int64, error) {
	var n int64
	q := d.db.WithContext(ctx).Model(&models.Sticker{})
	if tag != "" {
		q = q.Where("tags ? ?", tag)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR desc ILIKE ?", like, like)
	}
	err := q.Count(&n).Error
	return n, err
}

func (d *StickerDAO) Update(ctx context.Context, s *models.Sticker) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *StickerDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Sticker{}).Error
}

// GetByImageID 按图床图片长 UUID 查表情（防重复引用同一张图）。
func (d *StickerDAO) GetByImageID(ctx context.Context, imageID string) (*models.Sticker, error) {
	var s models.Sticker
	if err := d.db.WithContext(ctx).Where("image_id = ?", imageID).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// RemoveTagFromAll 删除标签时从所有表情的 Tags 数组中移除该标签。
func (d *StickerDAO) RemoveTagFromAll(ctx context.Context, tagName string) error {
	var list []models.Sticker
	if err := d.db.WithContext(ctx).Model(&models.Sticker{}).Find(&list).Error; err != nil {
		return err
	}
	for i := range list {
		removed := false
		tags := make([]string, 0, len(list[i].Tags))
		for _, t := range list[i].Tags {
			if t == tagName {
				removed = true
				continue
			}
			tags = append(tags, t)
		}
		if removed {
			list[i].Tags = tags
			if err := d.db.WithContext(ctx).Model(&models.Sticker{}).
				Where("id = ?", list[i].ID).
				Update("tags", models.JSONSlice(tags)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- 标签 ----------

func (d *StickerDAO) TagCreate(ctx context.Context, name string) (*models.StickerTag, error) {
	t := &models.StickerTag{ID: newUUID(), Name: name}
	if err := d.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (d *StickerDAO) TagList(ctx context.Context) ([]models.StickerTag, error) {
	var list []models.StickerTag
	err := d.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error
	return list, err
}

func (d *StickerDAO) TagGetByID(ctx context.Context, id string) (*models.StickerTag, error) {
	var t models.StickerTag
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *StickerDAO) TagDelete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.StickerTag{}).Error
}
