package breaker

import (
	"testing"
	"time"
)

func TestOpensAfterThreshold(t *testing.T) {
	b := New(3, 50*time.Millisecond)
	key := "/api/"
	for i := 0; i < 3; i++ {
		if !b.Allow(key) {
			t.Fatalf("expected allow before open")
		}
		b.Failure(key)
	}
	if b.Allow(key) {
		t.Fatalf("expected open circuit")
	}
	time.Sleep(60 * time.Millisecond)
	if !b.Allow(key) {
		t.Fatalf("expected half-open after cooldown")
	}
	b.Success(key)
	if !b.Allow(key) {
		t.Fatalf("expected closed after success")
	}
}
