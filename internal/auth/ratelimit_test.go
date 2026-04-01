package auth

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.1"

	for i := 0; i < maxAttempts; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		rl.RecordFailure(ip)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.2"

	for i := 0; i < maxAttempts; i++ {
		rl.RecordFailure(ip)
	}

	if rl.Allow(ip) {
		t.Error("should be blocked after max attempts")
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.3"

	// Use a controllable clock.
	now := time.Now()
	rl.now = func() time.Time { return now }

	for i := 0; i < maxAttempts; i++ {
		rl.RecordFailure(ip)
	}

	if rl.Allow(ip) {
		t.Fatal("should be blocked")
	}

	// Advance past the window.
	now = now.Add(windowDuration + time.Second)

	if !rl.Allow(ip) {
		t.Error("should be allowed after window expires")
	}
}

func TestRateLimiter_ResetOnSuccess(t *testing.T) {
	rl := NewRateLimiter()
	ip := "192.168.1.4"

	for i := 0; i < maxAttempts-1; i++ {
		rl.RecordFailure(ip)
	}

	// Simulate successful auth.
	rl.Reset(ip)

	// Should be allowed again from scratch.
	for i := 0; i < maxAttempts; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("attempt %d should be allowed after reset", i+1)
		}
		rl.RecordFailure(ip)
	}
}

func TestRateLimiter_IndependentIPs(t *testing.T) {
	rl := NewRateLimiter()

	for i := 0; i < maxAttempts; i++ {
		rl.RecordFailure("10.0.0.1")
	}

	if rl.Allow("10.0.0.1") {
		t.Error("10.0.0.1 should be blocked")
	}
	if !rl.Allow("10.0.0.2") {
		t.Error("10.0.0.2 should not be affected")
	}
}
