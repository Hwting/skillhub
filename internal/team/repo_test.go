//go:build integration

package team

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"gorm.io/gorm"
)

var teamDB *gorm.DB

func setupTeamDB(t *testing.T) Repo {
	t.Helper()
	if teamDB == nil {
		cfg, err := config.Load("../../config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		teamDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	teamDB.Exec("TRUNCATE team_members, teams, users RESTART IDENTITY CASCADE")
	teamDB.Exec("INSERT INTO teams(slug,name,owner_user_id,publish_policy) VALUES('global','Global',NULL,'admin_only')")
	return NewRepo(teamDB)
}

// makeUser inserts a minimal user row and returns its id.
func makeUser(t *testing.T) uuid.UUID {
	t.Helper()
	var s string
	if err := teamDB.Raw("INSERT INTO users(email,username,password_hash,role,status) VALUES (?,?,?,?, 'active') RETURNING id::text",
		uuid.New().String()+"@x.com", uuid.New().String(), "x", "user").Scan(&s).Error; err != nil {
		t.Fatal(err)
	}
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRepo_CreateGet(t *testing.T) {
	r := setupTeamDB(t)
	ctx := context.Background()
	owner := makeUser(t)
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	if err := r.Create(ctx, tm); err != nil {
		t.Fatal(err)
	}
	if tm.ID == uuid.Nil {
		t.Fatal("id not set")
	}
	got, err := r.GetBySlug(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Acme" {
		t.Fatalf("name=%s", got.Name)
	}
}

func TestRepo_Members(t *testing.T) {
	r := setupTeamDB(t)
	ctx := context.Background()
	owner := makeUser(t)
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	if err := r.Create(ctx, tm); err != nil {
		t.Fatal(err)
	}
	member := makeUser(t)
	if err := r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember}); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetMember(ctx, tm.ID, member)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleMember {
		t.Fatalf("role=%s", got.Role)
	}
	if err := r.UpdateMemberRole(ctx, tm.ID, member, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	ms, err := r.ListMembers(ctx, tm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].Role != RoleAdmin {
		t.Fatalf("members=%v", ms)
	}
	if err := r.RemoveMember(ctx, tm.ID, member); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetMember(ctx, tm.ID, member); err == nil {
		t.Fatal("expected not found after remove")
	}
}

func TestRepo_TransferOwnership(t *testing.T) {
	r := setupTeamDB(t)
	ctx := context.Background()
	owner := makeUser(t)
	tm := &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}
	if err := r.Create(ctx, tm); err != nil {
		t.Fatal(err)
	}
	newOwner := makeUser(t)
	if err := r.TransferOwnership(ctx, tm.ID, newOwner); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, tm.ID)
	if *got.OwnerUserID != newOwner {
		t.Fatal("ownership not transferred")
	}
}

func TestRepo_ListForUser(t *testing.T) {
	r := setupTeamDB(t)
	ctx := context.Background()
	owner := makeUser(t)
	if err := r.Create(ctx, &Team{Slug: "acme", Name: "Acme", OwnerUserID: &owner, PublishPolicy: PolicyAdminOnly}); err != nil {
		t.Fatal(err)
	}
	other := makeUser(t)
	if err := r.Create(ctx, &Team{Slug: "beta", Name: "Beta", OwnerUserID: &other, PublishPolicy: PolicyAdminOnly}); err != nil {
		t.Fatal(err)
	}
	teams, err := r.ListForUser(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].Slug != "acme" {
		t.Fatalf("teams=%v", teams)
	}
}
