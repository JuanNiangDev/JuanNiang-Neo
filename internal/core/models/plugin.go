package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Plugin ----------

// Plugin 插件记录。
// Name 使用部分唯一索引（WHERE deleted_at IS NULL）：软删除的插件不占用名字，
// 删除后可重建同名插件，否则旧软删行会阻塞唯一索引（SQLSTATE 23505）。
type Plugin struct {
	ID        string  `gorm:"primaryKey;type:uuid"`
	Name      string  `gorm:"uniqueIndex:uk_plugins_name,where:deleted_at IS NULL;not null"`
	Version   string  `gorm:"default:'1.0.0'"`
	Path      string  `gorm:"not null"`
	Config    JSONMap `gorm:"type:jsonb;default:'{}'"`
	IsActive  bool    `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Plugin) TableName() string {
	return "plugins"
}
