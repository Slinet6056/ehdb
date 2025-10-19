package logger

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a new zap logger with pretty console output
func New(levelStr string) (*zap.Logger, error) {
	// Parse log level
	level := parseLevel(levelStr)

	// Always use pretty console output with colors
	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    NewConsoleEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return config.Build()
}

// parseLevel converts string to zapcore.Level
func parseLevel(levelStr string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn", "warning":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "fatal":
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}

// NewConsoleEncoderConfig returns a human-friendly encoder config with colors
func NewConsoleEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		CallerKey:      zapcore.OmitKey,
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006/01/02 - 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}
