# SkillHub 社交（子项目 G）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** skill 级收藏（star）——幂等 star/unstar、GET /me/stars、skill 详情带 star_count/is_starred。可见即可 star（TeamScoped(member)）。

**Architecture:** 迁移加 skill_stars。skill.Repo 加 star 方法。skill.Service 加 Star/Unstar/GetSkillDetail/ListMyStars（取代 GetSkillWithVersions）。handlers 加 star/unstar/list-my-stars，GetSkill 改用 GetSkillDetail。

**Tech Stack:** GORM、Gin、复用 team/skill/auth。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A–F。
- service/repo 失败返回 `*apperr.Error`；handler 错误经 errors 中间件。
- 单元测试无外部依赖（mock repo）；集成测试 build tag `//go:build integration`。
- star 幂等：`ON CONFLICT DO NOTHING`；unstar 不要求 RowsAffected。
- star/unstar 不落审计（频繁，避免噪音）。
- 可见性：TeamScoped(member)（global 任意认证）。
- 每任务结束提交，conventional commits。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `migrations/000006_skill_stars.up.sql` / `.down.sql` | skill_stars 表 |
| `internal/skill/repo.go` | star 方法 |
| `internal/skill/repo_test.go` | star 集成测试 |
| `internal/skill/service.go` | Star/Unstar/GetSkillDetail/ListMyStars |
| `internal/skill/service_test.go` | 单测 |
| `internal/httpserver/handlers/skills.go` | star/unstar/list-my-stars handler + GetSkill 改造 |
| `internal/httpserver/handlers/routes.go` | 注册路由 |
| `internal/httpserver/handlers/skills_test.go` | e2e |

---

### Task 1: skill_stars 迁移

- [ ] **up** `migrations/000006_skill_stars.up.sql`:
```sql
CREATE TABLE skill_stars (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id   UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, skill_id)
);
CREATE INDEX skill_stars_skill_idx ON skill_stars(skill_id);
```
- [ ] **down** `migrations/000006_skill_stars.down.sql`:
```sql
DROP TABLE IF EXISTS skill_stars;
```
- [ ] **验证**：`make migrate-up` 后 skill_stars 表存在。
- [ ] **提交**：`git add migrations && git commit -m "feat(db): add skill_stars table"`

---

### Task 2: repo star 方法 + 集成测试

- [ ] **加到 Repo 接口 + 实现**：
```go
Star(ctx, userID, skillID uuid.UUID) error
Unstar(ctx, userID, skillID uuid.UUID) error
IsStarred(ctx, userID, skillID uuid.UUID) (bool, error)
CountStars(ctx, skillID uuid.UUID) (int64, error)
ListStarredSkills(ctx, userID uuid.UUID, limit, offset int) ([]SearchRow, error)
```
实现：
```go
func (r *repo) Star(ctx context.Context, userID, skillID uuid.UUID) error {
	return r.db.WithContext(ctx).Exec("INSERT INTO skill_stars(user_id, skill_id) VALUES (?, ?) ON CONFLICT DO NOTHING", userID, skillID).Error
}
func (r *repo) Unstar(ctx context.Context, userID, skillID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND skill_id = ?", userID, skillID).Delete(&skillStar{}).Error
}
func (r *repo) IsStarred(ctx context.Context, userID, skillID uuid.UUID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("skill_stars").Where("user_id = ? AND skill_id = ?", userID, skillID).Count(&n).Error; err != nil {
		return false, fmt.Errorf("is starred: %w", err)
	}
	return n > 0, nil
}
func (r *repo) CountStars(ctx context.Context, skillID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("skill_stars").Where("skill_id = ?", skillID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count stars: %w", err)
	}
	return n, nil
}
func (r *repo) ListStarredSkills(ctx context.Context, userID uuid.UUID, limit, offset int) ([]SearchRow, error) {
	if limit <= 0 || limit > 100 { limit = 20 }
	if offset < 0 { offset = 0 }
	type searchRow struct {
		Skill
		TeamSlug string `gorm:"column:team_slug"`
	}
	var rows []searchRow
	if err := r.db.WithContext(ctx).Table("skill_stars").
		Select("skills.*, teams.slug AS team_slug").
		Joins("JOIN skills ON skills.id = skill_stars.skill_id").
		Joins("JOIN teams ON teams.id = skills.team_id").
		Where("skill_stars.user_id = ?", userID).
		Order("skill_stars.created_at DESC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list starred: %w", err)
	}
	out := make([]SearchRow, len(rows))
	for i, rr := range rows {
		out[i] = SearchRow{Skill: rr.Skill, TeamSlug: rr.TeamSlug}
	}
	return out, nil
}
```
加 `type skillStar struct{ UserID uuid.UUID; SkillID uuid.UUID; CreatedAt time.Time }` + `func (skillStar) TableName() string { return "skill_stars" }`（仅给 Delete 用；或直接用 Table("skill_stars") 避免 model）。**决定**：Unstar 用 `Table("skill_stars").Where(...).Delete(...)` 不需 model。

