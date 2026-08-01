package postgres

import (
	"fmt"
	"time"

	"JuanNiang-Neo/internal/logging"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
	return func(info *BasicInfo) { info.Host = host }
}
func WithPort(port string) Option {
	return func(info *BasicInfo) { info.Port = port }
}
func WithUser(user string) Option {
	return func(info *BasicInfo) { info.User = user }
}
func WithPassword(password string) Option {
	return func(info *BasicInfo) { info.Password = password }
}
func WithDefaultDB(defaultDB string) Option {
	return func(info *BasicInfo) { info.DefaultDB = defaultDB }
}
func WithSSlMode(sslMode string) Option {
	return func(info *BasicInfo) { info.SSLMode = sslMode }
}

func NewPostgresClient(opts ...Option) (*gorm.DB, error) {
	basicInfo := &BasicInfo{
		Host:      "localhost",
		Port:      "5432",
		User:      "postgres",
		Password:  "postgres",
		DefaultDB: "adminer",
		SSLMode:   "disable",
	}
	for _, opt := range opts {
		opt(basicInfo)
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		basicInfo.Host, basicInfo.Port, basicInfo.User, basicInfo.Password,
		basicInfo.DefaultDB, basicInfo.SSLMode,
	)

	// 注入自定义 GORM Logger
	gormLog := logging.NewGormLogger()
	gormLog.LogLevel = logger.Warn // 默认只记录 Warn/Error
	if basicInfo.DefaultDB == "test" {
		gormLog.LogLevel = logger.Info
	}

	db, err := gorm.Open(
		postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}),
		&gorm.Config{
			PrepareStmt: false,
			Logger:      gormLog,
		},
	)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(150)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(1 * time.Hour)
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	return db, nil
}

// SetDBLogLevel 运行时调整 GORM 日志级别（debug 模式调用）。
func SetDBLogLevel(db *gorm.DB, level logger.LogLevel) {
	if gl, ok := db.Logger.(*logging.GormLogger); ok {
		gl.SetLogLevel(level)
	}
}
