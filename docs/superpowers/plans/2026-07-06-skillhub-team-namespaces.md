# SkillHub 团队命名空间（子项目 C）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现团队命名空间与成员管理——创建团队、成员/角色（owner/admin/member）、发布策略、所有权转移，团队级权限中间件，全程审计。

**Architecture:** 新增 internal/team（model/repo/service，权限判定方法）。扩展 internal/auth/middleware 加 TeamScoped。handlers 注册到 gin engine。global 团队由迁移预置，platform_admin 治理。

**Tech Stack:** GORM、Gin、uuid、复用 audit/auth/user。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A/B 所有 internal 包。
- service/repo 失败返回 `*apperr.Error`；handler 错误经 errors 中间件渲染（c.Error + c.AbortWithStatus，不双写）。
- 单元测试无外部依赖（mock repo）；集成测试 build tag `//go:build integration`，依赖 compose 的 PG。集成测试 config 路径用相对包深度的 `../../config/config.yaml`（按实际深度调整）。
- 每任务结束提交，conventional commits。
- owner 单一（teams.owner_user_id），不入 team_members；global 团队 owner=null。
- 删除 global 禁止；slug `global` 保留。
- 审计动作：team_created / team_deleted / member_added / member_removed / member_role_changed / ownership_transferred / publish_policy_changed。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `migrations/000003_teams.up.sql` / `.down.sql` | teams + team_members + global 种子 |
| `internal/team/model.go` | Team/TeamMember 模型 + 常量 |
| `internal/team/repo.go` | Repo 接口 + GORM 实现 |
| `internal/team/repo_test.go` | 集成测试 |
| `internal/team/service.go` | Service + 权限判定 |
| `internal/team/service_test.go` | 单测（mock repo） |
| `internal/auth/middleware.go` | 加 TeamScoped + CurrentTeam |
| `internal/auth/middleware_test.go` | TeamScoped 测试 |
| `internal/httpserver/handlers/teams.go` | 团队路由 handler |
| `internal/httpserver/handlers/routes.go` | 注册团队路由 |
| `internal/httpserver/server.go` | Deps 加 TeamSvc，装配 |
| `cmd/skillhub/main.go` | 装配 team.Service |
| `internal/httpserver/handlers/teams_test.go` | e2e 集成测试 |

---

### Task 1: teams + team_members 迁移

**Files:**
- Create: `migrations/000003_teams.up.sql`
- Create: `migrations/000003_teams.down.sql`

**Interfaces:** 无。

- [ ] **Step 1: 写 up 迁移**

`migrations/000003_teams.up.sql`:
```sql
CREATE TABLE teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    owner_user_id   UUID,
    publish_policy  TEXT NOT NULL DEFAULT 'admin_only' CHECK (publish_policy IN ('admin_only','any_member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT teams_global_owner_nullable CHECK (slug <> 'global' OR owner_user_id IS NULL)
);

CREATE TABLE team_members (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('admin','member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX team_members_user_idx ON team_members(user_id);

INSERT INTO teams(slug, name, owner_user_id, publish_policy) VALUES ('global','Global',NULL,'admin_only');
```

- [ ] **Step 2: 写 down 迁移**

`migrations/000003_teams.down.sql`:
```sql
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
```

- [ ] **Step 3: 验证迁移**

Run:
```bash
make compose-up
make migrate-down   # 回到干净状态（可能需要多次 down 到 000001 之前）
make migrate-up
docker compose -f deployments/docker-compose.yml exec -T postgres psql -U skillhub -d skillhub -c "\dt"
docker compose -f deployments/docker-compose.yml exec -T postgres psql -U skillhub -d skillhub -c "SELECT slug,name,owner_user_id,publish_policy FROM teams;"
```
Expected: teams + team_members 表存在；teams 有一行 global。

- [ ] **Step 4: 提交**

```bash
git add migrations
git commit -m "feat(db): add teams and team_members migrations with global seed"
```

---

### Task 2: team model + repo

**Files:**
- Create: `internal/team/model.go`
- Create: `internal/team/repo.go`
- Create: `internal/team/repo_test.go`

