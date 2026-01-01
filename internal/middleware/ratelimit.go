package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string]*requestInfo
	limit    int
	window   time.Duration
	now      func() time.Time
}

type requestInfo struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return NewRateLimiterWithNow(limit, window, time.Now)
}

func NewRateLimiterWithNow(limit int, window time.Duration, now func() time.Time) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*requestInfo),
		limit:    limit,
		window:   window,
		now:      now,
	}
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	info, exists := rl.requests[key]
	if !exists || now.After(info.resetAt) {
		rl.requests[key] = &requestInfo{count: 1, resetAt: now.Add(rl.window)}
		return true
	}

	if info.count >= rl.limit {
		return false
	}

	info.count++
	return true
}

func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !rl.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}
