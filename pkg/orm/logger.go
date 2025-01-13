package orm

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"
)

type GormLogger struct {
	ZapLogger *zap.Logger
	Config    gormlogger.Config
}

func DisableGormLogger() gormlogger.Interface {
	return gormlogger.Discard
}

func NewGormLogger(slowThreshold time.Duration, logLeveal string) gormlogger.Interface {
	ll := gormlogger.Error
	switch logLeveal {
	case "info":
		ll = gormlogger.Info
	case "warn":
		ll = gormlogger.Warn
	case "error":
		ll = gormlogger.Error
	case "silent", "none":
		ll = gormlogger.Silent
	}

	return &GormLogger{
		ZapLogger: zap.L(),
		Config: gormlogger.Config{
			SlowThreshold:             slowThreshold,
			LogLevel:                  ll,
			IgnoreRecordNotFoundError: false,
			Colorful:                  true,
		},
	}
}

func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newlogger := *l
	newlogger.Config.LogLevel = level
	return &newlogger
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.Config.LogLevel >= gormlogger.Info {
		l.ZapLogger.Sugar().Infof(msg, data...)
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.Config.LogLevel >= gormlogger.Warn {
		l.ZapLogger.Sugar().Warnf(msg, data...)
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.Config.LogLevel >= gormlogger.Error {
		l.ZapLogger.Sugar().Errorf(msg, data...)
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.Config.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// Log error
	if err != nil && !errors.Is(err, gormlogger.ErrRecordNotFound) {
		l.ZapLogger.Error("gorm trace",
			zap.Error(err),
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
		)
		return
	}

	// Log slow queries
	if l.Config.SlowThreshold != 0 && elapsed > l.Config.SlowThreshold && l.Config.LogLevel >= gormlogger.Warn {
		l.ZapLogger.Warn("gorm slow sql",
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
		)
		return
	}

	// Log debug
	if l.Config.LogLevel >= gormlogger.Info {
		l.ZapLogger.Debug("gorm trace",
			zap.Duration("elapsed", elapsed),
			zap.String("sql", sql),
			zap.Int64("rows", rows),
		)
	}
}
