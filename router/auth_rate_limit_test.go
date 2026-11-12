package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuthRateLimitRouter(t *testing.T, redisEnabled bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())

	previousRedisEnabled, previousRedis := common.RedisEnabled, common.RDB
	previousGlobalLimit := common.GlobalApiRateLimitEnable
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousCriticalNum, previousCriticalDuration := common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	previousSecure, previousOrigins := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	previousPasswordLogin, previousTurnstile := common.PasswordLoginEnabled, common.TurnstileCheckEnabled
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRedis
		common.GlobalApiRateLimitEnable = previousGlobalLimit
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.CriticalRateLimitNum, common.CriticalRateLimitDuration = previousCriticalNum, previousCriticalDuration
		common.SessionCookieSecure, common.SessionCookieTrustedURLs = previousSecure, previousOrigins
		common.PasswordLoginEnabled, common.TurnstileCheckEnabled = previousPasswordLogin, previousTurnstile
	})
	common.RedisEnabled = redisEnabled
	if redisEnabled {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		require.NoError(t, client.Ping(context.Background()).Err())
		t.Cleanup(func() { _ = client.Close() })
		common.RDB = client
	}
	common.GlobalApiRateLimitEnable = false
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum, common.CriticalRateLimitDuration = 2, 1200
	common.SessionCookieSecure, common.SessionCookieTrustedURLs = true, nil
	common.PasswordLoginEnabled, common.TurnstileCheckEnabled = true, false

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetApiRouter(engine)
	engine.GET("/sensitive", middleware.CriticalRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return engine
}

func authRateLimitRequest(engine http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "https://dashboard.example.com"+path, strings.NewReader("{}"))
	request.RemoteAddr = "192.0.2.201:12345"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthRateLimitRoutesKeepLoginAndSessionBudgetsIndependent(t *testing.T) {
	for _, backend := range []struct {
		name  string
		redis bool
	}{
		{name: "redis", redis: true},
		{name: "memory", redis: false},
	} {
		t.Run(backend.name, func(t *testing.T) {
			engine := setupAuthRateLimitRouter(t, backend.redis)
			const origin = "https://dashboard.example.com"
			for _, path := range []string{"/api/user/auth/refresh", "/api/user/auth/logout"} {
				response := authRateLimitRequest(engine, http.MethodPost, path, "https://untrusted.example.com")
				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.Contains(t, response.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
			}

			for range 2 {
				assert.Equal(t, http.StatusNoContent, authRateLimitRequest(engine, http.MethodGet, "/sensitive", origin).Code)
			}
			assert.Equal(t, http.StatusTooManyRequests, authRateLimitRequest(engine, http.MethodGet, "/sensitive", origin).Code)

			// Page reloads must not use up the login allowance. Missing cookies still
			// reach authentication and are rejected, rather than bypassing it.
			for range 21 {
				response := authRateLimitRequest(engine, http.MethodPost, "/api/user/auth/refresh", origin)
				require.Equal(t, http.StatusUnauthorized, response.Code)
			}
			for range 2 {
				response := authRateLimitRequest(engine, http.MethodPost, "/api/user/login", origin)
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), `"success":false`)
			}
			for _, path := range []string{
				"/api/user/login", "/api/user/login/2fa", "/api/user/login/verify",
				"/api/user/login/passkey/begin", "/api/user/login/passkey/finish",
				"/api/user/passkey/login/begin", "/api/user/passkey/login/finish", "/api/oauth/state",
			} {
				response := authRateLimitRequest(engine, http.MethodPost, path, origin)
				assert.Equal(t, http.StatusTooManyRequests, response.Code, path)
				assert.Equal(t, "1200", response.Header().Get("Retry-After"), path)
			}
			for _, path := range []string{"/api/oauth/wechat", "/api/oauth/telegram/login", "/api/oauth/github"} {
				assert.Equal(t, http.StatusTooManyRequests, authRateLimitRequest(engine, http.MethodGet, path, origin).Code, path)
			}

			// Exhausted login and sensitive-action budgets must not block renewal.
			for range 39 {
				assert.Equal(t, http.StatusUnauthorized, authRateLimitRequest(engine, http.MethodPost, "/api/user/auth/refresh", origin).Code)
			}
			response := authRateLimitRequest(engine, http.MethodPost, "/api/user/auth/refresh", origin)
			assert.Equal(t, http.StatusTooManyRequests, response.Code)
			assert.Equal(t, "60", response.Header().Get("Retry-After"))
			assert.Equal(t, http.StatusOK, authRateLimitRequest(engine, http.MethodPost, "/api/user/auth/logout", origin).Code)

			common.CriticalRateLimitEnable = false
			unlimited := gin.New()
			require.NoError(t, unlimited.SetTrustedProxies(nil))
			SetApiRouter(unlimited)
			assert.Equal(t, http.StatusOK, authRateLimitRequest(unlimited, http.MethodPost, "/api/user/login", origin).Code)
			assert.Equal(t, http.StatusUnauthorized, authRateLimitRequest(unlimited, http.MethodPost, "/api/user/auth/refresh", origin).Code)
			assert.Equal(t, http.StatusForbidden, authRateLimitRequest(unlimited, http.MethodPost, "/api/user/auth/refresh", "https://untrusted.example.com").Code)
		})
	}
}
