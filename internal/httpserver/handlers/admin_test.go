//go:build integration

package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/user"
)

// makePlatformAdmin registers a user via API, then promotes their role to
// platform_admin in the DB. AuthRequired reads role from the DB on each
// request, so the returned cookie is effective immediately.
func makePlatformAdmin(t *testing.T, r *gin.Engine) *http.Cookie {
	t.Helper()
	cookie := registerAndLogin(t, r, "admin@x.com", "password1")
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	if err := gdb.Exec("UPDATE users SET role = ? WHERE email = 'admin@x.com'", user.RolePlatformAdmin).Error; err != nil {
		t.Fatal(err)
	}
	return cookie
}

func TestE2E_PromoteToGlobal(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	payload := []byte("lint-payload")
	w := publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", payload)
	if w.Code != 201 {
		t.Fatalf("publish: %d", w.Code)
	}

	admin := makePlatformAdmin(t, r)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 201 {
		t.Fatalf("promote: %d %s", w.Code, w.Body.String())
	}

	// 任意用户搜索见到 global/go-lint
	searcher := registerAndLogin(t, r, "searcher@x.com", "password1")
	w = getWithCookie(t, r, searcher, "/skills?q=lint")
	if w.Code != 200 || !contains(w.Body.String(), "go-lint") {
		t.Fatalf("search after promote: %d %s", w.Code, w.Body.String())
	}

	// 任意用户下载 global/go-lint 内容一致
	w = getWithCookie(t, r, searcher, "/teams/global/skills/go-lint/versions/1.0.0")
	if w.Code != 200 {
		t.Fatalf("global download: %d %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatalf("content mismatch: %q", w.Body.String())
	}

	// 重复提升同 version → 409
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 409 {
		t.Fatalf("dup promote: got %d", w.Code)
	}
}

func TestE2E_Promote_NonAdmin_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", owner, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 403 {
		t.Fatalf("non-admin promote: got %d", w.Code)
	}
}

func TestE2E_AuditList(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	admin := makePlatformAdmin(t, r)
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))

	// audit 异步落库：轮询
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	for i := 0; i < 50; i++ {
		var n int64
		gdb.Raw("SELECT count(*) FROM audit_logs WHERE action='skill_promoted_to_global'").Scan(&n)
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	w := getWithCookie(t, r, admin, "/admin/audit?action=skill_promoted_to_global")
	if w.Code != 200 {
		t.Fatalf("audit list: %d %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "skill_promoted_to_global") {
		t.Fatalf("audit missing action: %s", w.Body.String())
	}

	// 非 admin 403
	other := registerAndLogin(t, r, "other@x.com", "password1")
	w = getWithCookie(t, r, other, "/admin/audit")
	if w.Code != 403 {
		t.Fatalf("non-admin audit: got %d", w.Code)
	}
}
