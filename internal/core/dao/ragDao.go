package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RAGConfigDAO struct{ db *gorm.DB }

func NewRAGConfigDAO(db *gorm.DB) *RAGConfigDAO {
	return &RAGConfigDAO{db: db}
}

// InitConfig 初始化默认配置（已存在则忽略）。默认未启用（IsActive=false），
// 保证未配置时所有调用方走降级路径（记忆/知识召回不依赖 RAG）。
func (d *RAGConfigDAO) InitConfig(ctx context.Context) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.RAGConfig{
		ID:       1,
		BaseURL:  "http://localhost:3000",
		Timeout:  30,
		IsActive: false,
	}).Error
}

// GetConfig 获取 RAG 配置。
func (d *RAGConfigDAO) GetConfig(ctx context.Context) (*models.RAGConfig, error) {
	var data models.RAGConfig
	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}
	return &data, nil
}

// UpdateConfig 更新 RAG 配置。
func (d *RAGConfigDAO) UpdateConfig(ctx context.Context, conf *models.RAGConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Select("*").Updates(conf).Error
}
