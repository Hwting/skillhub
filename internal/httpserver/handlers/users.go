package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

type UserHandlers struct {
	svc *user.Service
}

func NewUserHandlers(svc *user.Service) *UserHandlers { return &UserHandlers{svc: svc} }

func (h *UserHandlers) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	users, total, err := h.svc.ListForAdmin(c.Request.Context(), limit, offset)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]userResp, len(users))
	for i, u := range users {
		out[i] = toUserResp(&u)
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": total})
}

func (h *UserHandlers) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	u, err := h.svc.GetForAdmin(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResp(u))
}

type patchReq struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

func (h *UserHandlers) Patch(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	var req patchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	actor, _ := auth.CurrentUser(c)
	if req.Role != nil {
		if err := h.svc.UpdateRole(c.Request.Context(), actor.ID, id, *req.Role, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
			c.Error(err)
			return
		}
	}
	if req.Status != nil && *req.Status == user.StatusDisabled {
		if err := h.svc.Disable(c.Request.Context(), actor.ID, id, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
			c.Error(err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandlers) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	actor, _ := auth.CurrentUser(c)
	if err := h.svc.Disable(c.Request.Context(), actor.ID, id, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
