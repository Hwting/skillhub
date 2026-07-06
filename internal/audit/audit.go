package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Action string

const (
	ActionLoginSuccess    Action = "login_success"
	ActionLoginFailure    Action = "login_failure"
	ActionLogout          Action = "logout"
	ActionRegister        Action = "register"
	ActionUserRoleChanged Action = "user_role_changed"
	ActionUserDisabled    Action = "user_disabled"
)

type Entry struct {
	ActorUserID *uuid.UUID
	Action      Action
	TargetType  string
	TargetID    string
	IP          string
	UserAgent   string
	Metadata    map[string]any
}

type auditRow struct {
	ID          int64      `gorm:"primaryKey;autoIncrement"`
	ActorUserID *uuid.UUID `gorm:"type:uuid"`
	Action      string     `gorm:"not null"`
	TargetType  string
	TargetID    string
	IP          string
	UserAgent   string
	Metadata    datatypes.JSON
	CreatedAt   time.Time
}

func (auditRow) TableName() string { return "audit_logs" }

type Logger struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewLogger(db *gorm.DB, logger *zap.Logger) *Logger {
	return &Logger{db: db, logger: logger}
}

func (l *Logger) Log(ctx context.Context, e Entry) error {
	var meta datatypes.JSON
	if e.Metadata != nil {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		meta = b
	}
	row := auditRow{
		ActorUserID: e.ActorUserID,
		Action:      string(e.Action),
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		IP:          e.IP,
		UserAgent:   e.UserAgent,
		Metadata:    meta,
	}
	// 异步写：失败不阻塞主流程，仅记日志
	// Decouple from the request context: once the handler returns the request
	// ctx is cancelled, which would abort an in-flight DB write. Use a
	// non-cancellable context so the audit row is persisted.
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("audit log panic", zap.Any("panic", r))
			}
		}()
		if err := l.db.WithContext(bgCtx).Create(&row).Error; err != nil {
			l.logger.Error("audit log write failed", zap.Error(err), zap.String("action", string(e.Action)))
		}
	}()
	return nil
}
