package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"
	"errors"

	"gorm.io/gorm"
)

type ChatAreaDAO struct{ db *gorm.DB }

func NewChatAreaDAO(db *gorm.DB) *ChatAreaDAO { return &ChatAreaDAO{db: db} }

func (d *ChatAreaDAO) GetOrCreate(ctx context.Context, areaType models.AreaType, targetID int64) (*models.ChatArea, error) {
	var area models.ChatArea
	err := d.db.WithContext(ctx).
		Where("area_type = ? AND target_id = ?", areaType, targetID).
		First(&area).Error
	if err == nil {
		return &area, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	area = models.ChatArea{
		ID:       newUUID(),
		AreaType: areaType,
		TargetID: targetID,
	}
	if err := d.db.WithContext(ctx).Create(&area).Error; err != nil {
		return nil, err
	}
	return &area, nil
}

func (d *ChatAreaDAO) GetByID(ctx context.Context, id string) (*models.ChatArea, error) {
	var a models.ChatArea
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *ChatAreaDAO) List(ctx context.Context) ([]models.ChatArea, error) {
	var list []models.ChatArea
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ChatAreaDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.ChatArea{}).Count(&count).Error
	return count, err
}

func (d *ChatAreaDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ChatArea{}).Error
}