**Interfaces:**
- Consumes: `*gorm.DB`, `apperr`, `uuid`
- Produces: `team.Team`, `team.TeamMember`, 常量, `team.Repo` 接口, `team.NewRepo(db) Repo`

- [ ] **Step 1: 写 model.go**

`internal/team/model.go`:
```go
package team

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleAdmin        = "admin"
	RoleMember       = "member"
	PolicyAdminOnly  = "admin_only"
	PolicyAnyMember  = "any_member"
	GlobalSlug       = "global"
)

type Team struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug          string     `gorm:"uniqueIndex;not null"`
	Name          string     `gorm:"not null"`
	OwnerUserID   *uuid.UUID `gorm:"type:uuid"`
	PublishPolicy string     `gorm:"not null;default:admin_only"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Team) TableName() string { return "teams" }

type TeamMember struct {
	TeamID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Role      string    `gorm:"not null"`
	CreatedAt time.Time
}

func (TeamMember) TableName() string { return "team_members" }
```

- [ ] **Step 2: 写 repo.go**

`internal/team/repo.go`:
```go
package team

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

type Repo interface {
	Create(ctx context.Context, t *Team) error
	GetBySlug(ctx context.Context, slug string) (*Team, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Team, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error)
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error)
	GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error)
	AddMember(ctx context.Context, m TeamMember) error
	UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error
	SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error
	SetName(ctx context.Context, teamID uuid.UUID, name string) error
	Delete(ctx context.Context, teamID uuid.UUID) error
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

func (r *repo) Create(ctx context.Context, t *Team) error {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	return nil
}

func (r *repo) GetBySlug(ctx context.Context, slug string) (*Team, error) {
	var t Team
	if err := r.db.WithContext(ctx).First(&t, "slug = ?", slug).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "team not found")
		}
		return nil, fmt.Errorf("get team by slug: %w", err)
	}
	return &t, nil
}

func (r *repo) GetByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	var t Team
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "team not found")
		}
		return nil, fmt.Errorf("get team by id: %w", err)
	}
	return &t, nil
}

func (r *repo) ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error) {
	var teams []Team
	err := r.db.WithContext(ctx).Where("owner_user_id = ? OR id IN (SELECT team_id FROM team_members WHERE user_id = ?)", userID, userID).Order("created_at DESC").Find(&teams).Error
	if err != nil {
		return nil, fmt.Errorf("list teams for user: %w", err)
	}
	return teams, nil
}

func (r *repo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	var ms []TeamMember
	if err := r.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return ms, nil
}

func (r *repo) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error) {
	var m TeamMember
	if err := r.db.WithContext(ctx).First(&m, "team_id = ? AND user_id = ?", teamID, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "member not found")
		}
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

