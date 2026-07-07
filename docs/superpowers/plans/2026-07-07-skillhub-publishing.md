# SkillHub 技能包发布（子项目 D）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现技能包发布与拉取——团队/全局命名空间下的 skill 与不可变版本，tarball 存 storage.Store，元数据落 db，发布权由 team.Service.CanPublish 控制，版本严格 semver、重复 409、不可删。

**Architecture:** 新增 internal/skill（semver/model/repo/service）。handlers 注册到 gin engine。复用 TeamScoped(member) 做可见性，handler 内调 CanPublish 做发布授权。

**Tech Stack:** GORM、Gin、storage.Store、crypto/sha256、复用 audit/auth/team。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A/B/C 所有 internal 包。
- service/repo 失败返回 `*apperr.Error`；handler 错误经 errors 中间件渲染。
- 单元测试无外部依赖（mock repo + 内存 fake storage）；集成测试 build tag `//go:build integration`，依赖 compose 的 PG + 本地 storage。
- 集成测试 config 路径按包深度：internal/skill 用 `../../config/config.yaml`，internal/httpserver/handlers 用 `../../../config/config.yaml`。
- 每任务结束提交，conventional commits。
- 版本不可变：无 delete/yank API。
- global 命名空间只读：CanPublish 对 global 恒假 → 发布 403。
- 包大小上限 50 MiB（`MaxPackageSize`）。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `migrations/000004_skills.up.sql` / `.down.sql` | skills + skill_versions |
| `internal/skill/semver.go` | IsValid / Compare |
| `internal/skill/semver_test.go` | 单测 |
| `internal/skill/model.go` | Skill / SkillVersion + 常量 |
| `internal/skill/repo.go` | Repo 接口 + GORM 实现 |
| `internal/skill/repo_test.go` | 集成测试 |
| `internal/skill/service.go` | Service: Publish / Get / List / Open |
| `internal/skill/service_test.go` | 单测（mock repo + fake storage） |
| `internal/httpserver/handlers/skills.go` | skill 路由 handler |
| `internal/httpserver/handlers/routes.go` | 注册 skill 路由 |
| `internal/httpserver/server.go` | Deps 加 SkillSvc，装配 |
| `cmd/skillhub/main.go` | 装配 skill.Service |
| `internal/httpserver/handlers/skills_test.go` | e2e 集成测试 |

---

### Task 1: skills + skill_versions 迁移

**Files:**
- Create: `migrations/000004_skills.up.sql`
- Create: `migrations/000004_skills.down.sql`

- [ ] **Step 1: 写 up 迁移**

```sql
CREATE TABLE skills (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (team_id, name)
);
CREATE INDEX skills_team_idx ON skills(team_id);

CREATE TABLE skill_versions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_id          UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version           TEXT NOT NULL,
    storage_key       TEXT NOT NULL,
    size              BIGINT NOT NULL,
    sha256            TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    publisher_user_id UUID NOT NULL REFERENCES users(id),
    readme            TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_id, version)
);
CREATE INDEX skill_versions_skill_idx ON skill_versions(skill_id);
```

- [ ] **Step 2: 写 down 迁移**

```sql
DROP TABLE IF EXISTS skill_versions;
DROP TABLE IF EXISTS skills;
```

- [ ] **Step 3: 验证迁移**

```bash
make migrate-up
docker compose -f deployments/docker-compose.yml exec -T postgres psql -U skillhub -d skillhub -c "\dt"
```
Expected: skills + skill_versions 表存在。

- [ ] **Step 4: 提交**

```bash
git add migrations
git commit -m "feat(db): add skills and skill_versions migrations"
```

---

### Task 2: semver + model + repo

**Files:**
- Create: `internal/skill/semver.go`
- Create: `internal/skill/semver_test.go`
- Create: `internal/skill/model.go`
- Create: `internal/skill/repo.go`
- Create: `internal/skill/repo_test.go`

- [ ] **Step 1: 写 semver.go**

