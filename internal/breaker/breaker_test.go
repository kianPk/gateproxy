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
	if b.StateOf(key) != StateOpen {
		t.Fatalf("expected open state")
	}
	time.Sleep(60 * time.Millisecond)
	if !b.Allow(key) {
		t.Fatalf("expected half-open probe after cooldown")
	}
	if b.StateOf(key) != StateHalfOpen {
		t.Fatalf("expected half_open while probing")
	}
	// Second caller blocked during probe.
	if b.Allow(key) {
		t.Fatalf("expected only one probe in half-open")
	}
	b.Success(key)
	if b.StateOf(key) != StateClosed {
		t.Fatalf("expected closed after successful probe")
	}
	if !b.Allow(key) {
		t.Fatalf("expected closed after success")
	}
}

func TestProbeFailureReopens(t *testing.T) {
	b := New(2, 40*time.Millisecond)
	key := "/pay/"
	b.Failure(key)
	b.Failure(key)
	if b.Allow(key) {
		t.Fatalf("expected open")
	}
	time.Sleep(50 * time.Millisecond)
	if !b.Allow(key) {
		t.Fatalf("expected probe")
	}
	b.Failure(key)
	if b.Allow(key) {
		t.Fatalf("expected re-open after failed probe")
	}
}