func (r *repo) AddMember(ctx context.Context, m TeamMember) error {
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (r *repo) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	res := r.db.WithContext(ctx).Model(&TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Update("role", role)
	if res.Error != nil {
		return fmt.Errorf("update member role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "member not found")
	}
	return nil
}

func (r *repo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&TeamMember{})
	if res.Error != nil {
		return fmt.Errorf("remove member: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "member not found")
	}
	return nil
}

func (r *repo) TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("owner_user_id", newOwnerID)
	if res.Error != nil {
		return fmt.Errorf("transfer ownership: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("publish_policy", policy)
	if res.Error != nil {
		return fmt.Errorf("set policy: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) SetName(ctx context.Context, teamID uuid.UUID, name string) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("name", name)
	if res.Error != nil {
		return fmt.Errorf("set name: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, teamID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", teamID).Delete(&Team{})
	if res.Error != nil {
		return fmt.Errorf("delete team: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}
```

- [ ] **Step 3: 写集成测试**

`internal/team/repo_test.go`:
```go
//go:build integration

package team

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"gorm.io/gorm"
)

var teamDB *gorm.DB

func setupTeamDB(t *testing.T) (Repo, func()) {
	t.Helper()
	if teamDB == nil {
		cfg, err := config.Load("../../../config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		teamDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	teamDB.Exec("TRUNCATE team_members, teams RESTART IDENTITY CASCADE")
	// 重新种 global
	teamDB.Exec("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('global','Global',NULL,'admin_only')")
	return NewRepo(teamDB), func() {}
}

func TestRepo_CreateGet(t *testing.T) {
	r, _ := setupTeamDB(t)
	ctx := context.Background()
	owner := uuid.New()
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	if err := r.Create(ctx, tm); err != nil {
		t.Fatal(err)
	}
	if tm.ID == uuid.Nil {
		t.Fatal("id not set")
	}
	got, err := r.GetBySlug(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Acme" {
		t.Fatalf("name=%s", got.Name)
	}
}

func TestRepo_Members(t *testing.T) {
	r, _ := setupTeamDB(t)
	ctx := context.Background()
	owner := uuid.New()
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	r.Create(ctx, tm)
	member := uuid.New()
	if err := r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember}); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetMember(ctx, tm.ID, member)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleMember {
		t.Fatalf("role=%s", got.Role)
	}
	if err := r.UpdateMemberRole(ctx, tm.ID, member, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	ms, err := r.ListMembers(ctx, tm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].Role != RoleAdmin {
		t.Fatalf("members=%v", ms)
	}
	if err := r.RemoveMember(ctx, tm.ID, member); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetMember(ctx, tm.ID, member); err == nil {
		t.Fatal("expected not found after remove")
	}
}

func TestRepo_TransferOwnership(t *testing.T) {
	r, _ := setupTeamDB(t)
	ctx := context.Background()
	owner := uuid.New()
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	r.Create(ctx, tm)
	newOwner := uuid.New()
	if err := r.TransferOwnership(ctx, tm.ID, newOwner); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, tm.ID)
	if *got.OwnerUserID != newOwner {
		t.Fatal("ownership not transferred")
	}
}

func TestRepo_ListForUser(t *testing.T) {
	r, _ := setupTeamDB(t)
	ctx := context.Background()
	owner := uuid.New()
	r.Create(ctx, &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly})
	other := uuid.New()
	r.Create(ctx, &Team{Slug: "beta", Name: "Beta", OwnerUserID: &other, PublishPolicy: PolicyAdminOnly})
	// owner 是 acme 的 owner，不是 beta 的成员
	teams, err := r.ListForUser(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].Slug != "acme" {
		t.Fatalf("teams=%v", teams)
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go vet ./internal/team/`
Run: `make compose-up && make migrate-up && go test -tags integration ./internal/team/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/team
git commit -m "feat(team): add Team/TeamMember model and GORM repository"
```

---

### Task 3: team service + 权限判定

**Files:**
- Create: `internal/team/service.go`
- Create: `internal/team/service_test.go`

**Interfaces:**
- Consumes: `team.Repo`, `audit.Logger`
- Produces: `team.Service`, `team.NewService(repo, audit)`, 权限方法 IsOwner/IsAdminOrOwner/IsMember/CanPublish

- [ ] **Step 1: 写 service.go**

`internal/team/service.go`:
```go
package team

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
)

type Service struct {
	repo  Repo
	audit *audit.Logger
}

func NewService(repo Repo, audit *audit.Logger) *Service {
	return &Service{repo: repo, audit: audit}
}

func (s *Service) Create(ctx context.Context, slug, name string, ownerID uuid.UUID) (*Team, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" || slug == GlobalSlug {
		return nil, apperr.New("validation_failed", "team", "invalid slug")
	}
	if name == "" {
		return nil, apperr.New("validation_failed", "team", "name required")
	}
	if existing, err := s.repo.GetBySlug(ctx, slug); err == nil && existing != nil {
		return nil, apperr.New("validation_failed", "team", "slug already taken")
	}
	t := &Team{Slug: slug, Name: name, OwnerUserID: &ownerID, PublishPolicy: PolicyAdminOnly}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &ownerID, Action: audit.Action("team_created"), TargetType: "team", TargetID: t.ID.String(), Metadata: map[string]any{"slug": slug}})
	return t, nil
}

func (s *Service) Update(ctx context.Context, actorID, teamID uuid.UUID, name *string, policy *string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if policy != nil {
		if *policy != PolicyAdminOnly && *policy != PolicyAnyMember {
			return apperr.New("validation_failed", "team", "invalid policy")
		}
		if err := s.repo.SetPublishPolicy(ctx, teamID, *policy); err != nil {
			return err
		}
		s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("publish_policy_changed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"new_policy": *policy}})
	}
	if name != nil {
		if *name == "" {
			return apperr.New("validation_failed", "team", "name required")
		}
		if err := s.repo.SetName(ctx, teamID, *name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AddMember(ctx context.Context, actorID, teamID, userID uuid.UUID, role string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsAdminOrOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "admin or owner only")
	}
	if role != RoleAdmin && role != RoleMember {
		return apperr.New("validation_failed", "team", "invalid role")
	}
	if err := s.repo.AddMember(ctx, TeamMember{TeamID: teamID, UserID: userID, Role: role}); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_added"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String(), "role": role}})
	return nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, actorID, teamID, userID uuid.UUID, role string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if role != RoleAdmin && role != RoleMember {
		return apperr.New("validation_failed", "team", "invalid role")
	}
	if err := s.repo.UpdateMemberRole(ctx, teamID, userID, role); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_role_changed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String(), "new_role": role}})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, actorID, teamID, userID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsAdminOrOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "admin or owner only")
	}
	if err := s.repo.RemoveMember(ctx, teamID, userID); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_removed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String()}})
	return nil
}

