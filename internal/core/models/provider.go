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
	IsActive    bool      `gorm:"default:true;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
