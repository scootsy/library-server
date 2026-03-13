package api

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// ipRateLimiter tracks request counts per IP address using a sliding window.
type ipRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int           // max requests per window
	window   time.Duration // window duration
}

type visitor struct {
	count    int
	windowStart time.Time
}

// newIPRateLimiter creates a rate limiter that allows limit requests per window per IP.
func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	rl := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
	}
	// Periodically clean up stale entries.
	go rl.cleanup()
	return rl
}

// allow checks whether the given IP is within the rate limit.
func (rl *ipRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]
	if !exists || now.Sub(v.windowStart) >= rl.window {
		rl.visitors[ip] = &visitor{count: 1, windowStart: now}
		return true
	}

	v.count++
	return v.count <= rl.limit
}

// cleanup removes stale entries every 2x the window duration.
func (rl *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, v := range rl.visitors {
			if now.Sub(v.windowStart) >= rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware wraps an http.Handler with per-IP rate limiting.
// Returns HTTP 429 when the limit is exceeded.
func rateLimitMiddleware(limiter *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !limiter.allow(ip) {
				slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
				writeError(w, http.StatusTooManyRequests, "too many requests, please try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client IP from the request, checking X-Forwarded-For
// and X-Real-IP headers before falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	// Trust X-Real-IP first (typically set by reverse proxy).
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Check X-Forwarded-For (take the first entry, which is the original client).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First comma-separated value is the original client IP.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to RemoteAddr.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
