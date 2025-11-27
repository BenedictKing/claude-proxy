package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/gin-gonic/gin"
)

// setupRouterWithAuth builds a minimal router with the auth middleware wired.
func setupRouterWithAuth(envCfg *config.EnvConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(WebAuthMiddleware(envCfg, nil))

	// Protected management API
	r.GET("/api/channels", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// SPA routes should pass through without access key
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "home")
	})
	r.GET("/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "dashboard")
	})

	return r
}

func TestWebAuthMiddleware_APIRequiresKey(t *testing.T) {
	envCfg := &config.EnvConfig{
		ProxyAccessKey:  "secret-key",
		EnableWebUI:     true,
		HealthCheckPath: "/health",
	}
	router := setupRouterWithAuth(envCfg)

	t.Run("missing key returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("wrong key returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
		req.Header.Set("x-api-key", "wrong")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("correct key allows access", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
		req.Header.Set("x-api-key", envCfg.ProxyAccessKey)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestWebAuthMiddleware_SPAPassesThrough(t *testing.T) {
	envCfg := &config.EnvConfig{
		ProxyAccessKey:  "secret-key",
		EnableWebUI:     true,
		HealthCheckPath: "/health",
	}
	router := setupRouterWithAuth(envCfg)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
