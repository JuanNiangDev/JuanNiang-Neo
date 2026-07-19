package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type UserDAO struct{ db *gorm.DB }

func NewUserDAO(db *gorm.DB) *UserDAO { return &UserDAO{db: db} }

func (d *UserDAO) Create(ctx context.Context, u *models.AdminUser) error {
	return d.db.WithContext(ctx).Create(u).Error
}

func (d *UserDAO) GetByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	var u models.AdminUser
	err := d.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *UserDAO) UpdatePassword(ctx context.Context, id uint, hash string) error {
	return d.db.WithContext(ctx).Model(&models.AdminUser{}).Where("id = ?", id).
		Update("password_hash", hash).Error
}

func (d *UserDAO) Exists(ctx context.Context) (bool, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.AdminUser{}).Count(&count).Error
	return count > 0, err
}
