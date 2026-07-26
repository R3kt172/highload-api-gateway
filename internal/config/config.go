package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Route struct {
	Prefix   string `json:"prefix"`
	Upstream string `json:"upstream"`
}

type Config struct {
	ListenAddr        string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	JWTSecret         string
	JWTIssuer         string
	JWTAudience       string
	Routes            []Route
	RoleLimits        map[string]int
	DefaultLimit      int
	Window            time.Duration
	FailOpen          bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        env("LISTEN_ADDR", ":8080"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		JWTIssuer:         env("JWT_ISSUER", "gateway"),
		JWTAudience:       env("JWT_AUDIENCE", "gateway-clients"),
		DefaultLimit:      envInt("DEFAULT_RPS", 5),
		Window:            envDuration("RATE_WINDOW", time.Second),
		FailOpen:          envBool("RATE_LIMIT_FAIL_OPEN", false),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	cfg.RedisDB = envInt("REDIS_DB", 0)
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if err := json.Unmarshal([]byte(env("ROLE_LIMITS_JSON", `{"admin":1000,"user":10}`)), &cfg.RoleLimits); err != nil {
		return Config{}, fmt.Errorf("parse ROLE_LIMITS_JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(env("ROUTES_JSON", `[{"prefix":"/api","upstream":"http://localhost:9000"}]`)), &cfg.Routes); err != nil {
		return Config{}, fmt.Errorf("parse ROUTES_JSON: %w", err)
	}
	if len(cfg.Routes) == 0 {
		return Config{}, fmt.Errorf("at least one route is required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
