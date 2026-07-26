package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/auth"
	"github.com/R3kt172/highload-api-gateway/internal/config"
	redislimiter "github.com/R3kt172/highload-api-gateway/internal/limiter/redis"
	"github.com/R3kt172/highload-api-gateway/internal/middleware"
	"github.com/R3kt172/highload-api-gateway/internal/observability"
	"github.com/R3kt172/highload-api-gateway/internal/proxy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func New(cfg config.Config, logger *slog.Logger) (http.Handler, func(), error) {
	registry := prometheus.NewRegistry()
	metrics := observability.NewMetrics(registry)

	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PoolSize:     100,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		if !cfg.FailOpen {
			_ = redisClient.Close()
			return nil, func() {}, fmt.Errorf("connect to redis: %w", err)
		}
		logger.Warn("redis unavailable during startup; continuing in fail-open mode", "error", err)
	}

	proxyRouter, err := proxy.NewRouter(cfg.Routes, logger, metrics)
	if err != nil {
		_ = redisClient.Close()
		return nil, func() {}, err
	}

	protected := http.Handler(proxyRouter)
	protected = middleware.BodyLimit(10<<20, protected)
	protected = middleware.RateLimit(middleware.RateLimitConfig{
		Limiter:      redislimiter.New(redisClient),
		RoleLimits:   cfg.RoleLimits,
		DefaultLimit: cfg.DefaultLimit,
		Window:       cfg.Window,
		FailOpen:     cfg.FailOpen,
		Logger:       logger,
		Metrics:      metrics,
	}, protected)
	protected = auth.NewValidator(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience).Middleware(protected)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
	mux.Handle("/", protected)

	var handler http.Handler = mux
	handler = middleware.Logging(logger, metrics, handler)
	handler = middleware.Recover(logger, handler)
	handler = middleware.Trace(handler)
	return handler, func() { _ = redisClient.Close() }, nil
}
