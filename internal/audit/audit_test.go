//go:build integration

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var auditDB *gorm.DB

func setupAuditDB(t *testing.T) *Logger {
	t.Helper()
	if auditDB == nil {
		cfg, err := config.Load("../../config/config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		auditDB, err = db.New(cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
	}
	auditDB.Exec("TRUNCATE audit_logs RESTART IDENTITY")
	return NewLogger(auditDB, zap.NewNop())
}

func TestLogger_Log(t *testing.T) {
	l := setupAuditDB(t)
	uid := uuid.New()
	err := l.Log(context.Background(), Entry{
		ActorUserID: &uid,
		Action:      ActionRegister,
		TargetType:  "user",
		TargetID:    uid.String(),
		IP:          "1.2.3.4",
		Metadata:    map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 异步写，等一下
	time.Sleep(200 * time.Millisecond)
	var n int64
	auditDB.Table("audit_logs").Where("action = ?", string(ActionRegister)).Count(&n)
	if n != 1 {
		t.Fatalf("expected 1 audit row, got %d", n)
	}
}
