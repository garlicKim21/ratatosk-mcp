# Design note: logging, audit, and trace integration

Status: **adopted roadmap (2026-07-30).** The goal is a product an operator
can adopt without a leap of faith: failures make noise, enterprises can audit
inside their own perimeter, a conversation is traceable down to the upstream
API call — and the privacy invariant (argument-free by default) survives all
of it. We build the whole ladder deliberately, ahead of demand; what keeps
that from burdening casual users is that **every capability defaults to
inert** (quiet logs, audit off, exporter off).

## Roadmap

| Phase | What | Gate / prerequisite | Done means |
|---|---|---|---|
| **P0** | go-sdk v1.6.1 → v1.7.0, dual-revision (2026-07-28 + legacy) support | Hub re-campaign results reviewed; bump decision | tests + chart-smoke green; legacy client (kagent) negotiation verified live; `Stateless` option measured and decided; what the SDK does natively with `_meta` trace keys is written down |
| **P1** | Operational error log (spec below) | with P0 | probe CI test guards the invariant; a forced upstream failure shows one ERROR line in `kubectl logs`; collector lifts `level` into a severity label without config |
| **P2** | Trace phase 0: read `traceparent` from `_meta`, stamp `trace_id` into logs, forward the header upstream | with P0/P1 | a request carrying a traceparent produces a log line with the same trace_id, and the upstream HTTP request carries the header (integration test) |
| **P3** | Audit stream (spec below): `MCP_AUDIT=metadata\|full` | after P1 ships | dogfooding: a kagent conversation's tool calls reconstructed from the audit stream alone; docs (3 languages) state the layer boundary; default-off verified by the probe test |
| **P4** | Trace phase 1: real spans, OTLP export gated on `OTEL_EXPORTER_OTLP_ENDPOINT` | after P2; needs a collector to test against | spans visible in a local collector in the smoke rig; endpoint unset ⇒ bit-identical behavior to P2 |
| **P5** | Upstream joins the trace: ratatosk.io `/v1` accepts/propagates traceparent, web logs carry trace_id | app-repo work; after P2 | one trace id resolves across MCP log and web log for the same call |

Release mapping: P0–P2 bundle into the next tagged release (its headline:
MCP 2026-07-28 support). P3 and P4 ride whichever bump comes after. The
hosted-endpoint decision (post-launch, separate track) *depends on* P1
existing — operating a public endpoint without an error log is not on the
table — and makes P4 more valuable, but nothing here waits for it.

**Status (2026-07-31): P0–P3 are implemented on main; P0–P2 shipped in
v0.5.0.** P3 = `audit.go`, a receiving middleware observing `tools/call`
(`MCP_AUDIT=metadata|full`, chart `auditMode`, default off — the off path is
tested to emit zero bytes; `TestAuditWireRoundTrip` covers a real session).
Remaining: P4 (needs an OTLP backend to test against — hub asked), P5 beyond
phase 0 (web-internal propagation, bundled with app observability phase 2).

**Status (2026-07-30): P0, P1 and P2 are implemented on main, unreleased.**
P1 = `log.go` + classified logging at the `apiClient` choke points (levels as
specced; the marker probe and the client-mistake discipline are tests:
`TestLogInvariantMarkerProbe`, `TestClientMistakeIsNotError`). P2 =
`requestContext` reads `_meta["traceparent"]` (W3C-validated, malformed
dropped), stamps `tool`/`trace_id` on every log line via the context handler,
and forwards the header on upstream `/v1` calls (`TestTracePropagation`).
Chart knobs: `statelessHttp`, `logLevel`. Defaults: everything inert.

## P0 findings (2026-07-30, measured on go-sdk v1.7.0)

- **Dual revision is native.** The SDK ships all five revisions (2024-11-05 →
  2026-07-28). Wire-probed against our server: a legacy `initialize`
  handshake (2025-06-18 and 2024-11-05 shapes, i.e. what the kagent Go ADK
  sends) negotiates the client's own version and works in BOTH transport
  modes; the deprecated handshake caps at 2025-11-25 by SDK design.
- **2026-07-28 over HTTP requires `Stateless`.** A stateful server answers
  new-revision calls with an explicit 400 pointing at the option. Decision:
  `MCP_HTTP_STATELESS=1` env + chart value `statelessHttp` (default **off** —
  inert-by-default; existing installs keep their transport behavior
  bit-for-bit). stdio speaks every revision regardless.
- **Stateless mode measured**: legacy `initialize` still served (temporary
  session, no `Mcp-Session-Id`), handshake-free 2026-07-28 `tools/call`
  works (needs `Mcp-Protocol-Version` + `Mcp-Method` + `Mcp-Name` headers and
  `_meta` `io.modelcontextprotocol/{protocolVersion,clientCapabilities}`),
  `server/discover` returns all five `supportedVersions` with
  `cacheScope:"public"`, GET/DELETE answer 405.
- **`_meta` trace keys: the SDK does nothing natively.** v1.7.0 consults
  `_meta` only for its own `io.modelcontextprotocol/*` protocol keys; there
  is no traceparent/OTel handling anywhere in the request path (grep and
  wire-probe agree). P2 is therefore entirely ours: read `traceparent` from
  `_meta`, stamp, forward.

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

## Operational error log (P1)

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

## Audit log (P3)

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

## Trace integration (P2 / P4 / P5)

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
