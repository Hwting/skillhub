# SkillHub 技能包发现（子项目 E）设计

状态：草案
日期：2026-07-08
范围：仅子项目 E（技能包发现/搜索）。审核与提升至 global（F）、社交（G）、i18n（H）不在本范围内。
依赖：子项目 A（骨架）、B（认证）、C（团队/IsMember）、D（skills/skill_versions/team.Service.CanPublish）。

## 1. 背景与目标

提供跨命名空间的技能包检索与浏览。调用者可见 global 命名空间的 skill 以及自己所属团队（owner/member）的 skill；非成员看不到私有团队 skill。支持按 name 全文匹配，分页返回，每条含最新版本摘要。

**目标**
- `GET /skills?q=&page=&page_size=`：返回调用者可见的 skill，按相关性或更新时间排序，offset 分页。
- 全文检索基于 PG tsvector（`simple` 配置，覆盖 skill.name），GIN 索引。
- 每条结果含 skill 元数据 + 最新版本（按 semver 最高）摘要：version/size/updated_at。
- 可见性：global ∪ 用户所属团队，与 D 的拉取可见性一致。

**非目标**
- 搜索 skill_versions 内部内容（readme 全文）。
- 搜索团队/用户。
- 排行/推荐/下载量统计。
- 按团队参数过滤（本版只做「我可见的全部」；团队内列表已由 D 的 `GET /teams/:slug/skills` 提供）。

## 2. 技术选型

复用 A–D 的 GORM、Gin、auth、team.Service、skill.Repo/Service。检索用 PG 内建 `tsvector`/`tsquery` + GIN；无新依赖。

## 3. 数据模型变更

给 `skills` 加生成列与索引（迁移 000005）：
```sql
ALTER TABLE skills
  ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', name)) STORED;
CREATE INDEX skills_search_idx ON skills USING GIN (search_vector);
```
- `simple` 配置：小写化、按非字母数字拆词，适合含连字符的 skill 名（`my-skill` → `my` `skill`）。
- 生成列由 DB 维护，GORM 模型不映射该列（插入时不写，读取不需要）。
- down：`DROP INDEX skills_search_idx; ALTER TABLE skills DROP COLUMN search_vector;`

`skill_versions` 不变。

## 4. 组件设计

### 4.1 internal/skill/repo.go（扩展）
新增：
- `Search(ctx, teamIDs []uuid.UUID, q string, limit, offset int) ([]Skill, error)`
  - `teamIDs` 为调用者可见的团队 id 集合（含 global）。空集合 → 返回空。
  - q 非空 → `WHERE team_id IN (?) AND search_vector @@ plainto_tsquery('simple', ?)`，`ORDER BY ts_rank(search_vector, plainto_tsquery('simple', ?)) DESC, name ASC`。
  - q 空 → `WHERE team_id IN (?)`，`ORDER BY updated_at DESC, name ASC`。
  - `limit` 上限 100，`offset >= 0`。
- 复用已有 `ListVersions` 取最新版本。

### 4.2 internal/skill/service.go（扩展）
- `SearchResult` 结构：`Skill` + `LatestVersion *SkillVersion`（无版本时为 nil）。
- `Search(ctx, teamIDs []uuid.UUID, q string, page, pageSize int) ([]SearchResult, error)`
  - 校验/夹取分页（pageSize 默认 20、上限 100；page 从 1 起，offset=(page-1)*pageSize）。
  - `repo.Search(...)` 取一页 skill。
  - 批量取这些 skill 的版本：对每个 skill `ListVersions`，用 `skill.Compare` 选 semver 最高者。N+1 但页大小 ≤100、每 skill 版本数少，可接受。
  - 返回 `[]SearchResult`。

### 4.3 internal/httpserver/handlers/skills.go（扩展）
新增 `Search` handler，挂到 `authed` 组：
- `GET /skills` — AuthRequired。
  - 取 current_user。
  - `teamSvc.Repo().GetBySlug("global")` 得 global 团队 id。
  - `teamSvc.Repo().ListForUser(userID)` 得用户所属团队，合并 id 切片（含 global）。
  - 解析 q（query param，可选）、page（默认 1）、page_size（默认 20）。
  - `skillSvc.Search(teamIDs, q, page, pageSize)`。
  - 200 返回 `{"items":[{id,team_id,team_slug,name,latest_version:{version,size,updated_at}}], "page":..., "page_size":..., "total":null}`（total 不精确计数，省一次 count；如需可后补）。

team_slug 需在结果里展示：批量取 team id → slug 映射。handler 已有 teamSvc.Repo()，可一次 `GetByID` 每个，或加一个批量方法。本版用 N 次 GetByID（页小，可接受）；若嫌繁，service 可直接 JOIN teams 返回 slug。**决定**：repo.Search 直接 `SELECT skills.*, teams.slug AS team_slug` 并在 SearchResult 里带 TeamSlug，避免 N+1。相应调整 §4.2：SearchResult 含 TeamSlug，repo.Search 返回 `[]SearchRow`（Skill + TeamSlug）。

修订（取代上面段落）：repo.Search 返回 `[]SearchRow{Skill, TeamSlug string}`，单查询 JOIN teams。service 包裹为 SearchResult{Skill, TeamSlug, LatestVersion}。

### 4.4 server.go / main.go
无新增依赖；`handlers.Register` 签名不变（skillSvc 已注入）。仅新增一条路由。

## 5. 数据流

`GET /skills?q=foo → AuthRequired → handler 取 user → teamSvc.Repo().GetBySlug(global) + ListForUser(userID) → 合并 teamIDs → skillSvc.Search(teamIDs, q, page, size) → repo.Search (JOIN teams, FTS) → 批量 ListVersions 选最新 → 200 items`。

## 6. 错误处理

- page/page_size 非整数 → validation_failed (422)。
- page < 1 或 page_size < 1 → validation_failed (422)。
- teamIDs 为空（理论上不会，global 总在）→ 返回空 items。
- q 超长（> 256）→ validation_failed (422)。
- 其他 DB 错误 → 500（经 errors 中间件）。

## 7. 测试策略

- **单元测试**：service.Search 分页夹取、q 为空/非空分支、LatestVersion 选取（多版本取 semver 最高）；用 mock repo。
- **集成测试**（build tag `integration`）：repo.Search 命中 tsvector（建若干 skill 含 name 词，q 命中/不命中）；可见性（用户只见到 global + 自己团队的 skill，非成员团队不可见）；分页（page_size/offset）；e2e：`GET /skills?q=` 返回可见结果，非成员看不到私有团队 skill。

## 8. 交付物

- `make compose-up && make migrate-up && make run` 后 `GET /skills?q=&page=&page_size=` 可用。
- 搜索结果仅含调用者可见的 skill；每条含最新版本摘要；按相关性或更新时间排序。

## 9. 后续衔接

- F 治理：platform_admin 提升某 team skill 版本至 global；提升后该 skill 出现在所有人的搜索结果中。
- G 社交：搜索结果可附带点赞/下载数（需新表）。
- 排行：若需按下载量排序，需在 D 加下载计数。
