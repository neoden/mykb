package httpd

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter limits requests per IP address.
type IPRateLimiter struct {
	mu          sync.Mutex
	limiters    map[string]*visitorLimiter
	rate        rate.Limit
	burst       int
	behindProxy bool
	warnedProxy bool // log warning only once
	stop        chan struct{}
}

type visitorLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter creates a rate limiter with r requests/second and burst size b.
// If behindProxy is true, trusts X-Forwarded-For header for client IP.
func NewIPRateLimiter(r rate.Limit, b int, behindProxy bool) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters:    make(map[string]*visitorLimiter),
		rate:        r,
		burst:       b,
		behindProxy: behindProxy,
		stop:        make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop stops the cleanup goroutine.
func (rl *IPRateLimiter) Stop() {
	close(rl.stop)
}

func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.limiters[ip]
	if !exists {
		v = &visitorLimiter{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// Allow checks if request from ip is allowed.
func (rl *IPRateLimiter) Allow(ip string) bool {
	return rl.getLimiter(ip).Allow()
}

// cleanup removes stale entries every minute.
func (rl *IPRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.limiters {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(rl.limiters, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// getIP extracts client IP from request.
func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getIPFromXFF extracts first IP from X-Forwarded-For header.
func getIPFromXFF(xff string) string {
	for i := 0; i < len(xff); i++ {
		if xff[i] == ',' {
			return xff[:i]
		}
	}
	return xff
}

// RateLimit wraps a handler with rate limiting.
func (rl *IPRateLimiter) RateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ip string
		xff := r.Header.Get("X-Forwarded-For")

		if rl.behindProxy && xff != "" {
			ip = getIPFromXFF(xff)
		} else {
			ip = getIP(r)
			// Warn if we see XFF but are not configured to trust it
			if xff != "" && !rl.warnedProxy {
				rl.mu.Lock()
				if !rl.warnedProxy {
					log.Printf("WARNING: X-Forwarded-For header detected but --behind-proxy not set. " +
						"If behind a proxy, all clients will share one rate limit. " +
						"Use --behind-proxy flag if running behind a reverse proxy.")
					rl.warnedProxy = true
				}
				rl.mu.Unlock()
			}
		}

		if !rl.Allow(ip) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}
