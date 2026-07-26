package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- MCP Server ----------

type MCPServer struct {
	ID            string    `json:"id" gorm:"primaryKey;type:uuid"`
	Name          string    `json:"name" gorm:"not null"`
	ServerURL     string    `json:"server_url" gorm:"not null"`
	Headers       JSONMap   `json:"headers" gorm:"type:jsonb;default:'{}'"`
	Timeout       int       `json:"timeout" gorm:"default:30000"`
	RetryCount    int       `json:"retry_count" gorm:"default:3"`
	ToolFilter    JSONSlice `json:"tool_filter" gorm:"type:jsonb;default:'[]'"`
	AutoReconnect bool      `json:"auto_reconnect" gorm:"default:true"`
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (MCPServer) TableName() string {
	return "mcp_servers"
}
