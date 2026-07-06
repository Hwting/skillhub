package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/team"
	"github.com/skillhub/skillhub/internal/user"
)

const currentUserKey = "current_user"
const currentTeamKey = "current_team"

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

// TeamScoped loads the team identified by the :slug path param into the
// request context and enforces that the current user holds `required`
// (one of "owner", "admin", "member") membership on it.
//
// The global namespace is read-only: any authenticated user may read it
// (required == "member"), but all write operations (required "admin" or
// "owner") are forbidden.
func TeamScoped(teamSvc *team.Service, required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if slug == "" {
			c.Error(apperr.New("validation_failed", "team", "missing slug"))
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		t, err := teamSvc.Repo().GetBySlug(c.Request.Context(), slug)
		if err != nil {
			c.Error(err)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		u, ok := CurrentUser(c)
		if !ok {
			c.Error(apperr.New("unauthorized", "auth", "no user"))
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		// global 命名空间只读：member 级放行，admin/owner 级一律禁止
		if t.Slug == team.GlobalSlug {
			if required != "member" {
				c.Error(apperr.New("forbidden", "team", "global namespace is read-only"))
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Set(currentTeamKey, t)
			c.Next()
			return
		}
		ctx := c.Request.Context()
		var allowed bool
		switch required {
		case "owner":
			allowed = teamSvc.IsOwner(ctx, t, u.ID)
		case "admin":
			allowed = teamSvc.IsAdminOrOwner(ctx, t, u.ID)
		case "member":
			allowed = teamSvc.IsMember(ctx, t, u.ID)
		default:
			allowed = false
		}
		if !allowed {
			c.Error(apperr.New("forbidden", "team", "forbidden"))
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Set(currentTeamKey, t)
		c.Next()
	}
}

func CurrentTeam(c *gin.Context) (*team.Team, bool) {
	v, exists := c.Get(currentTeamKey)
	if !exists {
		return nil, false
	}
	t, ok := v.(*team.Team)
	return t, ok
}
