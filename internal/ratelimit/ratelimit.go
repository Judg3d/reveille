package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a per-key token bucket. Each key gets `burst` tokens that refill
// at `rate` tokens per second.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func New(ratePerSecond, burst float64) *Limiter {
	return &Limiter{
		buckets: map[string]*bucket{},
		rate:    ratePerSecond,
		burst:   burst,
		now:     time.Now,
	}
}

// Allow consumes a token for the key, reporting whether the request may
// proceed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		l.pruneLocked(now)
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// pruneLocked drops buckets that have fully refilled so the map does not grow
// without bound under many distinct keys.
func (l *Limiter) pruneLocked(now time.Time) {
	if len(l.buckets) < 1024 {
		return
	}
	full := time.Duration(l.burst / l.rate * float64(time.Second))
	for key, b := range l.buckets {
		if now.Sub(b.last) > full {
			delete(l.buckets, key)
		}
	}
}