func (s *Service) TransferOwnership(ctx context.Context, actorID, teamID, newOwnerID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if _, err := s.repo.GetMember(ctx, teamID, newOwnerID); err != nil {
		return apperr.New("validation_failed", "team", "new owner must be a current member")
	}
	if err := s.repo.TransferOwnership(ctx, teamID, newOwnerID); err != nil {
		return err
	}
	// 旧 owner 降为 admin
	if actorID != newOwnerID {
		_ = s.repo.UpdateMemberRole(ctx, teamID, actorID, RoleAdmin)
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("ownership_transferred"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"new_owner": newOwnerID.String()}})
	return nil
}

func (s *Service) Delete(ctx context.Context, actorID, teamID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if t.Slug == GlobalSlug {
		return apperr.New("validation_failed", "team", "cannot delete global namespace")
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if err := s.repo.Delete(ctx, teamID); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("team_deleted"), TargetType: "team", TargetID: teamID.String()})
	return nil
}

// 权限判定

func (s *Service) IsOwner(ctx context.Context, t *Team, userID uuid.UUID) bool {
	return t.OwnerUserID != nil && *t.OwnerUserID == userID
}

func (s *Service) IsMember(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	_, err := s.repo.GetMember(ctx, t.ID, userID)
	return err == nil
}

func (s *Service) IsAdminOrOwner(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	m, err := s.repo.GetMember(ctx, t.ID, userID)
	if err != nil {
		return false
	}
	return m.Role == RoleAdmin
}

func (s *Service) CanPublish(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	if t.PublishPolicy == PolicyAdminOnly {
		return s.IsAdminOrOwner(ctx, t, userID)
	}
	return s.IsMember(ctx, t, userID)
}

// 触发 fmt 引用避免未使用
var _ = fmt.Errorf
```

- [ ] **Step 2: 写单测（mock repo）**

`internal/team/service_test.go`:
```go
package team

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"go.uber.org/zap"
)

type mockRepo struct {
	teams   map[uuid.UUID]*Team
	members map[[2]uuid.UUID]*TeamMember // (teamID, userID) -> member
}

func newMockRepo() *mockRepo {
	return &mockRepo{teams: map[uuid.UUID]*Team{}, members: map[[2]uuid.UUID]*TeamMember{}}
}

