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

func setupSkillDB(t *testing.T) (Repo, uuid.UUID, uuid.UUID) {
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
	var ownerID, teamID string
	skillDB.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES('o@x.com','o','x','user','active') RETURNING id::text").Scan(&ownerID)
	skillDB.Raw("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('acme','Acme',?,'admin_only') RETURNING id::text", ownerID).Scan(&teamID)
	oid, _ := uuid.Parse(ownerID)
	tid, _ := uuid.Parse(teamID)
	return NewRepo(skillDB), tid, oid
}

// newStarUser inserts a fresh user and returns its id (skill_stars FKs users).
func newStarUser(t *testing.T) uuid.UUID {
	t.Helper()
	var id string
	if err := skillDB.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES(?,?,'x','user','active') RETURNING id::text", uuid.New().String()+"@x.com", uuid.New().String()).Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	u, _ := uuid.Parse(id)
	return u
}

func TestRepo_SkillCRUD(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
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

func TestRepo_SkillDuplicate_Conflict(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
	ctx := context.Background()
	if err := r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "dup"}); err != nil {
		t.Fatal(err)
	}
	err := r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "dup"})
	if err == nil {
		t.Fatal("expected conflict")
	}
}

func TestRepo_VersionConflict(t *testing.T) {
	r, tid, oid := setupSkillDB(t)
	ctx := context.Background()
	s := &Skill{TeamID: tid, Name: "my-skill"}
	if err := r.CreateSkill(ctx, s); err != nil {
		t.Fatal(err)
	}
	v := &SkillVersion{SkillID: s.ID, Version: "1.0.0", StorageKey: "k", Size: 1, Sha256: "x", ContentType: ContentTypeTarball, PublisherUserID: oid}
	if err := r.CreateVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	v2 := &SkillVersion{SkillID: s.ID, Version: "1.0.0", StorageKey: "k2", Size: 1, Sha256: "x", ContentType: ContentTypeTarball, PublisherUserID: oid}
	if err := r.CreateVersion(ctx, v2); err == nil {
		t.Fatal("expected conflict")
	}
	// 不同 version 应成功
	v3 := &SkillVersion{SkillID: s.ID, Version: "1.0.1", StorageKey: "k3", Size: 1, Sha256: "x", ContentType: ContentTypeTarball, PublisherUserID: oid}
	if err := r.CreateVersion(ctx, v3); err != nil {
		t.Fatalf("expected ok for new version: %v", err)
	}
}

func TestRepo_Search(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
	ctx := context.Background()
	if err := r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "go-lint"}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateSkill(ctx, &Skill{TeamID: tid, Name: "go-format"}); err != nil {
		t.Fatal(err)
	}

	// 命中 lint
	rows, err := r.Search(ctx, []uuid.UUID{tid}, "lint", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "go-lint" {
		t.Fatalf("search lint: %+v", rows)
	}
	if rows[0].TeamSlug != "acme" {
		t.Fatalf("team_slug=%s", rows[0].TeamSlug)
	}

	// 空查询返回全部
	rows, _ = r.Search(ctx, []uuid.UUID{tid}, "", 100, 0)
	if len(rows) != 2 {
		t.Fatalf("empty q: %d", len(rows))
	}

	// 不可见团队：随机 id 不命中
	hidden := uuid.New()
	rows, _ = r.Search(ctx, []uuid.UUID{hidden}, "lint", 100, 0)
	if len(rows) != 0 {
		t.Fatalf("hidden team leaked: %d", len(rows))
	}

	// 分页
	rows, _ = r.Search(ctx, []uuid.UUID{tid}, "", 1, 0)
	if len(rows) != 1 {
		t.Fatalf("limit: %d", len(rows))
	}
	rows, _ = r.Search(ctx, []uuid.UUID{tid}, "", 1, 1)
	if len(rows) != 1 {
		t.Fatalf("offset: %d", len(rows))
	}
}

func TestRepo_StarIdempotent(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
	ctx := context.Background()
	s := &Skill{TeamID: tid, Name: "starred"}
	if err := r.CreateSkill(ctx, s); err != nil {
		t.Fatal(err)
	}
	starUser := newStarUser(t)
	if err := r.Star(ctx, starUser, s.ID); err != nil {
		t.Fatal(err)
	}
	// 幂等：再 star 不报错，count 仍为 1
	if err := r.Star(ctx, starUser, s.ID); err != nil {
		t.Fatal(err)
	}
	n, err := r.CountStars(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count=%d", n)
	}
	ok, err := r.IsStarred(ctx, starUser, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("should be starred")
	}
	// 另一用户 star → count=2
	other := newStarUser(t)
	if err := r.Star(ctx, other, s.ID); err != nil {
		t.Fatal(err)
	}
	n, _ = r.CountStars(ctx, s.ID)
	if n != 2 {
		t.Fatalf("count=%d", n)
	}
}

func TestRepo_Unstar(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
	ctx := context.Background()
	s := &Skill{TeamID: tid, Name: "starred"}
	r.CreateSkill(ctx, s)
	starUser := newStarUser(t)
	r.Star(ctx, starUser, s.ID)
	if err := r.Unstar(ctx, starUser, s.ID); err != nil {
		t.Fatal(err)
	}
	ok, _ := r.IsStarred(ctx, starUser, s.ID)
	if ok {
		t.Fatal("should not be starred after unstar")
	}
	n, _ := r.CountStars(ctx, s.ID)
	if n != 0 {
		t.Fatalf("count=%d", n)
	}
	// unstar 不存在的行不报错
	if err := r.Unstar(ctx, starUser, s.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_ListStarredSkills(t *testing.T) {
	r, tid, _ := setupSkillDB(t)
	ctx := context.Background()
	s1 := &Skill{TeamID: tid, Name: "alpha"}
	s2 := &Skill{TeamID: tid, Name: "beta"}
	r.CreateSkill(ctx, s1)
	r.CreateSkill(ctx, s2)
	starUser := newStarUser(t)
	r.Star(ctx, starUser, s1.ID)
	r.Star(ctx, starUser, s2.ID)

	rows, err := r.ListStarredSkills(ctx, starUser, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	names := map[string]bool{rows[0].Name: true, rows[1].Name: true}
	if !names["alpha"] || !names["beta"] {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].TeamSlug != "acme" {
		t.Fatalf("team_slug=%s", rows[0].TeamSlug)
	}

	// 未 star 的用户返回空
	other := newStarUser(t)
	rows, _ = r.ListStarredSkills(ctx, other, 100, 0)
	if len(rows) != 0 {
		t.Fatalf("expected empty, got %d", len(rows))
	}

	// 分页夹取
	rows, _ = r.ListStarredSkills(ctx, starUser, 1, 0)
	if len(rows) != 1 {
		t.Fatalf("limit: %d", len(rows))
	}
}
