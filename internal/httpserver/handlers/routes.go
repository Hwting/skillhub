package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

func Register(r *gin.Engine, svc *user.Service, sm *auth.SessionManager, userRepo user.Repo) {
	authH := NewAuthHandlers(svc, sm)
	userH := NewUserHandlers(svc)

	r.POST("/register", authH.Register)
	r.POST("/login", authH.Login)

	authed := r.Group("")
	authed.Use(auth.AuthRequired(sm, userRepo))
	{
		authed.POST("/logout", authH.Logout)
		authed.GET("/me", authH.Me)
	}

	admin := r.Group("/admin")
	admin.Use(auth.AuthRequired(sm, userRepo), auth.RequireRole(user.RolePlatformAdmin))
	{
		admin.GET("/users", userH.List)
		admin.GET("/users/:id", userH.Get)
		admin.PATCH("/users/:id", userH.Patch)
		admin.DELETE("/users/:id", userH.Delete)
	}
}
