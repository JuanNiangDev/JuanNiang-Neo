package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 长期记忆 ----------

type LongTermMemory struct {
	ID           string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID   string   `gorm:"uniqueIndex;not null"`
	ChatArea     ChatArea `gorm:"foreignKey:ChatAreaID"`
	HotAreaSize  int      `gorm:"default:10"`
	HotMemoryTTL int      `gorm:"default:86400"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// ---------- 长期记忆条目 ----------

type LongTermMemoryItem struct {
	ID         string    `gorm:"primaryKey;type:uuid"`
	ChatAreaID string    `gorm:"not null;index"`
	Content    string    `gorm:"type:text;not null"`
	Embedding  []byte    `gorm:"type:bytea"`
	Metadata   JSONMap   `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time `gorm:"index"`
}
