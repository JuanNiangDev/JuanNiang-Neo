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

// ListSystemLocked 返回所有 IsSystem=true 的锁定提示词，无论 IsActive 状态。
// 这些提示词由系统在启动时种子，每次构建 SystemPrompt 时强制拼接。
func (d *PromptDAO) ListSystemLocked(ctx context.Context) ([]models.Prompt, error) {
	var list []models.Prompt
	err := d.db.WithContext(ctx).Where("is_system = ?", true).
		Order("created_at ASC").Find(&list).Error
	return list, err
}

// GetByName 按名称查询 Prompt（用于系统启动时去重种子）。
func (d *PromptDAO) GetByName(ctx context.Context, name string) (*models.Prompt, error) {
	var p models.Prompt
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}
