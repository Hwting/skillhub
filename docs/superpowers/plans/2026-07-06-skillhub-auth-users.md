# SkillHub 认证与用户（子项目 B）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 A 阶段骨架上实现用户注册/登录/登出/当前用户/用户管理（platform_admin）+ 审计日志，会话存 Redis，密码用 argon2id。

**Architecture:** 新增 internal/user（model+repo+service）、internal/auth（password+session+middleware）、internal/audit（logger）。handler 注册到 A 阶段的 gin engine。session 自定义存 Redis，cookie 维持。审计异步落 audit_logs 表。

**Tech Stack:** argon2id（golang.org/x/crypto/argon2）、miniredis（单测）、GORM、Gin、uuid。

## Global Constraints

- module `github.com/skillhub/skillhub`，复用 A 阶段所有 internal 包。
- 构造函数失败返回 `*apperr.Error`（service/repo 层）；handler 错误经 errors 中间件渲染。
- 单元测试无外部依赖（miniredis）；集成测试用 build tag `//go:build integration`，依赖 compose 的 PG+Redis。
- 每任务结束提交，conventional commits。
- argon2 参数硬编码：memory=64*1024, time=3, threads=2, keyLen=32, saltLen=16。
- 登录失败统一返回 `unauthorized` (401)，不区分原因；审计记真实原因。
- session 只存 Redis，不入库。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `internal/config/config.go` | 加 AuthConfig 段 |
| `config/config.yaml` | 加 auth 段 |
| `migrations/000002_users.up.sql` / `.down.sql` | users + audit_logs 建表 |
| `internal/user/model.go` | User GORM 模型 + 角色状态常量 |
| `internal/user/repo.go` | Repo 接口 + GORM 实现 |
| `internal/user/repo_test.go` | 集成测试 |
| `internal/user/service.go` | Service（Register/Login/UpdateRole/Disable） |
| `internal/user/service_test.go` | 单测（mock repo） |
| `internal/audit/audit.go` | Action 枚举 + Entry + Logger |
| `internal/audit/audit_test.go` | 集成测试 |
| `internal/auth/password.go` | argon2id 哈希/验证 |
| `internal/auth/password_test.go` | 单测 |
| `internal/auth/session.go` | SessionManager（Redis） |
| `internal/auth/session_test.go` | 单测（miniredis） |
| `internal/auth/middleware.go` | AuthRequired/RequireRole/CurrentUser |
| `internal/httpserver/handlers/auth.go` | register/login/logout/me |
| `internal/httpserver/handlers/users.go` | admin users CRUD |
| `internal/httpserver/handlers/routes.go` | 路由注册 |
| `internal/httpserver/server.go` | Deps 加 UserSvc/SessionMgr，装配路由 |
| `internal/httpserver/handlers/handlers_test.go` | e2e 集成测试 |

---

### Task 1: AuthConfig 配置段

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config/config.yaml`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.AuthConfig{SessionTTL time.Duration; CookieName string; CookieSecure bool; CookieDomain string; CookieSameSite string}`，加入 `Config.Auth`；Validate 校验 session_ttl>0、cookie_name 非空、cookie_samesite ∈ {strict,lax,none}。

- [ ] **Step 1: 改 config.go — 加 AuthConfig 与字段**

在 Config struct 加：
```go
	Auth AuthConfig `mapstructure:"auth"`
```
新增类型：
```go
type AuthConfig struct {
	SessionTTL     time.Duration `mapstructure:"session_ttl"`
	CookieName     string        `mapstructure:"cookie_name"`
	CookieSecure   bool          `mapstructure:"cookie_secure"`
	CookieDomain   string        `mapstructure:"cookie_domain"`
	CookieSameSite string        `mapstructure:"cookie_samesite"`
}
```
在 Validate() 末尾 return 前加：
```go
	if c.Auth.SessionTTL <= 0 {
		return fmt.Errorf("auth.session_ttl must be > 0")
	}
	if c.Auth.CookieName == "" {
		return fmt.Errorf("auth.cookie_name is required")
	}
	switch c.Auth.CookieSameSite {
	case "strict", "lax", "none":
	default:
		return fmt.Errorf("auth.cookie_samesite must be strict|lax|none, got %q", c.Auth.CookieSameSite)
	}
```

- [ ] **Step 2: 改 config/config.yaml — 加 auth 段**

