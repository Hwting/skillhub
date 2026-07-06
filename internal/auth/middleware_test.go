package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/user"
)

// 用 stub SessionManager 不便；这里只测 RequireRole + CurrentUser 的纯逻辑。
func TestRequireRole_Denies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(currentUserKey, &user.User{Role: user.RoleUser}); c.Next() })
	r.GET("/", RequireRole(user.RolePlatformAdmin), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d", w.Code)
	}
}

func TestRequireRole_Allows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(currentUserKey, &user.User{Role: user.RolePlatformAdmin}); c.Next() })
	r.GET("/", RequireRole(user.RolePlatformAdmin), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestCurrentUser_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	if _, ok := CurrentUser(c); ok {
		t.Fatal("expected no current user")
	}
}

// 触发 uuid 引用避免未使用
var _ = uuid.Nil

func TestCurrentTeam_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	if _, ok := CurrentTeam(c); ok {
		t.Fatal("expected no current team")
	}
}
