package models

import "time"

// SkillMemory 技能记忆：记录聊天中的黑话、网络热词、梗等。
// 全局共享（跨 ChatArea），每次 Compact 时由 LLM 自动更新。
// 表中只有一行记录（ID 固定为 "global"）。
type SkillMemory struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Content   string    `gorm:"type:text;not null;default:''" json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}
