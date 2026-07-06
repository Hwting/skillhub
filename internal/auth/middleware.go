package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/user"
)

const currentUserKey = "current_user"

func AuthRequired(sm *SessionManager, userRepo user.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(sm.cookieCfg.CookieName)
		if err != nil {
			c.Error(apperr.New("unauthorized", "auth", "missing session cookie"))
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		uid, err := sm.Get(c.Request.Context(), sid)
		if err != nil {
			c.Error(err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		u, err := userRepo.GetByID(c.Request.Context(), uid)
		if err != nil {
			c.Error(err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if u.Status != user.StatusActive {
			c.Error(apperr.New("unauthorized", "auth", "user disabled"))
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set(currentUserKey, u)
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := CurrentUser(c)
		if !ok {
			c.Error(apperr.New("unauthorized", "auth", "missing current user"))
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if u.Role != role {
			c.Error(apperr.New("forbidden", "auth", "forbidden"))
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*user.User, bool) {
	v, exists := c.Get(currentUserKey)
	if !exists {
		return nil, false
	}
	u, ok := v.(*user.User)
	return u, ok
}
