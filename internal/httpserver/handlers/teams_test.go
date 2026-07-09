//go:build integration

package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/storage"
	"github.com/skillhub/skillhub/internal/team"
	"github.com/skillhub/skillhub/internal/user"
	"go.uber.org/zap"
)

func setupTeamApp(t *testing.T) *gin.Engine {
	t.Helper()
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	gdb.Exec("TRUNCATE skill_versions, skills, team_members, teams, users RESTART IDENTITY CASCADE")
	gdb.Exec("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('global','Global',NULL,'admin_only')")
	rdb, _ := redispkg.New(cfg.Redis)
	rdb.FlushDB(context.Background())
	store, err := storage.New(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}

	auditLogger := audit.NewLogger(gdb, zap.NewNop())
	userRepo := user.NewRepo(gdb)
	userSvc := user.NewService(userRepo, auditLogger)
	teamRepo := team.NewRepo(gdb)
	teamSvc := team.NewService(teamRepo, auditLogger)
	skillRepo := skill.NewRepo(gdb)
	skillSvc := skill.NewService(skillRepo, store, auditLogger)
	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth)
	return httpserver.New(httpserver.Deps{
		Logger: zap.NewNop(), DB: gdb, Redis: rdb, Storage: store,
		UserSvc: userSvc, SessionMgr: sessionMgr, UserRepo: userRepo, TeamSvc: teamSvc, SkillSvc: skillSvc, AuditLogger: auditLogger,
	})
}

func registerAndLogin(t *testing.T, r *gin.Engine, email, pw string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/register", bytes.NewReader([]byte(`{"email":"`+email+`","username":"`+email+`","password":"`+pw+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("register %s: %d %s", email, w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/login", bytes.NewReader([]byte(`{"email":"`+email+`","password":"`+pw+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login %s: %d", email, w.Code)
	}
	return w.Result().Cookies()[0]
}

func reqWithCookie(method, path string, cookie *http.Cookie, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

// userIDByEmail looks up a user id straight from the DB (the register/login
// responses don't expose id, and /me returns the current user only).
func userIDByEmail(t *testing.T, email string) string {
	t.Helper()
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := gdb.Raw("SELECT id::text FROM users WHERE email = ?", email).Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func TestE2E_CreateTeamAddMemberTransfer(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")

	// 创建团队
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	if w.Code != 201 {
		t.Fatalf("create team: %d %s", w.Code, w.Body.String())
	}

	// 注册第二个用户作为成员
	member := registerAndLogin(t, r, "member@x.com", "password1")
	memberID := userIDByEmail(t, "member@x.com")

	// owner 加 member 为 admin
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/members", owner, `{"user_id":"`+memberID+`","role":"admin"}`))
	if w.Code != 204 {
		t.Fatalf("add member: %d %s", w.Code, w.Body.String())
	}

	// member 现在能访问团队
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("GET", "/teams/acme", member, ""))
	if w.Code != 200 {
		t.Fatalf("member get team: %d %s", w.Code, w.Body.String())
	}

	// 转移所有权给 member
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/transfer", owner, `{"new_owner_id":"`+memberID+`"}`))
	if w.Code != 204 {
		t.Fatalf("transfer: %d %s", w.Code, w.Body.String())
	}

	// 现在 member 是 owner，能 patch policy
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("PATCH", "/teams/acme", member, `{"publish_policy":"any_member"}`))
	if w.Code != 204 {
		t.Fatalf("new owner patch: %d %s", w.Code, w.Body.String())
	}

	// 旧 owner 现在是 admin，仍能 list members
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("GET", "/teams/acme/members", owner, ""))
	if w.Code != 200 {
		t.Fatalf("old owner list members: %d %s", w.Code, w.Body.String())
	}
}

func TestE2E_NonMember_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	other := registerAndLogin(t, r, "other@x.com", "password1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("GET", "/teams/acme", other, ""))
	if w.Code != 403 {
		t.Fatalf("non-member: got %d", w.Code)
	}
}

func TestE2E_GlobalReadOnly(t *testing.T) {
	r := setupTeamApp(t)
	u := registerAndLogin(t, r, "user@x.com", "password1")

	// 任何认证用户可读 global
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("GET", "/teams/global", u, ""))
	if w.Code != 200 {
		t.Fatalf("read global: got %d %s", w.Code, w.Body.String())
	}

	// 写操作一律禁止：delete / patch / add member
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("DELETE", "/teams/global", u, ""))
	if w.Code != 403 {
		t.Fatalf("delete global: got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("PATCH", "/teams/global", u, `{"name":"X"}`))
	if w.Code != 403 {
		t.Fatalf("patch global: got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/global/members", u, `{"user_id":"00000000-0000-0000-0000-000000000000","role":"member"}`))
	if w.Code != 403 {
		t.Fatalf("add member global: got %d", w.Code)
	}
}
