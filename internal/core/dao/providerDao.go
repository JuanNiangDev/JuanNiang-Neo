package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProviderDAO struct{ db *gorm.DB }

func NewProviderDAO(db *gorm.DB) *ProviderDAO { return &ProviderDAO{db: db} }

// WithTx 返回绑定到指定事务的 DAO 副本，供 Service 层在事务内执行校验与写操作。
func (d *ProviderDAO) WithTx(tx *gorm.DB) *ProviderDAO { return &ProviderDAO{db: tx} }

// ListForUpdate 行级锁查询 Provider 列表（SELECT ... FOR UPDATE），
// 用于事务内校验"至少保留一个启用 Text 模型"不变量：并发写事务会阻塞在锁上，
// 提交后重查拿到最新数据，避免校验通过后写操作破坏不变量。
func (d *ProviderDAO) ListForUpdate(ctx context.Context, typ models.ModelType) ([]models.Provider, error) {
	var list []models.Provider
	q := d.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

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
