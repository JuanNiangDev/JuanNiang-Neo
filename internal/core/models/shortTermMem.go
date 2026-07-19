package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Memory ----------

type ShortTermMemory struct {
	ID          string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID  string   `gorm:"uniqueIndex;not null"`
	ChatArea    ChatArea `gorm:"foreignKey:ChatAreaID"`
	WindowSize  int      `gorm:"default:20"`
	AutoCompact bool     `gorm:"default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
