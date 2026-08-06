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
	ID       string     `gorm:"primaryKey;type:uuid"`
	Name     string     `gorm:"not null"`
	Content  string     `gorm:"type:text;not null"`
	Type     PromptType `gorm:"not null;index"`
	IsActive bool       `gorm:"default:true"`
	// IsSystem=true 表示系统内置锁定提示词：每次构建 SystemPrompt 时强制拼接，
	// 不允许通过 API 修改或删除（仅允许查看）。AutoMigrate 会自动添加该列。
	IsSystem  bool `gorm:"default:false;comment:系统锁定提示词"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Prompt) TableName() string {
	return "prompts"
}
