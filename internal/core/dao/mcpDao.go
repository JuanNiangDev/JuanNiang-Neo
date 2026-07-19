package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type MCPServerDAO struct{ db *gorm.DB }

func NewMCPServerDAO(db *gorm.DB) *MCPServerDAO { return &MCPServerDAO{db: db} }

func (d *MCPServerDAO) Create(ctx context.Context, m *models.MCPServer) error {
	if m.ID == "" {
		m.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(m).Error
}

func (d *MCPServerDAO) GetByID(ctx context.Context, id string) (*models.MCPServer, error) {
	var m models.MCPServer
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *MCPServerDAO) Update(ctx context.Context, m *models.MCPServer) error {
	return d.db.WithContext(ctx).Save(m).Error
}

func (d *MCPServerDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.MCPServer{}).Error
}

func (d *MCPServerDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.MCPServer{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *MCPServerDAO) List(ctx context.Context) ([]models.MCPServer, error) {
	var list []models.MCPServer
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *MCPServerDAO) ListActive(ctx context.Context) ([]models.MCPServer, error) {
	var list []models.MCPServer
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("created_at DESC").Find(&list).Error
	return list, err
}
