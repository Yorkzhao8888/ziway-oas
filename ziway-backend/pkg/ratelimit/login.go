// Package ratelimit provides login attempt rate limiting.
package ratelimit

import (
	"sync"
	"time"
)

// LoginLimiter tracks failed login attempts by IP and username.
type LoginLimiter struct {
	mu       sync.RWMutex
	attempts map[string]*attempt // key: "ip:username" or "ip" or "username"
	maxFail  int
	lockout  time.Duration
}

type attempt struct {
	count    int
	lastFail time.Time
	locked   bool
	lockUntil time.Time
}

// NewLoginLimiter creates a new login rate limiter.
// maxFail: max failed attempts before lockout
// lockout: duration of lockout after maxFail
func NewLoginLimiter(maxFail int, lockout time.Duration) *LoginLimiter {
	l := &LoginLimiter{
		attempts: make(map[string]*attempt),
		maxFail:  maxFail,
		lockout:  lockout,
	}
	// Start cleanup goroutine
	go l.cleanup()
	return l
}

// Check returns true if the login attempt is allowed.
func (l *LoginLimiter) Check(ip, username string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now()

	// Check IP lock
	if a, ok := l.attempts["ip:"+ip]; ok && a.locked && now.Before(a.lockUntil) {
		return false
	}

	// Check username lock
	if a, ok := l.attempts["user:"+username]; ok && a.locked && now.Before(a.lockUntil) {
		return false
	}

	return true
}

// RecordFailure records a failed login attempt.
func (l *LoginLimiter) RecordFailure(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Record IP failure
	l.recordKey("ip:"+ip, now)
	// Record username failure
	l.recordKey("user:"+username, now)
}

// RecordSuccess clears the failure count for successful login.
func (l *LoginLimiter) RecordSuccess(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, "ip:"+ip)
	delete(l.attempts, "user:"+username)
}

func (l *LoginLimiter) recordKey(key string, now time.Time) {
	a, ok := l.attempts[key]
	if !ok {
		a = &attempt{}
		l.attempts[key] = a
	}

	a.count++
	a.lastFail = now

	if a.count >= l.maxFail {
		a.locked = true
		a.lockUntil = now.Add(l.lockout)
		a.count = 0 // Reset counter after lockout
	}
}

// RemainingAttempts returns the number of remaining attempts before lockout.
func (l *LoginLimiter) RemainingAttempts(ip, username string) int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	minRemaining := l.maxFail

	for _, key := range []string{"ip:" + ip, "user:" + username} {
		if a, ok := l.attempts[key]; ok {
			remaining := l.maxFail - a.count
			if remaining < minRemaining {
				minRemaining = remaining
			}
		}
	}

	if minRemaining < 0 {
		minRemaining = 0
	}
	return minRemaining
}

// LockoutRemaining returns the remaining lockout duration, or 0 if not locked.
func (l *LoginLimiter) LockoutRemaining(ip, username string) time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now()
	maxRemaining := time.Duration(0)

	for _, key := range []string{"ip:" + ip, "user:" + username} {
		if a, ok := l.attempts[key]; ok && a.locked && now.Before(a.lockUntil) {
			remaining := a.lockUntil.Sub(now)
			if remaining > maxRemaining {
				maxRemaining = remaining
			}
		}
	}

	return maxRemaining
}

// cleanup periodically removes expired entries.
func (l *LoginLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, a := range l.attempts {
			// Remove entries that are not locked and haven't been updated in 30 minutes
			if !a.locked && now.Sub(a.lastFail) > 30*time.Minute {
				delete(l.attempts, key)
			}
			// Remove locked entries that have expired
			if a.locked && now.After(a.lockUntil) {
				delete(l.attempts, key)
			}
		}
		l.mu.Unlock()
	}
}
