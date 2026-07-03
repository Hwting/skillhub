package db

import (
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestNew_InvalidHost(t *testing.T) {
	// 连不上的 host 应返回错误而非 panic
	_, err := New(config.DBConfig{
		Host: "127.0.0.1", Port: 1, Name: "x", User: "x", SSLMode: "disable",
		MaxOpen: 1, MaxIdle: 1,
	})
	if err == nil {
		t.Fatal("expected error for unreachable db")
	}
}
