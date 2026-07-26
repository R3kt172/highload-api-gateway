package limiter

import (
	"context"
	"time"
)

type Decision struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error)
}
