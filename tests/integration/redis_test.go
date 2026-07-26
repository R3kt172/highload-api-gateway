//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	redislimiter "github.com/R3kt172/highload-api-gateway/internal/limiter/redis"
	"github.com/redis/go-redis/v9"
)

func TestRedisSlidingWindowIsShared(t *testing.T) {
	address := os.Getenv("REDIS_ADDR")
	if address == "" {
		address = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis is required for integration test: %v", err)
	}

	first := redislimiter.New(client)
	second := redislimiter.New(client)
	key := "integration:" + time.Now().Format("150405.000000000")
	t.Cleanup(func() { client.Del(ctx, "ratelimit:"+key) })

	for i := range 3 {
		instance := first
		if i%2 == 1 {
			instance = second
		}
		decision, err := instance.Allow(ctx, key, 2, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if (i < 2) != decision.Allowed {
			t.Fatalf("request %d: allowed=%v", i+1, decision.Allowed)
		}
	}
}
