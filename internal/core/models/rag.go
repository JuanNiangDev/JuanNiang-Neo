package models

import (
	"time"

	"gorm.io/gorm"
)

// RAGConfig RAG 向量检索服务配置（单行表，仿 T2IConfig）。
// 服务本体：JuanNiang-RAG-Service（Rust，tag↔向量，长文透明分块）。
type RAGConfig struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	BaseURL  string `json:"base_url" gorm:"column:base_url;type:varchar(512);not null;comment:RAG-Service 地址"`
	Timeout  int    `json:"timeout" gorm:"column:timeout;default:30;comment:超时(秒)"`
	IsActive bool   `json:"is_active" gorm:"column:is_active;type:boolean;default:false;comment:是否启用（未配置时记忆/知识检索自动降级）"`
}

func (RAGConfig) TableName() string {
	return "rag_configs"
}
