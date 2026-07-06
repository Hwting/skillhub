package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

type AuthHandlers struct {
	svc *user.Service
	sm  *auth.SessionManager
}

func NewAuthHandlers(svc *user.Service, sm *auth.SessionManager) *AuthHandlers {
	return &AuthHandlers{svc: svc, sm: sm}
}

type registerReq struct {
	Email    string `json:"email" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandlers) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Username, req.Password)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toUserResp(u))
}

type loginReq struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandlers) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	u, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		c.Error(err)
		return
	}
	sid, err := h.sm.Create(c.Request.Context(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	h.sm.SetCookie(c, sid)
	c.JSON(http.StatusOK, toUserResp(u))
}

func (h *AuthHandlers) Logout(c *gin.Context) {
	sid, err := c.Cookie(h.sm.CookieName())
	if err == nil {
		h.sm.Delete(c.Request.Context(), sid)
	}
	h.sm.ClearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandlers) Me(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	c.JSON(http.StatusOK, toUserResp(u))
}

type userResp struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

func toUserResp(u *user.User) userResp {
	return userResp{ID: u.ID.String(), Email: u.Email, Username: u.Username, Role: u.Role, Status: u.Status}
}
