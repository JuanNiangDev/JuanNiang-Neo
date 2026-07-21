package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type BackgroundTaskDAO struct{ db *gorm.DB }

func NewBackgroundTaskDAO(db *gorm.DB) *BackgroundTaskDAO { return &BackgroundTaskDAO{db: db} }

func (d *BackgroundTaskDAO) Create(ctx context.Context, t *models.BackgroundTask) error {
	if t.ID == "" {
		t.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(t).Error
}

func (d *BackgroundTaskDAO) GetByID(ctx context.Context, id string) (*models.BackgroundTask, error) {
	var t models.BackgroundTask
	err := d.db.WithContext(ctx).First(&t, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *BackgroundTaskDAO) Update(ctx context.Context, t *models.BackgroundTask) error {
	return d.db.WithContext(ctx).Model(&models.BackgroundTask{}).Where("id = ?", t.ID).
		Select("status", "results").Updates(t).Error
}

func (d *BackgroundTaskDAO) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	return d.db.WithContext(ctx).Model(&models.BackgroundTask{}).Where("id = ?", id).
		Update("status", status).Error
}

func (d *BackgroundTaskDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.BackgroundTask{}).Error
}

func (d *BackgroundTaskDAO) ListByChatArea(ctx context.Context, chatAreaID string) ([]models.BackgroundTask, error) {
	var list []models.BackgroundTask
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").Find(&list).Error
	return list, err
}
