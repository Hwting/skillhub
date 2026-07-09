# SkillHub 治理（子项目 F）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** platform_admin 把团队 skill 版本提升至 global（复制 storage 对象 + 插 global skill/version 行），并提供审计日志只读接口。

**Architecture:** skill.Service 加 PromoteToGlobal（复制对象）。audit 加 List/Record/Filter 读模型。handlers 加 promote + audit list，挂到 admin 组。Deps 加 AuditLogger。无新迁移。

**Tech Stack:** GORM、Gin、storage.Store、复用 audit/team/skill/auth.RequireRole。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A–E。
- service/repo 失败返回 `*apperr.Error`；handler 错误经 errors 中间件。
- 单元测试无外部依赖（mock repo + mem store）；集成测试 build tag `//go:build integration`。
- 仅 platform_admin 可调 F 的两个端点（admin 路由组已有 RequireRole）。
- 提升复制对象到 global 拥有的 key，global 自包含。
- 每任务结束提交，conventional commits。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `internal/skill/service.go` | 加 PromoteToGlobal |
| `internal/skill/service_test.go` | PromoteToGlobal 单测 |
| `internal/audit/query.go` | Record/Filter + Logger.List |
| `internal/audit/query_test.go` | List 集成测试 |
| `internal/httpserver/handlers/admin.go` | promote + audit list handler（新文件） |
| `internal/httpserver/handlers/routes.go` | 注册 admin 路由 |
| `internal/httpserver/server.go` | Deps 加 AuditLogger |
| `cmd/skillhub/main.go` | 传 AuditLogger |
| `internal/httpserver/handlers/admin_test.go` | e2e 集成测试 |

---

### Task 1: skill.Service.PromoteToGlobal + 单测

**Files:**
- Modify: `internal/skill/service.go`
- Modify: `internal/skill/service_test.go`

- [ ] **Step 1: 加 PromoteToGlobal 到 service.go**

```go
// PromoteToGlobal copies a team skill's version into the global namespace
// under targetName. If a global skill with targetName already exists, the
// version is added to it (version must not already exist there). The source
// storage object is copied to a global-owned key so global is self-contained.
func (s *Service) PromoteToGlobal(ctx context.Context, srcSkillID uuid.UUID, version string, globalTeamID uuid.UUID, targetName string, adminID uuid.UUID) (*SkillVersion, error) {
	if !IsValidName(targetName) {
		return nil, apperr.New("validation_failed", "skill", "invalid target name")
	}
	srcSkill, err := s.repo.GetSkillByID(ctx, srcSkillID)
	if err != nil {
		return nil, err
	}
	srcVer, err := s.repo.GetVersion(ctx, srcSkillID, version)
	if err != nil {
		return nil, err
	}

	// 找或建 global skill
	gSkill, err := s.repo.GetSkill(ctx, globalTeamID, targetName)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		gSkill = &Skill{TeamID: globalTeamID, Name: targetName}
		if err := s.repo.CreateSkill(ctx, gSkill); err != nil {
			return nil, err
		}
	}
	// version 已存在 → conflict
	if existing, err := s.repo.GetVersion(ctx, gSkill.ID, version); err == nil && existing != nil {
		return nil, apperr.New("conflict", "skill", "version already exists in global")
	}

	// 复制对象
	rc, err := s.store.Get(ctx, srcVer.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("store get source: %w", err)
	}
	defer rc.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, rc); err != nil {
		return nil, apperr.New("validation_failed", "skill", "read source failed")
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])
	if sha != srcVer.Sha256 {
		return nil, apperr.New("db_error", "skill", "integrity check failed")
	}
	newKey := fmt.Sprintf("skills/%s/%s/%s.tar.gz", gSkill.ID.String(), version, sha)
	if _, err := s.store.Put(ctx, newKey, bytes.NewReader(buf.Bytes()), int64(buf.Len()), srcVer.ContentType); err != nil {
		return nil, fmt.Errorf("store put global: %w", err)
	}

	gv := &SkillVersion{
		SkillID:         gSkill.ID,
		Version:         version,
		StorageKey:      newKey,
		Size:            int64(buf.Len()),
		Sha256:          sha,
		ContentType:     srcVer.ContentType,
		PublisherUserID: adminID,
	}
	if err := s.repo.CreateVersion(ctx, gv); err != nil {
		if !isConflict(err) {
			_ = s.store.Delete(ctx, newKey)
		}
		return nil, err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &adminID, Action: audit.Action("skill_promoted_to_global"), TargetType: "skill_version", TargetID: gv.ID.String(), Metadata: map[string]any{"src_skill_id": srcSkillID.String(), "global_skill_id": gSkill.ID.String(), "version": version, "target_name": targetName}})
	_ = srcSkill
	return gv, nil
}
```
**需给 Repo 接口加 `GetSkillByID(ctx, skillID) (*Skill, error)`**（现有只有按 teamID+name 的 GetSkill）。PromoteToGlobal 用 srcSkillID 直接查。实现：
```go
func (r *repo) GetSkillByID(ctx context.Context, skillID uuid.UUID) (*Skill, error) {
	var s Skill
	if err := r.db.WithContext(ctx).First(&s, "id = ?", skillID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "skill", "skill not found")
		}
		return nil, fmt.Errorf("get skill by id: %w", err)
	}
	return &s, nil
}
```
（`srcSkill` 目前未使用，可去掉以避免 lint——实际上可用来校验 srcVer 属于它，但 GetVersion 已按 skillID 过滤。去掉 srcSkill 查询与 `_ = srcSkill`。）

