package team

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

type Repo interface {
	Create(ctx context.Context, t *Team) error
	GetBySlug(ctx context.Context, slug string) (*Team, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Team, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error)
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error)
	GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error)
	AddMember(ctx context.Context, m TeamMember) error
	UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error
	SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error
	SetName(ctx context.Context, teamID uuid.UUID, name string) error
	Delete(ctx context.Context, teamID uuid.UUID) error
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

func (r *repo) Create(ctx context.Context, t *Team) error {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	return nil
}

func (r *repo) GetBySlug(ctx context.Context, slug string) (*Team, error) {
	var t Team
	if err := r.db.WithContext(ctx).First(&t, "slug = ?", slug).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "team not found")
		}
		return nil, fmt.Errorf("get team by slug: %w", err)
	}
	return &t, nil
}

func (r *repo) GetByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	var t Team
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "team not found")
		}
		return nil, fmt.Errorf("get team by id: %w", err)
	}
	return &t, nil
}

func (r *repo) ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error) {
	var teams []Team
	err := r.db.WithContext(ctx).
		Where("owner_user_id = ? OR id IN (SELECT team_id FROM team_members WHERE user_id = ?)", userID, userID).
		Order("created_at DESC").Find(&teams).Error
	if err != nil {
		return nil, fmt.Errorf("list teams for user: %w", err)
	}
	return teams, nil
}

func (r *repo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	var ms []TeamMember
	if err := r.db.WithContext(ctx).Where("team_id = ?", teamID).Order("created_at ASC").Find(&ms).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return ms, nil
}

func (r *repo) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error) {
	var m TeamMember
	if err := r.db.WithContext(ctx).First(&m, "team_id = ? AND user_id = ?", teamID, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "team", "member not found")
		}
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

func (r *repo) AddMember(ctx context.Context, m TeamMember) error {
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (r *repo) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	res := r.db.WithContext(ctx).Model(&TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Update("role", role)
	if res.Error != nil {
		return fmt.Errorf("update member role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "member not found")
	}
	return nil
}

func (r *repo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&TeamMember{})
	if res.Error != nil {
		return fmt.Errorf("remove member: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "member not found")
	}
	return nil
}

func (r *repo) TransferOwnership(ctx context.Context, teamID, newOwnerID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("owner_user_id", newOwnerID)
	if res.Error != nil {
		return fmt.Errorf("transfer ownership: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) SetPublishPolicy(ctx context.Context, teamID uuid.UUID, policy string) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("publish_policy", policy)
	if res.Error != nil {
		return fmt.Errorf("set policy: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) SetName(ctx context.Context, teamID uuid.UUID, name string) error {
	res := r.db.WithContext(ctx).Model(&Team{}).Where("id = ?", teamID).Update("name", name)
	if res.Error != nil {
		return fmt.Errorf("set name: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, teamID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", teamID).Delete(&Team{})
	if res.Error != nil {
		return fmt.Errorf("delete team: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "team", "team not found")
	}
	return nil
}
