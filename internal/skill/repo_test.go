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
