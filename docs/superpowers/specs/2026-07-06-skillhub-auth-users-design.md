# SkillHub 认证与用户（子项目 B）设计

状态：草案
日期：2026-07-06
范围：仅子项目 B（认证与用户）。邮箱验证、密码重置、MFA、登录限流、OAuth、团队角色均不在本范围内。
依赖：子项目 A（config/log/db/redis/httpserver/apperr/middleware/migrate 已交付）。

## 1. 背景与目标

在 A 阶段骨架上实现用户体系与认证：注册、登录、登出、当前用户查询、用户管理（platform_admin），以及认证事件与用户管理操作的审计日志。

**目标**
- 用户可注册、登录、登出；登录后 cookie 维持会话，会话存 Redis。
- 受保护路由由中间件鉴权；platform_admin 可管理用户。
- 认证事件与用户管理操作写入审计表。

**非目标**
- 邮箱验证、密码重置、MFA、登录限流、OAuth。
- 团队命名空间与团队角色（owner/admin/member）——子项目 C。
- session 入库（无法在 DB 层批量踢出用户，接受此限制以换取简单）。

## 2. 技术选型

| 关注点 | 选型 |
|--------|------|
| 密码哈希 | argon2id（`golang.org/x/crypto/argon2`），参数硬编码 memory=64MiB time=3 threads=2 keyLen=32 |
| Session | 自定义 Redis 存储，随机 32 字节 session id |
| ORM | GORM（复用 A） |
| 路由 | Gin（复用 A） |
| 测试 Redis | miniredis（单元测试无外部依赖） |

## 3. 数据模型

### 3.1 users 表
```sql
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
```

### 3.2 audit_logs 表
```sql
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    actor_user_id UUID,
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    ip          TEXT,
    user_agent  TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX audit_logs_actor_idx ON audit_logs(actor_user_id);
CREATE INDEX audit_logs_action_idx ON audit_logs(action);
```

session 不入库，只存 Redis。

## 4. 组件设计

### 4.1 internal/password
**password.go**（独立叶子包，避免 internal/auth ↔ internal/user 循环依赖）
- `Hash(plain string) (string, error)` — argon2id，输出 `$argon2id$v=19$m=65536,t=3,p=2$<salt_b64>$<hash_b64>` 标准编码。
- `Verify(plain, encoded string) (bool, error)` — 解析编码、重算、常量时间比较。
- 参数常量：`memory=64*1024, time=3, threads=2, keyLen=32, saltLen=16`。

> 实现说明：原设计将 password 放在 internal/auth，但 auth/middleware 依赖 internal/user（取 current_user），而 user/service 依赖密码哈希，会成环。故 password 提取为无依赖的 internal/password 包。

### 4.1b internal/auth

**session.go**
- `SessionManager` 持有 `*redis.Client`、sessionTTL、cookie 配置。
- `Create(ctx, userID uuid.UUID) (sessionID string, err error)` — 生成 32 字节随机 id，Redis SET `session:<id>` = `<userID>` 带 TTL。
- `Get(ctx, sessionID string) (userID uuid.UUID, err error)` — 查 Redis，未命中返回 apperr `unauthorized`。
- `Delete(ctx, sessionID string) error` — 删 Redis key。
- `SetCookie(c *gin.Context, sessionID string)` / `ClearCookie(c *gin.Context)` — 按 config 写 HttpOnly cookie。

**middleware.go**
- `AuthRequired(sm *SessionManager, userRepo user.Repo) gin.HandlerFunc` — 读 cookie → sm.Get → userRepo.GetByID → 注入 `c.Set("current_user", user)`；失败返回 401。
- `RequireRole(role string) gin.HandlerFunc` — 从 context 取 current_user，校验 role，不符返回 403。
- `CurrentUser(c *gin.Context) (*user.User, bool)` — 从 context 取用户。

### 4.2 internal/user
**model.go**
- `User` GORM 模型，字段对应 users 表。`Role`/`Status` 用字符串枚举常量。
- 常量：`RoleUser`、`RolePlatformAdmin`；`StatusActive`、`StatusDisabled`。

**repo.go**
- `Repo interface { Create(ctx, *User) error; GetByID(ctx, id) (*User, error); GetByEmail(ctx, email) (*User, error); List(ctx, limit, offset) ([]User, int64, error); UpdateRole(ctx, id, role) error; UpdateStatus(ctx, id, status) error; TouchLastLogin(ctx, id) error }`
- GORM 实现。未找到返回 apperr `not_found`。

