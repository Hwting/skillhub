package skill

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// IsValidName reports whether name is a legal skill name: lowercase alnum
// and hyphens, 1–63 chars, leading alnum.
func IsValidName(name string) bool { return nameRe.MatchString(name) }

type Repo interface {
	CreateSkill(ctx context.Context, s *Skill) error
	GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error)
	ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error)
	CreateVersion(ctx context.Context, v *SkillVersion) error
	GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error)
	ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error)
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

// isUniqueViolation detects a PG unique-constraint violation (SQLSTATE 23505)
// or gorm's typed duplicate-key error.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return err == gorm.ErrDuplicatedKey || strings.Contains(err.Error(), "23505")
}

func (r *repo) CreateSkill(ctx context.Context, s *Skill) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.New("conflict", "skill", "skill already exists")
		}
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

func (r *repo) GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error) {
	var s Skill
	if err := r.db.WithContext(ctx).First(&s, "team_id = ? AND name = ?", teamID, name).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "skill", "skill not found")
		}
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return &s, nil
}

func (r *repo) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	var ss []Skill
	if err := r.db.WithContext(ctx).Where("team_id = ?", teamID).Order("name ASC").Find(&ss).Error; err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return ss, nil
}

func (r *repo) CreateVersion(ctx context.Context, v *SkillVersion) error {
	if err := r.db.WithContext(ctx).Create(v).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.New("conflict", "skill", "version already exists")
		}
		return fmt.Errorf("create version: %w", err)
	}
	return nil
}

func (r *repo) GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error) {
	var v SkillVersion
	if err := r.db.WithContext(ctx).First(&v, "skill_id = ? AND version = ?", skillID, version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "skill", "version not found")
		}
		return nil, fmt.Errorf("get version: %w", err)
	}
	return &v, nil
}

func (r *repo) ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error) {
	var vs []SkillVersion
	if err := r.db.WithContext(ctx).Where("skill_id = ?", skillID).Order("created_at DESC").Find(&vs).Error; err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return vs, nil
}
