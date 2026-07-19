package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type PluginDAO struct{ db *gorm.DB }

func NewPluginDAO(db *gorm.DB) *PluginDAO { return &PluginDAO{db: db} }

func (d *PluginDAO) Create(ctx context.Context, p *models.Plugin) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *PluginDAO) GetByID(ctx context.Context, id string) (*models.Plugin, error) {
	var p models.Plugin
	err := d.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PluginDAO) GetByName(ctx context.Context, name string) (*models.Plugin, error) {
	var p models.Plugin
	err := d.db.WithContext(ctx).First(&p, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PluginDAO) Update(ctx context.Context, p *models.Plugin) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *PluginDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Plugin{}).Error
}

func (d *PluginDAO) List(ctx context.Context) ([]models.Plugin, error) {
	var list []models.Plugin
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *PluginDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Plugin{}).Where("id = ?", id).
		Update("is_active", active).Error
}