修订：去掉 `srcSkill` 查询（GetVersion(srcSkillID, version) 已足够），避免多余调用与未使用变量。

- [ ] **Step 2: mock repo 加 GetSkillByID**

```go
func (m *mockSkillRepo) GetSkillByID(ctx context.Context, skillID uuid.UUID) (*Skill, error) {
	if s, ok := m.skills[skillID]; ok {
		return s, nil
	}
	return nil, apperr.New("not_found", "skill", "skill not found")
}
```

- [ ] **Step 3: 单测**

```go
func TestPromoteToGlobal(t *testing.T) {
	s, _, st := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	// 源 skill + 版本
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()

	gv, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	if err != nil { t.Fatal(err) }
	if gv.SkillID == src.SkillID { t.Fatal("global version should belong to a different skill") }
	if gv.Sha256 != src.Sha256 { t.Fatal("sha mismatch") }
	// 对象被复制到新 key
	if _, ok := st.objs[gv.StorageKey]; !ok { t.Fatal("global object not stored") }
	if gv.StorageKey == src.StorageKey { t.Fatal("should be a copy, not same key") }
}

func TestPromoteToGlobal_VersionConflict(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, _ = s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	// 再提升同 version → conflict
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	if err == nil { t.Fatal("expected conflict") }
	e, ok := err.(*apperr.Error)
	if !ok || e.Code != "conflict" { t.Fatalf("expected conflict, got %v", err) }
}

func TestPromoteToGlobal_SourceVersionMissing(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "9.9.9", globalTeam, "go-lint", admin)
	if err == nil { t.Fatal("expected not found") }
}

func TestPromoteToGlobal_InvalidTargetName(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "BadName", admin)
	if err == nil { t.Fatal("expected invalid name") }
}
```

