package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

type SessionManager struct {
	client    *rdb.Client
	ttl       time.Duration
	cookieCfg config.AuthConfig
}

func NewSessionManager(client *rdb.Client, cfg config.AuthConfig) *SessionManager {
	return &SessionManager{client: client, ttl: cfg.SessionTTL, cookieCfg: cfg}
}

func (sm *SessionManager) Create(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gen session id: %w", err)
	}
	sid := hex.EncodeToString(b)
	key := "session:" + sid
	if err := sm.client.Set(ctx, key, userID.String(), sm.ttl).Err(); err != nil {
		return "", fmt.Errorf("set session: %w", err)
	}
	return sid, nil
}

func (sm *SessionManager) Get(ctx context.Context, sessionID string) (uuid.UUID, error) {
	key := "session:" + sessionID
	val, err := sm.client.Get(ctx, key).Result()
	if err != nil {
		if err == rdb.Nil {
			return uuid.Nil, apperr.New("unauthorized", "auth", "session not found")
		}
		return uuid.Nil, fmt.Errorf("get session: %w", err)
	}
	uid, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, apperr.New("unauthorized", "auth", "invalid session")
	}
	return uid, nil
}

func (sm *SessionManager) Delete(ctx context.Context, sessionID string) error {
	key := "session:" + sessionID
	if err := sm.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (sm *SessionManager) sameSite() http.SameSite {
	switch sm.cookieCfg.CookieSameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (sm *SessionManager) SetCookie(c *gin.Context, sessionID string) {
	c.SetSameSite(sm.sameSite())
	c.SetCookie(sm.cookieCfg.CookieName, sessionID, int(sm.ttl.Seconds()), "/", sm.cookieCfg.CookieDomain, sm.cookieCfg.CookieSecure, true)
}

func (sm *SessionManager) ClearCookie(c *gin.Context) {
	c.SetSameSite(sm.sameSite())
	c.SetCookie(sm.cookieCfg.CookieName, "", -1, "/", sm.cookieCfg.CookieDomain, sm.cookieCfg.CookieSecure, true)
}

func (sm *SessionManager) CookieName() string { return sm.cookieCfg.CookieName }
