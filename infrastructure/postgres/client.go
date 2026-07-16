package postgres

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Option func(info *BasicInfo)

type BasicInfo struct {
	Host      string
	Port      string
	User      string
	Password  string
	DefaultDB string
	SSLMode   string
}

func WithHost(host string) Option {
	return func(info *BasicInfo) {
		info.Host = host
	}
}

func WithPort(port string) Option {
	return func(info *BasicInfo) {
		info.Port = port
	}
}

func WithUser(user string) Option {
	return func(info *BasicInfo) {
		info.User = user
	}
}

func WithPassword(password string) Option {
	return func(info *BasicInfo) {
		info.Password = password
	}
}

func WithDefaultDB(defaultDB string) Option {
	return func(info *BasicInfo) {
		info.DefaultDB = defaultDB
	}
}

func WithSSlMode(sslMode string) Option {
	return func(info *BasicInfo) {
		info.SSLMode = sslMode
	}
}

func NewPostgresClient(opts ...Option) (*gorm.DB, error) {
	// 生成基础数据
	basicInfo := &BasicInfo{
		Host:      "localhost",
		Port:      "5432",
		User:      "postgres",
		Password:  "postgres",
		DefaultDB: "adminer",
		SSLMode:   "disable",
	}
	// 编译应用选项
	for _, opt := range opts {
		opt(basicInfo)
	}
	// 创建dsn
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		basicInfo.Host,
		basicInfo.Port,
		basicInfo.User,
		basicInfo.Password,
		basicInfo.DefaultDB,
		basicInfo.SSLMode,
	)
	// 获取连接
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{PrepareStmt: false})
	if err != nil {
		return nil, err
	}
	// 调整连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(150)                 // 限制对pgpool的总连接数
	sqlDB.SetMaxIdleConns(10)                  // 保持一定数量的空闲连接以备重用
	sqlDB.SetConnMaxLifetime(1 * time.Hour)    // 连接最大存活时间
	sqlDB.SetConnMaxIdleTime(15 * time.Minute) // 空闲连接最大存活时间

	return db, nil
}