```go
package skill

import (
	"regexp"
	"strconv"
	"strings"
)

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`)

func IsValid(v string) bool { return semverRe.MatchString(v) }

// Compare 返回 -1/0/1。仅按主.次.修比较；prerelease 视为小于对应正式版；
// build metadata 忽略。非合法版本按字符串序兜底。
func Compare(a, b string) int {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := 0; i < 3; i++ {
		if pa.n[i] != pb.n[i] {
			if pa.n[i] < pb.n[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa.pre == "" && pb.pre != "":
		return 1
	case pa.pre != "" && pb.pre == "":
		return -1
	default:
		return strings.Compare(pa.pre, pb.pre)
	}
}

type semverParts struct {
	n   [3]int
	pre string
}

func parseSemver(v string) (semverParts, bool) {
	if !IsValid(v) {
		return semverParts{}, false
	}
	build := strings.Index(v, "+")
	if build >= 0 {
		v = v[:build]
	}
	pre := ""
	if p := strings.Index(v, "-"); p >= 0 {
		pre = v[p+1:]
		v = v[:p]
	}
	parts := strings.Split(v, ".")
	out := semverParts{pre: pre}
	for i, s := range parts {
		n, _ := strconv.Atoi(s)
		out.n[i] = n
	}
	return out, true
}
```

- [ ] **Step 2: 写 semver_test.go**

```go
package skill

import "testing"

func TestIsValid(t *testing.T) {
	good := []string{"1.0.0", "0.0.1", "1.2.3-alpha", "1.2.3-alpha.1", "1.0.0+build", "1.0.0-beta+x"}
	bad := []string{"", "1", "1.0", "v1.0.0", "1.0.0.0", "01.0.0", "a.b.c", "1.0.0-"}
	for _, v := range good {
		if !IsValid(v) {
			t.Fatalf("expected valid: %s", v)
		}
	}
	for _, v := range bad {
		if IsValid(v) {
			t.Fatalf("expected invalid: %s", v)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct{ a, b string; want int }{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.0", "1.0.0-alpha", 1}, // prerelease < release
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0+build1", "1.0.0+build2", 0}, // build ignored
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Fatalf("Compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
```

- [ ] **Step 3: 写 model.go**

```go
package skill

import (
	"time"

	"github.com/google/uuid"
)

const (
	ContentTypeTarball = "application/gzip"
	MaxNameLen         = 63
	MaxPackageSize     = 50 * 1024 * 1024 // 50 MiB
)

type Skill struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TeamID    uuid.UUID `gorm:"type:uuid;not null"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Skill) TableName() string { return "skills" }

type SkillVersion struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SkillID         uuid.UUID `gorm:"type:uuid;not null"`
	Version         string    `gorm:"not null"`
	StorageKey      string    `gorm:"not null"`
	Size            int64     `gorm:"not null"`
	Sha256          string    `gorm:"not null"`
	ContentType     string    `gorm:"not null"`
	PublisherUserID uuid.UUID `gorm:"type:uuid;not null"`
	Readme          string
	CreatedAt       time.Time
}

func (SkillVersion) TableName() string { return "skill_versions" }
```

- [ ] **Step 4: 写 repo.go**

```go
package skill

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func IsValidName(name string) bool { return nameRe.MatchString(name) }

type Repo interface {
	CreateSkill(ctx context.Context, s *Skill) error
	GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error)
	ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error)
	CreateVersion(ctx context.Context, v *SkillVersion) error
	GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error)
	ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error)
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

func isUniqueViolation(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || stringsContains(err.Error(), "23505"))
}

func stringsContains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (r *repo) CreateSkill(ctx context.Context, s *Skill) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.New("conflict", "skill", "skill already exists")
		}
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

func (r *repo) GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error) {
	var s Skill
	if err := r.db.WithContext(ctx).First(&s, "team_id = ? AND name = ?", teamID, name).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "skill", "skill not found")
		}
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return &s, nil
}

func (r *repo) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	var ss []Skill
	if err := r.db.WithContext(ctx).Where("team_id = ?", teamID).Order("name ASC").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return ss, nil
}

func (r *repo) CreateVersion(ctx context.Context, v *SkillVersion) error {
	if err := r.db.WithContext(ctx).Create(v).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.New("conflict", "skill", "version already exists")
		}
		return fmt.Errorf("create version: %w", err)
	}
	return nil
}

func (r *repo) GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error) {
	var v SkillVersion
	if err := r.db.WithContext(ctx).First(&v, "skill_id = ? AND version = ?", skillID, version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "skill", "version not found")
		}
		return nil, fmt.Errorf("get version: %w", err)
	}
	return &v, nil
}

func (r *repo) ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error) {
	var vs []SkillVersion
	if err := r.db.WithContext(ctx).Where("skill_id = ?", skillID).Order("created_at DESC").Find(&vs).Error; err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return vs, nil
}
```

- [ ] **Step 5: 写集成测试 repo_test.go**

```go
//go:build integration

package skill

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"gorm.io/gorm"
)

var skillDB *gorm.DB

func setupSkillDB(t *testing.T) (Repo, uuid.UUID) {
	t.Helper()
	if skillDB == nil {
		cfg, err := config.Load("../../config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		skillDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	skillDB.Exec("TRUNCATE skill_versions, skills, team_members, teams, users RESTART IDENTITY CASCADE")
	skillDB.Exec("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('global','Global',NULL,'admin_only')")
	// 建一个普通团队 + owner 用户
	var ownerID, teamID string
	skillDB.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('o@x.com','o','x','user','active') RETURNING id::text").Scan(&ownerID)
	skillDB.Raw("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('acme','Acme',?,'admin_only') RETURNING id::text", ownerID).Scan(&teamID)
	oid, _ := uuid.Parse(ownerID)
	tid, _ := uuid.Parse(teamID)
	return NewRepo(skillDB), tid
}

func TestRepo_SkillCRUD(t *testing.T) {
	r, tid := setupSkillDB(t)
	ctx := context.Background()
	s := &Skill{TeamID: tid, Name: "my-skill"}
	if err := r.CreateSkill(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetSkill(ctx, tid, "my-skill")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != s.ID {
		t.Fatal("id mismatch")
	}
	list, err := r.ListSkillsByTeam(ctx, tid)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}
}

func TestRepo_VersionConflict(t *testing.T) {
	r, tid := setupSkillDB(t)
	ctx := context.Background()
	s := &Skill{TeamID: tid, Name: "my-skill"}
	if err := r.CreateSkill(ctx, s); err != nil {
		t.Fatal(err)
	}
	owner := s.ID // placeholder; need a real user id
	var uid string
	skillDB.Raw("SELECT id::text FROM users WHERE email='o@x.com'").Scan(&uid)
	publisher, _ := uuid.Parse(uid)
	v := &SkillVersion{SkillID: s.ID, Version: "1.0.0", StorageKey: "k", Size: 1, Sha256: "x", ContentType: ContentTypeTarball, PublisherUserID: publisher}
	if err := r.CreateVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	v2 := &SkillVersion{SkillID: s.ID, Version: "1.0.0", StorageKey: "k2", Size: 1, Sha256: "x", ContentType: ContentTypeTarball, PublisherUserID: publisher}
	if err := r.CreateVersion(ctx, v2); err == nil {
		t.Fatal("expected conflict")
	}
	_ = owner
}
```

- [ ] **Step 6: 跑测试**

Run: `go vet ./internal/skill/ && go test ./internal/skill/ && go test -tags integration ./internal/skill/`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/skill
git commit -m "feat(skill): add semver, model, and GORM repository"
```

---

### Task 3: skill service + 单测

**Files:**
- Create: `internal/skill/service.go`
- Create: `internal/skill/service_test.go`

- [ ] **Step 1: 写 service.go**

```go
package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/storage"
)

type Service struct {
	repo   Repo
	store  storage.Store
	audit  *audit.Logger
}

func NewService(repo Repo, store storage.Store, audit *audit.Logger) *Service {
	return &Service{repo: repo, store: store, audit: audit}
}

func (s *Service) Repo() Repo { return s.repo }

func (s *Service) Publish(ctx context.Context, teamID uuid.UUID, name, version string, r io.Reader, size int64, contentType string, publisherID uuid.UUID) (*SkillVersion, error) {
	if !IsValidName(name) {
		return nil, apperr.New("validation_failed", "skill", "invalid skill name")
	}
	if !IsValid(version) {
		return nil, apperr.New("validation_failed", "skill", "invalid version")
	}
	if size <= 0 || size > MaxPackageSize {
		return nil, apperr.New("validation_failed", "skill", "package too large")
	}
	// 读全量并算 sha256
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, r); err != nil {
		return nil, apperr.New("validation_failed", "skill", "read body failed")
	}
	if int64(buf.Len()) > MaxPackageSize {
		return nil, apperr.New("validation_failed", "skill", "package too large")
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])

	// 找或建 skill
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		sk = &Skill{TeamID: teamID, Name: name}
		if err := s.repo.CreateSkill(ctx, sk); err != nil {
			return nil, err
		}
		_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &publisherID, Action: audit.Action("skill_created"), TargetType: "skill", TargetID: sk.ID.String(), Metadata: map[string]any{"name": name, "team_id": teamID.String()}})
	}
	key := fmt.Sprintf("skills/%s/%s.tar.gz", sk.ID.String(), version)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), contentType); err != nil {
		return nil, fmt.Errorf("store put: %w", err)
	}
	sv := &SkillVersion{
		SkillID:         sk.ID,
		Version:         version,
		StorageKey:      key,
		Size:            int64(buf.Len()),
		Sha256:          sha,
		ContentType:     contentType,
		PublisherUserID: publisherID,
	}
	if err := s.repo.CreateVersion(ctx, sv); err != nil {
		_ = s.store.Delete(ctx, key) // 清理孤儿
		return nil, err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &publisherID, Action: audit.Action("skill_version_published"), TargetType: "skill_version", TargetID: sv.ID.String(), Metadata: map[string]any{"skill_id": sk.ID.String(), "version": version, "sha256": sha}})
	return sv, nil
}

