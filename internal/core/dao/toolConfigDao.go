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

// EnsureBuiltin 幂等创建内置工具配置行（按 name 唯一索引，已存在则忽略）。
// adminOnly 为该内置工具默认的"仅管理员"标志（仅首次创建时生效）。
func (d *ToolConfigDAO) EnsureBuiltin(ctx context.Context, name, desc string, adminOnly bool) error {
	var existing models.ToolConfig
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return d.Create(ctx, &models.ToolConfig{
		Name:        name,
		Description: desc,
		IsActive:    true,
		IsBuiltin:   true,
		AdminOnly:   adminOnly,
	})
}

// SetAdminOnly 更新指定工具的"仅管理员"标志。
func (d *ToolConfigDAO) SetAdminOnly(ctx context.Context, id string, adminOnly bool) error {
	return d.db.WithContext(ctx).Model(&models.ToolConfig{}).Where("id = ?", id).
		Update("admin_only", adminOnly).Error
}

// ListAdminOnly 返回所有启用"仅管理员"的工具名集合（供运行时权限校验）。
func (d *ToolConfigDAO) ListAdminOnly(ctx context.Context) (map[string]bool, error) {
	var list []models.ToolConfig
	if err := d.db.WithContext(ctx).Where("admin_only = ?", true).Find(&list).Error; err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, t := range list {
		out[t.Name] = true
	}
	return out, nil
}
