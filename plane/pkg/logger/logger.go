package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

// Config 日志配置
type Config struct {
	Level      string // debug, info, warn, error
	Format     string // json, console
	OutputPath string // 日志文件路径
	MaxSize    int    // 单个日志文件大小(MB)
	MaxBackups int    // 保留的旧日志文件数量
	MaxAge     int    // 保留天数
	Compress   bool   // 是否压缩
}

// Init 初始化日志系统
func Init(cfg *Config) error {
	// 默认配置
	if cfg.Level == "" {
		cfg.Level = "info"
	}
	if cfg.Format == "" {
		cfg.Format = "console"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100 // 100MB
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 10
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 30 // 30天
	}

	// 日志级别
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Encoder 配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// JSON 格式的 encoder 配置
	jsonEncoderConfig := encoderConfig
	jsonEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// 创建 encoder
	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(jsonEncoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 输出配置
	var writeSyncer zapcore.WriteSyncer

	if cfg.OutputPath != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(cfg.OutputPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}

		// 文件输出（带轮转）
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.OutputPath,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		// 同时输出到文件和控制台
		writeSyncer = zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(fileWriter),
			zapcore.AddSync(os.Stdout),
		)
	} else {
		// 仅输出到控制台
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 创建 Core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 创建 Logger
	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()

	return nil
}

// Sync 刷新日志缓冲
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// Debug 日志
func Debug(msg string, fields ...zap.Field) {
	Logger.Debug(msg, fields...)
}

// Info 日志
func Info(msg string, fields ...zap.Field) {
	Logger.Info(msg, fields...)
}

// Warn 日志
func Warn(msg string, fields ...zap.Field) {
	Logger.Warn(msg, fields...)
}

// Error 日志
func Error(msg string, fields ...zap.Field) {
	Logger.Error(msg, fields...)
}

// Fatal 日志
func Fatal(msg string, fields ...zap.Field) {
	Logger.Fatal(msg, fields...)
}

// Debugf 格式化日志
func Debugf(template string, args ...interface{}) {
	Sugar.Debugf(template, args...)
}

// Infof 格式化日志
func Infof(template string, args ...interface{}) {
	Sugar.Infof(template, args...)
}

// Warnf 格式化日志
func Warnf(template string, args ...interface{}) {
	Sugar.Warnf(template, args...)
}

// Errorf 格式化日志
func Errorf(template string, args ...interface{}) {
	Sugar.Errorf(template, args...)
}

// Fatalf 格式化日志
func Fatalf(template string, args ...interface{}) {
	Sugar.Fatalf(template, args...)
}

// With 添加字段
func With(fields ...zap.Field) *zap.Logger {
	return Logger.With(fields...)
}

