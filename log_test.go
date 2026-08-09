package main

// The P1/P2 "done means" from docs/logging-design.md, as tests: a forced
// upstream failure produces exactly one ERROR line; the marker probe holds
// the invariant (no request arguments in any log field, text included); a
// client mistake never logs at ERROR; a traceparent in _meta reaches the
// upstream HTTP request and stamps trace_id on the log lines of the same call.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// captureLogs redirects the default logger (at DEBUG, the most talkative
// level — the invariant must hold even there) and returns the buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(newLogger(&buf, slog.LevelDebug))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func swapAPI(t *testing.T, ts *httptest.Server) {
	t.Helper()
	orig := api
	api = newAPIClient(ts.URL)
	t.Cleanup(func() { api = orig; ts.Close() })
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not one-line JSON: %q (%v)", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestLogInvariantMarkerProbe(t *testing.T) {
	const markerVersion = "9.9.9-MRKVER"
	const markerSource = "MRKSRC daemonset image tag"
	buf := captureLogs(t)
	swapAPI(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	})))

	_, _, err := checkStackTool(context.Background(), nil, checkStackArgs{
		Components: []stackComponent{{Project: "envoy", Version: markerVersion, VersionSource: markerSource}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if s := buf.String(); strings.Contains(s, "MRKVER") || strings.Contains(s, "MRKSRC") {
		t.Fatalf("request arguments leaked into the log:\n%s", s)
	}
	var errorLines int
	for _, m := range logLines(t, buf) {
		if m["service"] != "mcp" {
			t.Errorf("line missing service:mcp: %v", m)
		}
		if m["level"] == "ERROR" {
			errorLines++
			if m["upstream"] != "/v1/changes" || m["status"] != float64(http.StatusBadGateway) {
				t.Errorf("ERROR line fields wrong: %v", m)
			}
			if m["tool"] != "check_stack" {
				t.Errorf("ERROR line missing tool attribution: %v", m)
			}
		}
	}
	if errorLines != 1 {
		t.Errorf("forced upstream failure must log exactly one ERROR line, got %d", errorLines)
	}
}

func TestClientMistakeIsNotError(t *testing.T) {
	buf := captureLogs(t)
	swapAPI(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unknown project"}`, http.StatusNotFound)
	})))

	if _, _, err := getReleaseTool(context.Background(), nil, getReleaseArgs{Project: "no-such-thing"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range logLines(t, buf) {
		if m["level"] == "ERROR" || m["level"] == "WARN" {
			t.Errorf("a client mistake surfaced above DEBUG: %v", m)
		}
	}
}

func TestTracePropagation(t *testing.T) {
	const tp = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	buf := captureLogs(t)
	var upstreamSaw []string
	swapAPI(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamSaw = append(upstreamSaw, r.Header.Get("traceparent"))
		w.Write([]byte(`{"projects":[]}`))
	})))

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"traceparent": tp}}}
	if _, _, err := listProjectsTool(context.Background(), req, listProjectsArgs{}); err != nil {
		t.Fatal(err)
	}

	if len(upstreamSaw) == 0 || upstreamSaw[0] != tp {
		t.Errorf("upstream request must carry the caller's traceparent, saw %v", upstreamSaw)
	}
	lines := logLines(t, buf)
	if len(lines) == 0 {
		t.Fatal("expected at least one debug line for the upstream call")
	}
	for _, m := range lines {
		if m["trace_id"] != traceID {
			t.Errorf("log line missing trace_id stamp: %v", m)
		}
		if m["tool"] != "list_projects" {
			t.Errorf("log line missing tool stamp: %v", m)
		}
	}

	// A malformed traceparent is dropped, not forwarded.
	upstreamSaw = nil
	bad := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{"traceparent": "garbage"}}}
	if _, _, err := listProjectsTool(context.Background(), bad, listProjectsArgs{}); err != nil {
		t.Fatal(err)
	}
	if len(upstreamSaw) == 0 || upstreamSaw[0] != "" {
		t.Errorf("malformed traceparent must not be forwarded, saw %v", upstreamSaw)
	}
}
