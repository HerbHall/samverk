package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a logger based on SAMVERK_ENV.
//   - "development" or "dev": colorized console output, debug level
//   - anything else (including unset): structured JSON, info level
func New() (*zap.Logger, error) {
	env := os.Getenv("SAMVERK_ENV")

	switch env {
	case "development", "dev":
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
		return cfg.Build()
	default:
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg.Build()
	}
}