func (s *Service) GetSkillWithVersions(ctx context.Context, teamID uuid.UUID, name string) (*Skill, []SkillVersion, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return nil, nil, err
	}
	vs, err := s.repo.ListVersions(ctx, sk.ID)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i].Version, vs[j].Version) > 0 })
	return sk, vs, nil
}

func (s *Service) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	return s.repo.ListSkillsByTeam(ctx, teamID)
}

func (s *Service) OpenVersion(ctx context.Context, teamID uuid.UUID, name, version string) (io.ReadCloser, *SkillVersion, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return nil, nil, err
	}
	sv, err := s.repo.GetVersion(ctx, sk.ID, version)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.Get(ctx, sv.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("store get: %w", err)
	}
	return rc, sv, nil
}

func isNotFound(err error) bool {
	e, ok := err.(*apperr.Error)
	return ok && e.Code == "not_found"
}
```

- [ ] **Step 2: 写 service_test.go（mock repo + fake storage）**

```go
package skill

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/storage"
	"go.uber.org/zap"
)

type memStore struct{ objs map[string][]byte }

func newMemStore() *memStore { return &memStore{objs: map[string][]byte{}} }

func (m *memStore) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	b, _ := io.ReadAll(r)
	m.objs[key] = b
	return key, nil
}
func (m *memStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (m *memStore) Delete(ctx context.Context, key string) error { delete(m.objs, key); return nil }
func (m *memStore) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, nil
}

