package memory

import (
	"context"
	"sync"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/limiter"
)

type bucket struct {
	mu     sync.Mutex
	events []time.Time
}

type Limiter struct {
	buckets sync.Map
	now     func() time.Time
}

func New() *Limiter {
	return &Limiter{now: time.Now}
}

func (l *Limiter) Allow(_ context.Context, key string, limitValue int, window time.Duration) (limiter.Decision, error) {
	value, _ := l.buckets.LoadOrStore(key, &bucket{})
	b := value.(*bucket)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-window)
	firstActive := 0
	for firstActive < len(b.events) && !b.events[firstActive].After(cutoff) {
		firstActive++
	}
	b.events = append(b.events[:0], b.events[firstActive:]...)
	if len(b.events) >= limitValue {
		return limiter.Decision{
			Allowed: false, Limit: limitValue, Remaining: 0, ResetAt: b.events[0].Add(window),
		}, nil
	}
	b.events = append(b.events, now)
	return limiter.Decision{
		Allowed: true, Limit: limitValue, Remaining: limitValue - len(b.events), ResetAt: now.Add(window),
	}, nil
}
