package models

import (
	"time"

	"gorm.io/gorm"
)

// SandboxConfig 沙箱服务配置（单行表）。
type SandboxConfig struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	BaseURL  string `json:"base_url" gorm:"column:base_url;type:varchar(512);not null;comment:沙箱服务地址"`
	APIKey   string `json:"api_key" gorm:"column:api_key;type:varchar(512);comment:API 密钥"`
	Timeout  int    `json:"timeout" gorm:"column:timeout;default:30;comment:超时(秒)"`
	IsActive bool   `json:"is_active" gorm:"column:is_active;type:boolean;default:true;comment:是否启用"`
}

func (SandboxConfig) TableName() string {
	return "sandbox_configs"
}
