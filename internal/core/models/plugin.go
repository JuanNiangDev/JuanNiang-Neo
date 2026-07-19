package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Plugin ----------

type Plugin struct {
	ID        string  `gorm:"primaryKey;type:uuid"`
	Name      string  `gorm:"uniqueIndex;not null"`
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
