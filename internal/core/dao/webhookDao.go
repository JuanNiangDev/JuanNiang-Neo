package dao

import (
	"context"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebhookConfigDAO struct{ db *gorm.DB }

func NewWebhookConfigDAO(db *gorm.DB) *WebhookConfigDAO {
	return &WebhookConfigDAO{db: db}
}

// InitConfig 初始化默认配置（已存在则忽略）。
func (d *WebhookConfigDAO) InitConfig(ctx context.Context) error {
	return d.db.Create(&models.WebhookConfig{
		ID:      1,
		Addr:    "0.0.0.0",
		Port:    8091,
		Enabled: false,
	}).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Error
}

// GetConfig 获取 Webhook 配置。
func (d *WebhookConfigDAO) GetConfig(ctx context.Context) (*models.WebhookConfig, error) {
	var data models.WebhookConfig
	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}
	return &data, nil
}

// UpdateConfig 更新 Webhook 配置。
func (d *WebhookConfigDAO) UpdateConfig(ctx context.Context, conf *models.WebhookConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Select("*").Updates(conf).Error
}
