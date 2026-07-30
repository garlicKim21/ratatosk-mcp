package main

// Operational logging (P1) and trace propagation phase 0 (P2) of
// docs/logging-design.md. The invariant lives here as much as in the call
// sites: no request arguments in any log line or field, in any mode, at any
// level — error text is reconstructed (endpoint pattern, status class, error
// kind), never copied, because upstream URLs embed running versions.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ctxKey int

const (
	ctxKeyTool ctxKey = iota
	ctxKeyTraceparent
	ctxKeyTraceID
)

// W3C trace context: version(2)-trace_id(32)-parent_id(16)-flags(2), lowercase hex.
var traceparentRe = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// requestContext tags the context with the tool name and, when the caller sent
// W3C trace context in _meta (the MCP 2026-07-28 convention), the traceparent —
// the join key across the agent layer, this server's log lines, and the
// upstream /v1 call. Malformed values are dropped, not forwarded.
func requestContext(ctx context.Context, tool string, req *mcp.CallToolRequest) context.Context {
	ctx = context.WithValue(ctx, ctxKeyTool, tool)
	if req == nil || req.Params == nil {
		return ctx
	}
	if tp, ok := req.Params.Meta["traceparent"].(string); ok && traceparentRe.MatchString(tp) {
		ctx = context.WithValue(ctx, ctxKeyTraceparent, tp)
		ctx = context.WithValue(ctx, ctxKeyTraceID, tp[3:35])
	}
	return ctx
}

func traceparentFrom(ctx context.Context) string {
	tp, _ := ctx.Value(ctxKeyTraceparent).(string)
	return tp
}

// ctxHandler stamps tool and trace_id from the request context onto every
// record, so correlation costs nothing at the call sites.
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if tool, ok := ctx.Value(ctxKeyTool).(string); ok {
		r.AddAttrs(slog.String("tool", tool))
	}
	if tid, ok := ctx.Value(ctxKeyTraceID).(string); ok {
		r.AddAttrs(slog.String("trace_id", tid))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}

// newLogger builds the house-schema JSON logger (time, level, msg,
// service:"mcp", plus event fields) — the same shape the ratatosk worker
// emits, so a collector lifts level into a severity label with no config.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(ctxHandler{h}).With("service", "mcp")
}

// setupLogging wires the default logger to stderr — stdout belongs to the
// stdio transport — at the level MCP_LOG selects (default info; "debug" adds
// per-call timing, still argument-free).
func setupLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("MCP_LOG")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(newLogger(os.Stderr, level))
}

// errKind classifies a transport error without quoting it: the error string
// of a failed request embeds the full URL, and get_release URLs embed the
// running version. Coarse buckets are exactly what an operator can act on.
func errKind(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	return "transport"
}
