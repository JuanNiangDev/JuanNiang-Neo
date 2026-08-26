package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 长期记忆 ----------

type LongTermMemory struct {
	ID             string   `gorm:"primaryKey;type:uuid"`
	ChatAreaID     string   `gorm:"uniqueIndex;not null"`
	ChatArea       ChatArea `gorm:"foreignKey:ChatAreaID"`
	HotAreaSize    int      `gorm:"default:10"`
	HotMemoryTTL   int      `gorm:"default:86400"`
	GCIntervalDays int      `gorm:"default:7"` // 记忆 GC 周期（天）：周期内未被召回的条目会被清理
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (LongTermMemory) TableName() string {
	return "long_term_memories"
}

// ---------- 长期记忆条目 ----------

type LongTermMemoryItem struct {
	ID             string     `gorm:"primaryKey;type:uuid"`
	ChatAreaID     string     `gorm:"not null;index"`
	Content        string     `gorm:"type:text;not null"`
	Embedding      []byte     `gorm:"type:bytea"`
	Metadata       JSONMap    `gorm:"type:jsonb;default:'{}'"`
	LastRecalledAt *time.Time `gorm:"index"`                  // 最近一次被对话召回（GC 清理未使用记忆用；NULL=从未被召回）
	RAGSynced      bool       `gorm:"not null;default:false"` // 已同步到 RAG 向量库（GC 删除失败时置 false 供重试，防孤儿向量）
	CreatedAt      time.Time  `gorm:"index"`
}

func (LongTermMemoryItem) TableName() string {
	return "long_term_memory_items"
}
