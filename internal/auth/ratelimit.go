package auth

import (
	"sync"
	"time"
)

const (
	maxAttempts    = 5
	windowDuration = 15 * time.Minute
)

type attemptRecord struct {
	count    int
	windowStart time.Time
}

// RateLimiter tracks failed authentication attempts per IP.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptRecord
	now      func() time.Time // for testing
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string]*attemptRecord),
		now:      time.Now,
	}
}

// Allow returns true if the IP is allowed to attempt authentication.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, ok := rl.attempts[ip]
	if !ok {
		return true
	}

	// Window expired — reset.
	if rl.now().Sub(rec.windowStart) >= windowDuration {
		delete(rl.attempts, ip)
		return true
	}

	return rec.count < maxAttempts
}

// RecordFailure records a failed authentication attempt for the given IP.
func (rl *RateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	rec, ok := rl.attempts[ip]
	if !ok || now.Sub(rec.windowStart) >= windowDuration {
		rl.attempts[ip] = &attemptRecord{count: 1, windowStart: now}
		return
	}
	rec.count++
}

// Reset clears the failure count for an IP (called on successful auth).
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}
