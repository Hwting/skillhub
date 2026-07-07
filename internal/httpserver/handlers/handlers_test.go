//go:build integration

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver"
	"github.com/skillhub/skillhub/internal/password"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/storage"
	"github.com/skillhub/skillhub/internal/team"
	"github.com/skillhub/skillhub/internal/user"
	"go.uber.org/zap"
)

func setupApp(t *testing.T) *gin.Engine {
	t.Helper()
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	gdb.Exec("TRUNCATE skill_versions, skills, team_members, users RESTART IDENTITY CASCADE")
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
		UserSvc: userSvc, SessionMgr: sessionMgr, UserRepo: userRepo, TeamSvc: teamSvc, SkillSvc: skillSvc,
	})
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestE2E_RegisterLoginMeLogout(t *testing.T) {
	r := setupApp(t)
	w := postJSON(t, r, "/register", `{"email":"e@x.com","username":"e","password":"password1"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d body=%s", w.Code, w.Body.String())
	}
	w = postJSON(t, r, "/login", `{"email":"e@x.com","password":"password1"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d body=%s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/me", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("me: %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
}

func TestE2E_Me_Unauth(t *testing.T) {
	r := setupApp(t)
	req := httptest.NewRequest("GET", "/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
	}
}

func TestE2E_Admin_ForbiddenForUser(t *testing.T) {
	r := setupApp(t)
	postJSON(t, r, "/register", `{"email":"u@x.com","username":"u","password":"password1"}`)
	w := postJSON(t, r, "/login", `{"email":"u@x.com","password":"password1"}`)
	cookie := w.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d", w.Code)
	}
}

func TestE2E_Admin_ListUsers(t *testing.T) {
	r := setupApp(t)
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := password.Hash("password1")
	gdb.Exec("INSERT INTO users(email,username,password_hash,role,status) VALUES('a@x.com','a',?,?,'active')", hash, user.RolePlatformAdmin)
	w := postJSON(t, r, "/login", `{"email":"a@x.com","password":"password1"}`)
	cookie := w.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total < 1 {
		t.Fatalf("expected users, got %d", resp.Total)
	}
}
