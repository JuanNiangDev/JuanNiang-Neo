package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"
	"errors"

	"gorm.io/gorm"
)

type SessionDAO struct{ db *gorm.DB }

func NewSessionDAO(db *gorm.DB) *SessionDAO { return &SessionDAO{db: db} }

func (d *SessionDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.Session, error) {
	var s models.Session
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&s).Error
	if err == nil {
		return &s, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	s = models.Session{
		ID:         newUUID(),
		ChatAreaID: chatAreaID,
	}
	if err := d.db.WithContext(ctx).Create(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SessionDAO) GetByID(ctx context.Context, id string) (*models.Session, error) {
	var s models.Session
	err := d.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SessionDAO) Update(ctx context.Context, s *models.Session) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *SessionDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Session{}).Error
}

func (d *SessionDAO) AddTokenUsage(ctx context.Context, id string, tokens int64) error {
	return d.db.WithContext(ctx).Model(&models.Session{}).Where("id = ?", id).
		Update("token_usage", gorm.Expr("token_usage + ?", tokens)).Error
}

func (d *SessionDAO) List(ctx context.Context) ([]models.Session, error) {
	var list []models.Session
	err := d.db.WithContext(ctx).Preload("ChatArea").Order("updated_at DESC").Find(&list).Error
	return list, err
}
