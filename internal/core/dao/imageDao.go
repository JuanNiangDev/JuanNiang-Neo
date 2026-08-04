package dao

import (
	"context"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// ImageDAO 图床图片 + 虚拟文件夹。
type ImageDAO struct{ db *gorm.DB }

func NewImageDAO(db *gorm.DB) *ImageDAO { return &ImageDAO{db: db} }

// ---------- 图片 ----------

func (d *ImageDAO) Create(ctx context.Context, img *models.ImageAsset) error {
	if img.ID == "" {
		img.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(img).Error
}

func (d *ImageDAO) GetByID(ctx context.Context, id string) (*models.ImageAsset, error) {
	var img models.ImageAsset
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&img).Error; err != nil {
		return nil, err
	}
	return &img, nil
}

// List 按虚拟文件夹分页列出（folder 为空或 "/" 表示根目录）。
func (d *ImageDAO) List(ctx context.Context, folder string, limit, offset int) ([]models.ImageAsset, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var list []models.ImageAsset
	q := d.db.WithContext(ctx).Where("folder = ?", normalizeFolder(folder))
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, err
}

// Count 统计某个虚拟文件夹下的图片数。
func (d *ImageDAO) Count(ctx context.Context, folder string) (int64, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&models.ImageAsset{}).
		Where("folder = ?", normalizeFolder(folder)).Count(&n).Error
	return n, err
}

func (d *ImageDAO) Update(ctx context.Context, img *models.ImageAsset) error {
	return d.db.WithContext(ctx).Save(img).Error
}

func (d *ImageDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ImageAsset{}).Error
}

// MoveFolderToRoot 把某个虚拟文件夹下的图片全部移到根目录（删除文件夹时调用）。
func (d *ImageDAO) MoveFolderToRoot(ctx context.Context, folder string) error {
	return d.db.WithContext(ctx).Model(&models.ImageAsset{}).
		Where("folder = ?", folder).
		Update("folder", "/").Error
}

// normalizeFolder 归一化虚拟文件夹路径：空值一律视为根 "/"。
func normalizeFolder(folder string) string {
	if folder == "" || folder == "/" {
		return "/"
	}
	return folder
}

// ---------- 虚拟文件夹 ----------

func (d *ImageDAO) FolderCreate(ctx context.Context, name string) (*models.ImageFolder, error) {
	f := &models.ImageFolder{ID: newUUID(), Name: name}
	if err := d.db.WithContext(ctx).Create(f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

func (d *ImageDAO) FolderList(ctx context.Context) ([]models.ImageFolder, error) {
	var list []models.ImageFolder
	err := d.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error
	return list, err
}

func (d *ImageDAO) FolderGetByID(ctx context.Context, id string) (*models.ImageFolder, error) {
	var f models.ImageFolder
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *ImageDAO) FolderDelete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ImageFolder{}).Error
}
