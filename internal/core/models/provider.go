package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- LLM Provider ----------

type ModelType string

const (
	ModelTypeText      ModelType = "text_model"
	ModelTypeImage     ModelType = "image_model"
	ModelTypeEmbedding ModelType = "embedding_model"
)

type Provider struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	Name        string    `gorm:"not null"`
	Type        ModelType `gorm:"not null;index"`
	Endpoint    string    `gorm:"not null"`
	Token       string    `gorm:"not null"`
	Model       string    `gorm:"not null"`
	Temperature float32   `gorm:"default:0.7"`
	// EnableThinking 模型思考开关：请求体携带 thinking/enable_thinking 扩展参数
	EnableThinking bool `gorm:"default:false"`
	IsActive       bool `gorm:"default:true;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (Provider) TableName() string {
	return "providers"
}
