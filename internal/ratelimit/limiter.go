package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

type Limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	byKey  map[string]*bucket
	lastGC time.Time
}

func New(rps float64, burst int) *Limiter {
	if rps <= 0 {
		rps = 10
	}
	if burst <= 0 {
		burst = int(rps)
	}
	return &Limiter{
		rate:   rps,
		burst:  float64(burst),
		byKey:  map[string]*bucket{},
		lastGC: time.Now(),
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.byKey[key]
	if !ok {
		l.byKey[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	if now.Sub(l.lastGC) > time.Minute {
		for k, v := range l.byKey {
			if now.Sub(v.last) > 5*time.Minute {
				delete(l.byKey, k)
			}
		}
		l.lastGC = now
	}
	return true
}
