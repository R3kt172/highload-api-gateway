package redislimiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/limiter"
	"github.com/redis/go-redis/v9"
)

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local oldest = now - window

redis.call("ZREMRANGEBYSCORE", key, "-inf", oldest)
local count = redis.call("ZCARD", key)
if count >= limit then
  local first = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
  local reset = now + window
  if #first == 2 then reset = tonumber(first[2]) + window end
  redis.call("PEXPIRE", key, window)
  return {0, count, reset}
end

redis.call("ZADD", key, now, member)
count = count + 1
redis.call("PEXPIRE", key, window)
return {1, count, now + window}
`)

type Limiter struct {
	client redis.UniversalClient
	now    func() time.Time
}

func New(client redis.UniversalClient) *Limiter {
	return &Limiter{client: client, now: time.Now}
}

func (l *Limiter) Allow(ctx context.Context, key string, limitValue int, window time.Duration) (limiter.Decision, error) {
	now := l.now()
	nowMS := now.UnixMilli()
	windowMS := window.Milliseconds()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return limiter.Decision{}, fmt.Errorf("generate event id: %w", err)
	}
	member := fmt.Sprintf("%d-%s", now.UnixNano(), hex.EncodeToString(random))

	raw, err := slidingWindowScript.Run(
		ctx,
		l.client,
		[]string{"ratelimit:" + key},
		nowMS,
		windowMS,
		limitValue,
		member,
	).Slice()
	if err != nil {
		return limiter.Decision{}, fmt.Errorf("execute sliding-window script: %w", err)
	}
	if len(raw) != 3 {
		return limiter.Decision{}, fmt.Errorf("unexpected redis response length: %d", len(raw))
	}
	allowed, err := asInt64(raw[0])
	if err != nil {
		return limiter.Decision{}, err
	}
	count, err := asInt64(raw[1])
	if err != nil {
		return limiter.Decision{}, err
	}
	resetMS, err := asInt64(raw[2])
	if err != nil {
		return limiter.Decision{}, err
	}
	remaining := max(0, limitValue-int(count))
	return limiter.Decision{
		Allowed:   allowed == 1,
		Limit:     limitValue,
		Remaining: remaining,
		ResetAt:   time.UnixMilli(resetMS),
	}, nil
}

func asInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case string:
		var result int64
		_, err := fmt.Sscan(v, &result)
		return result, err
	default:
		return 0, fmt.Errorf("unexpected redis value type %T", value)
	}
}
