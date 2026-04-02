package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"samverk.dev/samverk/internal/logstore"
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

// NewWithTee creates a logger that writes to both stdout and a SQLite
// log store. In production mode, a single JSON core tees through the
// logstore syncer. In dev mode, a console core writes to stdout and a
// separate JSON core writes to the logstore syncer.
//
// The returned *logstore.Syncer can be used to attach a WebSocket
// broadcaster via SetBroadcaster after the server is constructed.
func NewWithTee(ls *logstore.LogStore) (*zap.Logger, *logstore.Syncer, error) {
	env := os.Getenv("SAMVERK_ENV")

	switch env {
	case "development", "dev":
		// Console core for stdout (colorized).
		devCfg := zap.NewDevelopmentConfig()
		devCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		devCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
		consoleEnc := zapcore.NewConsoleEncoder(devCfg.EncoderConfig)
		stdoutCore := zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), zapcore.DebugLevel)

		// JSON core for SQLite (structured, via syncer that writes to /dev/null for stdout
		// since the console core already handles stdout).
		prodCfg := zap.NewProductionEncoderConfig()
		prodCfg.TimeKey = "ts"
		prodCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		jsonEnc := zapcore.NewJSONEncoder(prodCfg)
		syncer := logstore.NewSyncer(devNull{}, ls)
		jsonCore := zapcore.NewCore(jsonEnc, zapcore.AddSync(syncer), zapcore.DebugLevel)

		return zap.New(zapcore.NewTee(stdoutCore, jsonCore)), syncer, nil

	default:
		// Production: single JSON core through the tee syncer.
		prodCfg := zap.NewProductionConfig()
		prodCfg.EncoderConfig.TimeKey = "ts"
		prodCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		enc := zapcore.NewJSONEncoder(prodCfg.EncoderConfig)
		syncer := logstore.NewSyncer(os.Stdout, ls)
		core := zapcore.NewCore(enc, zapcore.AddSync(syncer), zapcore.InfoLevel)
		return zap.New(core), syncer, nil
	}
}

// devNull implements io.Writer and discards all data.
type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }
func (devNull) Sync() error                 { return nil }
