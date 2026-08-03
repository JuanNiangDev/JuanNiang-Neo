package models

import "time"

// TokenUsageDaily 每日 Token 用量统计（按自然日聚合，跨全部会话）。
// Date 为主键（YYYY-MM-DD），TokenCount 为该日累计 Token 开销。
type TokenUsageDaily struct {
	Date       string    `gorm:"primaryKey" json:"date"`
	TokenCount int64     `gorm:"default:0;not null" json:"token_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
