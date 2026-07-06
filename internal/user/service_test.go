package user

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"go.uber.org/zap"
)

type mockRepo struct {
	users map[string]*User
}

func (m *mockRepo) Create(ctx context.Context, u *User) error {
	u.ID = uuid.New()
	m.users[u.Email] = u
	return nil
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, apperr.New("not_found", "user", "not found")
}
func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, apperr.New("not_found", "user", "not found")
}
func (m *mockRepo) List(ctx context.Context, limit, offset int) ([]User, int64, error) {
	return nil, 0, nil
}
func (m *mockRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error    { return nil }
func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error { return nil }
func (m *mockRepo) TouchLastLogin(ctx context.Context, id uuid.UUID) error             { return nil }

func newSvc() *Service {
	return NewService(&mockRepo{users: map[string]*User{}}, audit.NewLogger(nil, zap.NewNop()))
}

func TestRegister_Success(t *testing.T) {
	s := newSvc()
	u, err := s.Register(context.Background(), "A@B.com", "alice", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "a@b.com" {
		t.Fatalf("email not lowercased: %s", u.Email)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	s := newSvc()
	s.Register(context.Background(), "a@b.com", "alice", "password1")
	if _, err := s.Register(context.Background(), "a@b.com", "bob", "password1"); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	s := newSvc()
	if _, err := s.Register(context.Background(), "a@b.com", "alice", "short"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	s := newSvc()
	if _, err := s.Login(context.Background(), "x@y.com", "password1", "1.1.1.1", "ua"); err == nil {
		t.Fatal("expected unauthorized")
	}
}
