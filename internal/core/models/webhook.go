package models

import (
	"time"

	"gorm.io/gorm"
)

// WebhookConfig Webhook 适配器配置（单行表）。
type WebhookConfig struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Addr    string `gorm:"column:addr;type:varchar(255);not null;default:0.0.0.0;comment:监听地址"`
	Port    int    `gorm:"column:port;not null;default:8091;comment:监听端口"`
	Token   string `gorm:"column:token;type:varchar(255);comment:访问令牌"`
	Enabled bool   `gorm:"column:enabled;type:boolean;default:false;comment:是否启用"`
}

func (WebhookConfig) TableName() string {
	return "webhook_configs"
}
