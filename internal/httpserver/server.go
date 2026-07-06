package httpserver

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver/handlers"
	"github.com/skillhub/skillhub/internal/httpserver/middleware"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/storage"
	"github.com/skillhub/skillhub/internal/user"
	"go.uber.org/zap"
	"gorm.io/gorm"

	rdb "github.com/redis/go-redis/v9"
)

type Deps struct {
	Logger     *zap.Logger
	DB         *gorm.DB
	Redis      *rdb.Client
	Storage    storage.Store
	UserSvc    *user.Service
	SessionMgr *auth.SessionManager
	UserRepo   user.Repo
}

func New(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recover(deps.Logger), middleware.AccessLog(deps.Logger), middleware.Errors())
	r.GET("/healthz", healthz(deps))
	if deps.UserSvc != nil && deps.SessionMgr != nil && deps.UserRepo != nil {
		handlers.Register(r, deps.UserSvc, deps.SessionMgr, deps.UserRepo)
	}
	return r
}

func healthz(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		checks := gin.H{}
		ok := true
		if deps.DB != nil {
			if err := db.Ping(deps.DB); err != nil {
				ok = false
				checks["db"] = err.Error()
			} else {
				checks["db"] = "ok"
			}
		} else {
			ok = false
			checks["db"] = "not configured"
		}
		if deps.Redis != nil {
			if err := redispkg.Ping(ctx, deps.Redis); err != nil {
				ok = false
				checks["redis"] = err.Error()
			} else {
				checks["redis"] = "ok"
			}
		} else {
			ok = false
			checks["redis"] = "not configured"
		}
		status := "ok"
		code := 200
		if !ok {
			status = "degraded"
			code = 503
		}
		c.JSON(code, gin.H{"status": status, "checks": checks})
	}
}

func Run(srv *http.Server, shutdownTimeout time.Duration, logger *zap.Logger) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
