package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	pw "github.com/skillhub/skillhub/internal/password"
)

type Service struct {
	repo  Repo
	audit *audit.Logger
}

func NewService(repo Repo, audit *audit.Logger) *Service {
	return &Service{repo: repo, audit: audit}
}

// dummyHash is a pre-computed argon2id hash used to equalize login timing on
// the not_found branch so that an attacker cannot distinguish "user not found"
// from "bad password" by response latency.
var dummyHash = func() string {
	h, err := pw.Hash("dummy-password-timing-equalizer")
	if err != nil {
		// Fallback to a syntactically valid argon2id encoded string; Verify
		// against it will simply return false, which is all we need.
		return "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	return h
}()

func validEmail(e string) bool {
	return strings.Contains(e, "@") && len(e) >= 3
}

func (s *Service) Register(ctx context.Context, email, username, password string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !validEmail(email) {
		return nil, apperr.New("validation_failed", "user", "invalid email")
	}
	if username == "" {
		return nil, apperr.New("validation_failed", "user", "username required")
	}
	if len(password) < 8 {
		return nil, apperr.New("validation_failed", "user", "password must be >= 8 chars")
	}
	if _, err := s.repo.GetByEmail(ctx, email); err == nil {
		return nil, apperr.New("validation_failed", "user", "email already registered")
	}
	hash, err := pw.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &User{Email: email, Username: username, PasswordHash: hash, Role: RoleUser, Status: StatusActive}
	if err := s.repo.Create(ctx, u); err != nil {
		// Create failed: detect a username unique-constraint conflict (email is
		// pre-checked above) and surface it as validation_failed per spec §7.
		if _,ByUsernameErr := s.repo.GetByUsername(ctx, username); ByUsernameErr == nil {
			return nil, apperr.New("validation_failed", "user", "username already taken")
		}
		return nil, err
	}
	s.audit.Log(ctx, audit.Entry{Action: audit.ActionRegister, TargetType: "user", TargetID: u.ID.String(), Metadata: map[string]any{"email": email}})
	return u, nil
}

func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		// Equalize timing with the bad_password branch by running a dummy argon2
		// verification, so all three failure paths take ~equal time and an
		// attacker cannot enumerate valid emails by latency.
		_, _ = pw.Verify(password, dummyHash)
		s.audit.Log(ctx, audit.Entry{Action: audit.ActionLoginFailure, TargetType: "user", IP: ip, UserAgent: ua, Metadata: map[string]any{"email": email, "reason": "not_found"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	ok, err := pw.Verify(password, u.PasswordHash)
	if err != nil || !ok {
		s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginFailure, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"reason": "bad_password"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	if u.Status != StatusActive {
		s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginFailure, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"reason": "disabled"}})
		return nil, apperr.New("unauthorized", "auth", "invalid credentials")
	}
	s.repo.TouchLastLogin(ctx, u.ID)
	s.audit.Log(ctx, audit.Entry{ActorUserID: &u.ID, Action: audit.ActionLoginSuccess, TargetType: "user", TargetID: u.ID.String(), IP: ip, UserAgent: ua})
	return u, nil
}

func (s *Service) UpdateRole(ctx context.Context, actorID, targetID uuid.UUID, role, ip, ua string) error {
	if role != RoleUser && role != RolePlatformAdmin {
		return apperr.New("validation_failed", "user", "invalid role")
	}
	if err := s.repo.UpdateRole(ctx, targetID, role); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.ActionUserRoleChanged, TargetType: "user", TargetID: targetID.String(), IP: ip, UserAgent: ua, Metadata: map[string]any{"new_role": role}})
	return nil
}

func (s *Service) Disable(ctx context.Context, actorID, targetID uuid.UUID, ip, ua string) error {
	if err := s.repo.UpdateStatus(ctx, targetID, StatusDisabled); err != nil {
		return err
	}
	s.audit.Log(ctx, audit.Entry{ActorUserID: &actorID, Action: audit.ActionUserDisabled, TargetType: "user", TargetID: targetID.String(), IP: ip, UserAgent: ua})
	return nil
}

func (s *Service) ListForAdmin(ctx context.Context, limit, offset int) ([]User, int64, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) GetForAdmin(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.GetByID(ctx, id)
}