在文件末尾追加：
```yaml
auth:
  session_ttl: 24h
  cookie_name: sid
  cookie_secure: false
  cookie_domain: ""
  cookie_samesite: lax
```

- [ ] **Step 3: 改 config_test.go — 加 auth 校验测试**

在 config_test.go 的 validYAML 末尾（log 段后）追加：
```yaml
auth:
  session_ttl: 24h
  cookie_name: sid
  cookie_secure: false
  cookie_domain: ""
  cookie_samesite: lax
```
新增测试：
```go
func TestValidate_InvalidSameSite(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Auth.CookieSameSite = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_MissingCookieName(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Auth.CookieName = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config config/config.yaml
git commit -m "feat(config): add auth config section with validation"
```

---

### Task 2: users + audit_logs 迁移

**Files:**
- Create: `migrations/000002_users.up.sql`
- Create: `migrations/000002_users.down.sql`

**Interfaces:** 无（纯 SQL）。

- [ ] **Step 1: 写 up 迁移**

`migrations/000002_users.up.sql`:
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','platform_admin')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);
CREATE INDEX users_role_idx ON users(role);

CREATE TABLE audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    actor_user_id UUID,
    action        TEXT NOT NULL,
    target_type   TEXT,
    target_id     TEXT,
    ip            TEXT,
    user_agent    TEXT,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX audit_logs_actor_idx ON audit_logs(actor_user_id);
CREATE INDEX audit_logs_action_idx ON audit_logs(action);
```

- [ ] **Step 2: 写 down 迁移**

`migrations/000002_users.down.sql`:
```sql
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 3: 验证迁移可跑（需 compose）**

Run:
```bash
make compose-up
make migrate-down  # 回滚占位迁移到干净状态可能失败，改用：
go run ./cmd/migrate up
```
Expected: 无错误。验证表存在：
```bash
docker compose -f deployments/docker-compose.yml exec -T postgres psql -U skillhub -d skillhub -c "\dt"
```
Expected: 列出 schema_meta、users、audit_logs。

- [ ] **Step 4: 提交**

```bash
git add migrations
git commit -m "feat(db): add users and audit_logs migrations"
```

---

### Task 3: user model + repo

**Files:**
- Create: `internal/user/model.go`
- Create: `internal/user/repo.go`
- Create: `internal/user/repo_test.go`

**Interfaces:**
- Consumes: `*gorm.DB`（A 阶段 db），`apperr`
- Produces: `user.User`，角色/状态常量，`user.Repo` 接口，`user.NewRepo(db *gorm.DB) Repo`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get github.com/google/uuid@latest
go get gorm.io/datatypes@latest  # for JSONB
```

- [ ] **Step 2: 写 model.go**

`internal/user/model.go`:
```go
package user

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleUser           = "user"
	RolePlatformAdmin  = "platform_admin"
	StatusActive       = "active"
	StatusDisabled     = "disabled"
)

type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string     `gorm:"uniqueIndex;not null"`
	Username     string     `gorm:"uniqueIndex;not null"`
	PasswordHash string     `gorm:"not null"`
	Role         string     `gorm:"not null;default:user"`
	Status       string     `gorm:"not null;default:active"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

func (User) TableName() string { return "users" }
```

- [ ] **Step 3: 写 repo.go**

`internal/user/repo.go`:
```go
package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

type Repo interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, limit, offset int) ([]User, int64, error)
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

func (r *repo) Create(ctx context.Context, u *User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *repo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "user", "user not found")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func (r *repo) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "email = ?", email).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "user", "user not found")
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (r *repo) List(ctx context.Context, limit, offset int) ([]User, int64, error) {
	var users []User
	var total int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return users, total, nil
}

func (r *repo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("role", role)
	if res.Error != nil {
		return fmt.Errorf("update role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}

func (r *repo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("update status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}

func (r *repo) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("last_login_at", &now)
	if res.Error != nil {
		return fmt.Errorf("touch last login: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}
```

- [ ] **Step 4: 写集成测试**

