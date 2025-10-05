package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-IP rate limiting for auth endpoints
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// getVisitor returns a rate limiter for the given key with specified limits
func (rl *RateLimiter) getVisitor(key string, requestsPerMinute int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	if !exists {
		limiter := rate.NewLimiter(rate.Every(time.Minute), requestsPerMinute)
		rl.visitors[key] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *RateLimiter) isAllowed(key string, requestsPerMinute int) bool {
	limiter := rl.getVisitor(key, requestsPerMinute)
	return limiter.Allow()
}

// cleanupVisitors removes old entries to prevent memory leaks
func (rl *RateLimiter) cleanupVisitors() {
	for {
		time.Sleep(5 * time.Minute)
		rl.mu.Lock()
		for key, v := range rl.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()
	}
}