- [ ] **Step 4: 跑测试**
Run: `go test ./internal/skill/`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add internal/skill
git commit -m "feat(skill): add PromoteToGlobal that copies a version into the global namespace"
```

---

### Task 2: audit 读模型 + List

**Files:**
- Create: `internal/audit/query.go`
- Create: `internal/audit/query_test.go`

- [ ] **Step 1: 写 query.go**

```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Record struct {
	ID          int64      `json:"id"`
	ActorUserID *uuid.UUID `json:"actor_user_id"`
	Action      string     `json:"action"`
	TargetType  string     `json:"target_type"`
	TargetID    string     `json:"target_id"`
	IP          string     `json:"ip"`
	UserAgent   string     `json:"user_agent"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Filter struct {
	Action     string
	ActorID    string
	TargetType string
	TargetID   string
	Limit      int
	Offset     int
}

func (l *Logger) List(ctx context.Context, f Filter) ([]Record, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	var rows []auditRow
	tx := l.db.WithContext(ctx).Model(&auditRow{})
	if f.Action != "" {
		tx = tx.Where("action = ?", f.Action)
	}
	if f.ActorID != "" {
		tx = tx.Where("actor_user_id = ?", f.ActorID)
	}
	if f.TargetType != "" {
		tx = tx.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		tx = tx.Where("target_id = ?", f.TargetID)
	}
	if err := tx.Order("created_at DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	out := make([]Record, len(rows))
	for i, r := range rows {
		rec := Record{ID: r.ID, ActorUserID: r.ActorUserID, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, IP: r.IP, UserAgent: r.UserAgent, CreatedAt: r.CreatedAt}
		if len(r.Metadata) > 0 {
			_ = json.Unmarshal(r.Metadata, &rec.Metadata)
		}
		out[i] = rec
	}
	return out, nil
}
```
需 import `"time"`。`auditRow` 在 audit.go 内定义（同包），可直接用。

- [ ] **Step 2: 集成测试 query_test.go**

```go
//go:build integration

package audit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"go.uber.org/zap"
)

func TestLogger_List_Filter(t *testing.T) {
	cfg, err := config.Load("../../config/config.yaml")
	if err != nil { t.Fatal(err) }
	gdb, err := db.New(cfg.DB)
	if err != nil { t.Fatal(err) }
	gdb.Exec("TRUNCATE audit_logs RESTART IDENTITY")
	l := NewLogger(gdb, zap.NewNop())
	ctx := context.Background()
	aid := uuid.New()
	_ = l.Log(ctx, Entry{ActorUserID: &aid, Action: Action("team_created"), TargetType: "team", TargetID: "x"})
	_ = l.Log(ctx, Entry{ActorUserID: &aid, Action: Action("skill_promoted_to_global"), TargetType: "skill_version", TargetID: "y"})
	// 异步写：等一下
	// 用直接插库保证同步更稳，但 Log 已用 goroutine。改用 gdb 直接查轮询。
	// 简化：直接插两行。
	// （Log 异步，测试可能拿不到。改同步插入：）
	// 清空再直接插：
	gdb.Exec("TRUNCATE audit_logs RESTART IDENTITY")
	gdb.Exec("INSERT INTO audit_logs(actor_user_id,action,target_type,target_id) VALUES(?, 'team_created','team','x')", aid)
	gdb.Exec("INSERT INTO audit_logs(actor_user_id,action,target_type,target_id) VALUES(?, 'skill_promoted_to_global','skill_version','y')", aid)

	recs, err := l.List(ctx, Filter{Action: "team_created", Limit: 100})
	if err != nil { t.Fatal(err) }
	if len(recs) != 1 || recs[0].Action != "team_created" { t.Fatalf("recs=%+v", recs) }

	all, _ := l.List(ctx, Filter{Limit: 100})
	if len(all) != 2 { t.Fatalf("all=%d", len(all)) }
}
```
（注：Log 异步，测试用直接插库保证确定性。）

- [ ] **Step 3: 跑测试**
Run: `go vet ./internal/audit/ && go test -tags integration ./internal/audit/`
Expected: PASS

- [ ] **Step 4: 提交**
```bash
git add internal/audit
git commit -m "feat(audit): add List query with filter and pagination"
```

---

### Task 3: handlers + routes + server wiring + e2e

**Files:**
- Create: `internal/httpserver/handlers/admin.go`
- Modify: `internal/httpserver/handlers/routes.go`
- Modify: `internal/httpserver/server.go`
- Modify: `cmd/skillhub/main.go`
- Modify: `internal/httpserver/handlers/handlers_test.go`（setupApp 补 AuditLogger）
- Modify: `internal/httpserver/handlers/teams_test.go`（setupTeamApp 补 AuditLogger）
- Create: `internal/httpserver/handlers/admin_test.go`

- [ ] **Step 1: 写 handlers/admin.go**

```go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/team"
)

type AdminHandlers struct {
	skillSvc   *skill.Service
	teamSvc    *team.Service
	auditLog   *audit.Logger
}

func NewAdminHandlers(skillSvc *skill.Service, teamSvc *team.Service, auditLog *audit.Logger) *AdminHandlers {
	return &AdminHandlers{skillSvc: skillSvc, teamSvc: teamSvc, auditLog: auditLog}
}

type promoteReq struct {
	TeamSlug   string `json:"team_slug" binding:"required"`
	SkillName  string `json:"skill_name" binding:"required"`
	Version    string `json:"version" binding:"required"`
	TargetName string `json:"target_name" binding:"required"`
}

func (h *AdminHandlers) Promote(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	var req promoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "admin", "invalid body"))
		return
	}
	ctx := c.Request.Context()
	srcTeam, err := h.teamSvc.Repo().GetBySlug(ctx, req.TeamSlug)
	if err != nil {
		c.Error(err)
		return
	}
	srcSkill, err := h.skillSvc.Repo().GetSkill(ctx, srcTeam.ID, req.SkillName)
	if err != nil {
		c.Error(err)
		return
	}
	globalTeam, err := h.teamSvc.Repo().GetBySlug(ctx, team.GlobalSlug)
	if err != nil {
		c.Error(err)
		return
	}
	gv, err := h.skillSvc.PromoteToGlobal(ctx, srcSkill.ID, req.Version, globalTeam.ID, req.TargetName, u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, versionResp{
		ID: gv.ID.String(), Version: gv.Version, Size: gv.Size, Sha256: gv.Sha256,
		ContentType: gv.ContentType, Publisher: gv.PublisherUserID.String(),
		CreatedAt: gv.CreatedAt.Format(timeRFC3339),
	})
}

func (h *AdminHandlers) ListAudit(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 { page = 1 }
	if pageSize < 1 { pageSize = 20 }
	if pageSize > 100 { pageSize = 100 }
	f := audit.Filter{
		Action: c.Query("action"),
		ActorID: c.Query("actor_id"),
		TargetType: c.Query("target_type"),
		TargetID: c.Query("target_id"),
		Limit: pageSize,
		Offset: (page - 1) * pageSize,
	}
	recs, err := h.auditLog.List(c.Request.Context(), f)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": recs, "page": page, "page_size": pageSize})
}

// 触发 uuid 引用
var _ = uuid.Nil
```

- [ ] **Step 2: routes.go 注册**

`Register` 加参数 `auditLog *audit.Logger`。在 admin 组内：
```go
	adminH := NewAdminHandlers(skillSvc, teamSvc, auditLog)
	admin.POST("/skills/promote", adminH.Promote)
	admin.GET("/audit", adminH.ListAudit)
```
加 import `"github.com/skillhub/skillhub/internal/audit"`。

- [ ] **Step 3: server.go — Deps 加 AuditLogger**

Deps 加 `AuditLogger *audit.Logger`。Register 调用加 `deps.AuditLogger`。条件加 `&& deps.AuditLogger != nil`。加 import `"github.com/skillhub/skillhub/internal/audit"`。

- [ ] **Step 4: main.go — 传 AuditLogger**

Deps 加 `AuditLogger: auditLogger`。

- [ ] **Step 5: setupApp / setupTeamApp 补 AuditLogger**

两处 Deps 加 `AuditLogger: auditLogger`（auditLogger 已在两处构造）。

- [ ] **Step 6: e2e 测试 admin_test.go**

```go
//go:build integration

package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/password"
	"github.com/skillhub/skillhub/internal/user"
)

// makePlatformAdmin 直接插一个 platform_admin 用户并登录。
func makePlatformAdmin(t *testing.T, r *gin.Engine) *http.Cookie {
	t.Helper()
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	hash, _ := password.Hash("password1")
	gdb.Exec("INSERT INTO users(email,username,password_hash,role,status) VALUES('admin@x.com','admin',?,?,'active') ON CONFLICT (email) DO UPDATE SET password_hash=EXCLUDED.password_hash, role=EXCLUDED.role", hash, user.RolePlatformAdmin)
	return registerAndLogin(t, r, "admin@x.com", "password1")
}

func TestE2E_PromoteToGlobal(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	payload := []byte("lint-payload")
	w := publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", payload)
	if w.Code != 201 { t.Fatalf("publish: %d", w.Code) }

	admin := makePlatformAdmin(t, r)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 201 { t.Fatalf("promote: %d %s", w.Code, w.Body.String()) }

	// 任意用户搜索见到 global/go-lint
	searcher := registerAndLogin(t, r, "searcher@x.com", "password1")
	w = getWithCookie(t, r, searcher, "/skills?q=lint")
	if w.Code != 200 || !contains(w.Body.String(), "go-lint") { t.Fatalf("search after promote: %d %s", w.Code, w.Body.String()) }

	// 任意用户下载 global/go-lint 内容一致
	w = getWithCookie(t, r, searcher, "/teams/global/skills/go-lint/versions/1.0.0")
	if w.Code != 200 { t.Fatalf("global download: %d %s", w.Code, w.Body.String()) }
	if !bytes.Equal(w.Body.Bytes(), payload) { t.Fatalf("content mismatch: %q", w.Body.String()) }

	// 重复提升同 version → 409
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 409 { t.Fatalf("dup promote: got %d", w.Code) }
}

func TestE2E_Promote_NonAdmin_Forbidden(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("POST", "/admin/skills/promote", owner, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))
	if w.Code != 403 { t.Fatalf("non-admin promote: got %d", w.Code) }
}

func TestE2E_AuditList(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	admin := makePlatformAdmin(t, r)
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/admin/skills/promote", admin, `{"team_slug":"acme","skill_name":"go-lint","version":"1.0.0","target_name":"go-lint"}`))

	// 等 audit 异步落库：轮询
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	for i := 0; i < 50; i++ {
		var n int64
		gdb.Raw("SELECT count(*) FROM audit_logs WHERE action='skill_promoted_to_global'").Scan(&n)
		if n > 0 { break }
		time.Sleep(20 * time.Millisecond)
	}

	w := getWithCookie(t, r, admin, "/admin/audit?action=skill_promoted_to_global")
	if w.Code != 200 { t.Fatalf("audit list: %d %s", w.Code, w.Body.String()) }
	if !contains(w.Body.String(), "skill_promoted_to_global") { t.Fatalf("audit missing action: %s", w.Body.String()) }

	// 非 admin 403
	other := registerAndLogin(t, r, "other@x.com", "password1")
	w = getWithCookie(t, r, other, "/admin/audit")
	if w.Code != 403 { t.Fatalf("non-admin audit: got %d", w.Code) }
}
```
需 import `"time"`、`"github.com/gin-gonic/gin"`。

- [ ] **Step 7: 编译验证**
Run: `go build ./...`

- [ ] **Step 8: 跑 e2e + 全量**
Run: `go test -tags integration ./internal/httpserver/handlers/ && go test ./...`
Expected: PASS

- [ ] **Step 9: 提交**
```bash
git add internal/httpserver cmd/skillhub internal/audit
git commit -m "feat(httpserver): add admin promote-to-global and audit list endpoints"
```

---

## Self-Review 记录

- **Spec 覆盖**：§4.1 PromoteToGlobal→Task 1；§4.2 audit.List→Task 2；§4.3 handlers→Task 3；§7 测试→各任务。覆盖完整。
- **占位符**：无 TBD/TODO。
- **类型一致**：`Repo.GetSkillByID` 新增，接口/mock/实现一致；`audit.Filter`/`Record` 在 Task 2/3 一致；`handlers.Register` 加 auditLog 参数，server.go/main.go 同步；Deps 加 AuditLogger。
- **关键不变量**：提升复制对象（global 自包含）；仅 platform_admin（admin 组 RequireRole）；version 冲突 409；sha 完整性校验。
- 已知小瑕疵：audit.Log 异步，e2e 用轮询等落库；PromoteToGlobal 读全量到 buffer（≤50MiB，可接受）。
