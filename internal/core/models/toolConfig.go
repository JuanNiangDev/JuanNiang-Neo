package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Tool Config ----------

type ToolConfig struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	Name        string  `gorm:"uniqueIndex;not null"`
	Description string  `gorm:"not null"`
	Parameters  JSONMap `gorm:"type:jsonb;default:'{}'"`
	Timeout     int     `gorm:"default:30000"`
	IsActive    bool    `gorm:"default:true"`
	IsBuiltin   bool    `gorm:"default:false"`
	// AdminOnly 仅管理员可调用：开启后该工具只能由 Admins 列表内的用户触发（防提示词注入）
	AdminOnly bool `gorm:"default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (ToolConfig) TableName() string {
	return "tool_configs"
}
