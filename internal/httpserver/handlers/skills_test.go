//go:build integration

package handlers_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/storage"
)

// publishSkill sends a raw tarball body to the publish endpoint.
func publishSkill(t *testing.T, r *gin.Engine, cookie *http.Cookie, slug, name, version string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/teams/"+slug+"/skills/"+name+"/versions/"+version, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/gzip")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getWithCookie(t *testing.T, r *gin.Engine, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := reqWithCookie("GET", path, cookie, "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestE2E_PublishDownload(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	// 建团队
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	if w.Code != 201 {
		t.Fatalf("create team: %d %s", w.Code, w.Body.String())
	}

	payload := []byte("fake-tarball-content-v1")
	w = publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", payload)
	if w.Code != 201 {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	wantSha := sha256Hex(payload)
	if got := w.Header().Get("X-Skillhub-Sha256"); got != "" && got != wantSha {
		t.Fatalf("publish resp sha mismatch: %s vs %s", got, wantSha)
	}

	// 下载
	w = getWithCookie(t, r, owner, "/teams/acme/skills/my-skill/versions/1.0.0")
	if w.Code != 200 {
		t.Fatalf("download: %d %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatalf("download body mismatch: %q", w.Body.String())
	}
	if w.Header().Get("X-Skillhub-Sha256") != wantSha {
		t.Fatalf("download sha mismatch: %s", w.Header().Get("X-Skillhub-Sha256"))
	}

	// 再发一个版本
	payload2 := []byte("fake-tarball-content-v2")
	w = publishSkill(t, r, owner, "acme", "my-skill", "1.1.0", payload2)
	if w.Code != 201 {
		t.Fatalf("publish v2: %d %s", w.Code, w.Body.String())
	}

	// GET skill 列两个版本
	w = getWithCookie(t, r, owner, "/teams/acme/skills/my-skill")
	if w.Code != 200 {
		t.Fatalf("get skill: %d %s", w.Code, w.Body.String())
	}
	if c := w.Body.String(); !contains(c, "1.0.0") || !contains(c, "1.1.0") {
		t.Fatalf("missing versions in response: %s", c)
	}

	// 列出团队 skills
	w = getWithCookie(t, r, owner, "/teams/acme/skills")
	if w.Code != 200 {
		t.Fatalf("list skills: %d %s", w.Code, w.Body.String())
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func TestE2E_NonMemberDownload_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("x"))

	other := registerAndLogin(t, r, "other@x.com", "password1")
	w := getWithCookie(t, r, other, "/teams/acme/skills/my-skill/versions/1.0.0")
	if w.Code != 403 {
		t.Fatalf("non-member download: got %d", w.Code)
	}
}

func TestE2E_MemberCannotPublishUnderAdminOnly(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	member := registerAndLogin(t, r, "member@x.com", "password1")
	memberID := userIDByEmail(t, "member@x.com")
	// owner 加 member 为普通 member（非 admin）
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/members", owner, `{"user_id":"`+memberID+`","role":"member"}`))
	if w.Code != 204 {
		t.Fatalf("add member: %d %s", w.Code, w.Body.String())
	}
	// member 在 admin_only 策略下不能发布
	w = publishSkill(t, r, member, "acme", "my-skill", "1.0.0", []byte("x"))
	if w.Code != 403 {
		t.Fatalf("member publish under admin_only: got %d", w.Code)
	}
}

func TestE2E_AnyMemberCanPublish(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	// 改策略为 any_member
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("PATCH", "/teams/acme", owner, `{"publish_policy":"any_member"}`))
	if w.Code != 204 {
		t.Fatalf("patch policy: %d %s", w.Code, w.Body.String())
	}
	member := registerAndLogin(t, r, "member@x.com", "password1")
	memberID := userIDByEmail(t, "member@x.com")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams/acme/members", owner, `{"user_id":"`+memberID+`","role":"member"}`))
	// member 在 any_member 下可发布
	w = publishSkill(t, r, member, "acme", "my-skill", "1.0.0", []byte("x"))
	if w.Code != 201 {
		t.Fatalf("member publish under any_member: got %d %s", w.Code, w.Body.String())
	}
}

func TestE2E_DuplicateVersion_Conflict(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	w := publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("a"))
	if w.Code != 201 {
		t.Fatalf("first publish: %d", w.Code)
	}
	w = publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("b"))
	if w.Code != 409 {
		t.Fatalf("duplicate publish: got %d %s", w.Code, w.Body.String())
	}
}

