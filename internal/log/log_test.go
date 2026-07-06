package log

import (
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestNew_Valid(t *testing.T) {
	l, err := New(config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("ok")
}

func TestNew_InvalidLevel(t *testing.T) {
	if _, err := New(config.LogConfig{Level: "nope"}); err == nil {
		t.Fatal("expected error")
	}
}
