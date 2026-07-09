# SkillHub 技能包发现（子项目 E）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 跨命名空间检索 skill——`GET /skills?q=&page=&page_size=` 返回调用者可见（global + 自己所属团队）的 skill，按 name 全文匹配（PG tsvector + GIN），每条含最新版本摘要，offset 分页。

**Architecture:** 迁移给 skills 加 search_vector 生成列 + GIN 索引。skill.Repo 加 Search（JOIN teams 带 team_slug，FTS + 可见性 team_id IN）。skill.Service 加 Search 包裹为 SearchResult{Skill, TeamSlug, LatestVersion}。handler 挂到 authed 组，用 teamSvc 算可见 teamIDs。

**Tech Stack:** GORM、Gin、PG tsvector/tsquery、复用 team/skill。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A–D 所有 internal 包。
- service/repo 失败返回 `*apperr.Error`；handler 错误经 errors 中间件渲染。
- 单元测试无外部依赖（mock repo）；集成测试 build tag `//go:build integration`。
- 分页：page 从 1 起，page_size 默认 20、上限 100；offset=(page-1)*page_size。
- q 最长 256 字符。
- 可见性：teamIDs = [global] ∪ ListForUser(userID)。
- 每任务结束提交，conventional commits。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `migrations/000005_skills_search.up.sql` / `.down.sql` | search_vector 生成列 + GIN |
| `internal/skill/repo.go` | 加 Search |
| `internal/skill/repo_test.go` | Search 集成测试 |
| `internal/skill/service.go` | 加 Search + SearchResult |
| `internal/skill/service_test.go` | Search 单测 |
| `internal/httpserver/handlers/skills.go` | 加 Search handler |
| `internal/httpserver/handlers/routes.go` | 注册 GET /skills |
| `internal/httpserver/handlers/skills_test.go` | e2e 集成测试 |

---

### Task 1: search_vector 迁移

**Files:**
- Create: `migrations/000005_skills_search.up.sql`
- Create: `migrations/000005_skills_search.down.sql`

- [ ] **Step 1: up**
```sql
ALTER TABLE skills
  ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', name)) STORED;
CREATE INDEX skills_search_idx ON skills USING GIN (search_vector);
```

- [ ] **Step 2: down**
```sql
DROP INDEX IF EXISTS skills_search_idx;
ALTER TABLE skills DROP COLUMN IF EXISTS search_vector;
```

- [ ] **Step 3: 验证**
```bash
make migrate-up
docker compose -f deployments/docker-compose.yml exec -T postgres psql -U skillhub -d skillhub -c "\d skills"
```
Expected: search_vector 列存在，skills_search_idx 索引存在。

- [ ] **Step 4: 提交**
```bash
git add migrations
git commit -m "feat(db): add skills search_vector generated column and GIN index"
```

---

### Task 2: repo.Search + 集成测试

**Files:**
- Modify: `internal/skill/repo.go`
- Modify: `internal/skill/repo_test.go`

- [ ] **Step 1: 加 SearchRow + Search 到 repo.go**

```go
type SearchRow struct {
	Skill
	TeamSlug string
}

// Search 返回 teamIDs 内、name 匹配 q 的 skill，JOIN teams 带 slug。
// q 为空则不做 FTS，按 updated_at DESC 排序。limit 上限 100。
func (r *repo) Search(ctx context.Context, teamIDs []uuid.UUID, q string, limit, offset int) ([]SearchRow, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.TrimSpace(q)
	type row struct {
		Skill
		TeamSlug string `gorm:"column:team_slug"`
	}
	var rows []row
	tx := r.db.WithContext(ctx).Table("skills").
		Select("skills.*, teams.slug AS team_slug").
		Joins("JOIN teams ON teams.id = skills.team_id").
		Where("skills.team_id IN ?", teamIDs)
	if q != "" {
		tsq := "plainto_tsquery('simple', ?)"
		tx = tx.Where("skills.search_vector @@ "+tsq, q).
			Order("ts_rank(skills.search_vector, plainto_tsquery('simple', ?)) DESC", q).
			Order("skills.name ASC")
	} else {
		tx = tx.Order("skills.updated_at DESC").Order("skills.name ASC")
	}
	if err := tx.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("search skills: %w", err)
	}
	out := make([]SearchRow, len(rows))
	for i, rr := range rows {
		out[i] = SearchRow{Skill: rr.Skill, TeamSlug: rr.TeamSlug}
	}
	return out, nil
}
```
加 `Search` 到 `Repo` 接口签名。注意 `SearchRow` 嵌入 `Skill`，GORM 会把 skills.* 列填入 Skill 字段，team_slug 填入 TeamSlug（通过 column tag）。需确认 GORM 对嵌入 struct 的扫描——若不便，改用显式字段 struct。

