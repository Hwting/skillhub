# SkillHub 技能包发布（子项目 D）设计

状态：草案
日期：2026-07-07
范围：仅子项目 D（技能包发布与拉取）。包发现/搜索（E）、审核与提升至 global（F）、团队配额均不在本范围内。
依赖：子项目 A（骨架、storage.Store）、B（用户/认证/审计）、C（团队命名空间、team.Service.CanPublish）。

## 1. 背景与目标

将技能包发布到团队或 global 命名空间下。包属于某个团队，有名字与多个不可变版本。发布时包内容（tarball）写入 storage.Store，元数据落 db。拉取按版本从 storage 流式返回。

**目标**
- 已认证用户经 `team.Service.CanPublish` 授权后，可往某团队命名空间发布某 skill 的新版本。
- 团队成员可拉取该团队任意版本；global 命名空间任意认证用户可拉取。
- 版本严格 semver，不可变：同 skill+version 重复发布 → 409；无删除/yank。
- 发布落审计。

**非目标**
- 包搜索/发现/排行（E）。
- 审核流与提升至 global（F）——global 命名空间在本子项目内**只读**，无人可通过发布 API 写入 global。
- 解析包内容/manifest：服务端原样存 tarball，不校验内部结构。
- 删除、yank、覆盖。

## 2. 技术选型

复用 A/B/C 的 GORM、Gin、apperr、audit、auth 中间件、team.Service、storage.Store。无新依赖：semver 用包内正则校验。

## 3. 数据模型

### 3.1 skills
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
```
name 格式：`^[a-z0-9][a-z0-9-]{0,62}$`（小写字母数字与连字符，1–63 字符）。

### 3.2 skill_versions
```sql
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
version 为合法 semver（见 §4.1）。`storage_key` 形如 `skills/{skill_id}/{version}.tar.gz`。

## 4. 组件设计

### 4.1 internal/skill
**semver.go**
- `IsValid(v string) bool` —— 正则 `^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`（npm 风格，不带前导 v）。
- `Compare(a, b string) int` —— 按主.次.修排序，prerelease 视为小于对应正式版。仅用于列表排序，不实现完整 semver 规范。

**model.go**
- `Skill`（id, team_id, name, created_at, updated_at）、`SkillVersion`（字段对应表）。
- 常量：`NamePattern`、`MaxNameLen = 63`、`ContentTypeTarball = "application/gzip"`。

**repo.go** — `Repo interface`
- `CreateSkill(ctx, *Skill) error`
- `GetSkill(ctx, teamID uuid.UUID, name string) (*Skill, error)` —— 未命中 apperr not_found
- `ListSkillsByTeam(ctx, teamID) ([]Skill, error)`
- `CreateVersion(ctx, *SkillVersion) error` —— 唯一冲突映射 apperr conflict
- `GetVersion(ctx, skillID, version string) (*SkillVersion, error)`
- `ListVersions(ctx, skillID) ([]SkillVersion, error)`
- GORM 实现，唯一冲突（pg `23505`）映射为 `apperr.New("conflict", "skill", "version already exists")`。

**service.go** — `Service`
- 持有 repo + storage.Store + audit。
- `Publish(ctx, teamID uuid.UUID, name, version string, r io.Reader, size int64, contentType, publisherID) (*SkillVersion, error)`
  - 校验 name 格式、version semver；拒绝 global（team slug == global → forbidden，由调用方/中间件保证，service 内不查 team；但 service 校验 teamID != uuid.Nil）。
  - 读全量内容到 buffer（包大小有上限，见 §6），算 sha256。
  - `GetSkill`；未找到则 `CreateSkill`（新建 skill 行）。
  - `storage.Put(key, bytes, size, contentType)`；key = `skills/{skill_id}/{version}.tar.gz`。
  - `CreateVersion`；若冲突，清理已上传对象后返回 conflict。
  - 审计 `skill_version_published`（首次发布附带 `skill_created`）。
- `GetSkillWithVersions(ctx, teamID, name) (*Skill, []SkillVersion, error)`
- `OpenVersion(ctx, teamID, name, version) (io.ReadCloser, *SkillVersion, error)` —— 校验 team/skill/version 存在，返回 storage.Get 流。
- `ListSkillsByTeam(ctx, teamID) ([]Skill, error)`

### 4.2 HTTP handlers — `internal/httpserver/handlers/skills.go`
路由（均在 `auth.AuthRequired` 之后）：
- `POST /teams/:slug/skills/:name/versions/:version` —— `TeamScoped("member")` 加载 team；handler 内 `teamSvc.CanPublish(ctx, t, user.ID)` 否则 403；body 为 tarball 原始字节（Content-Type `application/gzip`）；`skillSvc.Publish`；201 返回版本元数据。global 团队经 TeamScoped 放行（any authed），但 CanPublish 对 global 恒假 → 403，等价于「global 不可发布」。
- `GET /teams/:slug/skills` —— `TeamScoped("member")`；列出团队内 skills。
- `GET /teams/:slug/skills/:name` —— `TeamScoped("member")`；skill 元数据 + 版本列表。
- `GET /teams/:slug/skills/:name/versions/:version` —— `TeamScoped("member")`；流式下载（Content-Type/Length/Disposition）。

拉取可见性由 `TeamScoped("member")` 统一保证：非 global 团队仅成员，global 任意认证用户。

### 4.3 server.go / main.go 装配
- `httpserver.Deps` 加 `SkillSvc *skill.Service`。
- `handlers.Register` 加 `skillSvc` 参数。
- `main.go` 构造 `skill.NewRepo(gdb)`、`skill.NewService(skillRepo, store, auditLogger)`。

## 5. 数据流

发布：`POST /teams/:slug/skills/:name/versions/:version → AuthRequired → TeamScoped(member) → CanPublish? → skillSvc.Publish → storage.Put + repo.CreateVersion → audit → 201`。
拉取：`GET .../versions/:version → AuthRequired → TeamScoped(member) → skillSvc.OpenVersion → storage.Get → io.Copy → 200 (stream)`。

## 6. 错误处理

- name 格式非法 → validation_failed (422)。
- version 非 semver → validation_failed (422) "invalid version"。
- 同 skill+version 已存在 → conflict (409) "version already exists"。
- 包体超过上限（默认 50 MiB）→ validation_failed (422) "package too large"。
- 非成员拉取团队包 → 403（由 TeamScoped）。
- 无 CanPublish 权限发布 → 403 "forbidden"。
- skill/version 不存在 → 404。
- 发布到 global → 403（CanPublish 恒假）。

## 7. 测试策略

- **单元测试**：semver IsValid/Compare 矩阵；service.Publish 重复版本 → conflict；name/version 校验；mock repo + fake storage。
- **集成测试**（build tag `integration`，PG + local storage）：repo 全方法；e2e：成员发布→拉取（校验 sha256）→再发布新版本→列表；非成员拉取 403；非 CanPublish 用户发布 403；重复版本 409；global 发布 403；global 拉取成功（任意认证用户）。

## 8. 交付物

- `make compose-up && make migrate-up && make run` 后上述 4 条路由可用。
- 团队成员可往所属团队发包、拉包；global 可拉不可发；版本不可变、不可删。

## 9. 后续衔接

- E 发现：按 team slug 过滤、可见性复用 IsMember；按 version 列表/最新版快捷拉取。
- F 治理：platform_admin 将某 team skill 的某版本「提升」为 global skill 的一个版本（复制 storage 对象 + 插 global skill 行），复用本子项目的 repo/service。
