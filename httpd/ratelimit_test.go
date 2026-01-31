package httpd

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"192.168.1.1:12345", "192.168.1.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"[::1]:8080", "::1"},
		{"invalid", "invalid"}, // fallback
	}

	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remoteAddr
		got := getIP(r)
		if got != tt.want {
			t.Errorf("getIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}

func TestGetIPFromXFF(t *testing.T) {
	tests := []struct {
		xff  string
		want string
	}{
		{"1.2.3.4", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8, 9.10.11.12", "1.2.3.4"},
		{"", ""},
	}

	for _, tt := range tests {
		got := getIPFromXFF(tt.xff)
		if got != tt.want {
			t.Errorf("getIPFromXFF(%q) = %q, want %q", tt.xff, got, tt.want)
		}
	}
}

func TestRateLimiterUsesRemoteAddrByDefault(t *testing.T) {
	rl := NewIPRateLimiter(100, 100, false)

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request with XFF header but behindProxy=false
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()

	handler(w, r)

	// Should use RemoteAddr (10.0.0.1), not XFF (1.2.3.4)
	// Verify by checking limiter state
	rl.mu.Lock()
	_, hasRemoteIP := rl.limiters["10.0.0.1"]
	_, hasXFFIP := rl.limiters["1.2.3.4"]
	rl.mu.Unlock()

	if !hasRemoteIP {
		t.Error("expected limiter for RemoteAddr IP")
	}
	if hasXFFIP {
		t.Error("should not create limiter for XFF IP when behindProxy=false")
	}
}

func TestRateLimiterUsesXFFWhenBehindProxy(t *testing.T) {
	rl := NewIPRateLimiter(100, 100, true)

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	w := httptest.NewRecorder()

	handler(w, r)

	rl.mu.Lock()
	_, hasRemoteIP := rl.limiters["10.0.0.1"]
	_, hasXFFIP := rl.limiters["1.2.3.4"]
	rl.mu.Unlock()

	if hasRemoteIP {
		t.Error("should not use RemoteAddr when behindProxy=true and XFF present")
	}
	if !hasXFFIP {
		t.Error("expected limiter for first XFF IP")
	}
}

func TestRateLimiterWarnsOnXFFWithoutBehindProxy(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	rl := NewIPRateLimiter(100, 100, false)

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request with XFF
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	handler(w, r)

	if !strings.Contains(buf.String(), "WARNING") {
		t.Error("expected warning about X-Forwarded-For")
	}
	if !strings.Contains(buf.String(), "--behind-proxy") {
		t.Error("expected warning to mention --behind-proxy flag")
	}

	// Second request - should not warn again
	buf.Reset()
	handler(w, r)

	if strings.Contains(buf.String(), "WARNING") {
		t.Error("should only warn once")
	}
}

func TestRateLimiterNoWarningWhenBehindProxy(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	rl := NewIPRateLimiter(100, 100, true)

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	handler(w, r)

	if strings.Contains(buf.String(), "WARNING") {
		t.Error("should not warn when behindProxy=true")
	}
}

func TestRateLimiterNoWarningWithoutXFF(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	rl := NewIPRateLimiter(100, 100, false)

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	// No XFF header
	w := httptest.NewRecorder()
	handler(w, r)

	if strings.Contains(buf.String(), "WARNING") {
		t.Error("should not warn without XFF header")
	}
}

func TestRateLimiterEnforcesLimit(t *testing.T) {
	rl := NewIPRateLimiter(1, 2, false) // 1 req/sec, burst 2

	handler := rl.RateLimit(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"

	// First 2 requests should succeed (burst)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Third request should be rate limited
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request 3: expected 429, got %d", w.Code)
	}
}