`internal/user/repo_test.go`:
```go
//go:build integration

package user

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func setupDB(t *testing.T) Repo {
	t.Helper()
	if testDB == nil {
		cfg, err := config.Load("config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		testDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	// 清表
	if err := testDB.Exec("TRUNCATE users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	return NewRepo(testDB)
}

func TestRepo_CreateGet(t *testing.T) {
	r := setupDB(t)
	ctx := context.Background()
	u := &User{Email: "a@b.com", Username: "a", PasswordHash: "x", Role: RoleUser, Status: StatusActive}
	if err := r.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.ID == uuid.Nil {
		t.Fatal("id not set")
	}
	got, err := r.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "a@b.com" {
		t.Fatalf("email=%s", got.Email)
	}
	byEmail, err := r.GetByEmail(ctx, "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if byEmail.ID != u.ID {
		t.Fatal("email lookup mismatch")
	}
}

func TestRepo_GetByID_NotFound(t *testing.T) {
	r := setupDB(t)
	_, err := r.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepo_UpdateRole_Status(t *testing.T) {
	r := setupDB(t)
	ctx := context.Background()
	u := &User{Email: "a@b.com", Username: "a", PasswordHash: "x", Role: RoleUser, Status: StatusActive}
	r.Create(ctx, u)
	if err := r.UpdateRole(ctx, u.ID, RolePlatformAdmin); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateStatus(ctx, u.ID, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, u.ID)
	if got.Role != RolePlatformAdmin || got.Status != StatusDisabled {
		t.Fatalf("got role=%s status=%s", got.Role, got.Status)
	}
}

func TestRepo_List(t *testing.T) {
	r := setupDB(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r.Create(ctx, &User{Email: string(rune('a'+i)) + "@b.com", Username: string(rune('a'+i)), PasswordHash: "x"})
	}
	users, total, err := r.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(users) != 3 {
		t.Fatalf("total=%d len=%d", total, len(users))
	}
}
```

- [ ] **Step 5: 跑测试**

Run: `go vet ./internal/user/`（确认编译）
Run: `make compose-up && go test -tags integration ./internal/user/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/user go.mod go.sum
git commit -m "feat(user): add User model and GORM repository"
```

---

### Task 4: audit logger

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

**Interfaces:**
- Consumes: `*gorm.DB`, `*zap.Logger`
- Produces: `audit.Action` 枚举，`audit.Entry`，`audit.Logger`，`audit.NewLogger(db, logger) *Logger`，`Logger.Log(ctx, Entry) error`

- [ ] **Step 1: 写 audit.go**

`internal/audit/audit.go`:
```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Action string

const (
	ActionLoginSuccess    Action = "login_success"
	ActionLoginFailure    Action = "login_failure"
	ActionLogout          Action = "logout"
	ActionRegister        Action = "register"
	ActionUserRoleChanged Action = "user_role_changed"
	ActionUserDisabled    Action = "user_disabled"
)

type Entry struct {
	ActorUserID *uuid.UUID
	Action      Action
	TargetType  string
	TargetID    string
	IP          string
	UserAgent   string
	Metadata    map[string]any
}

type auditRow struct {
	ID           int64          `gorm:"primaryKey;autoIncrement"`
	ActorUserID  *uuid.UUID     `gorm:"type:uuid"`
	Action       string         `gorm:"not null"`
	TargetType   string
	TargetID     string
	IP           string
	UserAgent    string
	Metadata     datatypes.JSON
	CreatedAt    time.Time
}

func (auditRow) TableName() string { return "audit_logs" }

type Logger struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewLogger(db *gorm.DB, logger *zap.Logger) *Logger {
	return &Logger{db: db, logger: logger}
}

func (l *Logger) Log(ctx context.Context, e Entry) error {
	var meta datatypes.JSON
	if e.Metadata != nil {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		meta = b
	}
	row := auditRow{
		ActorUserID: e.ActorUserID,
		Action:      string(e.Action),
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		IP:          e.IP,
		UserAgent:   e.UserAgent,
		Metadata:    meta,
	}
	// 异步写：失败不阻塞主流程，仅记日志
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("audit log panic", zap.Any("panic", r))
			}
		}()
		if err := l.db.WithContext(ctx).Create(&row).Error; err != nil {
			l.logger.Error("audit log write failed", zap.Error(err), zap.String("action", string(e.Action)))
		}
	}()
	return nil
}
```

注意：需 import `"time"`。补上。

- [ ] **Step 2: 写集成测试**

