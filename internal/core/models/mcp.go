package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- MCP Server ----------

type MCPServer struct {
	ID            string    `gorm:"primaryKey;type:uuid"`
	Name          string    `gorm:"not null"`
	ServerURL     string    `gorm:"not null"`
	Headers       JSONMap   `gorm:"type:jsonb;default:'{}'"`
	Timeout       int       `gorm:"default:30000"`
	RetryCount    int       `gorm:"default:3"`
	ToolFilter    JSONSlice `gorm:"type:jsonb;default:'[]'"`
	AutoReconnect bool      `gorm:"default:true"`
	IsActive      bool      `gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}
