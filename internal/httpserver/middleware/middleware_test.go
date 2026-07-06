package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRequestID_GeneratesAndSets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing request id header")
	}
}

func TestRequestID_PreservesIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc")
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") != "abc" {
		t.Fatalf("got %s", w.Header().Get("X-Request-Id"))
	}
}

func TestRecover_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID(), Recover(zap.NewNop()))
	r.GET("/", func(c *gin.Context) { panic("boom") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", w.Code)
	}
}
