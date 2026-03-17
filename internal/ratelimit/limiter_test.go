package ratelimit

import (
	"strings"
	"sync"
	"testing"
)

func TestLimiter_AllowsUnderLimit(t *testing.T) {
	l := NewLimiter(60) // 60/min = 1/sec
	if !l.Allow() {
		t.Error("expected Allow() to return true when under limit")
	}
}

func TestLimiter_RejectsWhenExhausted(t *testing.T) {
	l := NewLimiter(10) // 10/min, burst = max(5, 10/6) = 5
	// Drain all burst tokens.
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Fatalf("call %d should succeed within burst", i+1)
		}
	}
	// Rapidly exhaust — without waiting, subsequent calls should fail.
	rejected := false
	for i := 0; i < 20; i++ {
		if !l.Allow() {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Error("expected at least one rejection when rate is exhausted")
	}
}

func TestLimiter_ZeroMeansUnlimited(t *testing.T) {
	l := NewLimiter(0)
	for i := 0; i < 1000; i++ {
		if !l.Allow() {
			t.Fatalf("Allow() returned false at iteration %d with unlimited limiter", i)
		}
	}
}

func TestLimiter_NegativeMeansUnlimited(t *testing.T) {
	l := NewLimiter(-5)
	for i := 0; i < 100; i++ {
		if !l.Allow() {
			t.Fatalf("Allow() returned false with negative rate")
		}
	}
}

func TestLimiter_DenyMessage(t *testing.T) {
	l := NewLimiter(30)
	msg := l.DenyMessage()
	if !strings.Contains(msg, "Rate limited") {
		t.Errorf("expected 'Rate limited' in message, got: %s", msg)
	}
	if !strings.Contains(msg, "30") {
		t.Errorf("expected rate '30' in message, got: %s", msg)
	}
	if !strings.Contains(msg, "slow down") || !strings.Contains(msg, "wait") {
		t.Errorf("expected actionable advice in message, got: %s", msg)
	}
}

func TestLimiter_DenyMessageUnlimited(t *testing.T) {
	l := NewLimiter(0)
	msg := l.DenyMessage()
	if !strings.Contains(msg, "unlimited") {
		t.Errorf("expected 'unlimited' in message for zero-rate limiter, got: %s", msg)
	}
}

func TestLimiter_BurstAllowsLLMParallelCalls(t *testing.T) {
	// Write limiter at 30/min should allow at least 5 rapid calls (burst=5).
	l := NewLimiter(30)
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Fatalf("call %d should succeed within burst for 30/min limiter", i+1)
		}
	}

	// Read limiter at 120/min should allow at least 15 rapid calls (burst=20).
	l2 := NewLimiter(120)
	for i := 0; i < 15; i++ {
		if !l2.Allow() {
			t.Fatalf("call %d should succeed within burst for 120/min limiter", i+1)
		}
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	l := NewLimiter(1000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Allow()
		}()
	}
	wg.Wait()
	// No race condition — test passes if no panic.
}
