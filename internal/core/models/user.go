package models

import (
	"gorm.io/gorm"
)

// ---------- 管理员用户 ----------

// AdminUser 管理员账户。
// Username 使用部分唯一索引（WHERE deleted_at IS NULL）：软删除的管理员不占用用户名，
// 删除后可重建同名账户，否则旧软删行会阻塞唯一索引（SQLSTATE 23505）。
type AdminUser struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex:uk_admin_users_username,where:deleted_at IS NULL;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"default:admin"`
}

func (AdminUser) TableName() string {
	return "admin_users"
}
