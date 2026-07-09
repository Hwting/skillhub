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
	Search(ctx context.Context, teamIDs []uuid.UUID, q string, limit, offset int) ([]SearchRow, error)
}

// SearchRow is a skill plus its team slug, the result of a search query.
type SearchRow struct {
	Skill
	TeamSlug string
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

// Search returns skills in the given teams whose name matches q (full-text via
// the search_vector generated column), JOINed to teams to carry the team slug.
// An empty q skips FTS and orders by updated_at DESC. limit is clamped to [1,100].
// An empty teamIDs returns nothing (no visible teams).
func (r *repo) Search(ctx context.Context, teamIDs []uuid.UUID, q string, limit, offset int) ([]SearchRow, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.TrimSpace(q)

	type searchRow struct {
		Skill
		TeamSlug string `gorm:"column:team_slug"`
	}
	var rows []searchRow
	tx := r.db.WithContext(ctx).Table("skills").
		Select("skills.*, teams.slug AS team_slug").
		Joins("JOIN teams ON teams.id = skills.team_id").
		Where("skills.team_id IN ?", teamIDs)
	if q != "" {
		tx = tx.
			Where("skills.search_vector @@ plainto_tsquery('simple', ?)", q).
			Order(gorm.Expr("ts_rank(skills.search_vector, plainto_tsquery('simple', ?)) DESC", q)).
			Order("skills.name ASC")
	} else {
		tx = tx.Order("skills.updated_at DESC").Order("skills.name ASC")
	}
	if err := tx.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("search skills: %w", err)
	}
	out := make([]SearchRow, len(rows))
	for i, rr := range rows {
		out[i] = SearchRow{Skill: rr.Skill, TeamSlug: rr.TeamSlug}
	}
	return out, nil
}
