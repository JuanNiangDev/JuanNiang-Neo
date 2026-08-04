package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- SQL 驱动知识库 ----------

// 关键词提取状态。
const (
	KeywordStatusPending = "pending" // 提取中
	KeywordStatusReady   = "ready"   // 已完成，参与对话匹配
	KeywordStatusFailed  = "failed"  // 提取失败（可手动重试）
)

// KnowledgeItem 知识库条目。
// 存入时由 Agent 异步提取 Keywords（关键词），对话前基于关键词/内容做模糊匹配，
// 命中结果注入系统提示词。
type KnowledgeItem struct {
	ID            string         `gorm:"primaryKey;type:uuid"`
	Title         string         `gorm:"not null"`                // 标题（Web 展示用）
	Content       string         `gorm:"type:text;not null"`      // 知识内容
	Keywords      JSONSlice      `gorm:"type:jsonb;default:'[]'"` // Agent 异步提取的关键词
	KeywordStatus string         `gorm:"default:'pending'"`       // pending / ready / failed
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index"` // 软删

	// HitKeywords 仅本次匹配实际命中的关键词（Match SQL 里 json_agg 填充，用于命中统计），非持久字段。
	HitKeywords JSONSlice `gorm:"-" json:"hit_keywords,omitempty"`
}

func (KnowledgeItem) TableName() string { return "knowledge_items" }

// KeywordCount 词云行：关键词 + 出现次数（按知识条目计数）。
type KeywordCount struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count"`
}

// KeywordHit 关键词命中统计（对话匹配命中次数，Web 命中排行数据源）。
type KeywordHit struct {
	Keyword   string    `gorm:"primaryKey"`
	HitCount  int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KeywordHit) TableName() string { return "keyword_hits" }
