package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

// GormLogger 将 GORM 的 SQL 日志路由到 logging 模块（module="db"）。
//
// 功能：
//   - SQL 语句自动格式化（长 SQL 多行显示）
//   - 慢查询（>500ms）输出 Warn 级别
//   - 错误查询输出 Error 级别（含调用栈）
//   - 正常查询 Debug 级别
//   - rows affected / elapsed 等元数据
type GormLogger struct {
	LogLevel      gormlogger.LogLevel
	SlowThreshold time.Duration
}

// NewGormLogger 创建 GORM 自定义日志器。
// level 为日志级别：1=Silent, 2=Error, 3=Warn, 4=Info（Debug 模式下传 4 显示全部 SQL）。
func NewGormLogger(level gormlogger.LogLevel) *GormLogger {
	return &GormLogger{
		LogLevel:      level,
		SlowThreshold: 500 * time.Millisecond,
	}
}

// LogMode 返回设置了新日志级别的 Logger 实例。
func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &GormLogger{
		LogLevel:      level,
		SlowThreshold: l.SlowThreshold,
	}
}

// Info 处理 GORM 的 Info 级别日志（连接信息、迁移提示等）。
func (l *GormLogger) Info(_ context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Info {
		log.Info(fmt.Sprintf(msg, data...))
	}
}

// Warn 处理 GORM 的 Warn 级别日志。
func (l *GormLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Warn {
		log.Warn(fmt.Sprintf(msg, data...))
	}
}

// Error 处理 GORM 的 Error 级别日志。
func (l *GormLogger) Error(_ context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Error {
		log.Error(fmt.Sprintf(msg, data...))
	}
}

// Trace 记录 SQL 执行追踪。
//
// 根据执行结果选择日志级别：
//   - 有错误 → Error（含调用栈）
//   - 慢查询 → Warn
//   - 正常   → Debug
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// 格式化 SQL（去除多余空白、长 SQL 换行展示）
	sql = formatSQL(sql)

	switch {
	case err != nil && l.LogLevel >= gormlogger.Error:
		// 忽略 ErrRecordNotFound（GORM 在 Find 无结果时返回，属正常行为）
		if errors.Is(err, gormlogger.ErrRecordNotFound) {
			if l.LogLevel >= gormlogger.Info {
				log.Info("SQL", "elapsed", elapsed.String(), "rows", rows, "sql", sql)
			}
			return
		}
		log.Error("SQL 执行失败",
			"err", err.Error(),
			"elapsed", elapsed.String(),
			"rows", rows,
			"sql", sql,
		)

	case l.SlowThreshold > 0 && elapsed > l.SlowThreshold && l.LogLevel >= gormlogger.Warn:
		log.Warn("SQL 慢查询",
			"elapsed", elapsed.String(),
			"threshold", l.SlowThreshold.String(),
			"rows", rows,
			"sql", sql,
		)

	case l.LogLevel >= gormlogger.Info:
		log.Debug("SQL",
			"elapsed", elapsed.String(),
			"rows", rows,
			"sql", sql,
		)
	}
}

// formatSQL 清理 SQL 语句：去除多余空白，长 SQL 自动换行。
func formatSQL(sql string) string {
	// 去除首尾空白
	sql = strings.TrimSpace(sql)
	// 将连续空白压缩为单个空格
	fields := strings.Fields(sql)
	sql = strings.Join(fields, " ")
	// 超过 200 字符时尝试按关键字换行
	if len(sql) > 200 {
		for _, kw := range []string{"SELECT", "FROM", "WHERE", "AND", "OR", "ORDER BY", "GROUP BY", "LIMIT", "INSERT INTO", "UPDATE", "SET", "DELETE FROM", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "ON"} {
			sql = strings.ReplaceAll(sql, " "+kw, "\n  "+kw)
		}
	}
	return sql
}
