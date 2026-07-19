package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type ToolConfigDAO struct{ db *gorm.DB }

func NewToolConfigDAO(db *gorm.DB) *ToolConfigDAO { return &ToolConfigDAO{db: db} }

func (d *ToolConfigDAO) Create(ctx context.Context, t *models.ToolConfig) error {
	if t.ID == "" {
		t.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(t).Error
}

func (d *ToolConfigDAO) GetByID(ctx context.Context, id string) (*models.ToolConfig, error) {
	var t models.ToolConfig
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *ToolConfigDAO) GetByName(ctx context.Context, name string) (*models.ToolConfig, error) {
	var t models.ToolConfig
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *ToolConfigDAO) Update(ctx context.Context, t *models.ToolConfig) error {
	return d.db.WithContext(ctx).Save(t).Error
}

func (d *ToolConfigDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ToolConfig{}).Error
}

func (d *ToolConfigDAO) List(ctx context.Context) ([]models.ToolConfig, error) {
	var list []models.ToolConfig
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ToolConfigDAO) ListActive(ctx context.Context) ([]models.ToolConfig, error) {
	var list []models.ToolConfig
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ToolConfigDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.ToolConfig{}).Where("id = ?", id).
		Update("is_active", active).Error
}
