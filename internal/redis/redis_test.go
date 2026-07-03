package redis

import (
	"context"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestPing_Unreachable(t *testing.T) {
	c, _ := New(config.RedisConfig{Addr: "127.0.0.1:1"})
	if err := Ping(context.Background(), c); err == nil {
		t.Fatal("expected error")
	}
}
