# SkillHub 治理（子项目 F）设计

状态：草案
日期：2026-07-09
范围：仅子项目 F（platform_admin 治理）。社交（G）、i18n（H）不在本范围内。
依赖：子项目 A–E（skills/skill_versions、storage.Store、audit、team、auth.RequireRole）。

## 1. 背景与目标

platform_admin 可将某团队 skill 的指定版本**提升**至 global 命名空间，使其对所有用户可见可拉。提升复制 storage 对象至 global 拥有的 key，global skill 自包含。同时提供审计日志只读接口供 platform_admin 监督。

**目标**
- `POST /admin/skills/promote`：platform_admin 指定源（team_slug + skill_name + version）与目标 global skill 名，复制对象 + 插 global skill/version 行，落审计。
- `GET /admin/audit`：platform_admin 分页查看审计日志，可按 action/actor_id/target_type 过滤。
- 提升后该 global skill 立即出现在所有人的 `GET /skills` 搜索结果与拉取中（复用 D/E）。

**非目标**
- 审核流（多人审批、草态）。
- 批量提升、定时提升。
- 审计日志修改/删除（只读）。
- 非 platform_admin 的任何治理操作。

## 2. 技术选型

复用 A–E 的 GORM、Gin、auth.RequireRole、skill.Repo/Service、storage.Store、audit。无新依赖、无新迁移。

## 3. 数据模型

无变更。提升复用 skills（team_id = global 团队）+ skill_versions 表；审计复用 audit_logs。

## 4. 组件设计

### 4.1 internal/skill/service.go（扩展）
- `PromoteToGlobal(ctx, srcSkillID, version uuid, globalTeamID uuid.UUID, targetName string, adminID uuid.UUID) (*SkillVersion, error)`
  - `srcVersion := repo.GetVersion(srcSkillID, version)` —— 404 若不存在。
  - 校验 `IsValidName(targetName)`。
  - `globalSkill := repo.GetSkill(globalTeamID, targetName)`；not_found 则 `repo.CreateSkill`。
  - `repo.GetVersion(globalSkill.ID, version)` 命中 → conflict (409) "version already exists"。
  - 复制对象：`rc := store.Get(srcVersion.StorageKey)`；读全量到 buffer（≤ MaxPackageSize）；算 sha256 并校验 == srcVersion.Sha256（完整性）；newKey = `skills/{globalSkill.ID}/{version}/{sha}.tar.gz`；`store.Put(newKey, buf, size, ct)`。
  - `repo.CreateVersion({SkillID: globalSkill.ID, Version, StorageKey: newKey, Size, Sha256, ContentType, PublisherUserID: adminID})`；若冲突（竞态）清理 newKey 后返回 conflict。
  - 审计 `skill_promoted_to_global`（metadata: src_skill_id, global_skill_id, version, target_name）。
  - 返回 global 版本。

### 4.2 internal/audit（扩展读）
新增 `query.go`：
- `Record` 读模型：`ID int64, ActorUserID *uuid.UUID, Action, TargetType, TargetID, IP, UserAgent string, Metadata map[string]any, CreatedAt time.Time`。
- `Filter`：`Action, TargetType, TargetID, ActorID *string`（nil 不过滤）+ `Limit, Offset int`。
- `Logger.List(ctx, filter) ([]Record, error)` —— 查 audit_logs，按 created_at DESC，分页。Metadata JSON 反解为 map。

### 4.3 internal/httpserver/handlers（扩展）
- `POST /admin/skills/promote` —— `RequireRole(platform_admin)`；body `{team_slug, skill_name, version, target_name}`：
  - handler 用 `teamSvc.Repo().GetBySlug(team_slug)` → srcTeam（404）；`skillSvc.Repo().GetSkill(srcTeam.ID, skill_name)` → srcSkill（404）；`teamSvc.Repo().GetBySlug("global")` → globalTeam。
  - `skillSvc.PromoteToGlobal(srcSkill.ID, version, globalTeam.ID, target_name, admin.ID)`。
  - 201 返回 global 版本元数据。
- `GET /admin/audit?action=&actor_id=&target_type=&page=&page_size=` —— `RequireRole(platform_admin)`：
  - 解析过滤与分页（page 1-based，page_size 默认 20、上限 100）。
  - `auditLogger.List(ctx, filter)`。
  - 200 `{items:[...], page, page_size}`。

均挂到 admin 路由组（已有 `RequireRole(platform_admin)`）。

### 4.4 server.go / main.go
`handlers.Register` 签名不变（auditLogger 已在 main 构造，但未注入 Deps）。需给 Deps 加 `AuditLogger *audit.Logger` 并传给 handlers。或 handlers 直接通过 skillSvc 间接拿——不妥。**决定**：Deps 加 `AuditLogger *audit.Logger`，Register 加该参数，admin handler 组用之。

## 5. 数据流

提升：`POST /admin/skills/promote → AuthRequired → RequireRole(platform_admin) → handler 解析源 team/skill + global team → skillSvc.PromoteToGlobal → store.Get+Put 复制对象 → repo.CreateSkill(若需)+CreateVersion → audit → 201`。
审计：`GET /admin/audit → RequireRole(platform_admin) → auditLogger.List → 200 items`。

## 6. 错误处理

- 非 platform_admin → 403（RequireRole）。
- 源 team/skill/version 不存在 → 404。
- target_name 非法 → validation_failed (422)。
- global 已有同名 skill + 同 version → conflict (409)。
- 复制时 sha 与源不符 → 500 "integrity check failed"（理论上不应发生）。
- 分页参数非法 → validation_failed (422)。

## 7. 测试策略

- **单元测试**：PromoteToGlobal 成功路径（mock repo + mem store，校验对象被复制到新 key、global version 行写入）；目标 skill 已存在则复用；version 冲突 → conflict；源 version 不存在 → not_found；target_name 非法 → validation_failed。audit.List 过滤/分页（用真实 PG 在集成测试里更合适，单测跳过或用 mock）。
- **集成测试**（build tag `integration`）：e2e 提升：owner 发布 acme/go-lint@1.0.0 → platform_admin 提升至 global/go-lint → 任意用户 `GET /skills?q=lint` 见到 global/go-lint → 任意用户下载成功且内容一致。重复提升同 version → 409。非 platform_admin 提升 → 403。`GET /admin/audit` 返回含 `skill_promoted_to_global` 的记录，按 action 过滤命中。

## 8. 交付物

- `make compose-up && make migrate-up && make run` 后 `POST /admin/skills/promote` 与 `GET /admin/audit` 可用。
- platform_admin 可提升团队 skill 版本至 global；提升后全员可见可拉。
- platform_admin 可分页查看审计日志。

## 9. 后续衔接

- G 社交：审计可记录点赞/收藏，复用 audit 表。
- H i18n：审计 action 名/消息可本地化。