func TestE2E_GlobalPublish_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	u := registerAndLogin(t, r, "user@x.com", "password1")
	w := publishSkill(t, r, u, "global", "my-skill", "1.0.0", []byte("x"))
	if w.Code != 403 {
		t.Fatalf("global publish: got %d", w.Code)
	}
}

func TestE2E_GlobalDownload_OK(t *testing.T) {
	r := setupTeamApp(t)
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}

	// 取 global 团队 id
	var globalID string
	if err := gdb.Raw("SELECT id::text FROM teams WHERE slug='global'").Scan(&globalID).Error; err != nil {
		t.Fatal(err)
	}
	// 用一个独立邮箱做种子 publisher（仅满足 FK），下载者另用 API 注册的用户。
	var userID, skillID string
	gdb.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('seed-publisher@x.com','seedpub','x','user','active') RETURNING id::text").Scan(&userID)
	gdb.Raw("INSERT INTO skills(team_id,name) VALUES(?,'global-skill') RETURNING id::text", globalID).Scan(&skillID)

	payload := []byte("global-payload")
	sha := sha256Hex(payload)
	key := "skills/" + skillID + "/1.0.0/" + sha + ".tar.gz"
	if _, err := store.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload)), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec("INSERT INTO skill_versions(skill_id,version,storage_key,size,sha256,content_type,publisher_user_id) VALUES(?,?,?,?,?,?,?)",
		skillID, "1.0.0", key, int64(len(payload)), sha, "application/gzip", userID).Error; err != nil {
		t.Fatal(err)
	}

	// 任意认证用户拉取 global skill
	downloader := registerAndLogin(t, r, "downloader@x.com", "password1")
	w := getWithCookie(t, r, downloader, "/teams/global/skills/global-skill/versions/1.0.0")
	if w.Code != 200 {
		t.Fatalf("global download: got %d %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatalf("global download body mismatch: %q", w.Body.String())
	}
}

func TestE2E_SkillSearch_Visibility(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	publishSkill(t, r, owner, "acme", "go-format", "1.0.0", []byte("b"))

	// owner 搜 "lint"：应见到 acme/go-lint，不见 go-format
	w := getWithCookie(t, r, owner, "/skills?q=lint")
	if w.Code != 200 {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "go-lint") || contains(w.Body.String(), "go-format") {
		t.Fatalf("owner search results: %s", w.Body.String())
	}

	// 非成员 other 搜 "lint"：不应见到 acme 的私有 skill
	other := registerAndLogin(t, r, "other@x.com", "password1")
	w = getWithCookie(t, r, other, "/skills?q=lint")
	if w.Code != 200 {
		t.Fatalf("search: %d", w.Code)
	}
	if contains(w.Body.String(), "go-lint") {
		t.Fatalf("private skill leaked to non-member: %s", w.Body.String())
	}
}

func TestE2E_SkillSearch_GlobalVisible(t *testing.T) {
	r := setupTeamApp(t)
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	var globalID, userID, skillID string
	gdb.Raw("SELECT id::text FROM teams WHERE slug='global'").Scan(&globalID)
	gdb.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('pub-e@x.com','pube','x','user','active') RETURNING id::text").Scan(&userID)
	gdb.Raw("INSERT INTO skills(team_id,name) VALUES(?,'global-lint') RETURNING id::text", globalID).Scan(&skillID)
	payload := []byte("x")
	sha := sha256Hex(payload)
	key := "skills/" + skillID + "/1.0.0/" + sha + ".tar.gz"
	if _, err := store.Put(context.Background(), key, bytes.NewReader(payload), 1, "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec("INSERT INTO skill_versions(skill_id,version,storage_key,size,sha256,content_type,publisher_user_id) VALUES(?,?,?,?,?,?,?)",
		skillID, "1.0.0", key, 1, sha, "application/gzip", userID).Error; err != nil {
		t.Fatal(err)
	}

	// 任意认证用户搜 "lint"：应见到 global-lint
	u := registerAndLogin(t, r, "searcher@x.com", "password1")
	w := getWithCookie(t, r, u, "/skills?q=lint")
	if w.Code != 200 {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "global-lint") {
		t.Fatalf("global skill not in results: %s", w.Body.String())
	}
	// latest_version 应带回
	if !contains(w.Body.String(), "1.0.0") {
		t.Fatalf("latest version missing: %s", w.Body.String())
	}
}

