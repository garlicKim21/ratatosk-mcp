package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	rl := newRateLimiter(3)
	now := time.Now()
	for i := 1; i <= 3; i++ {
		if ok, _ := rl.allow("1.2.3.4", now); !ok {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	ok, retry := rl.allow("1.2.3.4", now)
	if ok {
		t.Fatal("the 4th request should be blocked")
	}
	if retry < 1 {
		t.Fatalf("retry-after must be at least 1s, got %d", retry)
	}
}

// The whole point of the middleware: one caller exhausting its window must not
// affect another. If this fails, we are back to a single shared bucket.
func TestRateLimiterIsolatesCallers(t *testing.T) {
	rl := newRateLimiter(1)
	now := time.Now()
	if ok, _ := rl.allow("1.2.3.4", now); !ok {
		t.Fatal("first caller's first request should be allowed")
	}
	if ok, _ := rl.allow("1.2.3.4", now); ok {
		t.Fatal("first caller's second request should be blocked")
	}
	if ok, _ := rl.allow("5.6.7.8", now); !ok {
		t.Fatal("a different caller must have its own budget")
	}
}

func TestRateLimiterWindowResets(t *testing.T) {
	rl := newRateLimiter(1)
	now := time.Now()
	rl.allow("1.2.3.4", now)
	if ok, _ := rl.allow("1.2.3.4", now); ok {
		t.Fatal("second request inside the window should be blocked")
	}
	if ok, _ := rl.allow("1.2.3.4", now.Add(rateWindow+time.Second)); !ok {
		t.Fatal("a new window should start fresh")
	}
}

// Off by default is what the self-hosted binary needs: it is single-tenant, so
// limiting its one caller would only get in the way.
func TestRateLimitFromEnvOffUnlessSet(t *testing.T) {
	for _, raw := range []string{"", "0", "-5", "abc"} {
		if rl := rateLimitFromEnv(raw); rl != nil {
			t.Fatalf("MCP_RATE_LIMIT_PER_MIN=%q should leave the limiter off", raw)
		}
	}
	rl := rateLimitFromEnv("60")
	if rl == nil || rl.limit != 60 {
		t.Fatalf("MCP_RATE_LIMIT_PER_MIN=60 should build a limiter of 60, got %+v", rl)
	}
}

func TestRateLimitMiddlewareNilIsIdentity(t *testing.T) {
	called := 0
	h := rateLimitMiddleware(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ }))
	for i := 0; i < 100; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", nil))
	}
	if called != 100 {
		t.Fatalf("a nil limiter must pass everything through, got %d/100", called)
	}
}

func TestRateLimitMiddlewareAnswers429WithRetryAfter(t *testing.T) {
	h := rateLimitMiddleware(newRateLimiter(1), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.Header.Set("X-Forwarded-For", "9.9.9.9")
		return r
	}
	first := httptest.NewRecorder()
	h.ServeHTTP(first, req())
	if first.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", first.Code)
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After so a client knows when to come back")
	}
}

// The caller address is a bucket key. It must never appear in what we send
// back, because the response is the one thing that leaves this process.
func TestRateLimitResponseDoesNotEchoCaller(t *testing.T) {
	h := rateLimitMiddleware(newRateLimiter(1), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		return r
	}
	h.ServeHTTP(httptest.NewRecorder(), req())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req())
	if body := rec.Body.String(); strings.Contains(body, "203.0.113.7") {
		t.Fatalf("429 body leaked the caller address: %s", body)
	}
}

func TestCallerKeyPrefersForwardedFor(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"cf-connecting-ip wins", map[string]string{"Cf-Connecting-Ip": "198.51.100.1", "X-Forwarded-For": "203.0.113.7"}, "10.0.0.9:5555", "198.51.100.1"},
		{"forwarded-for first entry", map[string]string{"X-Forwarded-For": "203.0.113.7, 10.0.0.1"}, "10.0.0.9:5555", "203.0.113.7"},
		{"forwarded-for single", map[string]string{"X-Forwarded-For": "203.0.113.7"}, "10.0.0.9:5555", "203.0.113.7"},
		{"real-ip fallback", map[string]string{"X-Real-Ip": "198.51.100.4"}, "10.0.0.9:5555", "198.51.100.4"},
		{"remote addr last", nil, "10.0.0.9:5555", "10.0.0.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := callerKey(r); got != tc.want {
				t.Fatalf("callerKey = %q, want %q", got, tc.want)
			}
		})
	}
}
