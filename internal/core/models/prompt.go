package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Prompt ----------

type PromptType string

const (
	PromptTypeSystem      PromptType = "system"
	PromptTypePersonality PromptType = "personality"
	PromptTypeCustom      PromptType = "custom"
)

type Prompt struct {
	ID        string     `gorm:"primaryKey;type:uuid"`
	Name      string     `gorm:"not null"`
	Content   string     `gorm:"type:text;not null"`
	Type      PromptType `gorm:"not null;index"`
	IsActive  bool       `gorm:"default:true"`
	Variables JSONSlice  `gorm:"type:jsonb;default:'[]'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Prompt) TableName() string {
	return "prompts"
}
