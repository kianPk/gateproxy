package breaker

import (
	"sync"
	"time"
)

// Breaker is a simple consecutive-failure circuit breaker per key.
type Breaker struct {
	mu          sync.Mutex
	failures    map[string]int
	openUntil   map[string]time.Time
	threshold   int
	cooldown    time.Duration
}

func New(threshold int, cooldown time.Duration) *Breaker {
	if threshold < 1 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 20 * time.Second
	}
	return &Breaker{
		failures:  map[string]int{},
		openUntil: map[string]time.Time{},
		threshold: threshold,
		cooldown:  cooldown,
	}
}

func (b *Breaker) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if until, ok := b.openUntil[key]; ok {
		if time.Now().Before(until) {
			return false
		}
		delete(b.openUntil, key)
		b.failures[key] = 0
	}
	return true
}

func (b *Breaker) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures[key] = 0
	delete(b.openUntil, key)
}

func (b *Breaker) Failure(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures[key]++
	if b.failures[key] >= b.threshold {
		b.openUntil[key] = time.Now().Add(b.cooldown)
		b.failures[key] = 0
	}
}

func (b *Breaker) Snapshot() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	open := map[string]string{}
	for k, until := range b.openUntil {
		if time.Now().Before(until) {
			open[k] = until.UTC().Format(time.RFC3339)
		}
	}
	fails := map[string]int{}
	for k, v := range b.failures {
		fails[k] = v
	}
	return map[string]any{"open": open, "failures": fails, "threshold": b.threshold}
}
