package crawler

import (
	"code/internal/models"
	"context"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func newRateLimiter(opts models.Options) *rateLimiter {
	if opts.RPS > 0 {
		return &rateLimiter{
			interval: time.Second / time.Duration(opts.RPS),
		}
	}

	if opts.Delay > 0 {
		return &rateLimiter{
			interval: opts.Delay,
		}
	}

	return &rateLimiter{}
}

func (l *rateLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.interval <= 0 {
		l.last = time.Now()
		return nil
	}

	now := time.Now()

	if l.last.IsZero() {
		l.last = now
		return nil
	}

	nextAllowed := l.last.Add(l.interval)
	if now.Before(nextAllowed) {
		timer := time.NewTimer(nextAllowed.Sub(now))
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	l.last = time.Now()
	return nil
}
