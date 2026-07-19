package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type ProviderDAO struct{ db *gorm.DB }

func NewProviderDAO(db *gorm.DB) *ProviderDAO { return &ProviderDAO{db: db} }

func (d *ProviderDAO) Create(ctx context.Context, p *models.Provider) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *ProviderDAO) GetByID(ctx context.Context, id string) (*models.Provider, error) {
	var p models.Provider
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *ProviderDAO) Update(ctx context.Context, p *models.Provider) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *ProviderDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Provider{}).Error
}

func (d *ProviderDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Provider{}).Where("id = ?", id).
		Update("is_active", active).Error
}

// DeactivateByType 将指定类型的所有 Provider 设为非激活，排除 exceptID。
func (d *ProviderDAO) DeactivateByType(ctx context.Context, typ models.ModelType, exceptID string) error {
	return d.db.WithContext(ctx).Model(&models.Provider{}).
		Where("type = ? AND id != ?", typ, exceptID).
		Update("is_active", false).Error
}

func (d *ProviderDAO) List(ctx context.Context, typ models.ModelType) ([]models.Provider, error) {
	var list []models.Provider
	q := d.db.WithContext(ctx)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ProviderDAO) ListActive(ctx context.Context, typ models.ModelType) ([]models.Provider, error) {
	var list []models.Provider
	q := d.db.WithContext(ctx).Where("is_active = ?", true)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}
