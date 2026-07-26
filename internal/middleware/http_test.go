package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/auth"
	"github.com/R3kt172/highload-api-gateway/internal/limiter"
	"github.com/R3kt172/highload-api-gateway/internal/observability"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
)

type stubLimiter struct {
	decision limiter.Decision
	err      error
}

func (s stubLimiter) Allow(context.Context, string, int, time.Duration) (limiter.Decision, error) {
	return s.decision, s.err
}

func TestRateLimitRejectsRequest(t *testing.T) {
	metrics := observability.NewMetrics(prometheus.NewRegistry())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not run")
	})
	handler := RateLimit(RateLimitConfig{
		Limiter: stubLimiter{decision: limiter.Decision{
			Allowed: false, Limit: 10, Remaining: 0, ResetAt: time.Now().Add(time.Second),
		}},
		RoleLimits: map[string]int{"user": 10}, DefaultLimit: 5, Window: time.Second,
		Logger: logger, Metrics: metrics,
	}, next)
	validator := auth.NewValidator("test-secret", "issuer", "audience")
	handler = validator.Middleware(handler)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		UserID: "u1", Role: "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "issuer", Audience: jwt.ClaimStrings{"audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	raw, _ := token.SignedString([]byte("test-secret"))
	request := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is missing")
	}
}