- [ ] **Step 2: 集成测试**

在 repo_test.go 加：
```go
func TestRepo_Search(t *testing.T) {
	r, tid, oid := setupSkillDB(t)
	ctx := context.Background()
	// 两个 skill：一个含 "lint"，一个含 "format"
	r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "go-lint"})
	r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "go-format"})
	// 命中 lint
	rows, err := r.Search(ctx, []uuid.UUID{tid}, "lint", 100, 0)
	if err != nil { t.Fatal(err) }
	if len(rows) != 1 || rows[0].Name != "go-lint" {
		t.Fatalf("search lint: %+v", rows)
	}
	// 空查询返回全部
	rows, _ = r.Search(ctx, []uuid.UUID{tid}, "", 100, 0)
	if len(rows) != 2 { t.Fatalf("empty q: %d", len(rows)) }
	// team_slug 带回
	if rows[0].TeamSlug != "acme" { t.Fatalf("slug=%s", rows[0].TeamSlug) }
	// 不可见团队：用随机 id
	hidden := uuid.New()
	rows, _ = r.Search(ctx, []uuid.UUID{hidden}, "lint", 100, 0)
	if len(rows) != 0 { t.Fatalf("hidden team leaked: %d", len(rows)) }
	_ = oid
}
```

- [ ] **Step 3: 跑测试**
Run: `go vet ./internal/skill/ && go test -tags integration ./internal/skill/`
Expected: PASS

- [ ] **Step 4: 提交**
```bash
git add internal/skill
git commit -m "feat(skill): add Search repo method with tsvector FTS and team visibility"
```

---

### Task 3: service.Search + 单测

**Files:**
- Modify: `internal/skill/service.go`
- Modify: `internal/skill/service_test.go`

- [ ] **Step 1: 加 SearchResult + Search 到 service.go**

```go
type SearchResult struct {
	Skill
	TeamSlug     string
	LatestVersion *SkillVersion // 无版本时 nil
}

func (s *Service) Search(ctx context.Context, teamIDs []uuid.UUID, q string, page, pageSize int) ([]SearchResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if len(q) > 256 {
		return nil, apperr.New("validation_failed", "skill", "query too long")
	}
	offset := (page - 1) * pageSize
	rows, err := s.repo.Search(ctx, teamIDs, q, pageSize, offset)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, len(rows))
	for i, row := range rows {
		out[i] = SearchResult{Skill: row.Skill, TeamSlug: row.TeamSlug}
		vs, err := s.repo.ListVersions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out[i].LatestVersion = latestVersion(vs)
	}
	return out, nil
}

func latestVersion(vs []SkillVersion) *SkillVersion {
	if len(vs) == 0 {
		return nil
	}
	best := &vs[0]
	for i := 1; i < len(vs); i++ {
		if Compare(vs[i].Version, best.Version) > 0 {
			best = &vs[i]
		}
	}
	return best
}
```

- [ ] **Step 2: mock repo 加 Search**

在 service_test.go 的 mockSkillRepo 加：
```go
func (m *mockSkillRepo) Search(ctx context.Context, teamIDs []uuid.UUID, q string, limit, offset int) ([]SearchRow, error) {
	// 简化：按 name 子串匹配 + teamIDs 过滤
	var out []SearchRow
	for _, sk := range m.skills {
		visible := false
		for _, tid := range teamIDs {
			if sk.TeamID == tid { visible = true; break }
		}
		if !visible { continue }
		if q != "" && !strings.Contains(sk.Name, q) { continue }
		out = append(out, SearchRow{Skill: *sk, TeamSlug: "mock"})
	}
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = nil
	}
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}
```
（需 import "strings"。）

