package auth

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

func newTestSM(t *testing.T) (*SessionManager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := rdb.NewClient(&rdb.Options{Addr: mr.Addr()})
	cfg := config.AuthConfig{
		SessionTTL:     time.Hour,
		CookieName:     "sid",
		CookieSecure:   false,
		CookieSameSite: "lax",
	}
	return NewSessionManager(client, cfg), mr
}

func TestSession_CreateGetDelete(t *testing.T) {
	sm, _ := newTestSM(t)
	ctx := context.Background()
	uid := uuid.New()
	sid, err := sm.Create(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if sid == "" {
		t.Fatal("empty session id")
	}
	got, err := sm.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if got != uid {
		t.Fatalf("got %v want %v", got, uid)
	}
	if err := sm.Delete(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Get(ctx, sid); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSession_Get_Miss(t *testing.T) {
	sm, _ := newTestSM(t)
	if _, err := sm.Get(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestSession_SetCookie(t *testing.T) {
	sm, _ := newTestSM(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	sm.SetCookie(c, "abc")
	if c.Writer.Header().Get("Set-Cookie") == "" {
		t.Fatal("no set-cookie")
	}
}
