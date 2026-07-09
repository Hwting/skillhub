package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/team"
	"github.com/skillhub/skillhub/internal/user"
)

func Register(r *gin.Engine, svc *user.Service, sm *auth.SessionManager, userRepo user.Repo, teamSvc *team.Service, skillSvc *skill.Service, auditLog *audit.Logger) {
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

	skillH := NewSkillHandlers(skillSvc, teamSvc)
	authed.GET("/skills", skillH.Search)
	authed.GET("/me/stars", skillH.ListMyStars)

	admin := r.Group("/admin")
	admin.Use(auth.AuthRequired(sm, userRepo), auth.RequireRole(user.RolePlatformAdmin))
	{
		admin.GET("/users", userH.List)
		admin.GET("/users/:id", userH.Get)
		admin.PATCH("/users/:id", userH.Patch)
		admin.DELETE("/users/:id", userH.Delete)
		adminH := NewAdminHandlers(skillSvc, teamSvc, auditLog)
		admin.POST("/skills/promote", adminH.Promote)
		admin.GET("/audit", adminH.ListAudit)
	}

	teamH := NewTeamHandlers(teamSvc)
	authed.POST("/teams", teamH.Create)
	authed.GET("/teams", teamH.ListMine)

	teamGroup := r.Group("/teams/:slug")
	teamGroup.Use(auth.AuthRequired(sm, userRepo))
	{
		teamGroup.GET("", auth.TeamScoped(teamSvc, "member"), teamH.Get)
		teamGroup.PATCH("", auth.TeamScoped(teamSvc, "owner"), teamH.Patch)
		teamGroup.DELETE("", auth.TeamScoped(teamSvc, "owner"), teamH.Delete)
		teamGroup.GET("/members", auth.TeamScoped(teamSvc, "member"), teamH.ListMembers)
		teamGroup.POST("/members", auth.TeamScoped(teamSvc, "admin"), teamH.AddMember)
		teamGroup.PATCH("/members/:uid", auth.TeamScoped(teamSvc, "owner"), teamH.PatchMember)
		teamGroup.DELETE("/members/:uid", auth.TeamScoped(teamSvc, "admin"), teamH.RemoveMember)
		teamGroup.POST("/transfer", auth.TeamScoped(teamSvc, "owner"), teamH.Transfer)
	}

	skillGroup := r.Group("/teams/:slug/skills")
	skillGroup.Use(auth.AuthRequired(sm, userRepo))
	{
		skillGroup.GET("", auth.TeamScoped(teamSvc, "member"), skillH.ListSkills)
		skillGroup.GET("/:name", auth.TeamScoped(teamSvc, "member"), skillH.GetSkill)
		skillGroup.POST("/:name/versions/:version", auth.TeamScoped(teamSvc, "member"), skillH.Publish)
		skillGroup.GET("/:name/versions/:version", auth.TeamScoped(teamSvc, "member"), skillH.Download)
		skillGroup.POST("/:name/star", auth.TeamScoped(teamSvc, "member"), skillH.Star)
		skillGroup.DELETE("/:name/star", auth.TeamScoped(teamSvc, "member"), skillH.Unstar)
	}
}
