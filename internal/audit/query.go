package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Record is the read model for an audit log row.
type Record struct {
	ID          int64          `json:"id"`
	ActorUserID *uuid.UUID     `json:"actor_user_id"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	IP          string         `json:"ip"`
	UserAgent   string         `json:"user_agent"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Filter narrows a List query. Empty string / nil fields are not filtered.
type Filter struct {
	Action     string
	ActorID    string
	TargetType string
	TargetID   string
	Limit      int
	Offset     int
}

// List returns audit records matching the filter, newest first.
func (l *Logger) List(ctx context.Context, f Filter) ([]Record, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	var rows []auditRow
	tx := l.db.WithContext(ctx).Model(&auditRow{})
	if f.Action != "" {
		tx = tx.Where("action = ?", f.Action)
	}
	if f.ActorID != "" {
		tx = tx.Where("actor_user_id = ?", f.ActorID)
	}
	if f.TargetType != "" {
		tx = tx.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		tx = tx.Where("target_id = ?", f.TargetID)
	}
	if err := tx.Order("created_at DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	out := make([]Record, len(rows))
	for i, r := range rows {
		rec := Record{ID: r.ID, ActorUserID: r.ActorUserID, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, IP: r.IP, UserAgent: r.UserAgent, CreatedAt: r.CreatedAt}
		if len(r.Metadata) > 0 {
			_ = json.Unmarshal(r.Metadata, &rec.Metadata)
		}
		out[i] = rec
	}
	return out, nil
}
