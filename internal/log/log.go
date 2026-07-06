package log

import (
	"fmt"

	"github.com/skillhub/skillhub/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg config.LogConfig) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("invalid log.level %q: %w", cfg.Level, err)
	}
	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(level)
	if cfg.Format == "console" {
		zcfg = zap.NewDevelopmentConfig()
		zcfg.Level = zap.NewAtomicLevelAt(level)
	}
	logger, err := zcfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}
