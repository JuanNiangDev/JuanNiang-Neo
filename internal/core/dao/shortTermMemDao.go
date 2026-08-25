package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"
	"errors"

	"gorm.io/gorm"
)

type ShortTermMemoryDAO struct{ db *gorm.DB }

func NewShortTermMemoryDAO(db *gorm.DB) *ShortTermMemoryDAO { return &ShortTermMemoryDAO{db: db} }

func (d *ShortTermMemoryDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.ShortTermMemory, error) {
	var m models.ShortTermMemory
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	m = models.ShortTermMemory{
		ID:          newUUID(),
		ChatAreaID:  chatAreaID,
		WindowSize:  100,
		AutoCompact: true,
	}
	if err := d.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *ShortTermMemoryDAO) Update(ctx context.Context, m *models.ShortTermMemory) error {
	return d.db.WithContext(ctx).Save(m).Error
}

type LongTermMemoryDAO struct{ db *gorm.DB }

func NewLongTermMemoryDAO(db *gorm.DB) *LongTermMemoryDAO { return &LongTermMemoryDAO{db: db} }

func (d *LongTermMemoryDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.LongTermMemory, error) {
	var m models.LongTermMemory
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	m = models.LongTermMemory{
		ID:             newUUID(),
		ChatAreaID:     chatAreaID,
		HotAreaSize:    10,
		HotMemoryTTL:   86400,
		GCIntervalDays: 7,
	}
	if err := d.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *LongTermMemoryDAO) Update(ctx context.Context, m *models.LongTermMemory) error {
	return d.db.WithContext(ctx).Save(m).Error
}
