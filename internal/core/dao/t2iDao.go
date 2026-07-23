package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type T2IConfigDAO struct{ db *gorm.DB }

func NewT2IConfigDAO(db *gorm.DB) *T2IConfigDAO {
	return &T2IConfigDAO{db: db}
}

// InitConfig 初始化默认配置（已存在则忽略）。
func (d *T2IConfigDAO) InitConfig(ctx context.Context) error {
	return d.db.Create(&models.T2IConfig{
		ID:      1,
		BaseURL: "http://localhost:8999",
		Timeout: 30,
	}).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Error
}

// GetConfig 获取 T2I 配置。
func (d *T2IConfigDAO) GetConfig(ctx context.Context) (*models.T2IConfig, error) {
	var data models.T2IConfig
	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}
	return &data, nil
}

// UpdateConfig 更新 T2I 配置。
func (d *T2IConfigDAO) UpdateConfig(ctx context.Context, conf *models.T2IConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Select("*").Updates(conf).Error
}
