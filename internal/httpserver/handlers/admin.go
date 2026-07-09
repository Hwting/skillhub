package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/team"
)

type AdminHandlers struct {
	skillSvc *skill.Service
	teamSvc  *team.Service
	auditLog *audit.Logger
}

func NewAdminHandlers(skillSvc *skill.Service, teamSvc *team.Service, auditLog *audit.Logger) *AdminHandlers {
	return &AdminHandlers{skillSvc: skillSvc, teamSvc: teamSvc, auditLog: auditLog}
}

type promoteReq struct {
	TeamSlug   string `json:"team_slug" binding:"required"`
	SkillName  string `json:"skill_name" binding:"required"`
	Version    string `json:"version" binding:"required"`
	TargetName string `json:"target_name" binding:"required"`
}

func (h *AdminHandlers) Promote(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	var req promoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "admin", "invalid body"))
		return
	}
	ctx := c.Request.Context()
	srcTeam, err := h.teamSvc.Repo().GetBySlug(ctx, req.TeamSlug)
	if err != nil {
		c.Error(err)
		return
	}
	srcSkill, err := h.skillSvc.Repo().GetSkill(ctx, srcTeam.ID, req.SkillName)
	if err != nil {
		c.Error(err)
		return
	}
	globalTeam, err := h.teamSvc.Repo().GetBySlug(ctx, team.GlobalSlug)
	if err != nil {
		c.Error(err)
		return
	}
	gv, err := h.skillSvc.PromoteToGlobal(ctx, srcSkill.ID, req.Version, globalTeam.ID, req.TargetName, u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, versionResp{
		ID:          gv.ID.String(),
		Version:     gv.Version,
		Size:        gv.Size,
		Sha256:      gv.Sha256,
		ContentType: gv.ContentType,
		Publisher:   gv.PublisherUserID.String(),
		CreatedAt:   gv.CreatedAt.Format(timeRFC3339),
	})
}

func (h *AdminHandlers) ListAudit(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	f := audit.Filter{
		Action:     c.Query("action"),
		ActorID:    c.Query("actor_id"),
		TargetType: c.Query("target_type"),
		TargetID:   c.Query("target_id"),
		Limit:      pageSize,
		Offset:     (page - 1) * pageSize,
	}
	recs, err := h.auditLog.List(c.Request.Context(), f)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": recs, "page": page, "page_size": pageSize})
}
