package models

import (
	"gorm.io/gorm"
)

// ---------- 管理员用户 ----------

type AdminUser struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"default:admin"`
}

func (AdminUser) TableName() string {
	return "admin_users"
}
