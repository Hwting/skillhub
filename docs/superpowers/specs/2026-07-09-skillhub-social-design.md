# SkillHub 社交（子项目 G）设计

状态：草案
日期：2026-07-09
范围：仅子项目 G（skill 收藏/star）。i18n（H）不在本范围内。
依赖：子项目 A–F（skills、team、auth、skill.Service）。

## 1. 背景与目标

用户可收藏（star）可见的 skill，查看自己的收藏列表，skill 详情展示收藏数与当前用户是否已收藏。star 挂在 skill 级（不区分版本），幂等。

**目标**
- `POST /teams/:slug/skills/:name/star` —— 收藏（幂等，204）。
- `DELETE /teams/:slug/skills/:name/star` —— 取消收藏（幂等，204）。
- `GET /me/stars` —— 分页返回当前用户收藏的 skill（含 team_slug/name/latest_version）。
- `GET /teams/:slug/skills/:name` 响应增加 `star_count` 与 `is_starred`。

**非目标**
- 版本级 star、评论、关注用户、动态 feed。
- star 通知。
- 按收藏数排行（可后补，复用 star_count）。

## 2. 技术选型

复用 A–F 的 GORM、Gin、auth、team、skill。无新依赖。

## 3. 数据模型

迁移 000006：
```sql
CREATE TABLE skill_stars (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id   UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, skill_id)
);
CREATE INDEX skill_stars_skill_idx ON skill_stars(skill_id);
```
幂等 star 用 `ON CONFLICT DO NOTHING`；unstar `DELETE` 不要求 RowsAffected。

## 4. 组件设计

### 4.1 internal/skill/repo.go（扩展）
- `Star(ctx, userID, skillID) error` —— `INSERT ... ON CONFLICT DO NOTHING`。
- `Unstar(ctx, userID, skillID) error` —— `DELETE`。
- `IsStarred(ctx, userID, skillID) (bool, error)`。
- `CountStars(ctx, skillID) (int64, error)`。
- `ListStarredSkills(ctx, userID, limit, offset) ([]SearchRow, error)` —— JOIN skills + teams，按 star 的 created_at DESC，返回 SearchRow（复用搜索行类型）。

### 4.2 internal/skill/service.go（扩展）
- `SkillDetail`：`{ Skill; Versions []SkillVersion; StarCount int64; IsStarred bool }`。
- `GetSkillDetail(ctx, teamID, name, viewerID) (*SkillDetail, error)` —— 取 skill + 版本（semver 降序）+ star_count + is_starred。取代 `GetSkillWithVersions`。
- `Star(ctx, userID, teamID, name) error` / `Unstar(...)` —— 先 GetSkill（404 若不存在），再 repo.Star/Unstar。落审计 `skill_starred` / `skill_unstarred`（可选；star 频繁，审计可省——**决定**：star/unstar 不落审计，避免噪音）。
- `ListMyStars(ctx, userID, page, pageSize) ([]SearchResult, error)` —— repo.ListStarredSkills + 附加 LatestVersion，复用 SearchResult。

### 4.3 internal/httpserver/handlers/skills.go（扩展）
- `Star` / `Unstar`：TeamScoped(member) 已加载 team；取 current_user；`skillSvc.Star/Unstar(userID, team.ID, name)`；204。
- `GetSkill` 改用 `GetSkillDetail`，响应加 `star_count`、`is_starred`。
- `ListMyStars`：authed；`skillSvc.ListMyStars(userID, page, pageSize)`；200 items。

### 4.4 路由
- `POST /teams/:slug/skills/:name/star` —— TeamScoped(member)。
- `DELETE /teams/:slug/skills/:name/star` —— TeamScoped(member)。
- `GET /me/stars` —— authed。

## 5. 数据流

star：`POST .../star → AuthRequired → TeamScoped(member) → skillSvc.Star → repo.Star (ON CONFLICT DO NOTHING) → 204`。
详情：`GET .../skills/:name → TeamScoped(member) → skillSvc.GetSkillDetail → repo (skill+versions+count+isStarred) → 200 {…,star_count,is_starred}`。
我的收藏：`GET /me/stars → AuthRequired → skillSvc.ListMyStars → repo.ListStarredSkills (JOIN) + LatestVersion → 200 items`。

## 6. 错误处理

- star/unstar 不存在的 skill → 404（service 先 GetSkill）。
- 非成员 star 团队 skill → 403（TeamScoped）。
- 分页参数非法 → validation_failed (422)。

## 7. 测试策略

- **单元测试**：service.Star/Unstar 幂等性（mock repo）；GetSkillDetail 字段；ListMyStars 分页。
- **集成测试**（build tag `integration`）：repo star 方法；e2e：成员 star → 详情 is_starred=true/star_count=1 → 再 star 仍 204 且 count=1（幂等）→ unstar → is_starred=false → 非成员 star 403 → GET /me/stars 返回已收藏 → global skill 任意认证用户可 star。

## 8. 交付物

- 上述 3 条新路由可用；skill 详情含 star_count/is_starred。
- star 幂等；可见性与拉取一致。

## 9. 后续衔接

- H i18n：错误消息本地化。
- 排行：按 star_count 排序的搜索可复用 E。
