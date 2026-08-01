package logging

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/logger"
)

// GormLogger 将 GORM 内部日志适配到项目日志系统。
type GormLogger struct {
	*Logger
	SlowThreshold        time.Duration
	IgnoreRecordNotFound bool
	ParameterizedQueries bool
	LogLevel             logger.LogLevel
}

// NewGormLogger 创建 GORM 日志适配器。
func NewGormLogger() *GormLogger {
	return &GormLogger{
		Logger:               NewLogger("gorm"),
		SlowThreshold:        200 * time.Millisecond,
		IgnoreRecordNotFound: true,
		ParameterizedQueries: false,
		LogLevel:             logger.Warn,
	}
}

// SetLogLevel 设置 GORM 日志级别 (由 debug 模式控制)。
func (l *GormLogger) SetLogLevel(level logger.LogLevel) {
	l.LogLevel = level
}

// LogMode 实现 gorm.logger.Interface。
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	clone := *l
	clone.LogLevel = level
	return &clone
}

// Info 实现 gorm.logger.Interface。
func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Info {
		l.Logger.Info("gorm", "msg", msg, "data", fmt.Sprint(data...))
	}
}

// Warn 实现 gorm.logger.Interface。
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Warn {
		l.Logger.Warn("gorm", "msg", msg, "data", fmt.Sprint(data...))
	}
}

// Error 实现 gorm.logger.Interface。
func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Error {
		l.Logger.Error("gorm", "msg", msg, "data", fmt.Sprint(data...))
	}
}

// Trace 实现 gorm.logger.Interface。处理 SQL 执行追踪。
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && (!l.IgnoreRecordNotFound || err.Error() != "record not found"):
		l.Logger.Error("SQL 错误",
			"elapsed_ms", elapsed.Milliseconds(),
			"rows", rows,
			"sql", sql,
			"err", err,
		)
	case elapsed > l.SlowThreshold && l.SlowThreshold > 0:
		l.Logger.Warn("SQL 慢查询",
			"elapsed_ms", elapsed.Milliseconds(),
			"rows", rows,
			"sql", sql,
			"threshold_ms", l.SlowThreshold.Milliseconds(),
		)
	case l.LogLevel >= logger.Info:
		l.Logger.Debug("SQL",
			"elapsed_ms", elapsed.Milliseconds(),
			"rows", rows,
			"sql", sql,
		)
	}
}

// 确保实现 gorm.logger.Interface。
var _ logger.Interface = (*GormLogger)(nil)
