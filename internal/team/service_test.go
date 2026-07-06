package team

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"go.uber.org/zap"
)

type mockRepo struct {
	teams   map[uuid.UUID]*Team
	members map[[2]uuid.UUID]*TeamMember // (teamID, userID) -> member
}

func newMockRepo() *mockRepo {
	return &mockRepo{teams: map[uuid.UUID]*Team{}, members: map[[2]uuid.UUID]*TeamMember{}}
}

func (m *mockRepo) Create(ctx context.Context, t *Team) error {
	t.ID = uuid.New()
	m.teams[t.ID] = t
	return nil
}
func (m *mockRepo) GetBySlug(ctx context.Context, slug string) (*Team, error) {
	for _, t := range m.teams {
		if t.Slug == slug {
			return t, nil
		}
	}
	return nil, apperr.New("not_found", "team", "not found")
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	if t, ok := m.teams[id]; ok {
		return t, nil
	}
	return nil, apperr.New("not_found", "team", "not found")
}
func (m *mockRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error) { return nil, nil }
func (m *mockRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	var ms []TeamMember
	for _, mm := range m.members {
		if mm.TeamID == teamID {
			ms = append(ms, *mm)
		}
	}
	return ms, nil
}
func (m *mockRepo) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error) {
	if mm, ok := m.members[[2]uuid.UUID{teamID, userID}]; ok {
		return mm, nil
	}
	return nil, apperr.New("not_found", "team", "member not found")
}
func (m *mockRepo) AddMember(ctx context.Context, mm TeamMember) error {
	m.members[[2]uuid.UUID{mm.TeamID, mm.UserID}] = &mm
	return nil
}
func (m *mockRepo) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	mm, ok := m.members[[2]uuid.UUID{teamID, userID}]
	if !ok {
		return apperr.New("not_found", "team", "member not found")
	}
	mm.Role = role
	return nil
}
func (m *mockRepo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	delete(m.members, [2]uuid.UUID{teamID, userID})
	return nil
}
func (m *mockRepo) TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error {
	m.teams[teamID].OwnerUserID = &newOwnerID
	return nil
}
func (m *mockRepo) SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error {
	m.teams[teamID].PublishPolicy = policy
	return nil
}
func (m *mockRepo) SetName(ctx context.Context, teamID uuid.UUID, name string) error {
	m.teams[teamID].Name = name
	return nil
}
func (m *mockRepo) Delete(ctx context.Context, teamID uuid.UUID) error {
	delete(m.teams, teamID)
	return nil
}

func newSvc() (*Service, *mockRepo) {
	r := newMockRepo()
	return NewService(r, audit.NewLogger(nil, zap.NewNop())), r
}

func TestCreate_InvalidSlug(t *testing.T) {
	s, _ := newSvc()
	if _, err := s.Create(context.Background(), "global", "x", uuid.New()); err == nil {
		t.Fatal("expected error for global slug")
	}
	if _, err := s.Create(context.Background(), "", "x", uuid.New()); err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestPermissions(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	admin := uuid.New()
	_ = r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: admin, Role: RoleAdmin})
	member := uuid.New()
	_ = r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember})
	other := uuid.New()

	if !s.IsOwner(ctx, tm, owner) {
		t.Fatal("owner should be owner")
	}
	if s.IsOwner(ctx, tm, admin) {
		t.Fatal("admin is not owner")
	}
	if !s.IsAdminOrOwner(ctx, tm, admin) {
		t.Fatal("admin should be admin+")
	}
	if !s.IsMember(ctx, tm, member) {
		t.Fatal("member should be member")
	}
	if s.IsMember(ctx, tm, other) {
		t.Fatal("other is not member")
	}
}

func TestCanPublish(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	member := uuid.New()
	_ = r.AddMember(ctx, TeamMember{TeamID: tm.ID, UserID: member, Role: RoleMember})

	// admin_only: member 不能发布
	if s.CanPublish(ctx, tm, member) {
		t.Fatal("member cannot publish under admin_only")
	}
	// any_member: member 可发布
	_ = r.SetPublishPolicy(ctx, tm.ID, PolicyAnyMember)
	tm.PublishPolicy = PolicyAnyMember
	if !s.CanPublish(ctx, tm, member) {
		t.Fatal("member can publish under any_member")
	}
}

func TestTransferOwnership_NonMember(t *testing.T) {
	s, _ := newSvc()
	ctx := context.Background()
	owner := uuid.New()
	tm, _ := s.Create(ctx, "acme", "Acme", owner)
	if err := s.TransferOwnership(ctx, owner, tm.ID, uuid.New()); err == nil {
		t.Fatal("expected error transferring to non-member")
	}
}

func TestDelete_Global(t *testing.T) {
	s, r := newSvc()
	ctx := context.Background()
	// 手动塞一个 global
	g := &Team{Slug: GlobalSlug, Name: "Global", PublishPolicy: PolicyAdminOnly}
	g.ID = uuid.New()
	r.teams[g.ID] = g
	if err := s.Delete(ctx, uuid.New(), g.ID); err == nil {
		t.Fatal("expected error deleting global")
	}
}
