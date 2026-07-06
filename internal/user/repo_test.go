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
		cfg, err := config.Load("../../config/config.yaml")
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
