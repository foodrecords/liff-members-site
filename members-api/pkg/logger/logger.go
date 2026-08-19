package logger

import (
	"fmt"
	"runtime"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func Setup() (func(), error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	l, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	logger = l
	return func() {
		logger.Sync()
	}, nil
}

func Info(format string, params ...interface{}) {
	pc, file, line, _ := runtime.Caller(1)
	message := fmt.Sprintf(format, params...)
	logger.Info(message,
		zap.String("log_id", uuid.New().String()),
		zap.String("call", runtime.FuncForPC(pc).Name()),
		zap.String("file", file),
		zap.Int("line", line),
	)
}

func Error(format string, params ...interface{}) {
	pc, file, line, _ := runtime.Caller(1)
	message := fmt.Sprintf(format, params...)
	logger.Error(message,
		zap.String("log_id", uuid.New().String()),
		zap.String("call", runtime.FuncForPC(pc).Name()),
		zap.String("file", file),
		zap.Int("line", line),
	)
}

func Fatal(format string, params ...interface{}) {
	pc, file, line, _ := runtime.Caller(1)
	message := fmt.Sprintf(format, params...)
	logger.Fatal(message,
		zap.String("log_id", uuid.New().String()),
		zap.String("call", runtime.FuncForPC(pc).Name()),
		zap.String("file", file),
		zap.Int("line", line),
	)
}
