package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Chat Record ----------

type ChatRecord struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	ChatAreaID string    `gorm:"not null;index"`
	ChatArea   ChatArea  `gorm:"foreignKey:ChatAreaID"`
	UserID     int64     `gorm:"not null;index"`
	Role       string    `gorm:"not null"`
	Content    string    `gorm:"type:text"`
	TokenCount int       `gorm:"default:0"`
	ToolCalls  JSONMap   `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time `gorm:"index"`
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (ChatRecord) TableName() string {
	return "chat_records"
}
