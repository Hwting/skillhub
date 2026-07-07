package skill

import (
	"time"

	"github.com/google/uuid"
)

const (
	ContentTypeTarball = "application/gzip"
	MaxNameLen         = 63
	MaxPackageSize     = 50 * 1024 * 1024 // 50 MiB
)

type Skill struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TeamID    uuid.UUID `gorm:"type:uuid;not null"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Skill) TableName() string { return "skills" }

type SkillVersion struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SkillID         uuid.UUID `gorm:"type:uuid;not null"`
	Version         string    `gorm:"not null"`
	StorageKey      string    `gorm:"not null"`
	Size            int64     `gorm:"not null"`
	Sha256          string    `gorm:"not null"`
	ContentType     string    `gorm:"not null"`
	PublisherUserID uuid.UUID `gorm:"type:uuid;not null"`
	Readme          string
	CreatedAt       time.Time
}

func (SkillVersion) TableName() string { return "skill_versions" }
