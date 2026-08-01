package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Skill ----------

type Skill struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Name         string `gorm:"not null"`
	Description  string
	Keywords     JSONSlice `gorm:"type:jsonb;default:'[]'"`
	RegexPattern string
	PromptRefs   JSONSlice `gorm:"type:jsonb;default:'[]'"`
	ToolRefs     JSONSlice `gorm:"type:jsonb;default:'[]'"`
	McpRefs      JSONSlice `gorm:"type:jsonb;default:'[]'"`
	IsActive     bool      `gorm:"default:true"`
	IsSystem     bool      `gorm:"default:false"`
	Priority     int       `gorm:"default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (Skill) TableName() string {
	return "skills"
}