- [ ] **Step 3: 单测**

```go
func TestService_Search_LatestVersion(t *testing.T) {
	s, r, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	_, _ = s.Publish(ctx, tid, "go-lint", "1.2.0", bytes.NewReader([]byte("b")), 1, ContentTypeTarball, pub)
	_, _ = s.Publish(ctx, tid, "go-format", "0.1.0", bytes.NewReader([]byte("c")), 1, ContentTypeTarball, pub)

	res, err := s.Search(ctx, []uuid.UUID{tid}, "lint", 1, 20)
	if err != nil { t.Fatal(err) }
	if len(res) != 1 || res[0].Name != "go-lint" { t.Fatalf("res=%+v", res) }
	if res[0].LatestVersion == nil || res[0].LatestVersion.Version != "1.2.0" {
		t.Fatalf("latest=%+v", res[0].LatestVersion)
	}
}

func TestService_Search_PaginationClamp(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	// page<1 → 1; pageSize>100 → 100
	res, err := s.Search(ctx, []uuid.UUID{tid}, "", 0, 500)
	if err != nil { t.Fatal(err) }
	_ = res
	// q 太长
	longQ := strings.Repeat("a", 257)
	if _, err := s.Search(ctx, []uuid.UUID{tid}, longQ, 1, 20); err == nil {
		t.Fatal("expected query too long")
	}
}
```

- [ ] **Step 4: 跑测试**
Run: `go test ./internal/skill/`
Expected: PASS

- [ ] **Step 5: 提交**
```bash
git add internal/skill
git commit -m "feat(skill): add Service.Search with pagination and latest-version summary"
```

---

### Task 4: handler + route + e2e

**Files:**
- Modify: `internal/httpserver/handlers/skills.go`
- Modify: `internal/httpserver/handlers/routes.go`
- Modify: `internal/httpserver/handlers/skills_test.go`

- [ ] **Step 1: 加 Search handler 到 skills.go**

```go
type searchResultResp struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	TeamSlug    string `json:"team_slug"`
	Name        string `json:"name"`
	LatestVersion *versionResp `json:"latest_version"`
}

func (h *SkillHandlers) Search(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 可见 teamIDs：global + 用户所属团队
	ctx := c.Request.Context()
	teamIDs := []uuid.UUID{}
	if g, err := h.teamSvc.Repo().GetBySlug(ctx, team.GlobalSlug); err == nil {
		teamIDs = append(teamIDs, g.ID)
	}
	if mine, err := h.teamSvc.Repo().ListForUser(ctx, u.ID); err == nil {
		for _, t := range mine {
			teamIDs = append(teamIDs, t.ID)
		}
	}
	results, err := h.svc.Search(ctx, teamIDs, q, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]searchResultResp, len(results))
	for i, r := range results {
		out[i] = searchResultResp{ID: r.ID.String(), TeamID: r.TeamID.String(), TeamSlug: r.TeamSlug, Name: r.Name}
		if r.LatestVersion != nil {
			out[i].LatestVersion = &versionResp{
				ID: r.LatestVersion.ID.String(), Version: r.LatestVersion.Version,
				Size: r.LatestVersion.Size, Sha256: r.LatestVersion.Sha256,
				ContentType: r.LatestVersion.ContentType, Publisher: r.LatestVersion.PublisherUserID.String(),
				CreatedAt: r.LatestVersion.CreatedAt.Format(timeRFC3339),
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "page": page, "page_size": pageSize})
}
```
需 import `"github.com/google/uuid"`（handler 文件）和 `"github.com/skillhub/skillhub/internal/team"`（已有）。

- [ ] **Step 2: 注册路由 routes.go**

在 authed 组内加：
```go
	authed.GET("/skills", skillH.Search)
```
（skillH 已在 Register 内构造。）

- [ ] **Step 3: e2e 测试 skills_test.go**