`internal/audit/audit_test.go`:
```go
//go:build integration

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var auditDB *gorm.DB

func setupAuditDB(t *testing.T) *Logger {
	t.Helper()
	if auditDB == nil {
		cfg, err := config.Load("config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		auditDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	auditDB.Exec("TRUNCATE audit_logs RESTART IDENTITY")
	return NewLogger(auditDB, zap.NewNop())
}

func TestLogger_Log(t *testing.T) {
	l := setupAuditDB(t)
	uid := uuid.New()
	err := l.Log(context.Background(), Entry{
		ActorUserID: &uid,
		Action:      ActionRegister,
		TargetType:  "user",
		TargetID:    uid.String(),
		IP:          "1.2.3.4",
		Metadata:    map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 异步写，等一下
	time.Sleep(200 * time.Millisecond)
	var n int64
	auditDB.Table("audit_logs").Where("action = ?", string(ActionRegister)).Count(&n)
	if n != 1 {
		t.Fatalf("expected 1 audit row, got %d", n)
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `go vet ./internal/audit/`
Run: `go test -tags integration ./internal/audit/`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/audit go.mod go.sum
git commit -m "feat(audit): add async audit logger writing to audit_logs"
```

---

### Task 5: auth/password (argon2id)

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`

**Interfaces:**
- Produces: `auth.HashPassword(plain string) (string, error)`，`auth.VerifyPassword(plain, encoded string) (bool, error)`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get golang.org/x/crypto@latest
```

- [ ] **Step 2: 写失败测试**

`internal/auth/password_test.go`:
```go
package auth

import (
	"testing"
)

func TestHashVerify_RoundTrip(t *testing.T) {
	encoded, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("hunter2", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected match")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	encoded, _ := HashPassword("hunter2")
	ok, err := VerifyPassword("wrong", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no match")
	}
}

func TestVerify_InvalidEncoded(t *testing.T) {
	_, err := VerifyPassword("x", "not-a-valid-hash")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/auth/`
Expected: FAIL（HashPassword 未定义）

- [ ] **Step 4: 写实现**

`internal/auth/password.go`:
```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

func HashPassword(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("gen salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(plain, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("parse version: %w", err)
	}
	var memory, time, threads int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}
	gotHash := argon2.IDKey([]byte(plain), salt, uint32(time), uint32(memory), uint8(threads), uint32(len(wantHash)))
	if subtle.ConstantTimeCompare(gotHash, wantHash) == 1 {
		return true, nil
	}
	return false, nil
}
```

- [ ] **Step 5: 跑测试**

Run: `go test ./internal/auth/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat(auth): add argon2id password hashing and verification"
```

---

### Task 6: auth/session (Redis)

**Files:**
- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`

**Interfaces:**
- Consumes: `*redis.Client`，`config.AuthConfig`
- Produces: `auth.SessionManager`，`auth.NewSessionManager(client *redis.Client, cfg config.AuthConfig) *SessionManager`，方法 Create/Get/Delete/SetCookie/ClearCookie

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get github.com/alicebob/miniredis/v2@latest
```

- [ ] **Step 2: 写失败测试**

`internal/auth/session_test.go`:
```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

func newTestSM(t *testing.T) (*SessionManager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := rdb.NewClient(&rdb.Options{Addr: mr.Addr()})
	cfg := config.AuthConfig{
		SessionTTL:     time.Hour,
		CookieName:     "sid",
		CookieSecure:   false,
		CookieSameSite: "lax",
	}
	return NewSessionManager(client, cfg), mr
}

func TestSession_CreateGetDelete(t *testing.T) {
	sm, _ := newTestSM(t)
	ctx := context.Background()
	uid := uuid.New()
	sid, err := sm.Create(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if sid == "" {
		t.Fatal("empty session id")
	}
	got, err := sm.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if got != uid {
		t.Fatalf("got %v want %v", got, uid)
	}
	if err := sm.Delete(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Get(ctx, sid); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSession_Get_Miss(t *testing.T) {
	sm, _ := newTestSM(t)
	if _, err := sm.Get(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestSession_SetCookie(t *testing.T) {
	sm, _ := newTestSM(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest("GET", "/", nil)
	sm.SetCookie(c, "abc")
	if c.Writer.Header().Get("Set-Cookie") == "" {
		t.Fatal("no set-cookie")
	}
}
```

注意：session_test.go 需 import `"net/http/httptest"`。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/auth/`
Expected: FAIL（NewSessionManager 未定义）

- [ ] **Step 4: 写实现**

`internal/auth/session.go`:
```go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

type SessionManager struct {
	client   *rdb.Client
	ttl      time.Duration
	cookieCfg config.AuthConfig
}

func NewSessionManager(client *rdb.Client, cfg config.AuthConfig) *SessionManager {
	return &SessionManager{client: client, ttl: cfg.SessionTTL, cookieCfg: cfg}
}

