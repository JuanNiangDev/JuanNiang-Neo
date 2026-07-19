package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type ACLDAO struct{ db *gorm.DB }

func NewACLDAO(db *gorm.DB) *ACLDAO { return &ACLDAO{db: db} }

func (d *ACLDAO) Create(ctx context.Context, r *models.ACLRule) error {
	return d.db.WithContext(ctx).Create(r).Error
}

func (d *ACLDAO) Delete(ctx context.Context, id int64) error {
	return d.db.WithContext(ctx).Delete(&models.ACLRule{}, id).Error
}

func (d *ACLDAO) List(ctx context.Context) ([]models.ACLRule, error) {
	var list []models.ACLRule
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ACLDAO) GetByUserAndChatArea(ctx context.Context, userID int64, chatAreaID string) (*models.ACLRule, error) {
	var r models.ACLRule
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND chat_area_id = ?", userID, chatAreaID).
		First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}
