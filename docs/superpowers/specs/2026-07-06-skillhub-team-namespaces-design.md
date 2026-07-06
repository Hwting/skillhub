# SkillHub 团队命名空间（子项目 C）设计

状态：草案
日期：2026-07-06
范围：仅子项目 C（团队命名空间与成员）。技能包发布（D）、审核治理与提升至全局（F）、团队配额均不在本范围内。
依赖：子项目 A（骨架）、B（用户/认证/审计/RBAC 中间件）。

## 1. 背景与目标

将技能组织到团队或全局命名空间内。每个命名空间有成员、角色（owner/admin/member）与发布策略。任何登录用户可创建团队并成为 owner。global 命名空间由迁移预置，由 platform_admin 治理。

**目标**
- 用户可创建团队、管理成员与角色、设置发布策略、转移所有权。
- 团队级权限由中间件强制；后续子项目 D/F 复用权限判定。
- 所有成员/策略/所有权变更落审计。

**非目标**
- 技能包与发布（D）、审核与提升至全局（F）、团队配额/计费。
- 多 owner（owner 单一，提供转移）。

## 2. 技术选型

复用 A/B 的 GORM、Gin、apperr、audit、auth 中间件。无新依赖。

## 3. 数据模型

### 3.1 teams
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
```
迁移预置：`INSERT INTO teams(slug, name, owner_user_id, publish_policy) VALUES ('global','Global',NULL,'admin_only')`。
保留 slug `global`；其他团队 slug 非空且 ≠ `global`。

### 3.2 team_members
```sql
CREATE TABLE team_members (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('admin','member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX team_members_user_idx ON team_members(user_id);
```
owner 不入此表（由 teams.owner_user_id 隐式表示）。

## 4. 组件设计

### 4.1 internal/team
**model.go**
- `Team` GORM 模型（字段对应 teams 表）。
- `TeamMember` GORM 模型（联合主键 team_id+user_id, role, created_at）。
- 常量：`RoleAdmin`、`RoleMember`；`PolicyAdminOnly`、`PolicyAnyMember`；`GlobalSlug = "global"`。

**repo.go** — `Repo interface`
- `Create(ctx, *Team) error`
- `GetBySlug(ctx, slug) (*Team, error)` / `GetByID(ctx, id) (*Team, error)`
- `ListForUser(ctx, userID) ([]Team, error)` — 用户作为 owner 或 member 的所有团队
- `ListMembers(ctx, teamID) ([]TeamMember, error)`
- `GetMember(ctx, teamID, userID) (*TeamMember, error)` — 未命中返回 apperr not_found
- `AddMember(ctx, TeamMember) error` / `UpdateMemberRole(ctx, teamID, userID, role) error` / `RemoveMember(ctx, teamID, userID) error`
- `TransferOwnership(ctx, teamID, newOwnerID) error` — 更新 teams.owner_user_id
- `SetPublishPolicy(ctx, teamID, policy) error` / `SetName(ctx, teamID, name) error`
- `Delete(ctx, teamID) error`
- GORM 实现，未命中映射 apperr not_found。

**service.go** — `Service`
- 持有 repo + audit。
- `Create(ctx, slug, name, ownerID) (*Team, error)` — 校验 slug 非空/≠global/无冲突，创建，审计 team_created。
- `Update(ctx, actorID, teamID, name *string, policy *string) error` — owner only；审计 publish_policy_changed。
- `AddMember(ctx, actorID, teamID, userID, role) error` — admin+ only；审计 member_added。
- `UpdateMemberRole(ctx, actorID, teamID, userID, role) error` — owner only；审计 member_role_changed。
- `RemoveMember(ctx, actorID, teamID, userID) error` — admin+ only；审计 member_removed。
- `TransferOwnership(ctx, actorID, teamID, newOwnerID) error` — owner only；新 owner 必须已是成员（转移后旧 owner 降为 admin）；审计 ownership_transferred。
- `Delete(ctx, actorID, teamID) error` — owner only；禁止删 global；审计 team_deleted。
- 权限判定方法：`IsOwner(ctx, team, userID) bool`、`IsAdminOrOwner`、`IsMember`、`CanPublish(team, userID) bool`。

**permission 规则**
- `IsOwner(t, u)`：`t.owner_user_id != nil && *t.owner_user_id == u.id`
- `IsMember`：IsOwner 或 GetMember 命中
- `IsAdminOrOwner`：IsOwner 或 member.role==admin
- `CanPublish(t, u)`：IsOwner，或 policy==admin_only 且 IsAdminOrOwner，或 policy==any_member 且 IsMember
- global（owner=null）：IsOwner 永假；治理权由 platform_admin 角色单独判定（handler 层）。

### 4.2 internal/auth/middleware.go（扩展）
- `TeamScoped(teamSvc *team.Service, required string) gin.HandlerFunc` — `required` ∈ {owner, admin, member}。
  - 从 `:slug` 取 team，GetBySlug；未找到 404。
  - 取 current_user；按 required 调 IsOwner/IsAdminOrOwner/IsMember；不符 403。
  - global 团队 + required==owner：要求 current_user.Role==platform_admin。
  - 注入 `c.Set("current_team", team)`。
- `CurrentTeam(c) (*team.Team, bool)`。

### 4.3 HTTP handlers — `internal/httpserver/handlers/teams.go`
- `POST /teams` — authed；body {slug, name}；service.Create；201。
- `GET /teams` — authed；service.ListForUser(current_user)。
- `GET /teams/:slug` — TeamScoped(member)；返回 team + 成员数。
- `PATCH /teams/:slug` — TeamScoped(owner)；body {name?, publish_policy?}；service.Update。
- `DELETE /teams/:slug` — TeamScoped(owner)；service.Delete。
- `GET /teams/:slug/members` — TeamScoped(member)；ListMembers（含 owner 行，role=owner）。
- `POST /teams/:slug/members` — TeamScoped(admin)；body {user_id, role}；service.AddMember。
- `PATCH /teams/:slug/members/:uid` — TeamScoped(owner)；body {role}；service.UpdateMemberRole。
- `DELETE /teams/:slug/members/:uid` — TeamScoped(admin)；service.RemoveMember。
- `POST /teams/:slug/transfer` — TeamScoped(owner)；body {new_owner_id}；service.TransferOwnership。

响应中 team 不暴露 owner_user_id 为 null 的细节以外的敏感信息；成员列表含 user_id/role/created_at。

## 5. 数据流

创建：`POST /teams → service.Create → repo.Create → audit → 201`。
加成员：`POST /teams/:slug/members → TeamScoped(admin) → service.AddMember → repo.AddMember → audit → 204`。
权限校验：`请求 → AuthRequired → TeamScoped(role) → handler → service`。
转移：`POST /teams/:slug/transfer → TeamScoped(owner) → service.TransferOwnership（旧 owner 降 admin，新 owner 升 owner）→ audit → 204`。

## 6. 错误处理

- slug 冲突 → validation_failed (422) "slug already taken"。
- slug == global 由非平台管理员创建 → forbidden (403)。
- 删除 global → validation_failed (422) "cannot delete global namespace"。
- 权限不足 → forbidden (403)。
- 成员不存在 → not_found (404)。
- 转移给非成员 → validation_failed (422) "new owner must be a current member"。

## 7. 测试策略

- **单元测试**：service 权限判定矩阵（IsOwner/IsAdminOrOwner/IsMember/CanPublish 全组合）；Create slug 校验；TransferOwnership 前置条件。用 mock repo。
- **集成测试**（build tag `integration`，PG）：repo 全方法；e2e：创建团队→加成员→改角色→转移→删除；global 不可删；非成员访问 403；member 访问 admin 路由 403；platform_admin 治理 global。

## 8. 交付物

- `make compose-up && make migrate-up && make run` 后上述 9 条路由可用。
- 任意用户可创建团队并成为 owner；owner 可管理成员/策略/转移；admin 可加移成员；member 可查看。
- global 团队存在且仅 platform_admin 可治理。

## 9. 后续衔接

- D 发布：复用 team.Service.CanPublish 控制谁能往命名空间发包。
- F 治理：复用 team 模型与 audit；platform_admin 提升技能至 global 团队。
- E 发现：按 team slug 过滤，可见性规则复用 IsMember。
