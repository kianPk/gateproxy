package breaker

import (
	"sync"
	"time"
)

// State of a circuit for one key.
type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

// Breaker is a consecutive-failure circuit breaker with a single half-open probe.
type Breaker struct {
	mu        sync.Mutex
	failures  map[string]int
	openUntil map[string]time.Time
	probing   map[string]bool
	threshold int
	cooldown  time.Duration
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
		probing:   map[string]bool{},
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
		// Cooldown expired → admit exactly one probe in half-open.
		if b.probing[key] {
			return false
		}
		b.probing[key] = true
		delete(b.openUntil, key)
		return true
	}
	return true
}

func (b *Breaker) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures[key] = 0
	delete(b.openUntil, key)
	delete(b.probing, key)
}

func (b *Breaker) Failure(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.probing[key] {
		// Probe failed → reopen immediately.
		delete(b.probing, key)
		b.openUntil[key] = time.Now().Add(b.cooldown)
		b.failures[key] = 0
		return
	}
	b.failures[key]++
	if b.failures[key] >= b.threshold {
		b.openUntil[key] = time.Now().Add(b.cooldown)
		b.failures[key] = 0
	}
}

func (b *Breaker) StateOf(key string) State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.probing[key] {
		return StateHalfOpen
	}
	if until, ok := b.openUntil[key]; ok && time.Now().Before(until) {
		return StateOpen
	}
	return StateClosed
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
	half := []string{}
	for k, v := range b.probing {
		if v {
			half = append(half, k)
		}
	}
	fails := map[string]int{}
	for k, v := range b.failures {
		fails[k] = v
	}
	return map[string]any{
		"open":      open,
		"halfOpen":  half,
		"failures":  fails,
		"threshold": b.threshold,
	}
}
