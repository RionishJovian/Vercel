package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger to provide a clean interface
type Logger struct {
	*zap.Logger
}

// New creates a new structured logger
func New(service, environment string) *Logger {
	var cfg zap.Config

	if environment == "production" {
		cfg = zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Add service field to all logs
	cfg.InitialFields = map[string]interface{}{
		"service": service,
	}

	logger, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}

	return &Logger{Logger: logger}
}

// NewNoOp creates a no-op logger for testing
func NewNoOp() *Logger {
	return &Logger{Logger: zap.NewNop()}
}

// Sugar returns a sugared logger for convenient key-value logging
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.Logger.Sugar()
}