func TestE2E_StarLifecycle(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("x"))

	// 成员 star → 204
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/skills/my-skill/star", owner, ""))
	if w.Code != 204 {
		t.Fatalf("star: %d %s", w.Code, w.Body.String())
	}

	// 详情 is_starred=true, star_count=1
	w = getWithCookie(t, r, owner, "/teams/acme/skills/my-skill")
	if w.Code != 200 {
		t.Fatalf("get skill: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, `"is_starred":true`) {
		t.Fatalf("expected is_starred true: %s", body)
	}
	if !contains(body, `"star_count":1`) {
		t.Fatalf("expected star_count 1: %s", body)
	}

	// 幂等：再 star 仍 204，count 仍 1
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/skills/my-skill/star", owner, ""))
	if w.Code != 204 {
		t.Fatalf("star again: %d", w.Code)
	}
	w = getWithCookie(t, r, owner, "/teams/acme/skills/my-skill")
	if !contains(w.Body.String(), `"star_count":1`) {
		t.Fatalf("expected star_count still 1: %s", w.Body.String())
	}

	// unstar → is_starred=false, count=0
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("DELETE", "/teams/acme/skills/my-skill/star", owner, ""))
	if w.Code != 204 {
		t.Fatalf("unstar: %d", w.Code)
	}
	w = getWithCookie(t, r, owner, "/teams/acme/skills/my-skill")
	body = w.Body.String()
	if !contains(body, `"is_starred":false`) {
		t.Fatalf("expected is_starred false: %s", body)
	}
	if !contains(body, `"star_count":0`) {
		t.Fatalf("expected star_count 0: %s", body)
	}
}

func TestE2E_NonMemberStar_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("x"))

	other := registerAndLogin(t, r, "other@x.com", "password1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/skills/my-skill/star", other, ""))
	if w.Code != 403 {
		t.Fatalf("non-member star: got %d", w.Code)
	}
}

func TestE2E_ListMyStars(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "my-skill", "1.0.0", []byte("x"))
	// star
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/acme/skills/my-skill/star", owner, ""))
	if w.Code != 204 {
		t.Fatalf("star: %d", w.Code)
	}

	// GET /me/stars 含该 skill
	w = getWithCookie(t, r, owner, "/me/stars")
	if w.Code != 200 {
		t.Fatalf("my stars: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, "my-skill") {
		t.Fatalf("starred skill missing: %s", body)
	}
	if !contains(body, "1.0.0") {
		t.Fatalf("latest version missing: %s", body)
	}

	// unstar 后 /me/stars 不含
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("DELETE", "/teams/acme/skills/my-skill/star", owner, ""))
	w = getWithCookie(t, r, owner, "/me/stars")
	if contains(w.Body.String(), "my-skill") {
		t.Fatalf("starred skill should be gone: %s", w.Body.String())
	}
}

func TestE2E_GlobalSkill_AnyoneCanStar(t *testing.T) {
	r := setupTeamApp(t)
	cfg, err := config.Load("../../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	var globalID, userID, skillID string
	gdb.Raw("SELECT id::text FROM teams WHERE slug='global'").Scan(&globalID)
	gdb.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('gpub@x.com','gpub','x','user','active') RETURNING id::text").Scan(&userID)
	gdb.Raw("INSERT INTO skills(team_id,name) VALUES(?,'global-tool') RETURNING id::text", globalID).Scan(&skillID)
	payload := []byte("x")
	sha := sha256Hex(payload)
	key := "skills/" + skillID + "/1.0.0/" + sha + ".tar.gz"
	if _, err := store.Put(context.Background(), key, bytes.NewReader(payload), 1, "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec("INSERT INTO skill_versions(skill_id,version,storage_key,size,sha256,content_type,publisher_user_id) VALUES(?,?,?,?,?,?,?)",
		skillID, "1.0.0", key, 1, sha, "application/gzip", userID).Error; err != nil {
		t.Fatal(err)
	}

	// 任意认证用户可 star global skill
	u := registerAndLogin(t, r, "starrer@x.com", "password1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/teams/global/skills/global-tool/star", u, ""))
	if w.Code != 204 {
		t.Fatalf("star global: %d %s", w.Code, w.Body.String())
	}
	w = getWithCookie(t, r, u, "/teams/global/skills/global-tool")
	if !contains(w.Body.String(), `"is_starred":true`) {
		t.Fatalf("expected is_starred true: %s", w.Body.String())
	}
}
