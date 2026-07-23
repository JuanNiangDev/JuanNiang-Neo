package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SandboxConfigDAO struct{ db *gorm.DB }

func NewSandboxConfigDAO(db *gorm.DB) *SandboxConfigDAO {
	return &SandboxConfigDAO{db: db}
}

// InitConfig 初始化默认配置（已存在则忽略）。
func (d *SandboxConfigDAO) InitConfig(ctx context.Context) error {
	return d.db.Create(&models.SandboxConfig{
		ID:      1,
		BaseURL: "http://localhost:8080",
		Timeout: 30,
	}).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Error
}

// GetConfig 获取 Sandbox 配置。
func (d *SandboxConfigDAO) GetConfig(ctx context.Context) (*models.SandboxConfig, error) {
	var data models.SandboxConfig
	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}
	return &data, nil
}

// UpdateConfig 更新 Sandbox 配置。
// 使用 Select("*") 强制更新所有字段（包括 false 等零值），避免 GORM struct Updates 跳过零值字段。
func (d *SandboxConfigDAO) UpdateConfig(ctx context.Context, conf *models.SandboxConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Select("*").Updates(conf).Error
}
