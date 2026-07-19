package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type SkillDAO struct{ db *gorm.DB }

func NewSkillDAO(db *gorm.DB) *SkillDAO { return &SkillDAO{db: db} }

func (d *SkillDAO) Create(ctx context.Context, s *models.Skill) error {
	if s.ID == "" {
		s.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(s).Error
}

func (d *SkillDAO) GetByID(ctx context.Context, id string) (*models.Skill, error) {
	var s models.Skill
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SkillDAO) Update(ctx context.Context, s *models.Skill) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *SkillDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Skill{}).Error
}

func (d *SkillDAO) List(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Order("priority DESC, created_at DESC").Find(&list).Error
	return list, err
}

func (d *SkillDAO) ListActive(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("priority DESC, created_at DESC").Find(&list).Error
	return list, err
}

func (d *SkillDAO) ListSystem(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Where("is_active = ? AND is_system = ?", true, true).
		Order("priority DESC").Find(&list).Error
	return list, err
}
