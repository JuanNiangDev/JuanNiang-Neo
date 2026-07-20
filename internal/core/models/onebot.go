package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Onebot11 Adapter ----------

type Onebot11Adapter struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Addr           string   `gorm:"column:addr;type:varchar(255);not null;comment:连接地址"`
	Port           int      `gorm:"column:port;not null;comment:连接端口"`
	Token          string   `gorm:"column:token;type:varchar(255);comment:访问令牌"`
	AdminQQNumbers []string `gorm:"column:admin_qq_numbers;type:json;serializer:json;comment:管理员QQ号列表"`
	Enabled        bool     `gorm:"column:enabled;type:boolean;default:true;comment:是否启用"`
}

func (Onebot11Adapter) TableName() string {
	return "onebot11_adapters"
}
