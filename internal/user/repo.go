package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"gorm.io/gorm"
)

type Repo interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, limit, offset int) ([]User, int64, error)
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
}

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) Repo { return &repo{db: db} }

func (r *repo) Create(ctx context.Context, u *User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *repo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "user", "user not found")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func (r *repo) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).First(&u, "email = ?", email).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.New("not_found", "user", "user not found")
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (r *repo) List(ctx context.Context, limit, offset int) ([]User, int64, error) {
	var users []User
	var total int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return users, total, nil
}

func (r *repo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("role", role)
	if res.Error != nil {
		return fmt.Errorf("update role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}

func (r *repo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("update status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}

func (r *repo) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("last_login_at", &now)
	if res.Error != nil {
		return fmt.Errorf("touch last login: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New("not_found", "user", "user not found")
	}
	return nil
}
