package apperr

import (
	"errors"
	"testing"
)

func TestError_Message(t *testing.T) {
	e := New("db_ping_failed", "db", "ping failed")
	if got := e.Error(); got != "db: ping failed" {
		t.Fatalf("got %q", got)
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("conn refused")
	e := Wrap("db_ping_failed", "db", "ping failed", cause)
	if !errors.Is(e, cause) {
		t.Fatal("Unwrap should return cause")
	}
}

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"not_found", 404},
		{"unauthorized", 401},
		{"forbidden", 403},
		{"validation_failed", 422},
		{"db_ping_failed", 500},
		{"unknown", 500},
	}
	for _, c := range cases {
		e := New(c.code, "x", "msg")
		if got := HTTPStatus(e); got != c.want {
			t.Fatalf("code %s: got %d want %d", c.code, got, c.want)
		}
	}
}

func TestHTTPStatus_NonApperr(t *testing.T) {
	if got := HTTPStatus(errors.New("plain")); got != 500 {
		t.Fatalf("got %d", got)
	}
}