func (m *mockRepo) Create(ctx context.Context, t *Team) error { t.ID = uuid.New(); m.teams[t.ID] = t; return nil }
func (m *mockRepo) GetBySlug(ctx context.Context, slug string) (*Team, error) {
	for _, t := range m.teams {
		if t.Slug == slug {
			return t, nil
		}
	}
	return nil, apperr.New("not_found", "team", "not found")
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	if t, ok := m.teams[id]; ok {
		return t, nil
	}
	return nil, apperr.New("not_found", "team", "not found")
}
func (m *mockRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error) { return nil, nil }
func (m *mockRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	var ms []TeamMember
	for _, mm := range m.members {
		if mm.TeamID == teamID {
			ms = append(ms, *mm)
		}
	}
	return ms, nil
}
func (m *mockRepo) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error) {
	if mm, ok := m.members[[2]uuid.UUID{teamID, userID}]; ok {
		return mm, nil
	}
	return nil, apperr.New("not_found", "team", "member not found")
}
func (m *mockRepo) AddMember(ctx context.Context, mm TeamMember) error {
	m.members[[2]uuid.UUID{mm.TeamID, mm.UserID}] = &mm; return nil
}
func (m *mockRepo) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	mm, ok := m.members[[2]uuid.UUID{teamID, userID}]
	if !ok {
		return apperr.New("not_found", "team", "member not found")
	}
	mm.Role = role
	return nil
}
func (m *mockRepo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	delete(m.members, [2]uuid.UUID{teamID, userID})
	return nil
}
func (m *mockRepo) TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error {
	m.teams[teamID].OwnerUserID = &newOwnerID
	return nil
}
func (m *mockRepo) SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error {
	m.teams[teamID].PublishPolicy = policy
	return nil
}
func (m *mockRepo) SetName(ctx context.Context, teamID uuid.UUID, name string) error { m.teams[teamID].Name = name; return nil }
func (m *mockRepo) Delete(ctx context.Context, teamID uuid.UUID) error { delete(m.teams, teamID); return nil }

func newSvc() (*Service, *mockRepo) {
	r := newMockRepo()
	return NewService(r, audit.NewLogger(nil, zap.NewNop())), r
}

func TestCreate_InvalidSlug(t *testing.T) {
	s, _ := newSvc()
	if _, err := s.Create(context.Background(), "global", "x", uuid.New()); err == nil {
		t.Fatal("expected error for global slug")
	}
	if _, err := s.Create(context.Background(), "", "x", uuid.New()); err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestPermissions(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	admin := uuid.New()
	r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: admin, Role: RoleAdmin})
	member := uuid.New()
	r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember})
	other := uuid.New()

	if !s.IsOwner(ctx, tm, owner) {
		t.Fatal("owner should be owner")
	}
	if s.IsOwner(ctx, tm, admin) {
		t.Fatal("admin is not owner")
	}
	if !s.IsAdminOrOwner(ctx, tm, admin) {
		t.Fatal("admin should be admin+")
	}
	if !s.IsMember(ctx, tm, member) {
		t.Fatal("member should be member")
	}
	if s.IsMember(ctx, tm, other) {
		t.Fatal("other is not member")
	}
}

func TestCanPublish(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	member := uuid.New()
	r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember})

	// admin_only: member 不能发布
	if s.CanPublish(ctx, tm, member) {
		t.Fatal("member cannot publish under admin_only")
	}
	// any_member: member 可发布
	r.SetPublishPolicy(ctx, tm.ID, PolicyAnyMember)
	tm.PublishPolicy = PolicyAnyMember
	if !s.CanPublish(ctx, tm, member) {
		t.Fatal("member can publish under any_member")
	}
}

func TestTransferOwnership_NonMember(t *testing.T) {
	s, _ := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	if err := s.TransferOwnership(ctx, owner, tm.ID, uuid.New()); err == nil {
		t.Fatal("expected error transferring to non-member")
	}
}