func (sm *SessionManager) Create(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gen session id: %w", err)
	}
	sid := hex.EncodeToString(b)
	key := "session:" + sid
	if err := sm.client.Set(ctx, key, userID.String(), sm.ttl).Err(); err != nil {
		return "", fmt.Errorf("set session: %w", err)
	}
	return sid, nil
}

func (sm *SessionManager) Get(ctx context.Context, sessionID string) (uuid.UUID, error) {
	key := "session:" + sessionID
	val, err := sm.client.Get(ctx, key).Result()
	if err != nil {
		if err == rdb.Nil {
			return uuid.Nil, apperr.New("unauthorized", "auth", "session not found")
		}
		return uuid.Nil, fmt.Errorf("get session: %w", err)
	}
	uid, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, apperr.New("unauthorized", "auth", "invalid session")
	}
	return uid, nil
}

func (sm *SessionManager) Delete(ctx context.Context, sessionID string) error {
	key := "session:" + sessionID
	if err := sm.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (sm *SessionManager) sameSite() http.SameSite {
	switch sm.cookieCfg.CookieSameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (sm *SessionManager) SetCookie(c *gin.Context, sessionID string) {
	c.SetSameSite(sm.sameSite())
	c.SetCookie(sm.cookieCfg.CookieName, sessionID, int(sm.ttl.Seconds()), "/", sm.cookieCfg.CookieDomain, sm.cookieCfg.CookieSecure, true)
}

func (sm *SessionManager) ClearCookie(c *gin.Context) {
	c.SetSameSite(sm.sameSite())
	c.SetCookie(sm.cookieCfg.CookieName, "", -1, "/", sm.cookieCfg.CookieDomain, sm.cookieCfg.CookieSecure, true)
}
```

- [ ] **Step 5: 跑测试**

Run: `go test ./internal/auth/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat(auth): add Redis-backed session manager with cookie helpers"
```

---

### Task 7: user service

**Files:**
- Create: `internal/user/service.go`
- Create: `internal/user/service_test.go`

**Interfaces:**
- Consumes: `user.Repo`，`audit.Logger`
- Produces: `user.Service`，`user.NewService(repo Repo, audit *audit.Logger) *Service`，方法 Register/Login/UpdateRole/Disable

- [ ] **Step 1: 写实现**

`internal/user/service.go`:
```go
package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
)

type Service struct {
	repo  Repo
	audit *audit.Logger
}

func NewService(repo Repo, audit *audit.Logger) *Service {
	return &Service{repo: repo, audit: audit}
}

func validEmail(e string) bool {
	return strings.Contains(e, "@") && len(e) >= 3
}

func (s *Service) Register(ctx context.Context, email, username, password string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !validEmail(email) {
		return nil, apperr.New("validation_failed", "user", "invalid email")
	}
	if username == "" {
		return nil, apperr.New("validation_failed", "user", "username required")
	}
	if len(password) < 8 {
		return nil, apperr.New("validation_failed", "user", "password must be >= 8 chars")
	}
	if _, err := s.repo.GetByEmail(ctx, email); err == nil {
		return nil, apperr.New("validation_failed", "user", "email already registered")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &User{Email: email, Username: username, PasswordHash: hash, Role: RoleUser, Status: StatusActive}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, audit.Entry{Action: audit.ActionRegister, TargetType: "user", TargetID: u.ID.String(), Metadata: map[string]any{"email": email}})
	return u, nil
}

func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		s.audit.Log(ctx, audit.Entry{Action: audit.ActionLoginFailure, TargetType: "user", IP: ip, UserAgent: ua, Metadata: map[string]any{"email": email, "reason": "not_found"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	ok, err := auth.VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginFailure, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"reason": "bad_password"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	if u.Status != StatusActive {
		s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginFailure, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"reason": "disabled"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	s.repo.TouchLastLogin(ctx, u.ID)
	s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginSuccess, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua})
	return u, nil
}

func (s *Service) UpdateRole(ctx context.Context, actorID, targetID uuid.UUID, role, ip, ua string) error {
	if role != RoleUser && role != RolePlatformAdmin {
		return apperr.New("validation_failed", "user", "invalid role")
	}
	if err := s.repo.UpdateRole(ctx, targetID, role); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.ActionUserRoleChanged, TargetType: "user", TargetID: targetID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"new_role": role}})
	return nil
}

