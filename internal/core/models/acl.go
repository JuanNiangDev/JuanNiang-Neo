package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- ACL Rule ----------

type ACLPermission string

const (
	ACLPermissionAllowed ACLPermission = "allowed"
	ACLPermissionDenied  ACLPermission = "denied"
)

type ACLRule struct {
	ID         int64         `gorm:"primaryKey;autoIncrement"`
	UserID     int64         `gorm:"not null;index"`
	ChatAreaID string        `gorm:"not null;index"`
	Permission ACLPermission `gorm:"not null;default:allowed"`
	Actions    JSONSlice     `gorm:"type:jsonb;default:'[]'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
