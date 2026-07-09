//go:build integration

package audit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"go.uber.org/zap"
)

func TestLogger_List_Filter(t *testing.T) {
	cfg, err := config.Load("../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	gdb.Exec("TRUNCATE audit_logs RESTART IDENTITY")
	l := NewLogger(gdb, zap.NewNop())
	ctx := context.Background()
	aid := uuid.New()
	// 直接插库保证确定性（Log 是异步的）
	gdb.Exec("INSERT INTO audit_logs(actor_user_id,action,target_type,target_id) VALUES(?, 'team_created','team','x')", aid)
	gdb.Exec("INSERT INTO audit_logs(actor_user_id,action,target_type,target_id) VALUES(?, 'skill_promoted_to_global','skill_version','y')", aid)

	recs, err := l.List(ctx, Filter{Action: "team_created", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Action != "team_created" {
		t.Fatalf("recs=%+v", recs)
	}

	all, err := l.List(ctx, Filter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all=%d", len(all))
	}

	// 按 actor 过滤
	byActor, _ := l.List(ctx, Filter{ActorID: aid.String(), Limit: 100})
	if len(byActor) != 2 {
		t.Fatalf("byActor=%d", len(byActor))
	}
	other := uuid.New()
	byOther, _ := l.List(ctx, Filter{ActorID: other.String(), Limit: 100})
	if len(byOther) != 0 {
		t.Fatalf("byOther=%d", len(byOther))
	}
}