func (s *Service) Disable(ctx context.Context, actorID, targetID uuid.UUID, ip, ua string) error {
	if err := s.repo.UpdateStatus(ctx, targetID, StatusDisabled); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.ActionUserDisabled, TargetType: "user", TargetID: targetID.String(), IP: ip, UserAgent: ua})
	return nil
}
```

- [ ] **Step 2: 写单测（mock repo + no-op audit）**

`internal/user/service_test.go`:
```go
package user

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"go.uber.org/zap"
)

type mockRepo struct {
	users map[string]*User
}

func (m *mockRepo) Create(ctx context.Context, u *User) error {
	u.ID = uuid.New()
	m.users[u.Email] = u
	return nil
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, apperr.New("not_found", "user", "not found")
}
func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, apperr.New("not_found", "user", "not found")
}
func (m *mockRepo) List(ctx context.Context, limit, offset int) ([]User, int64, error) {
	return nil, 0, nil
}
func (m *mockRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error    { return nil }
func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error { return nil }
func (m *mockRepo) TouchLastLogin(ctx context.Context, id uuid.UUID) error             { return nil }

func newSvc() *Service {
	return NewService(&mockRepo{users: map[string]*User{}}, audit.NewLogger(nil, zap.NewNop()))
}

func TestRegister_Success(t *testing.T) {
	s := newSvc()
	u, err := s.Register(context.Background(), "A@B.com", "alice", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "a@b.com" {
		t.Fatalf("email not lowercased: %s", u.Email)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	s := newSvc()
	s.Register(context.Background(), "a@b.com", "alice", "password1")
	if _, err := s.Register(context.Background(), "a@b.com", "bob", "password1"); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	s := newSvc()
	if _, err := s.Register(context.Background(), "a@b.com", "alice", "short"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	s := newSvc()
	if _, err := s.Login(context.Background(), "x@y.com", "password1", "1.1.1.1", "ua"); err == nil {
		t.Fatal("expected unauthorized")
	}
}
```

注意：audit.NewLogger(nil, ...) — Logger.db 为 nil，Log 是异步 goroutine 调 db.Create 会 panic 但被 recover 吞掉。单测里这样安全但会刷 zap 错误日志。可接受。或改 mockRepo 测试不触发 audit 路径的断言。保持现状。

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/user/`
Expected: PASS（单测，无 integration tag）

- [ ] **Step 4: 提交**

```bash
git add internal/user
git commit -m "feat(user): add Service with Register/Login/UpdateRole/Disable"
```

---

### Task 8: auth middleware

**Files:**
- Create: `internal/auth/middleware.go`

**Interfaces:**
- Consumes: `SessionManager`，`user.Repo`
- Produces: `auth.AuthRequired(sm *SessionManager, userRepo user.Repo) gin.HandlerFunc`，`auth.RequireRole(role string) gin.HandlerFunc`，`auth.CurrentUser(c *gin.Context) (*user.User, bool)`

- [ ] **Step 1: 写实现**

`internal/auth/middleware.go`:
```go
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/user"
)

const currentUserKey = "current_user"

func AuthRequired(sm *SessionManager, userRepo user.Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(sm.cookieCfg.CookieName)
		if err != nil {
			c.Error(apperr.New("unauthorized", "auth", "missing session cookie"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
			return
		}
		uid, err := sm.Get(c.Request.Context(), sid)
		if err != nil {
			c.Error(err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
			return
		}
		u, err := userRepo.GetByID(c.Request.Context(), uid)
		if err != nil {
			c.Error(err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
			return
		}
		if u.Status != user.StatusActive {
			c.Error(apperr.New("unauthorized", "auth", "user disabled"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
			return
		}
		c.Set(currentUserKey, u)
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := CurrentUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "unauthorized"}})
			return
		}
		if u.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "forbidden", "message": "forbidden"}})
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*user.User, bool) {
	v, exists := c.Get(currentUserKey)
	if !exists {
		return nil, false
	}
	u, ok := v.(*user.User)
	return u, ok
}
```

- [ ] **Step 2: 写测试**

`internal/auth/middleware_test.go`:
```go
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
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/auth/`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/auth
git commit -m "feat(auth): add AuthRequired, RequireRole, CurrentUser middleware"
```

---

### Task 9: HTTP handlers + server wiring

**Files:**
- Create: `internal/httpserver/handlers/auth.go`
- Create: `internal/httpserver/handlers/users.go`
- Create: `internal/httpserver/handlers/routes.go`
- Modify: `internal/httpserver/server.go`

**Interfaces:**
- Consumes: `user.Service`，`SessionManager`，`user.Repo`，middleware
- Produces: 路由注册；`httpserver.Deps` 加 `UserSvc *user.Service`、`SessionMgr *auth.SessionManager`、`UserRepo user.Repo`

- [ ] **Step 1: 写 handlers/auth.go**

`internal/httpserver/handlers/auth.go`:
```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

type AuthHandlers struct {
	svc *user.Service
	sm  *auth.SessionManager
}

func NewAuthHandlers(svc *user.Service, sm *auth.SessionManager) *AuthHandlers {
	return &AuthHandlers{svc: svc, sm: sm}
}

type registerReq struct {
	Email    string `json:"email" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandlers) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Username, req.Password)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, toUserResp(u))
}

type loginReq struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandlers) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	u, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		c.Error(err)
		return
	}
	sid, err := h.sm.Create(c.Request.Context(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	h.sm.SetCookie(c, sid)
	c.JSON(http.StatusOK, toUserResp(u))
}

func (h *AuthHandlers) Logout(c *gin.Context) {
	sid, err := c.Cookie(h.sm.CookieName())
	if err == nil {
		h.sm.Delete(c.Request.Context(), sid)
	}
	h.sm.ClearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandlers) Me(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	c.JSON(http.StatusOK, toUserResp(u))
}

type userResp struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

func toUserResp(u *user.User) userResp {
	return userResp{ID: u.ID.String(), Email: u.Email, Username: u.Username, Role: u.Role, Status: u.Status}
}
```

- [ ] **Step 2: 写 handlers/users.go**

`internal/httpserver/handlers/users.go`:
```go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

type UserHandlers struct {
	svc *user.Service
}

func NewUserHandlers(svc *user.Service) *UserHandlers { return &UserHandlers{svc: svc} }

func (h *UserHandlers) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	users, total, err := h.svc.ListForAdmin(c.Request.Context(), limit, offset)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]userResp, len(users))
	for i, u := range users {
		out[i] = toUserResp(&u)
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": total})
}

func (h *UserHandlers) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	u, err := h.svc.GetForAdmin(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, toUserResp(u))
}

type patchReq struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

func (h *UserHandlers) Patch(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	var req patchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid body"))
		return
	}
	actor, _ := auth.CurrentUser(c)
	if req.Role != nil {
		if err := h.svc.UpdateRole(c.Request.Context(), actor.ID, id, *req.Role, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
			c.Error(err)
			return
		}
	}
	if req.Status != nil && *req.Status == user.StatusDisabled {
		if err := h.svc.Disable(c.Request.Context(), actor.ID, id, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
			c.Error(err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandlers) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperr.New("validation_failed", "user", "invalid id"))
		return
	}
	actor, _ := auth.CurrentUser(c)
	if err := h.svc.Disable(c.Request.Context(), actor.ID, id, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 3: 给 Service 加 ListForAdmin / GetForAdmin**

在 `internal/user/service.go` 末尾加：
```go
func (s *Service) ListForAdmin(ctx context.Context, limit, offset int) ([]User, int64, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) GetForAdmin(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}
```

- [ ] **Step 4: 给 SessionManager 加 CookieName 访问器**

在 `internal/auth/session.go` 末尾加：
```go
func (sm *SessionManager) CookieName() string { return sm.cookieCfg.CookieName }
```

- [ ] **Step 5: 写 handlers/routes.go**

`internal/httpserver/handlers/routes.go`:
```go
package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
)

func Register(r *gin.Engine, svc *user.Service, sm *auth.SessionManager, userRepo user.Repo) {
	authH := NewAuthHandlers(svc, sm)
	userH := NewUserHandlers(svc)

	r.POST("/register", authH.Register)
	r.POST("/login", authH.Login)

	authed := r.Group("")
	authed.Use(auth.AuthRequired(sm, userRepo))
	{
		authed.POST("/logout", authH.Logout)
		authed.GET("/me", authH.Me)
	}

	admin := r.Group("/admin")
	admin.Use(auth.AuthRequired(sm, userRepo), auth.RequireRole(user.RolePlatformAdmin))
	{
		admin.GET("/users", userH.List)
		admin.GET("/users/:id", userH.Get)
		admin.PATCH("/users/:id", userH.Patch)
		admin.DELETE("/users/:id", userH.Delete)
	}
}
```

- [ ] **Step 6: 改 server.go — Deps 加字段 + 装配路由**

在 `internal/httpserver/server.go` 的 Deps struct 加：
```go
	UserSvc   *user.Service
	SessionMgr *auth.SessionManager
	UserRepo  user.Repo
```
加 import：
```go
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/httpserver/handlers"
	"github.com/skillhub/skillhub/internal/user"
```
在 New() 里 `r.GET("/healthz", ...)` 之后加：
```go
	if deps.UserSvc != nil && deps.SessionMgr != nil && deps.UserRepo != nil {
		handlers.Register(r, deps.UserSvc, deps.SessionMgr, deps.UserRepo)
	}
```

- [ ] **Step 7: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 8: 提交**

```bash
git add internal/httpserver internal/user
git commit -m "feat(httpserver): add auth and user handlers, wire routes"
```

---

### Task 10: main 装配 + e2e 集成测试

**Files:**
- Modify: `cmd/skillhub/main.go`
- Create: `internal/httpserver/handlers/handlers_test.go`

**Interfaces:**
- Consumes: 所有 B 阶段组件

- [ ] **Step 1: 改 main.go — 装配 audit/user/session**

在 redis init 之后、httpserver.New 之前加：
```go
	auditLogger := audit.NewLogger(gdb, logger)
	userRepo := user.NewRepo(gdb)
	userSvc := user.NewService(userRepo, auditLogger)
	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth)
```
加 import：
```go
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/user"
```
把 httpserver.New 的 Deps 改为：
```go
	engine := httpserver.New(httpserver.Deps{
		Logger:    logger,
		DB:        gdb,
		Redis:     rdb,
		Storage:   store,
		UserSvc:   userSvc,
		SessionMgr: sessionMgr,
		UserRepo:  userRepo,
	})
```

- [ ] **Step 2: 写 e2e 集成测试**

`internal/httpserver/handlers/handlers_test.go`:
```go
//go:build integration

package handlers

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
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/user"
	"go.uber.org/zap"
)

func setupApp(t *testing.T) *gin.Engine {
	t.Helper()
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	gdb.Exec("TRUNCATE users RESTART IDENTITY CASCADE")
	rdb, _ := redispkg.New(cfg.Redis)
	rdb.FlushDB(context.Background())

	auditLogger := audit.NewLogger(gdb, zap.NewNop())
	userRepo := user.NewRepo(gdb)
	userSvc := user.NewService(userRepo, auditLogger)
	sessionMgr := auth.NewSessionManager(rdb, cfg.Auth)
	return httpserver.New(httpserver.Deps{
		Logger: zap.NewNop(), DB: gdb, Redis: rdb,
		UserSvc: userSvc, SessionMgr: sessionMgr, UserRepo: userRepo,
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
	// 直接造一个 platform_admin
	gdb, _ := db.New(func() config.Config { c, _ := config.Load("config/config.yaml"); return *c }().DB)
	hash, _ := auth.HashPassword("password1")
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
Expected: PASS（4 个测试）

- [ ] **Step 5: 跑全部单元测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/skillhub internal/httpserver/handlers
git commit -m "feat(main): wire auth/user/session; add e2e integration tests"
```

---

## Self-Review 记录

- **Spec 覆盖**：§3 数据模型→Task 2；§4.1 auth(password/session/middleware)→Task 5/6/8；§4.2 user(model/repo/service)→Task 3/7；§4.3 audit→Task 4；§4.4 handlers→Task 9；§5 config→Task 1；§8 测试→各任务 + Task 10 e2e；§9 交付物→Task 10。覆盖完整。
- **占位符**：无 TBD/TODO。
- **类型一致**：`user.User`/`user.Repo`/`user.Service` 签名在 Task 3/7/9 一致；`SessionManager` 方法在 Task 6/8/9 一致（CookieName 访问器在 Task 9 Step 4 补）；`audit.Logger.Log(ctx, Entry)` 在 Task 4/7 一致；`auth.HashPassword/VerifyPassword` 在 Task 5/7 一致。
- 已知小瑕疵：service 单测里 audit.NewLogger(nil,...) 的异步 goroutine 会尝试 db.Create(nil) 触发 panic 被 recover 吞掉，刷 zap 错误日志但不影响断言——可接受，已在 Task 7 Step 2 注明。
