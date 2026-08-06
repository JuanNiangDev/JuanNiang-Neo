package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 表情包库（图床二次封装） ----------

// Sticker 表情包：引用图床图片（ImageID 长 UUID），附名称/简介/标签。
// ID 是短 UUID（发送时对外使用，底层映射到图床图片长 UUID）。
type Sticker struct {
	ID        string         `gorm:"primaryKey"`              // 短 UUID（发送时用）
	ImageID   string         `gorm:"not null;index"`          // 图床图片长 UUID
	Name      string         `gorm:"not null"`                // 名称
	Desc      string         `gorm:"type:text"`               // 简介（支持模糊匹配）
	Tags      JSONSlice      `gorm:"type:jsonb;default:'[]'"` // 标签名数组（冗余存储，便于 jsonb 查询）
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删
}

func (Sticker) TableName() string { return "stickers" }

// StickerTag 表情标签。
// Name 使用部分唯一索引（WHERE deleted_at IS NULL）：软删除的标签不占用名字，
// 删除后可重建同名标签，否则旧软删行会阻塞唯一索引（SQLSTATE 23505）。
type StickerTag struct {
	ID        string         `gorm:"primaryKey;type:uuid"`
	Name      string         `gorm:"not null;uniqueIndex:uk_sticker_tags_name,where:deleted_at IS NULL"` // 标签名
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删
}

func (StickerTag) TableName() string { return "sticker_tags" }
