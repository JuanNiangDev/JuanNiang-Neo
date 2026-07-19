package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Session ----------

type Session struct {
	ID         string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID string   `gorm:"not null;index"`
	ChatArea   ChatArea `gorm:"foreignKey:ChatAreaID"`
	Model      string   `gorm:"default:''"`
	TokenUsage int64    `gorm:"default:0"`
	MetaData   JSONMap  `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (Session) TableName() string {
	return "sessions"
}
