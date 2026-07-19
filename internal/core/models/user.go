package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 管理员用户 ----------

type AdminUser struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"default:admin"`
}

// ---------- 管理员 QQ ----------

type AdminQQ struct {
	ID        int64 `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
