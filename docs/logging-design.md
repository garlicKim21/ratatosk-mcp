# Design note: logging, audit, and trace integration

Status: **accepted direction, mostly unimplemented.** The error log ships with
the post-0.4.1 SDK bump; the audit log waits for the first enterprise ask; the
trace phases are marked below. This note exists so those conversations start
from a position instead of a blank page.

Context: as of 0.4.1 the server logs almost nothing — three statements in the
whole codebase (one startup line, two fatals), and the request path is silent.
An empirical probe (a `check_stack` call carrying marker strings through both
transports) confirms zero bytes of request content reach stdout/stderr. That
silence is half right: it is the privacy guarantee working, and it is also an
operational gap — a pod that fails every request looks identical to a healthy
one in `kubectl logs`, which is exactly how our own collector once ran silent
for two weeks.

## The invariant

**No request arguments in any log line or field, in any mode, at any level.**
Not the version, not `version_source`, not free-text error strings that embed
them — `get_release` failures carry the version inside the URL
(`/v1/releases/cilium/v1.16.0: HTTP 404`), so error log lines are
*reconstructed* (tool, upstream kind, status class), never copied. Project
slugs are permitted: the privacy model already sends slugs upstream.

The single exception is the audit stream below — explicitly opted in, and in
the installed deployment the data never leaves the operator's own cluster.

This invariant is testable: the marker-string probe becomes a CI test the day
the error log lands, and it watches JSON fields as well as text.

## Operational error log (ships with the SDK bump)

One-line JSON via `log/slog`, the same house schema as the ratatosk worker
(`time`, `level`, `msg`, `service:"mcp"`, plus event fields):

```json
{"time":"2026-07-30T01:20:00Z","level":"ERROR","msg":"upstream fetch failed","service":"mcp","tool":"check_stack","upstream":"/v1/facts","status":502}
```

JSON because collectors (alloy/Loki here, whatever the operator runs there)
lift `level` into a severity label mechanically — "MCP error rate" alerting
falls out with no extra wiring.

Levels, few and disciplined:

| Level | When | Examples |
|---|---|---|
| `INFO` | lifecycle | startup, shutdown |
| `WARN` | degraded but working | upstream 429, slow responses |
| `ERROR` | we could not do the job, cause is infrastructure | upstream 5xx, timeout, connection refused (egress blocked) |
| `DEBUG` | opt-in via `MCP_LOG=debug` | per-method timing; still argument-free |

**A client mistake is not an ERROR.** Unknown slug, unparseable version — the
tool answers those with guidance, and that answer *is* the correct handling.
Logging them at ERROR lets one confused agent's typo loop page an operator
while a real outage drowns in the noise. The bar for ERROR is: *an operator
can fix it.* Client mistakes surface at DEBUG only.

## Audit log (designed; build on first enterprise demand)

The operational log answers "is the system sick"; an audit log answers "who
did what", and its whole point is request content. The two do not conflict —
the resolution is *whose data it is*:

- **Installed mode**: the audit stream is emitted inside the operator's
  cluster and lands in the operator's collectors. An enterprise auditing its
  own agents inside its own perimeter is the privacy model working, not an
  exception to it. The same property that keeps versions private — data stays
  home — is what makes them auditable at home.
- **Hosted mode: no audit stream, by design.** The hosted endpoint's stance is
  "nothing retained"; an audit trail on our side would break that stance and
  make us a retention target. Enterprises that need audit use the installed
  mode — the tier split is the answer, not a limitation.

Shape when built:

- Default **off**; `MCP_AUDIT=metadata` or `MCP_AUDIT=full`.
- `metadata`: timestamp, tool, caller identity as known, outcome, argument
  *names*. `full`: argument values too.
- Same stdout, distinguished by `"event":"audit"` so collectors can route it
  to a different retention policy (operational logs rotate in days; audit
  retention is measured in years and is the platform's job, as is
  tamper-evidence — we emit, the SIEM preserves).
- **Layer honesty**: this server has no authentication, so "caller" is
  self-reported `clientInfo` plus network peer — attestable down to *which
  client called which tool with which arguments*. Which human prompted the
  agent is knowledge the agent layer (e.g. kagent) holds; an MCP-layer audit
  record cannot manufacture it and the docs must say so. The join key across
  layers is the trace context below.

## Trace integration (OpenTelemetry)

MCP 2026-07-28 documents `_meta` conventions for W3C trace context
(`traceparent`, `tracestate`, `baggage`). That gives a standard way to stitch
*user conversation → agent run → MCP tool call → upstream API* into one trace.

- **Phase 0 — near-free, with the SDK bump**: read `traceparent` from
  incoming `_meta`, forward it as the standard HTTP header on upstream `/v1`
  calls, and stamp `trace_id` into every JSON log line (and future audit
  record). No OTel SDK, no exporter — just propagation plus log correlation,
  which is most of the enterprise value.
- **Phase 1 — on demand**: real spans via `go.opentelemetry.io/otel`, OTLP
  export active only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set (the standard
  env contract); default remains inert.
- **At bump time, check** what go-sdk v1.7.0 already does with `_meta` trace
  keys before writing any of this by hand.

The upstream ratatosk app has its own OTel phase-2 reservation (worker
`run_id` as correlation key, docs/observability.md) — when both ends exist,
the trace crosses the wire and the ladder is complete.
