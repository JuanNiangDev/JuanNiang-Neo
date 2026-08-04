package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 图床 ----------

// ImageAsset 图床图片元数据（二进制文件存 data/imgs/<id>.img，路径由 imgstore 管理）。
type ImageAsset struct {
	ID        string         `gorm:"primaryKey;type:uuid"`
	Name      string         `gorm:"not null"`                   // 展示名称（可编辑）
	FileName  string         `gorm:"not null"`                   // 磁盘文件名 <id>.img
	Folder    string         `gorm:"not null;default:'/';index"` // 虚拟文件夹路径（/ 或 /<name>）
	MimeType  string         `gorm:"not null"`                   // image/jpeg 等
	SizeBytes int64          `gorm:"not null"`                   // 字节数
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删
}

func (ImageAsset) TableName() string { return "image_assets" }

// ImageFolder 图床虚拟文件夹（仅一层，位于根 / 下，其下不能再建文件夹）。
// Name 使用部分唯一索引（WHERE deleted_at IS NULL）：软删除的文件夹不占用名字，
// 删除后可重建同名文件夹，否则旧软删行会阻塞唯一索引（SQLSTATE 23505）。
type ImageFolder struct {
	ID        string         `gorm:"primaryKey;type:uuid"`
	Name      string         `gorm:"not null;uniqueIndex:uk_image_folders_name,where:deleted_at IS NULL"` // 文件夹名（隐含路径 /<name>）
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (ImageFolder) TableName() string { return "image_folders" }
