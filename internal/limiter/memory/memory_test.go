package memory

import (
	"context"
	"testing"
	"time"
)

func TestSlidingWindow(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	l := New()
	l.now = func() time.Time { return clock }

	for i := range 3 {
		decision, err := l.Allow(context.Background(), "user:/api", 3, time.Second)
		if err != nil || !decision.Allowed {
			t.Fatalf("request %d should be allowed: decision=%+v err=%v", i, decision, err)
		}
	}
	decision, _ := l.Allow(context.Background(), "user:/api", 3, time.Second)
	if decision.Allowed {
		t.Fatal("fourth request should be rejected")
	}

	clock = clock.Add(time.Second + time.Nanosecond)
	decision, _ = l.Allow(context.Background(), "user:/api", 3, time.Second)
	if !decision.Allowed {
		t.Fatal("request should be allowed after the window")
	}
}

func BenchmarkSlidingWindow(b *testing.B) {
	l := New()
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = l.Allow(ctx, "benchmark-user", 1_000_000_000, time.Second)
		}
	})
}
