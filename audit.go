package main

// Audit stream (P3 of docs/logging-design.md). The operational log answers
// "is the system sick"; this answers "who did what" — and its whole point is
// request content, which is why it exists only behind an explicit opt-in
// (MCP_AUDIT=metadata|full, default off). In the installed deployment the
// stream is emitted inside the operator's cluster and lands in the operator's
// collectors: the same property that keeps versions private — data stays
// home — is what makes them auditable at home. Records share the house JSON
// schema and are distinguished by event:"audit", so a collector can route
// them to a different retention policy than operational noise.
//
// Layer honesty: this server has no authentication. "Caller" is the
// self-reported clientInfo plus whatever the transport shows — attestable
// down to WHICH CLIENT called WHICH TOOL with WHICH ARGUMENTS. Which human
// prompted the agent is knowledge the agent layer holds; an MCP-layer audit
// record cannot manufacture it, and the docs say so. The trace_id (when the
// caller sends W3C trace context) is the join key across those layers.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// auditMode is resolved once at startup: "" (off), "metadata", or "full".
var auditMode string

// auditOut is swappable for tests; records are one-line JSON, same stream as
// the operational log (stderr — stdout belongs to the stdio transport).
var auditOut io.Writer = os.Stderr

func initAudit() {
	switch os.Getenv("MCP_AUDIT") {
	case "metadata", "full":
		auditMode = os.Getenv("MCP_AUDIT")
	default:
		auditMode = ""
	}
}

// auditRecord builds one record. metadata mode carries argument NAMES only —
// values never appear at that level; full mode carries the raw argument
// object verbatim.
func auditRecord(mode, tool string, rawArgs json.RawMessage, outcome string, client *mcp.Implementation, transport, traceID string) map[string]any {
	rec := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
		"level":   "INFO",
		"msg":     "audit",
		"service": "mcp",
		"event":   "audit",
		"tool":    tool,
		"outcome": outcome,
	}
	if transport != "" {
		rec["transport"] = transport
	}
	if client != nil {
		rec["client_name"] = client.Name
		rec["client_version"] = client.Version
	}
	if traceID != "" {
		rec["trace_id"] = traceID
	}
	var parsed map[string]json.RawMessage
	if len(rawArgs) > 0 && json.Unmarshal(rawArgs, &parsed) == nil {
		names := make([]string, 0, len(parsed))
		for k := range parsed {
			names = append(names, k)
		}
		sort.Strings(names)
		rec["argument_names"] = names
	}
	if mode == "full" && len(rawArgs) > 0 {
		rec["arguments"] = rawArgs
	}
	return rec
}

func auditEmit(rec map[string]any) {
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	auditOut.Write(append(b, '\n'))
}

// auditMiddleware observes tools/call on the receiving side — one hook covers
// every tool, present and future. With auditing off it is the identity
// function: zero records, zero overhead beyond a nil check.
func auditMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if auditMode == "" || method != "tools/call" {
			return res, err
		}
		p, ok := req.GetParams().(*mcp.CallToolParamsRaw)
		if !ok {
			return res, err
		}
		outcome := "ok"
		if err != nil {
			outcome = "error"
		} else if r, ok := res.(*mcp.CallToolResult); ok && r.IsError {
			// The tool answered with guidance instead of data (unknown slug,
			// bad params) — a distinct outcome, not a protocol failure.
			outcome = "tool_error"
		}
		var client *mcp.Implementation
		transport := "stdio"
		sessionID := ""
		if s, ok := req.GetSession().(*mcp.ServerSession); ok {
			if ip := s.InitializeParams(); ip != nil {
				client = ip.ClientInfo
			}
			// The transport session id separates concurrent callers even when
			// none of them sends trace context (measured: the kagent Go ADK
			// doesn't, so trace_id sat empty and attribution fell back to time
			// windows). Named for what it is — a transport fact, not a
			// conversation id; the layer-honesty line does not move.
			sessionID = s.ID()
		}
		if re := req.GetExtra(); re != nil && re.Header != nil {
			transport = "http"
		}
		traceID := ""
		if tp, ok := p.Meta["traceparent"].(string); ok && traceparentRe.MatchString(tp) {
			traceID = tp[3:35]
		}
		rec := auditRecord(auditMode, p.Name, p.Arguments, outcome, client, transport, traceID)
		if sessionID != "" {
			rec["session_id"] = sessionID
		}
		auditEmit(rec)
		return res, err
	}
}