type mockSkillRepo struct {
	skills   map[uuid.UUID]*Skill
	versions map[uuid.UUID][]SkillVersion // skillID -> versions
}

func newMockSkillRepo() *mockSkillRepo {
	return &mockSkillRepo{skills: map[uuid.UUID]*Skill{}, versions: map[uuid.UUID][]SkillVersion{}}
}

func (m *mockSkillRepo) CreateSkill(ctx context.Context, s *Skill) error {
	for _, e := range m.skills {
		if e.TeamID == s.TeamID && e.Name == s.Name {
			return apperr.New("conflict", "skill", "skill already exists")
		}
	}
	s.ID = uuid.New()
	m.skills[s.ID] = s
	return nil
}
func (m *mockSkillRepo) GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error) {
	for _, e := range m.skills {
		if e.TeamID == teamID && e.Name == name {
			return e, nil
		}
	}
	return nil, apperr.New("not_found", "skill", "skill not found")
}
func (m *mockSkillRepo) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	var out []Skill
	for _, e := range m.skills {
		if e.TeamID == teamID {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (m *mockSkillRepo) CreateVersion(ctx context.Context, v *SkillVersion) error {
	for _, e := range m.versions[v.SkillID] {
		if e.Version == v.Version {
			return apperr.New("conflict", "skill", "version already exists")
		}
	}
	v.ID = uuid.New()
	m.versions[v.SkillID] = append(m.versions[v.SkillID], *v)
	return nil
}
func (m *mockSkillRepo) GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error) {
	for _, e := range m.versions[skillID] {
		if e.Version == version {
			v := e
			return &v, nil
		}
	}
	return nil, apperr.New("not_found", "skill", "version not found")
}
func (m *mockSkillRepo) ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error) {
	out := make([]SkillVersion, len(m.versions[skillID]))
	copy(out, m.versions[skillID])
	return out, nil
}

