package team

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
)

type Service struct {
	repo  Repo
	audit *audit.Logger
}

func NewService(repo Repo, audit *audit.Logger) *Service {
	return &Service{repo: repo, audit: audit}
}

// Repo exposes the underlying repository for read-only lookups in middleware/handlers.
func (s *Service) Repo() Repo { return s.repo }

func (s *Service) Create(ctx context.Context, slug, name string, ownerID uuid.UUID) (*Team, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" || slug == GlobalSlug {
		return nil, apperr.New("validation_failed", "team", "invalid slug")
	}
	if name == "" {
		return nil, apperr.New("validation_failed", "team", "name required")
	}
	if existing, err := s.repo.GetBySlug(ctx, slug); err == nil && existing != nil {
		return nil, apperr.New("validation_failed", "team", "slug already taken")
	}
	t := &Team{Slug: slug, Name: name, OwnerUserID: &ownerID, PublishPolicy: PolicyAdminOnly}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &ownerID, Action: audit.Action("team_created"), TargetType: "team", TargetID: t.ID.String(), Metadata: map[string]any{"slug": slug}})
	return t, nil
}

func (s *Service) Update(ctx context.Context, actorID, teamID uuid.UUID, name *string, policy *string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if policy != nil {
		if *policy != PolicyAdminOnly && *policy != PolicyAnyMember {
			return apperr.New("validation_failed", "team", "invalid policy")
		}
		if err := s.repo.SetPublishPolicy(ctx, teamID, *policy); err != nil {
			return err
		}
		_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("publish_policy_changed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"new_policy": *policy}})
	}
	if name != nil {
		if *name == "" {
			return apperr.New("validation_failed", "team", "name required")
		}
		if err := s.repo.SetName(ctx, teamID, *name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AddMember(ctx context.Context, actorID, teamID, userID uuid.UUID, role string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsAdminOrOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "admin or owner only")
	}
	if role != RoleAdmin && role != RoleMember {
		return apperr.New("validation_failed", "team", "invalid role")
	}
	if err := s.repo.AddMember(ctx, TeamMember{TeamID: teamID, UserID: userID, Role: role}); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_added"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String(), "role": role}})
	return nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, actorID, teamID, userID uuid.UUID, role string) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if role != RoleAdmin && role != RoleMember {
		return apperr.New("validation_failed", "team", "invalid role")
	}
	if err := s.repo.UpdateMemberRole(ctx, teamID, userID, role); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_role_changed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String(), "new_role": role}})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, actorID, teamID, userID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsAdminOrOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "admin or owner only")
	}
	if err := s.repo.RemoveMember(ctx, teamID, userID); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("member_removed"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"user_id": userID.String()}})
	return nil
}

func (s *Service) TransferOwnership(ctx context.Context, actorID, teamID, newOwnerID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if _, err := s.repo.GetMember(ctx, teamID, newOwnerID); err != nil {
		return apperr.New("validation_failed", "team", "new owner must be a current member")
	}
	if err := s.repo.TransferOwnership(ctx, teamID, newOwnerID); err != nil {
		return err
	}
	// 旧 owner 降为 admin
	if actorID != newOwnerID {
		_ = s.repo.UpdateMemberRole(ctx, teamID, actorID, RoleAdmin)
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("ownership_transferred"), TargetType: "team", TargetID: teamID.String(), Metadata: map[string]any{"new_owner": newOwnerID.String()}})
	return nil
}

func (s *Service) Delete(ctx context.Context, actorID, teamID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if t.Slug == GlobalSlug {
		return apperr.New("validation_failed", "team", "cannot delete global namespace")
	}
	if !s.IsOwner(ctx, t, actorID) {
		return apperr.New("forbidden", "team", "owner only")
	}
	if err := s.repo.Delete(ctx, teamID); err != nil {
		return err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.Action("team_deleted"), TargetType: "team", TargetID: teamID.String()})
	return nil
}

// 权限判定

func (s *Service) IsOwner(ctx context.Context, t *Team, userID uuid.UUID) bool {
	return t.OwnerUserID != nil && *t.OwnerUserID == userID
}

func (s *Service) IsMember(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	_, err := s.repo.GetMember(ctx, t.ID, userID)
	return err == nil
}

func (s *Service) IsAdminOrOwner(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	m, err := s.repo.GetMember(ctx, t.ID, userID)
	if err != nil {
		return false
	}
	return m.Role == RoleAdmin
}

func (s *Service) CanPublish(ctx context.Context, t *Team, userID uuid.UUID) bool {
	if s.IsOwner(ctx, t, userID) {
		return true
	}
	if t.PublishPolicy == PolicyAdminOnly {
		return s.IsAdminOrOwner(ctx, t, userID)
	}
	return s.IsMember(ctx, t, userID)
}
