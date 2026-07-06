package main

import (
	"net/http"

	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver"
	"github.com/skillhub/skillhub/internal/log"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/storage"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		panic(err)
	}
	logger, err := log.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		logger.Fatal("init db", zap.Error(err))
	}
	rdb, err := redispkg.New(cfg.Redis)
	if err != nil {
		logger.Fatal("init redis", zap.Error(err))
	}
	store, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Fatal("init storage", zap.Error(err))
	}

	engine := httpserver.New(httpserver.Deps{
		Logger:  logger,
		DB:      gdb,
		Redis:   rdb,
		Storage: store,
	})
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	logger.Info("starting server", zap.String("addr", srv.Addr))
	if err := httpserver.Run(srv, cfg.Server.ShutdownTimeout, logger); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}
