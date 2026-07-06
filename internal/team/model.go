package team

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleAdmin       = "admin"
	RoleMember      = "member"
	PolicyAdminOnly = "admin_only"
	PolicyAnyMember = "any_member"
	GlobalSlug      = "global"
)

type Team struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug          string     `gorm:"uniqueIndex;not null"`
	Name          string     `gorm:"not null"`
	OwnerUserID   *uuid.UUID `gorm:"type:uuid"`
	PublishPolicy string     `gorm:"not null;default:admin_only"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Team) TableName() string { return "teams" }

type TeamMember struct {
	TeamID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Role      string    `gorm:"not null"`
	CreatedAt time.Time
}

func (TeamMember) TableName() string { return "team_members" }