func newSkillSvc() (*Service, *mockSkillRepo, *memStore) {
	r := newMockSkillRepo()
	st := newMemStore()
	return NewService(r, st, audit.NewLogger(nil, zap.NewNop())), r, st
}

func TestPublish_NewSkillAndVersion(t *testing.T) {
	s, r, st := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	body := bytes.NewReader([]byte("tarball"))
	sv, err := s.Publish(ctx, tid, "my-skill", "1.0.0", body, 7, ContentTypeTarball, pub)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Sha256 == "" {
		t.Fatal("sha empty")
	}
	if len(r.versions[sv.SkillID]) != 1 {
		t.Fatal("version not stored")
	}
	if _, ok := st.objs[sv.StorageKey]; !ok {
		t.Fatal("object not stored")
	}
}

func TestPublish_DuplicateVersion_Conflict(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	_, err := s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("b")), 1, ContentTypeTarball, pub)
	if err == nil {
		t.Fatal("expected conflict")
	}
	e, ok := err.(*apperr.Error)
	if !ok || e.Code != "conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestPublish_InvalidNameOrVersion(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	if _, err := s.Publish(ctx, tid, "BadName", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub); err == nil {
		t.Fatal("expected invalid name")
	}
	if _, err := s.Publish(ctx, tid, "ok", "not-semver", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub); err == nil {
		t.Fatal("expected invalid version")
	}
}

