package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Onebot11AdapterDao struct{ db *gorm.DB }

func NewOnebot11AdapterDao(db *gorm.DB) *Onebot11AdapterDao {
	return &Onebot11AdapterDao{db: db}
}

func (d *Onebot11AdapterDao) InitAdapterConfig(ctx context.Context) error {
	return d.db.Create(&models.Onebot11Adapter{
		ID:    1,
		Addr:  "0.0.0.0",
		Port:  8081,
		Token: "wow-a-lovey-juan-niang",
	}).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Error
}

func (d *Onebot11AdapterDao) GetAdapterConfig(ctx context.Context) (*models.Onebot11Adapter, error) {
	var data models.Onebot11Adapter

	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}

	return &data, nil
}

func (d *Onebot11AdapterDao) UpdateAdapterConfig(ctx context.Context, conf *models.Onebot11Adapter) error {
	return d.db.WithContext(ctx).Where("id = 1").Updates(conf).Error
}
