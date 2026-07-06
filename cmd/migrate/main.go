package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/skillhub/skillhub/internal/config"
)

func dsn(c *config.DBConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate up|down|create <name>")
	}
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	m, err := migrate.New("file://migrations", dsn(&cfg.DB))
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	case "create":
		if len(os.Args) < 3 {
			log.Fatal("usage: migrate create <name>")
		}
		if err := m.Force(0); err != nil { // ensure clean state for naming
			_ = err
		}
		// golang-migrate 不支持运行时 create；用 CLI 替代
		log.Fatal("create via: migrate -path migrations -database <dsn> create <name>")
	default:
		log.Fatalf("unknown command: %s", os.Args[1])
	}
}
