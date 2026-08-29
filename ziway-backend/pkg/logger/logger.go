package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 创建日志实例
func New(env string) *zap.Logger {
	var cfg zap.Config
	if env == "prod" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	log, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		// fallback
		return zap.NewExample()
	}
	return log
}

// NewWithFile 同时输出到文件
func NewWithFile(env, filePath string) *zap.Logger {
	log := New(env)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return log
	}
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewTee(
		log.Core(),
		zapcore.NewCore(encoder, zapcore.AddSync(file), zap.InfoLevel),
	)
	return zap.New(core)
}
