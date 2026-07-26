package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Session ----------

type Session struct {
	ID         string   `json:"id" gorm:"primaryKey;type:uuid"`
	ChatAreaID string   `json:"chat_area_id" gorm:"not null;index"`
	ChatArea   ChatArea `gorm:"foreignKey:ChatAreaID"`
	Model      string   `json:"model" gorm:"default:''"`
	TokenUsage int64    `json:"token_usage" gorm:"default:0"`
	MetaData   JSONMap  `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (Session) TableName() string {
	return "sessions"
}
