package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

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
