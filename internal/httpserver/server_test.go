package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestHealthz_NilDeps_Returns503(t *testing.T) {
	// nil deps -> healthz must not panic; returns 503 degraded.
	r := New(Deps{Logger: zap.NewNop(), DB: nil, Redis: nil})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}
