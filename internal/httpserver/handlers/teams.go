package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/team"
)

type TeamHandlers struct {
	svc *team.Service
}

func NewTeamHandlers(svc *team.Service) *TeamHandlers { return &TeamHandlers{svc: svc} }

type teamResp struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	OwnerUserID   string `json:"owner_user_id"`
	PublishPolicy string `json:"publish_policy"`
}

func toTeamResp(t *team.Team) teamResp {
	owner := ""
	if t.OwnerUserID != nil {
		owner = t.OwnerUserID.String()
	}
	return teamResp{ID: t.ID.String(), Slug: t.Slug, Name: t.Name, OwnerUserID: owner, PublishPolicy: t.PublishPolicy}
}

type createTeamReq struct {
	Slug string `json:"slug" binding:"required"`
	Name string `json:"name" binding:"required"`
}

func (h *TeamHandlers) Create(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	t, err := h.svc.Create(c.Request.Context(), req.Slug, req.Name, u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toTeamResp(t))
}

func (h *TeamHandlers) ListMine(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	teams, err := h.svc.Repo().ListForUser(c.Request.Context(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]teamResp, len(teams))
	for i, t := range teams {
		out[i] = toTeamResp(&t)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *TeamHandlers) Get(c *gin.Context) {
	t, ok := auth.CurrentTeam(c)
	if !ok {
		c.Error(apperr.New("not_found", "team", "no team"))
		return
	}
	c.JSON(http.StatusOK, toTeamResp(t))
}

type patchTeamReq struct {
	Name          *string `json:"name"`
	PublishPolicy *string `json:"publish_policy"`
}

func (h *TeamHandlers) Patch(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req patchTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	if err := h.svc.Update(c.Request.Context(), u.ID, t.ID, req.Name, req.PublishPolicy); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandlers) Delete(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	if err := h.svc.Delete(c.Request.Context(), u.ID, t.ID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type memberResp struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *TeamHandlers) ListMembers(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	ms, err := h.svc.Repo().ListMembers(c.Request.Context(), t.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := []memberResp{}
	// owner 行
	if t.OwnerUserID != nil {
		out = append(out, memberResp{UserID: t.OwnerUserID.String(), Role: "owner"})
	}
	for _, m := range ms {
		out = append(out, memberResp{UserID: m.UserID.String(), Role: m.Role})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type addMemberReq struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

func (h *TeamHandlers) AddMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req addMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid user_id"))
		return
	}
	if err := h.svc.AddMember(c.Request.Context(), u.ID, t.ID, uid, req.Role); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type patchMemberReq struct {
	Role string `json:"role" binding:"required"`
}

func (h *TeamHandlers) PatchMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid uid"))
		return
	}
	var req patchMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	if err := h.svc.UpdateMemberRole(c.Request.Context(), u.ID, t.ID, uid, req.Role); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandlers) RemoveMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid uid"))
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), u.ID, t.ID, uid); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type transferReq struct {
	NewOwnerID string `json:"new_owner_id" binding:"required"`
}

func (h *TeamHandlers) Transfer(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req transferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	uid, err := uuid.Parse(req.NewOwnerID)
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid new_owner_id"))
		return
	}
	if err := h.svc.TransferOwnership(c.Request.Context(), u.ID, t.ID, uid); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
