package crawler

import (
	"context"
	"time"
)

type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

func newRateLimiter(opts Options) *rateLimiter {
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

func (l *rateLimiter) wait(ctx context.Context) error {
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