**service.go**
- `Service` 持有 repo + audit logger。
- `Register(ctx, email, username, password) (*User, error)` — 校验输入、查重、哈希、创建、审计 register。
- `Login(ctx, email, password, ip, ua) (*User, error)` — 取用户、VerifyPassword、检查 status、TouchLastLogin、审计 login_success/login_failure。
- `UpdateRole(ctx, actorID, targetID, role, ip, ua) error` — 平台管理员改角色，审计 user_role_changed。
- `Disable(ctx, actorID, targetID, ip, ua) error` — 置 disabled，审计 user_disabled。

### 4.3 internal/audit
**audit.go**
- `Action` 枚举：`LoginSuccess`、`LoginFailure`、`Logout`、`Register`、`UserRoleChanged`、`UserDisabled`。
- `Logger` 持有 `*gorm.DB`，`Log(ctx, entry Entry) error`，异步写（goroutine + recover，失败不阻塞主流程，仅 zap 记错误）。
- `Entry { ActorUserID *uuid.UUID; Action Action; TargetType string; TargetID string; IP string; UserAgent string; Metadata map[string]any }`。

### 4.4 HTTP handlers
`internal/httpserver/handlers/auth.go`、`users.go`，注册到 server.go 的 engine。
- `POST /register` — 解析 JSON，调 user.Service.Register，返回 201 + 用户（无 password_hash）。
- `POST /login` — 解析 JSON，调 Login，session.Create，SetCookie，返回 200 + 用户。
- `POST /logout` — AuthRequired，session.Delete，ClearCookie，204。
- `GET /me` — AuthRequired，返回 current_user。
- `GET /admin/users` — AuthRequired + RequireRole(platform_admin)，分页列表。
- `GET /admin/users/:id` — 同上，单个。
- `PATCH /admin/users/:id` — 同上，改 role 或 status。
- `DELETE /admin/users/:id` — 同上，调 Disable。

所有 handler 错误经 A 阶段 errors 中间件统一渲染。

## 5. 配置新增

`config/config.yaml` 加 `auth` 段：
```yaml
auth:
  session_ttl: 24h
  cookie_name: sid
  cookie_secure: false
  cookie_domain: ""
  cookie_samesite: lax      # strict | lax | none
```
config.go 加 `AuthConfig` 结构体与 Validate（session_ttl > 0、cookie_name 非空、cookie_samesite ∈ {strict,lax,none}）。

## 6. 数据流

**注册**：`POST /register → user.Service.Register → HashPassword → repo.Create → audit.Log → 201`。
**登录**：`POST /login → Service.Login（VerifyPassword + TouchLastLogin + audit）→ session.Create → SetCookie → 200`。
**鉴权**：`请求 → AuthRequired（cookie→session→user）→ RequireRole → handler`。
**登出**：`POST /logout → AuthRequired → session.Delete → ClearCookie → 204`。
**管理**：`/admin/users/* → AuthRequired + RequireRole(platform_admin) → Service → audit → 2xx`。

## 7. 错误处理

- 输入校验失败 → apperr `validation_failed` (422)。
- 登录失败（用户不存在/密码错/禁用）→ 统一返回 apperr `unauthorized` (401)，不区分原因（防枚举）；审计记真实原因。
- 未登录访问受保护路由 → 401。
- 角色不足 → 403。
- 重复 email/username → apperr `validation_failed` (422)，message 说明冲突字段。

## 8. 测试策略

- **单元测试**（无外部依赖）：
  - password 哈希/验证（含错误编码、常量时间）。
  - session manager 用 miniredis（Create/Get/Delete、TTL、未命中）。
  - user.Service.Register 输入校验、查重（用 mock repo）。
- **集成测试**（build tag `integration`，依赖 compose 的 PG + Redis）：
  - 真实 PG 跑 user.Repo 全部方法。
  - 端到端：注册→登录→/me→logout；登录失败；admin 改角色/禁用；审计落库。
- handler 测试用 httptest + 真实 service（集成 tag）。

## 9. 交付物

- `make compose-up && make migrate-up && make run` 后，上述 8 条路由可用。
- 注册→登录→访问 /me 成功；未登录访问 /me 返回 401；user 访问 /admin/users 返回 403。
- 审计表记录认证与管理事件。

## 10. 后续衔接

- C 团队命名空间：复用 users 表与 RBAC 中间件，加团队角色。
- D 发布：复用 AuthRequired + RequireRole 保护发布路由。
- F 治理：复用 audit.Logger 记所有治理操作。
