package main

// Per-caller rate limiting for the HTTP transport (2026-08-10).
//
// Why this lives here and not upstream in the web API. The hosted endpoint
// reaches ratatosk's /v1 over the compose network, deliberately bypassing the
// CDN and reverse proxy so that /v1 paths — which carry project slugs and
// running versions — never reach an access log. The cost of that design is
// that no forwarding header survives the hop, so upstream cannot tell hosted
// callers apart and every one of them shares a single bucket. Forwarding the
// caller address upstream would fix the fairness and break the separation:
// today the web tier *cannot* correlate who asked with what was asked,
// because it never receives the who. That is a structural guarantee, and a
// structural guarantee survives a future careless log line in a way that a
// policy one does not.
//
// This process, on the other hand, already receives the caller address from
// the reverse proxy, at a layer whose whole job is to front many users. So it
// is the right place to fair-share between them. The address is used as a map
// key and nothing else: it is never logged, never forwarded upstream, and the
// entry is dropped when its window expires.
//
// Off unless MCP_RATE_LIMIT_PER_MIN is set. The same binary is what people
// self-host, and a self-hosted server is single-tenant — limiting its one
// caller would only get in the way.

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const rateWindow = time.Minute

// rateLimiter is a fixed-window counter keyed by caller address. Restarts
// reset it, which is acceptable: it bounds abuse, it does not bill anyone.
type rateLimiter struct {
	limit int
	mu    sync.Mutex
	seen  map[string]*rateBucket
}

type rateBucket struct {
	count   int
	resetAt time.Time
}

// maxTrackedCallers bounds the map so a caller cycling source addresses cannot
// grow it without limit; the sweep runs only when the bound is crossed.
const maxTrackedCallers = 10_000

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, seen: map[string]*rateBucket{}}
}

// allow reports whether this caller may proceed, and how many seconds remain
// in the window if it may not.
func (r *rateLimiter) allow(key string, now time.Time) (bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.seen) > maxTrackedCallers {
		for k, b := range r.seen {
			if now.After(b.resetAt) {
				delete(r.seen, k)
			}
		}
	}
	b, ok := r.seen[key]
	if !ok || now.After(b.resetAt) {
		r.seen[key] = &rateBucket{count: 1, resetAt: now.Add(rateWindow)}
		return true, 0
	}
	b.count++
	if b.count > r.limit {
		retry := int(time.Until(b.resetAt).Seconds())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	return true, 0
}

// callerKey is the address the reverse proxy says the request came from. It is
// a bucket key and nothing more — never logged, never sent upstream.
// X-Forwarded-For is a list; the first entry is the original client.
func callerKey(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if v := strings.TrimSpace(first); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(req.Header.Get("X-Real-Ip")); v != "" {
		return v
	}
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}

// rateLimitMiddleware wraps a handler with per-caller limiting. A nil limiter
// is the identity, so the self-hosted path costs nothing.
func rateLimitMiddleware(rl *rateLimiter, next http.Handler) http.Handler {
	if rl == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ok, retry := rl.allow(callerKey(req), time.Now())
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			// JSON-RPC has no transport-level error shape, and a client that
			// reads the status code needs no body; keep it short and free of
			// anything that identifies the caller.
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded (` +
				strconv.Itoa(rl.limit) + ` tool calls/minute per caller)"}`))
			return
		}
		next.ServeHTTP(w, req)
	})
}

// rateLimitFromEnv reads MCP_RATE_LIMIT_PER_MIN. Absent, zero, or unparseable
// means off — the default, and what a self-hosted server wants.
func rateLimitFromEnv(raw string) *rateLimiter {
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil
	}
	return newRateLimiter(n)
}
