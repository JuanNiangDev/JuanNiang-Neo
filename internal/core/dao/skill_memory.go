package dao

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

type SkillMemoryDAO struct{ db *gorm.DB }

func NewSkillMemoryDAO(db *gorm.DB) *SkillMemoryDAO {
	return &SkillMemoryDAO{db: db}
}

const skillMemoryID = "global"

// GetOrCreate 获取全局技能记忆，不存在则创建空记录。
func (d *SkillMemoryDAO) GetOrCreate(ctx context.Context) (*models.SkillMemory, error) {
	var mem models.SkillMemory
	err := d.db.WithContext(ctx).Where("id = ?", skillMemoryID).First(&mem).Error
	if err == gorm.ErrRecordNotFound {
		mem = models.SkillMemory{ID: skillMemoryID, Content: "", UpdatedAt: time.Now()}
		if err := d.db.WithContext(ctx).Create(&mem).Error; err != nil {
			return nil, err
		}
		return &mem, nil
	}
	return &mem, err
}

// Upsert 使用 Postgres ON CONFLICT 原子更新技能记忆内容。
func (d *SkillMemoryDAO) Upsert(ctx context.Context, content string) error {
	return d.db.WithContext(ctx).
		Exec(`INSERT INTO skill_memories (id, content, updated_at) VALUES (?, ?, ?)
		      ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at`,
			skillMemoryID, content, time.Now()).Error
}
