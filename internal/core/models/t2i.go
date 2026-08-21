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
	// SelectedStyle 渲染风格选择：空/random = 随机，否则为风格名（来自 data/t2i_styles.json）。
	SelectedStyle string `json:"selected_style" gorm:"column:selected_style;type:varchar(64);default:'';comment:渲染风格选择(空=随机)"`
}

func (T2IConfig) TableName() string {
	return "t2i_configs"
}
