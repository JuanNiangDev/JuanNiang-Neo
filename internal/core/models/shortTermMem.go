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
	WindowSize  int      `gorm:"default:100"`
	AutoCompact bool     `gorm:"default:true"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (ShortTermMemory) TableName() string {
	return "short_term_memories"
}
