package httpapi

import (
	"math"
	"sync"
	"time"
)

// rateLimiter is a token bucket per key: a key may make `burst` requests at once and then
// `perMinute` per minute on average. A nil *rateLimiter allows everything.
type rateLimiter struct {
	mu        sync.Mutex
	rate      float64 // tokens added per second
	burst     float64
	now       func() time.Time
	buckets   map[string]*bucket
	lastPrune time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// newRateLimiter returns nil (no limit) when perMinute is not positive.
func newRateLimiter(perMinute int, now func() time.Time) *rateLimiter {
	if perMinute <= 0 {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{
		rate:    float64(perMinute) / 60,
		burst:   float64(perMinute),
		now:     now,
		buckets: map[string]*bucket{},
	}
}

// allow takes one token for the key. When there is none it reports how long until there will be one.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

// prune forgets keys that have been idle long enough to have a full bucket again, at most once a
// minute, so that a flood of distinct addresses cannot grow the map without bound.
func (l *rateLimiter) prune(now time.Time) {
	if now.Sub(l.lastPrune) < time.Minute {
		return
	}
	l.lastPrune = now
	idle := time.Duration(l.burst/l.rate*float64(time.Second)) + time.Minute
	for k, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, k)
		}
	}
}
