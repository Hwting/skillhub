package user

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleUser          = "user"
	RolePlatformAdmin = "platform_admin"
	StatusActive      = "active"
	StatusDisabled    = "disabled"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string    `gorm:"uniqueIndex;not null"`
	Username     string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         string    `gorm:"not null;default:user"`
	Status       string    `gorm:"not null;default:active"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

func (User) TableName() string { return "users" }