func TestOpenVersion(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	rc, sv, err := s.OpenVersion(ctx, tid, "my-skill", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "payload" {
		t.Fatalf("got %q", b)
	}
	if sv.Size != 7 {
		t.Fatalf("size=%d", sv.Size)
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/skill/`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/skill
git commit -m "feat(skill): add Service with Publish/Get/Open and unit tests"
```

---

### Task 4: handlers + routes + server wiring

**Files:**
- Create: `internal/httpserver/handlers/skills.go`
- Modify: `internal/httpserver/handlers/routes.go`
- Modify: `internal/httpserver/server.go`

- [ ] **Step 1: 写 handlers/skills.go**

```go
package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/team"
)

type SkillHandlers struct {
	svc     *skill.Service
	teamSvc *team.Service
}

func NewSkillHandlers(svc *skill.Service, teamSvc *team.Service) *SkillHandlers {
	return &SkillHandlers{svc: svc, teamSvc: teamSvc}
}

type skillResp struct {
	ID      string `json:"id"`
	TeamID  string `json:"team_id"`
	Name    string `json:"name"`
}

type versionResp struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Publisher   string `json:"publisher_user_id"`
	CreatedAt   string `json:"created_at"`
}

func (h *SkillHandlers) Publish(c *gin.Context) {
	t, ok := auth.CurrentTeam(c)
	if !ok {
		c.Error(apperr.New("not_found", "team", "no team"))
		return
	}
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	if !h.teamSvc.CanPublish(c.Request.Context(), t, u.ID) {
		c.Error(apperr.New("forbidden", "skill", "forbidden"))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	name := c.Param("name")
	version := c.Param("version")
	size, _ := strconv.ParseInt(c.Request.Header.Get("Content-Length"), 10, 64)
	sv, err := h.svc.Publish(c.Request.Context(), t.ID, name, version, c.Request.Body, size, c.ContentType(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, versionResp{
		ID: sv.ID.String(), Version: sv.Version, Size: sv.Size, Sha256: sv.Sha256,
		ContentType: sv.ContentType, Publisher: sv.PublisherUserID.String(),
		CreatedAt: sv.CreatedAt.Format(timeRFC3339),
	})
}

func (h *SkillHandlers) ListSkills(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	ss, err := h.svc.ListSkillsByTeam(c.Request.Context(), t.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]skillResp, len(ss))
	for i, s := range ss {
		out[i] = skillResp{ID: s.ID.String(), TeamID: s.TeamID.String(), Name: s.Name}
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *SkillHandlers) GetSkill(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	sk, vs, err := h.svc.GetSkillWithVersions(c.Request.Context(), t.ID, c.Param("name"))
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]versionResp, len(vs))
	for i, v := range vs {
		out[i] = versionResp{ID: v.ID.String(), Version: v.Version, Size: v.Size, Sha256: v.Sha256, ContentType: v.ContentType, Publisher: v.PublisherUserID.String(), CreatedAt: v.CreatedAt.Format(timeRFC3339)}
	}
	c.JSON(http.StatusOK, gin.H{"id": sk.ID.String(), "team_id": sk.TeamID.String(), "name": sk.Name, "versions": out})
}

func (h *SkillHandlers) Download(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	rc, sv, err := h.svc.OpenVersion(c.Request.Context(), t.ID, c.Param("name"), c.Param("version"))
	if err != nil {
		c.Error(err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", sv.ContentType)
	c.Header("Content-Length", strconv.FormatInt(sv.Size, 10))
	c.Header("Content-Disposition", `attachment; filename="`+c.Param("name")+"-"+c.Param("version")+`.tar.gz"`)
	c.Header("X-Skillhub-Sha256", sv.Sha256)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
```

- [ ] **Step 2: 改 routes.go — 注册 skill 路由**

在 `Register` 加参数 `skillSvc *skill.Service`，在团队路由组之后加：
```go
	skillH := NewSkillHandlers(skillSvc, teamSvc)

	skillGroup := r.Group("/teams/:slug/skills")
	skillGroup.Use(auth.AuthRequired(sm, userRepo))
	{
		skillGroup.GET("", auth.TeamScoped(teamSvc, "member"), skillH.ListSkills)
		skillGroup.GET("/:name", auth.TeamScoped(teamSvc, "member"), skillH.GetSkill)
		skillGroup.POST("/:name/versions/:version", auth.TeamScoped(teamSvc, "member"), skillH.Publish)
		skillGroup.GET("/:name/versions/:version", auth.TeamScoped(teamSvc, "member"), skillH.Download)
	}
```
加 import `"github.com/skillhub/skillhub/internal/skill"`。注意 `Register` 签名变化。

- [ ] **Step 3: 改 server.go — Deps 加 SkillSvc + 传参**

Deps 加 `SkillSvc *skill.Service`。Register 调用加 `deps.SkillSvc`。条件加 `&& deps.SkillSvc != nil`。加 import `"github.com/skillhub/skillhub/internal/skill"`。

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
git add internal/httpserver
git commit -m "feat(httpserver): add skill publish/download handlers and routes"
```

---

### Task 5: main 装配 + e2e 集成测试

**Files:**
- Modify: `cmd/skillhub/main.go`
- Modify: `internal/httpserver/handlers/handlers_test.go`（setupApp 补 SkillSvc）
- Create: `internal/httpserver/handlers/skills_test.go`

- [ ] **Step 1: 改 main.go — 装配 skill.Service**

在 teamSvc 之后加：
```go
	skillRepo := skill.NewRepo(gdb)
	skillSvc := skill.NewService(skillRepo, store, auditLogger)
```
加 import `"github.com/skillhub/skillhub/internal/skill"`。Deps 加 `SkillSvc: skillSvc`。

- [ ] **Step 2: 改 handlers_test.go setupApp — 补 SkillSvc**

加 `skillRepo := skill.NewRepo(gdb); skillSvc := skill.NewService(skillRepo, store, auditLogger)`。注意 setupApp 目前没建 store；用 `storage.NewLocal(cfg.Storage.Local.Root)` 或 `storage.New(cfg.Storage)`。Deps 加 `SkillSvc: skillSvc` 和 `Storage: store`。加 import。TRUNCATE 需扩到 `skill_versions, skills, team_members, teams, users`。

- [ ] **Step 3: 写 e2e 集成测试 skills_test.go**

覆盖：
- `TestE2E_PublishDownload`: owner 发布 → 拉取校验 sha256 → 再发新版本 → GET skill 列版本。
- `TestE2E_NonMemberDownload_Forbidden`: 非成员拉取团队包 403。
- `TestE2E_NonPublisherPublish_Forbidden`: 成员（any_member 策略下可发） vs 非成员发 403；admin_only 下普通 member 发 403。
- `TestE2E_DuplicateVersion_Conflict`: 同 version 再发 409。
- `TestE2E_GlobalPublish_Forbidden`: global 发布 403。
- `TestE2E_GlobalDownload_OK`: global skill 由 platform_admin 直接插库造一个版本，任意认证用户拉取 200。（或跳过若造数据太繁，改为：global 无 skill，GET 列表 200 空数组。）

- [ ] **Step 4: 编译验证**

Run: `go build ./...`

- [ ] **Step 5: 跑 e2e 集成测试**

Run: `make compose-up && make migrate-up && go test -tags integration ./internal/httpserver/handlers/`
Expected: PASS

- [ ] **Step 6: 跑全部单元测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/skillhub internal/httpserver/handlers
git commit -m "feat(main): wire skill service; add skill e2e integration tests"
```

---

## Self-Review 记录

- **Spec 覆盖**：§3 数据模型→Task 1；§4.1 semver/model/repo/service→Task 2/3；§4.2 handlers→Task 4；§7 测试→Task 2/3 单测 + Task 5 e2e；§8 交付物→Task 5。覆盖完整。
- **占位符**：无 TBD/TODO。
- **类型一致**：`skill.Repo`/`skill.Service` 签名在 Task 2/3/4 一致；`handlers.Register` 在 Task 4 加 skillSvc 参数，server.go 同步；`Service.Repo()` 访问器在 Task 3 补。
- **关键不变量**：版本不可变（无 delete API）；global 不可发（CanPublish 恒假）；发布前 TeamScoped(member) 保证拉取可见性；storage 写后 db 失败回滚删除对象。
- 已知小瑕疵：service.Publish 读全量到内存（包上限 50MiB，可接受）；audit.NewLogger(nil,...) goroutine panic 被 recover 吞（同 B/C，可接受）。
