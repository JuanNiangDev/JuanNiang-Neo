package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type PromptDAO struct{ db *gorm.DB }

func NewPromptDAO(db *gorm.DB) *PromptDAO { return &PromptDAO{db: db} }

func (d *PromptDAO) Create(ctx context.Context, p *models.Prompt) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *PromptDAO) GetByID(ctx context.Context, id string) (*models.Prompt, error) {
	var p models.Prompt
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PromptDAO) Update(ctx context.Context, p *models.Prompt) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *PromptDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Prompt{}).Error
}

func (d *PromptDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Prompt{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *PromptDAO) List(ctx context.Context) ([]models.Prompt, error) {
	var list []models.Prompt
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *PromptDAO) ListByType(ctx context.Context, typ models.PromptType) ([]models.Prompt, error) {
	var list []models.Prompt
	err := d.db.WithContext(ctx).Where("is_active = ? AND type = ?", true, typ).
		Order("created_at DESC").Find(&list).Error
	return list, err
}
