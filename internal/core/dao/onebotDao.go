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
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Onebot11Adapter{
		ID:    1,
		Addr:  "0.0.0.0",
		Port:  8081,
		Token: "wow-a-lovey-juan-niang",
	}).Error
}

func (d *Onebot11AdapterDao) GetAdapterConfig(ctx context.Context) (*models.Onebot11Adapter, error) {
	var data models.Onebot11Adapter

	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}

	return &data, nil
}

func (d *Onebot11AdapterDao) UpdateAdapterConfig(ctx context.Context, conf *models.Onebot11Adapter) error {
	return d.db.WithContext(ctx).Where("id = 1").Select("*").Updates(conf).Error
}

// AddAdminQQ 添加管理员 QQ 号，已存在则忽略。
func (d *Onebot11AdapterDao) AddAdminQQ(ctx context.Context, qq string) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conf models.Onebot11Adapter
		if err := tx.First(&conf).Error; err != nil {
			return err
		}
		for _, v := range conf.AdminQQNumbers {
			if v == qq {
				return nil
			}
		}
		conf.AdminQQNumbers = append(conf.AdminQQNumbers, qq)
		return tx.Where("id = 1").Update("admin_qq_numbers", conf.AdminQQNumbers).Error
	})
}

// RemoveAdminQQ 移除管理员 QQ 号。
func (d *Onebot11AdapterDao) RemoveAdminQQ(ctx context.Context, qq string) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conf models.Onebot11Adapter
		if err := tx.First(&conf).Error; err != nil {
			return err
		}
		list := make([]string, 0, len(conf.AdminQQNumbers))
		for _, v := range conf.AdminQQNumbers {
			if v != qq {
				list = append(list, v)
			}
		}
		if len(list) == len(conf.AdminQQNumbers) {
			return nil // 未找到，无需更新
		}
		return tx.Where("id = 1").Update("admin_qq_numbers", list).Error
	})
}
