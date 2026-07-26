package models

import (
	"time"

	"gorm.io/gorm"
)

// T2IConfig 文生图服务配置（单行表）。
type T2IConfig struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	BaseURL  string `json:"base_url" gorm:"column:base_url;type:varchar(512);not null;comment:T2I 服务地址"`
	Timeout  int    `json:"timeout" gorm:"column:timeout;default:30;comment:超时(秒)"`
	IsActive bool   `json:"is_active" gorm:"column:is_active;type:boolean;default:true;comment:是否启用"`
}

func (T2IConfig) TableName() string {
	return "t2i_configs"
}