```go
func TestE2E_SkillSearch_Visibility(t *testing.T) {
	r := setupTeamApp(t)
	owner := registerAndLogin(t, r, "owner@x.com", "password1")
	r.ServeHTTP(httptest.NewRecorder(), reqWithCookie("POST", "/teams", owner, `{"slug":"acme","name":"Acme"}`))
	publishSkill(t, r, owner, "acme", "go-lint", "1.0.0", []byte("a"))
	publishSkill(t, r, owner, "acme", "go-format", "1.0.0", []byte("b"))

	// owner 搜 "lint"：应见到 acme/go-lint
	w := getWithCookie(t, r, owner, "/skills?q=lint")
	if w.Code != 200 { t.Fatalf("search: %d %s", w.Code, w.Body.String()) }
	if !contains(w.Body.String(), "go-lint") || contains(w.Body.String(), "go-format") {
		t.Fatalf("search results: %s", w.Body.String())
	}

	// 非成员 other 搜 "lint"：不应见到 acme 的私有 skill
	other := registerAndLogin(t, r, "other@x.com", "password1")
	w = getWithCookie(t, r, other, "/skills?q=lint")
	if w.Code != 200 { t.Fatalf("search: %d", w.Code) }
	if contains(w.Body.String(), "go-lint") {
		t.Fatalf("private skill leaked to non-member: %s", w.Body.String())
	}
}

func TestE2E_SkillSearch_GlobalVisible(t *testing.T) {
	r := setupTeamApp(t)
	// 造一个 global skill（直插库，因 API 禁止发布到 global）
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	store, _ := storage.New(cfg.Storage)
	var globalID, userID, skillID string
	gdb.Raw("SELECT id::text FROM teams WHERE slug='global'").Scan(&globalID)
	gdb.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('pub2@x.com','pub2','x','user','active') RETURNING id::text").Scan(&userID)
	gdb.Raw("INSERT INTO skills(team_id,name) VALUES(?,'global-lint') RETURNING id::text", globalID).Scan(&skillID)
	payload := []byte("x")
	sha := sha256Hex(payload)
	key := "skills/" + skillID + "/1.0.0/" + sha + ".tar.gz"
	store.Put(context.Background(), key, bytes.NewReader(payload), 1, "application/gzip")
	gdb.Exec("INSERT INTO skill_versions(skill_id,version,storage_key,size,sha256,content_type,publisher_user_id) VALUES(?,?,?,?,?,?,?)", skillID, "1.0.0", key, 1, sha, "application/gzip", userID)

	// 任意认证用户搜 "lint"：应见到 global-lint
	u := registerAndLogin(t, r, "searcher@x.com", "password1")
	w := getWithCookie(t, r, u, "/skills?q=lint")
	if w.Code != 200 { t.Fatalf("search: %d %s", w.Code, w.Body.String()) }
	if !contains(w.Body.String(), "global-lint") {
		t.Fatalf("global skill not in results: %s", w.Body.String())
	}
}
```

- [ ] **Step 4: 编译验证**
Run: `go build ./...`

- [ ] **Step 5: 跑 e2e**
Run: `make compose-up && make migrate-up && go test -tags integration ./internal/httpserver/handlers/`
Expected: PASS

- [ ] **Step 6: 跑全部单测**
Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: 提交**
```bash
git add internal/httpserver
git commit -m "feat(httpserver): add GET /skills search endpoint with visibility"
```

---

## Self-Review 记录

- **Spec 覆盖**：§3 生成列→Task 1；§4.1 repo.Search→Task 2；§4.2 service.Search→Task 3；§4.3 handler→Task 4；§7 测试→Task 2/3/4。覆盖完整。
- **占位符**：无 TBD/TODO。
- **类型一致**：`SearchRow`/`SearchResult` 在 Task 2/3/4 一致；repo.Search 签名在接口与实现一致；mock repo Search 与接口一致。
- **可见性**：handler 用 teamSvc.Repo().GetBySlug(global) + ListForUser 合并 teamIDs，传给 service；repo 用 `team_id IN ?` 过滤。非成员 teamID 不在集合 → 不可见。
- **FTS**：`plainto_tsquery('simple', ?)` 对用户输入安全（不解析操作符）；q 超长由 service 拦截。
- 已知小瑕疵：LatestVersion 用 N+1 ListVersions（页 ≤100，可接受）；total 未返回（spec 已注明）。
