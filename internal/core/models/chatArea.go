package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Chat Area ----------

type AreaType string

const (
	AreaTypePrivate AreaType = "private"
	AreaTypeGroup   AreaType = "group"
)

type ChatArea struct {
	ID        string   `gorm:"primaryKey;type:uuid"`
	AreaType  AreaType `gorm:"not null;index"`
	TargetID  int64    `gorm:"not null;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (ChatArea) TableName() string {
	return "chat_areas"
}