- [ ] **集成测试**：star/unstar 幂等、CountStars、ListStarredSkills。
- [ ] **mock repo 加 star 方法**（service_test 编译需要）。
- [ ] **跑**：`go test -tags integration ./internal/skill/`
- [ ] **提交**：`git add internal/skill && git commit -m "feat(skill): add star repository methods"`

---

### Task 3: service Star/Unstar/GetSkillDetail/ListMyStars + 单测

- [ ] **service.go**：
```go
type SkillDetail struct {
	Skill
	Versions  []SkillVersion
	StarCount int64
	IsStarred bool
}

func (s *Service) GetSkillDetail(ctx context.Context, teamID uuid.UUID, name string, viewerID uuid.UUID) (*SkillDetail, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil { return nil, err }
	vs, err := s.repo.ListVersions(ctx, sk.ID)
	if err != nil { return nil, err }
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i].Version, vs[j].Version) > 0 })
	count, _ := s.repo.CountStars(ctx, sk.ID)
	starred, _ := s.repo.IsStarred(ctx, viewerID, sk.ID)
	return &SkillDetail{Skill: *sk, Versions: vs, StarCount: count, IsStarred: starred}, nil
}

func (s *Service) Star(ctx context.Context, userID, teamID uuid.UUID, name string) error {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil { return err }
	return s.repo.Star(ctx, userID, sk.ID)
}
func (s *Service) Unstar(ctx context.Context, userID, teamID uuid.UUID, name string) error {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil { return err }
	return s.repo.Unstar(ctx, userID, sk.ID)
}
func (s *Service) ListMyStars(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]SearchResult, error) {
	if page < 1 { page = 1 }
	if pageSize < 1 { pageSize = 20 }
	if pageSize > 100 { pageSize = 100 }
	offset := (page - 1) * pageSize
	rows, err := s.repo.ListStarredSkills(ctx, userID, pageSize, offset)
	if err != nil { return nil, err }
	out := make([]SearchResult, len(rows))
	for i, row := range rows {
		out[i] = SearchResult{Skill: row.Skill, TeamSlug: row.TeamSlug}
		vs, err := s.repo.ListVersions(ctx, row.ID)
		if err != nil { return nil, err }
		out[i].LatestVersion = latestVersion(vs)
	}
	return out, nil
}
```
删除 `GetSkillWithVersions`（仅 D handler 用，Task 4 改 handler）。

- [ ] **单测**：Star 幂等（mock）；GetSkillDetail 字段；ListMyStars 分页夹取。
- [ ] **跑**：`go test ./internal/skill/`
- [ ] **提交**：`git add internal/skill && git commit -m "feat(skill): add Star/Unstar/GetSkillDetail/ListMyStars service methods"`

---

### Task 4: handlers + routes + e2e

- [ ] **skills.go**：
  - GetSkill 改用 GetSkillDetail，响应加 `star_count`、`is_starred`。
  - 加 Star/Unstar/ListMyStars handler。
```go
func (h *SkillHandlers) Star(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	if err := h.svc.Star(c.Request.Context(), u.ID, t.ID, c.Param("name")); err != nil {
		c.Error(err); return
	}
	c.Status(http.StatusNoContent)
}
func (h *SkillHandlers) Unstar(c *gin.Context) { /* 同上，svc.Unstar */ }

func (h *SkillHandlers) ListMyStars(c *gin.Context) {
	u, _ := auth.CurrentUser(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	res, err := h.svc.ListMyStars(c.Request.Context(), u.ID, page, pageSize)
	if err != nil { c.Error(err); return }
	// 复用 searchResultResp 序列化
	out := make([]searchResultResp, len(res))
	for i, r := range res { /* 同 Search handler */ }
	c.JSON(http.StatusOK, gin.H{"items": out, "page": page, "page_size": pageSize})
}
```
- [ ] **routes.go**：
```go
	skillGroup.POST("/:name/star", auth.TeamScoped(teamSvc, "member"), skillH.Star)
	skillGroup.DELETE("/:name/star", auth.TeamScoped(teamSvc, "member"), skillH.Unstar)
	authed.GET("/me/stars", skillH.ListMyStars)
```
- [ ] **e2e**：成员 star → 详情 is_starred=true/count=1 → 再 star 204 count=1 → unstar → is_starred=false → 非成员 star 403 → GET /me/stars 含该 skill → global skill 任意认证可 star。
- [ ] **跑**：`go build ./... && go test -tags integration ./internal/httpserver/handlers/ && go test ./...`
- [ ] **提交**：`git add internal/httpserver && git commit -m "feat(httpserver): add skill star/unstar and my-stars endpoints"`

---

## Self-Review 记录

- **Spec 覆盖**：§3 表→Task 1；§4.1 repo→Task 2；§4.2 service→Task 3；§4.3/4.4 handler/route→Task 4；§7 测试→各任务。
- **类型一致**：`SearchRow` 复用；`SkillDetail` 新增；`GetSkillWithVersions` 删除，D handler 改用 GetSkillDetail；mock repo star 方法与接口一致。
- **关键不变量**：star 幂等（ON CONFLICT DO NOTHING）；可见即 star（TeamScoped(member)）；star 不落审计。
- 已知小瑕疵：GetSkillDetail 的 CountStars/IsStarred 错误被忽略（计数非关键，返回 0/false 兜底）。