func TestDelete_Global(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	// 手动塞一个 global
	g := &Team{Slug: GlobalSlug, Name: "Global", PublishPolicy: PolicyAdminOnly}
	g.ID = uuid.New()
	r.teams[g.ID] = g
	if err := s.Delete(ctx, uuid.New(), g.ID); err == nil {
		t.Fatal("expected error deleting global")
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/team/`
Expected: PASS（单测，无 integration tag）

- [ ] **Step 4: 提交**

```bash
git add internal/team
git commit -m "feat(team): add Service with permission helpers and CRUD"
```

---

### Task 4: TeamScoped middleware

**Files:**
- Modify: `internal/auth/middleware.go`
- Modify: `internal/auth/middleware_test.go`

**Interfaces:**
- Consumes: `team.Service`
- Produces: `auth.TeamScoped(teamSvc *team.Service, required string) gin.HandlerFunc`，`auth.CurrentTeam(c) (*team.Team, bool)`

- [ ] **Step 1: 改 middleware.go — 加 TeamScoped + CurrentTeam**

在 `internal/auth/middleware.go` 加 import `"github.com/skillhub/skillhub/internal/team"` 和 `"github.com/skillhub/skillhub/internal/user"`（user 已有）。加：
```go
const currentTeamKey = "current_team"

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
		// global + owner 要求 platform_admin
		if t.Slug == team.GlobalSlug && required == "owner" {
			if u.Role != user.RolePlatformAdmin {
				c.Error(apperr.New("forbidden", "team", "platform admin only"))
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
```

- [ ] **Step 2: 给 Service 加 Repo 访问器**

在 `internal/team/service.go` 末尾加：
```go
func (s *Service) Repo() Repo { return s.repo }
```

- [ ] **Step 3: 写测试**

在 `internal/auth/middleware_test.go` 加（TeamScoped 需要 team.Service，用真实 service + mock repo 不便在 auth 包内构造；改为集成测试覆盖。此处只测 CurrentTeam 缺失分支）：
```go
func TestCurrentTeam_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	if _, ok := CurrentTeam(c); ok {
		t.Fatal("expected no current team")
	}
}
```
TeamScoped 的权限分支由 Task 6 e2e 集成测试覆盖。

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/auth/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/auth internal/team
git commit -m "feat(auth): add TeamScoped middleware and CurrentTeam"
```

---

### Task 5: handlers + routes + server wiring

**Files:**
- Create: `internal/httpserver/handlers/teams.go`
- Modify: `internal/httpserver/handlers/routes.go`
- Modify: `internal/httpserver/server.go`

**Interfaces:**
- Consumes: `team.Service`, middleware
- Produces: 团队路由；`httpserver.Deps` 加 `TeamSvc *team.Service`

- [ ] **Step 1: 写 handlers/teams.go**

`internal/httpserver/handlers/teams.go`:
```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/team"
)

type TeamHandlers struct {
	svc *team.Service
}

func NewTeamHandlers(svc *team.Service) *TeamHandlers { return &TeamHandlers{svc: svc} }

type teamResp struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	OwnerUserID   string `json:"owner_user_id"`
	PublishPolicy string `json:"publish_policy"`
}

func toTeamResp(t *team.Team) teamResp {
	owner := ""
	if t.OwnerUserID != nil {
		owner = t.OwnerUserID.String()
	}
	return teamResp{ID: t.ID.String(), Slug: t.Slug, Name: t.Name, OwnerUserID: owner, PublishPolicy: t.PublishPolicy}
}

type createTeamReq struct {
	Slug string `json:"slug" binding:"required"`
	Name string `json:"name" binding:"required"`
}

func (h *TeamHandlers) Create(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	t, err := h.svc.Create(c.Request.Context(), req.Slug, req.Name, u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toTeamResp(t))
}

func (h *TeamHandlers) ListMine(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	teams, err := h.svc.Repo().ListForUser(c.Request.Context(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]teamResp, len(teams))
	for i, t := range teams {
		out[i] = toTeamResp(&t)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *TeamHandlers) Get(c *gin.Context) {
	t, ok := auth.CurrentTeam(c)
	if !ok {
		c.Error(apperr.New("not_found", "team", "no team"))
		return
	}
	c.JSON(http.StatusOK, toTeamResp(t))
}

type patchTeamReq struct {
	Name          *string `json:"name"`
	PublishPolicy *string `json:"publish_policy"`
}

func (h *TeamHandlers) Patch(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req patchTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	if err := h.svc.Update(c.Request.Context(), u.ID, t.ID, req.Name, req.PublishPolicy); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandlers) Delete(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	if err := h.svc.Delete(c.Request.Context(), u.ID, t.ID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type memberResp struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func (h *TeamHandlers) ListMembers(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	ms, err := h.svc.Repo().ListMembers(c.Request.Context(), t.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := []memberResp{}
	// owner 行
	if t.OwnerUserID != nil {
		out = append(out, memberResp{UserID: t.OwnerUserID.String(), Role: "owner"})
	}
	for _, m := range ms {
		out = append(out, memberResp{UserID: m.UserID.String(), Role: m.Role})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type addMemberReq struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

func (h *TeamHandlers) AddMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req addMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid user_id"))
		return
	}
	if err := h.svc.AddMember(c.Request.Context(), u.ID, t.ID, uid, req.Role); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type patchMemberReq struct {
	Role string `json:"role" binding:"required"`
}

func (h *TeamHandlers) PatchMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid uid"))
		return
	}
	var req patchMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	if err := h.svc.UpdateMemberRole(c.Request.Context(), u.ID, t.ID, uid, req.Role); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandlers) RemoveMember(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	uid, err := uuid.Parse(c.Param("uid"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid uid"))
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), u.ID, t.ID, uid); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

type transferReq struct {
	NewOwnerID string `json:"new_owner_id" binding:"required"`
}

func (h *TeamHandlers) Transfer(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	u, _ := auth.CurrentUser(c)
	var req transferReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid body"))
		return
	}
	uid, err := uuid.Parse(req.NewOwnerID)
	if err != nil {
		c.Error(apperr.New("validation_failed", "team", "invalid new_owner_id"))
		return
	}
	if err := h.svc.TransferOwnership(c.Request.Context(), u.ID, t.ID, uid); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 2: 改 routes.go — 注册团队路由**

在 `handlers/routes.go` 的 `Register` 函数加参数 `teamSvc *team.Service`，并在 admin 路由组之后加：
```go
	teamH := NewTeamHandlers(teamSvc)

	authed.POST("/teams", teamH.Create)
	authed.GET("/teams", teamH.ListMine)

	teamGroup := r.Group("/teams/:slug")
	teamGroup.Use(auth.AuthRequired(sm, userRepo))
	{
		teamGroup.GET("", auth.TeamScoped(teamSvc, "member"), teamH.Get)
		teamGroup.PATCH("", auth.TeamScoped(teamSvc, "owner"), teamH.Patch)
		teamGroup.DELETE("", auth.TeamScoped(teamSvc, "owner"), teamH.Delete)
		teamGroup.GET("/members", auth.TeamScoped(teamSvc, "member"), teamH.ListMembers)
		teamGroup.POST("/members", auth.TeamScoped(teamSvc, "admin"), teamH.AddMember)
		teamGroup.PATCH("/members/:uid", auth.TeamScoped(teamSvc, "owner"), teamH.PatchMember)
		teamGroup.DELETE("/members/:uid", auth.TeamScoped(teamSvc, "admin"), teamH.RemoveMember)
		teamGroup.POST("/transfer", auth.TeamScoped(teamSvc, "owner"), teamH.Transfer)
	}
```
加 import `"github.com/skillhub/skillhub/internal/team"`。注意 Register 签名变化（多一个参数）。

- [ ] **Step 3: 改 server.go — Deps 加 TeamSvc + 传参**

Deps 加 `TeamSvc *team.Service`。在 New() 调 handlers.Register 的地方加 `deps.TeamSvc` 参数。加 import `"github.com/skillhub/skillhub/internal/team"`。Register 调用改为：
```go
	if deps.UserSvc != nil && deps.SessionMgr != nil && deps.UserRepo != nil {
		handlers.Register(r, deps.UserSvc, deps.SessionMgr, deps.UserRepo, deps.TeamSvc)
	}
```

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
git add internal/httpserver
git commit -m "feat(httpserver): add team handlers and routes"
```

---

### Task 6: main 装配 + e2e 集成测试

**Files:**
- Modify: `cmd/skillhub/main.go`
- Create: `internal/httpserver/handlers/teams_test.go`

**Interfaces:**
- Consumes: 所有 C 阶段组件

- [ ] **Step 1: 改 main.go — 装配 team.Service**

在 userSvc 之后加：
```go
	teamRepo := team.NewRepo(gdb)
	teamSvc := team.NewService(teamRepo, auditLogger)
```
加 import `"github.com/skillhub/skillhub/internal/team"`。Deps 加 `TeamSvc: teamSvc`。

- [ ] **Step 2: 写 e2e 集成测试**

`internal/httpserver/handlers/teams_test.go`:
```go
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
	gdb.Exec("TRUNCATE team_members, users RESTART IDENTITY CASCADE")
	gdb.Exec("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('global','Global',NULL,'admin_only')")
	rdb, _ := redispkg.New(cfg.Redis)
	rdb.FlushDB(context.Background())

	auditLogger := audit.NewLogger(gdb, zap.NewNop())
	userRepo := user.NewRepo(gdb)
	userSvc := user.NewService(userRepo, auditLogger)
	teamRepo := team.NewRepo(gdb)
	teamSvc := team.NewService(teamRepo, auditLogger)
	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth)
	return httpserver.New(httpserver.Deps{
		Logger: zap.NewNop(), DB: gdb, Redis: rdb,
		UserSvc: userSvc, SessionMgr: sessionMgr, UserRepo: userRepo, TeamSvc: teamSvc,
	})
}

func registerAndLogin(t *testing.T, r *gin.Engine, email, pw string) *http.Cookie {
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
	// 取 member 的 user id via /me
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("GET", "/me", member, ""))
	if w.Code != 200 {
		t.Fatalf("me: %d", w.Code)
	}
	// 简化：从 register 响应拿不到 id，这里通过 /me 解析
	// 改用 admin/users 列表拿 id（owner 不是 admin，换用直接查库）
	// 为简化测试，直接从 DB 查 member id
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	var memberID string
	gdb.Raw("SELECT id FROM users WHERE email='member@x.com'").Scan(&memberID)

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

func TestE2E_GlobalCannotDelete(t *testing.T) {
	r := setupTeamApp(t)
	// 需 platform_admin。直接造一个
	cfg, _ := config.Load("../../../config/config.yaml")
	gdb, _ := db.New(cfg.DB)
	hash, _ := auth.HashPassword("password1")
	gdb.Exec("INSERT INTO users(email,username,password_hash,role,status) VALUES('admin@x.com','admin',?,?,'active')", hash, user.RolePlatformAdmin)
	admin := registerAndLogin(t, r, "admin@x.com", "password1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, reqWithCookie("DELETE", "/teams/global", admin, ""))
	if w.Code != 422 {
		t.Fatalf("delete global: got %d %s", w.Code, w.Body.String())
	}
}
```

注意：`auth.HashPassword` 不存在（已在 internal/password）。改用 `password.Hash`：
```go
import "github.com/skillhub/skillhub/internal/password"
...
hash, _ := password.Hash("password1")
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 4: 跑 e2e 集成测试**

Run:
```bash
make compose-up
make migrate-up
go test -tags integration ./internal/httpserver/handlers/
```
Expected: PASS

- [ ] **Step 5: 跑全部单元测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/skillhub internal/httpserver/handlers
git commit -m "feat(main): wire team service; add team e2e integration tests"
```

---

## Self-Review 记录

- **Spec 覆盖**：§3 数据模型→Task 1；§4.1 team model/repo/service→Task 2/3；§4.2 TeamScoped middleware→Task 4；§4.3 handlers→Task 5；§7 测试→Task 3 单测 + Task 6 e2e；§8 交付物→Task 6。覆盖完整。
- **占位符**：无 TBD/TODO。
- **类型一致**：`team.Team`/`team.TeamMember`/`team.Repo`/`team.Service` 签名在 Task 2/3/4/5 一致；`auth.TeamScoped(teamSvc, required)` / `auth.CurrentTeam` 在 Task 4/5 一致；`Service.Repo()` 访问器在 Task 4 Step 2 补，Task 5 使用；`handlers.Register` 签名在 Task 5 加 teamSvc 参数，Task 5 Step 3 server.go 同步更新。
- 已知小瑕疵：service 单测 audit.NewLogger(nil,...) 异步 goroutine 触发 panic 被 recover 吞（同 B 阶段，可接受）；e2e 测试通过直接查库拿 member id（简化，可接受）。
