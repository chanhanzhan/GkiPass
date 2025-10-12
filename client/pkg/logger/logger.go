package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var globalLogger *zap.Logger

// Initialize 初始化全局日志系统
func Initialize(level string) error {
	// 解析日志级别
	logLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("无效的日志级别 '%s': %w", level, err)
	}

	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 彩色级别
		EncodeTime:     zapcore.ISO8601TimeEncoder,       // ISO8601时间格式
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建控制台编码器
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 创建核心
	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		logLevel,
	)

	// 创建logger
	globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	// 设置为全局logger
	zap.ReplaceGlobals(globalLogger)

	return nil
}

// Sync 刷新日志缓冲区
func Sync() {
	if globalLogger != nil {
		globalLogger.Sync()
	}
}

// Debug 记录调试信息
func Debug(msg string, fields ...zap.Field) {
	zap.L().Debug(msg, fields...)
}

// Info 记录信息
func Info(msg string, fields ...zap.Field) {
	zap.L().Info(msg, fields...)
}

// Warn 记录警告
func Warn(msg string, fields ...zap.Field) {
	zap.L().Warn(msg, fields...)
}

// Error 记录错误
func Error(msg string, fields ...zap.Field) {
	zap.L().Error(msg, fields...)
}

// Fatal 记录致命错误并退出
func Fatal(msg string, fields ...zap.Field) {
	zap.L().Fatal(msg, fields...)
}

// With 创建带字段的子logger
func With(fields ...zap.Field) *zap.Logger {
	return zap.L().With(fields...)
}

// Named 创建命名子logger
func Named(name string) *zap.Logger {
	return zap.L().Named(name)
}





