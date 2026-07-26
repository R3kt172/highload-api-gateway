package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/auth"
	"github.com/R3kt172/highload-api-gateway/internal/limiter"
	"github.com/R3kt172/highload-api-gateway/internal/observability"
)

type traceKey struct{}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := sanitizeTraceID(r.Header.Get("X-Trace-ID"))
		if traceID == "" {
			traceID = newTraceID()
		}
		w.Header().Set("X-Trace-ID", traceID)
		r.Header.Set("X-Trace-ID", traceID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), traceKey{}, traceID)))
	})
}

func TraceID(ctx context.Context) string {
	value, _ := ctx.Value(traceKey{}).(string)
	return value
}

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "trace_id", TraceID(r.Context()), "panic", recovered, "stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Logging(logger *slog.Logger, metrics *observability.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		route := routeLabel(r.URL.Path)
		metrics.ObserveRequest(r.Method, route, recorder.status, time.Since(started))
		attrs := []any{
			"trace_id", TraceID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		}
		if claims, ok := auth.FromContext(r.Context()); ok {
			attrs = append(attrs, "user_id", claims.UserID, "role", claims.Role)
		}
		logger.Info("request completed", attrs...)
	})
}

type RateLimitConfig struct {
	Limiter      limiter.Limiter
	RoleLimits   map[string]int
	DefaultLimit int
	Window       time.Duration
	FailOpen     bool
	Logger       *slog.Logger
	Metrics      *observability.Metrics
}

func RateLimit(cfg RateLimitConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication context missing"})
			return
		}
		limitValue := cfg.RoleLimits[claims.Role]
		if limitValue == 0 {
			limitValue = cfg.DefaultLimit
		}
		key := claims.UserID + ":" + claims.Role + ":" + routeLabel(r.URL.Path)
		started := time.Now()
		decision, err := cfg.Limiter.Allow(r.Context(), key, limitValue, cfg.Window)
		cfg.Metrics.RedisDuration.Observe(time.Since(started).Seconds())
		if err != nil {
			cfg.Logger.Error("rate limiter unavailable", "trace_id", TraceID(r.Context()), "error", err, "fail_open", cfg.FailOpen)
			if cfg.FailOpen {
				w.Header().Set("X-RateLimit-Policy", "fail-open")
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rate limiter unavailable"})
			return
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(decision.ResetAt.Unix(), 10))
		if !decision.Allowed {
			cfg.Metrics.RateRejected.WithLabelValues(claims.Role).Inc()
			wait := time.Until(decision.ResetAt)
			retryAfter := max(1, int((wait+time.Second-1)/time.Second))
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func BodyLimit(bytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, bytes)
		next.ServeHTTP(w, r)
	})
}

func newTraceID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(data)
}

func sanitizeTraceID(value string) string {
	if len(value) < 8 || len(value) > 64 {
		return ""
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' && ch != '_' {
			return ""
		}
	}
	return value
}

func routeLabel(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "/"
	}
	return "/" + parts[0]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
