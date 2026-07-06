package db

import (
	"fmt"
	"time"

	"github.com/skillhub/skillhub/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func New(cfg config.DBConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, fmt.Errorf("raw db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	return g, nil
}

func Ping(g *gorm.DB) error {
	sqlDB, err := g.DB()
	if err != nil {
		return fmt.Errorf("raw db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}
