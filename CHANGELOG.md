# Changelog

Notable changes to ratatosk-mcp. Versions follow the container tag and the
Helm chart `appVersion`; see `docs/` for the release procedure.

## [Unreleased]

### Changed

- A `check_stack` briefing shortens quotes to 120 characters with an ellipsis;
  `detail:"full"` and `get_release` still carry them verbatim. Measured over a
  five-component stack, quotes were 36% of the entry payload (median 90
  characters, longest 548), so this bounds the tail without reducing the
  typical entry to a fragment. The briefing is where a caller decides what to
  look at; the verbatim quote is one call away, where the decision is made.

## [0.9.0] — 2026-09-04

### Changed

- The agent prompt now says what a tool answer costs, not just whether it is
  useful. A measured run pulled 1.4M characters from one cluster-wide pod
  listing in `json` — larger than the model's whole context window — because
  the rules called that call useless without calling it expensive, and a
  self-hosted model re-processes every past tool answer on every later turn.
  The prompt now asks for the default (`wide`) listing, reserves full specs
  for a named resource, starts `check_stack` at `brief`, and passes an explicit
  `limit` to `list_changes`. Chart and kagent example only — no image change,
  so a `helm upgrade` from a checkout picks it up without a release.
- `check_stack` briefings carry a shorter tail: `other_changes` is capped at 25
  per component instead of 100, with the overflow still counted in
  `other_changes_omitted`. `action_required` and `check_config` are untouched
  and remain uncapped. A measured five-component briefing was 85,948 characters
  and this tail was most of it, for changes the server had already judged to
  need no decision. Asking callers to narrow with `severity_min` did not work:
  it went unused in 13 of 13 measured calls across two model fleets, so the
  default had to be the budget.
- `list_changes` returns 20 changes when no `limit` is given, down from the
  upstream default of 50. One measured default call was 104,463 characters.
  Paging is unchanged — a caller syncing a local copy raises `limit` and follows
  `next_since` as before.
- `check_stack` accepts the argument shapes models actually send: `components`
  JSON-encoded as a string, and `name` as an alias for `project`. Both were
  refused before, and a refusal is not cheap on the other side — a self-hosted
  model spent about two minutes per rejection regenerating arguments, 8 out of 8
  calls in one measured session. Rejections that remain now carry a minimal
  correct example instead of only echoing what arrived.

## [0.8.0] — 2026-08-20

### Added

- HTTP mode classifies each caller as `self` or `external` against
  `MCP_SELF_CALLERS` and forwards only that class upstream
  (`X-Ratatosk-Caller`). A hosted deployment reaches its upstream over an
  internal network with no forwarding header, so upstream metrics counted every
  hosted user as the operator's own traffic and adoption read as zero. The
  address is compared in-process and discarded — it is neither logged nor sent.
  Unset (the default, and every self-hosted install) classifies all callers as
  external, which costs nothing and changes no behavior.

## [0.7.5] — 2026-08-16

### Security (hardening)

- HTTP server now sets explicit timeouts (`ReadHeaderTimeout`, `ReadTimeout`,
  `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`) instead of the unbounded
  zero-value `http.Server` — closes a slowloris vector on the public endpoint
  where a client dribbling headers held a goroutine below the rate limiter.
- `check_stack` now caps `components` at 100. An unbounded list fanned out to
  full upstream paging per component, amplifying one rate-limited call into
  thousands of upstream requests.
- Stateful HTTP mode now expires idle sessions after 30 minutes. The SDK never
  closes them when the timeout is zero, so a long-running install that
  reconnects often grew memory with no attacker involved. Stateless mode — what
  the 2026-07-28 revision requires — allocates no session and is unaffected.
- Upstream transport failures no longer return the request URL and resolved
  address to the caller. The class is reported (`upstream request failed
  (connection_refused)`); the detail stays in the server log.

## [0.7.4] — 2026-08-10

### A version whose line does not exist is a dropped prefix, not an empty answer

Agents read versions off image tags, and an image tag rarely carries the
release-line prefix. knative publishes `knative-v1.12.0` while its images say
`v1.12.0`, so every one of that project's releases lands on the other side of
0.7.3's line gate and the briefing reads as "nothing to do".

This is not confined to projects with several lines. A project with exactly
one line still has the problem when that line is a prefixed one — across the
tracked corpus five projects publish no main-line release at all
(cloudevents, flatcar, knative, linkerd, openfeature).

`check_stack` now says so: when the running version's line does not exist for
that project, `note` names the lines that do and shows a real tag to copy the
shape from. Projects whose main line genuinely exists — containerd, wasmcloud
— are unaffected, so the warning does not cry wolf.

## [0.7.3] — 2026-08-10

### A repository can publish more than one thing, and versions across them have no order

containerd tags its Go API module `api/v1.11.1` beside the runtime's own
`v2.2.5`. Flatcar ships `lts-4081.3.9` and `stable-4593.2.4` as parallel
channels. OpenFeature publishes `core/`, `flagd/` and `flagd-proxy/` from one
repository. Each of those is a separate release line, and asking which of two
lines is "newer" is not a hard question — it is a meaningless one.

`check_stack` was asking it anyway. An operator on containerd v1.7.28 was
offered `api/v1.11.0` as part of their upgrade path: a Go library, presented
as work to do on their container runtime. It parsed, so nothing complained.

- **Only the line your running version belongs to is compared.** Everything
  else is dropped before a key is even computed, and `note` says how many and
  why. The line is read from the tag — the maintainer already wrote it there —
  rather than kept in a hand-maintained prefix list. That list was itself the
  bug this morning: `stable-` was on it and `lts-` was not, so Flatcar's entire
  LTS train parsed to nothing and vanished from every range query.
- **Projects that prefix every tag the same way are unaffected.** knative-v…,
  edge-… and the like resolve to a single line for the whole project, so a
  same-line comparison behaves exactly as before. Across the tracked corpus,
  five projects out of seventy-five have more than one line.
- **Pass the tag as published, prefix included** — `flagd/v0.16.1`,
  `lts-4081.3.9`. A bare `v0.16.1` is read as the main line and will not find
  the flagd releases.

This is the counterpart to 0.7.1's branch awareness, and deliberately not an
extension of it. A branch is an ordered position within one line, which is why
"already fixed at or below" means something there. Lines have no order at all,
so they are excluded from candidacy rather than compared and filtered.

## [0.7.2] — 2026-08-10

### The per-caller limit now actually engages behind a CDN

0.7.1 shipped the limiter with the bucket keyed on `X-Forwarded-For` first.
Behind Cloudflare, with no `trusted_proxies` configured, the reverse proxy
sees a Cloudflare *edge* as its peer and writes that into the header — and
edges rotate, so one caller collected a fresh bucket every few requests and
the limit never fired. Sixty-two consecutive calls through the hosted
endpoint all returned 200 where the sixty-first should have been a `429`.

`CF-Connecting-IP` now comes first, then `X-Forwarded-For`, `X-Real-IP`, and
the peer address. Cloudflare writes that header itself and it cannot be
forged from outside. Whatever proxy sits in front must set one of these
**and strip whatever the client supplied**: a limiter keyed on a
caller-controlled value is not a limiter. The documentation now says so.

## [0.7.1] — 2026-08-10

### A briefing that knows which branch you are on

An operator running containerd 2.2.5 was told to act on two CVEs their own
branch had already closed — CVE-2026-46680 in v2.2.4, CVE-2026-47262 in
v2.2.5 itself. Both were real fixes; both had been backported across every
supported branch on the same day; and `check_stack`, which reads only
releases *newer* than the running version, saw the v2.3.x occurrences and
nothing else. It is the same failure `get_matter` was built to prevent,
pointing the other way: not crediting an install with fixes it never
received, but demanding work it finished a release ago.

- **A matter already fixed at or below the running version, on the running
  branch, is no longer reported.** The count and the reason go into `note`,
  so nothing disappears without saying so. Branch equality is what makes the
  rule safe: a backport to v2.1.9 proves nothing about v2.2.4, which was cut
  from a different branch and may never have received it — only the running
  version's own branch is evidence.
- **`same_matter_also_addressed_in` now names every release on record that
  carried the matter**, not only those inside the returned window. The window
  starts above the running version, which meant the branch an operator most
  needed to see was the one systematically missing from it.

### Rate limits that hold up when several people share an endpoint

The hosted endpoint reaches the upstream API over an internal network,
deliberately bypassing the CDN and reverse proxy so that `/v1` paths — which
carry project slugs and running versions — never reach an access log. The
cost of that design turned out to be steeper than intended: with no
forwarding header surviving the hop, the upstream could not tell hosted
callers apart, so **every hosted user in the world shared one bucket of 60
requests a minute**. A single fifteen-component `check_stack` ate a quarter
of it, and what a caller saw when the bucket ran dry was not an error but a
briefing with `fetch failed` quietly filling in for components.

- **`MCP_RATE_LIMIT_PER_MIN` (chart: `rateLimitPerMin`), off by default.**
  When set, each caller address gets its own budget of N tool calls a minute;
  over it, `429` with `Retry-After`. This lives here rather than upstream on
  purpose. Forwarding the caller address to the API would have fixed the
  fairness and broken the separation that makes the hosted endpoint what it
  is: the web tier currently *cannot* correlate who asked with what was
  asked, because it never receives the who. A structural guarantee like that
  survives a future careless log line; a policy one does not. This process
  already receives the address from the reverse proxy and is the layer whose
  job is to front many callers, so it is where fair-sharing belongs. The
  address is a bucket key and nothing else — never logged, never sent
  upstream, dropped when its window expires.
- **Off is the default because the same binary is what people self-host**,
  and a self-hosted server is single-tenant: limiting its one caller would
  only get in the way.
- **The upstream per-IP limit is now 1200 requests/minute** (was 60). One
  tool call is not one request — `check_stack` costs one per component — and
  the old figure was tight enough that ordinary stack questions brushed it.
  Documentation across both languages now states the two limits in their own
  units instead of implying they are the same number.

## [0.7.0] — 2026-08-10

### The change model, end to end

**Upgrading:** two tools were renamed. `facts_by_entity` is now
`changes_by_entity` and `list_facts` is now `list_changes`; a client that
names the old ones will not find them. Allow-lists that enumerate tools (the
kagent `toolNames` field, for one) need the new names plus `get_matter`.

Ratatosk split its pipeline: what can be decided mechanically is now done
mechanically, and the model is asked only for what needs judgment. The unit
that comes out the other side is no longer a `fact` with a severity — it is a
**change**, described by three independent axes: `family` (security /
breaking / deprecated), `bucket` (action / check / plan) and `applies_if`
(the condition, structured where the server could). This release moves every
part of the server onto that model.

- **Tools are now seven.** `facts_by_entity` → `changes_by_entity`,
  `list_facts` → `list_changes`, and `get_matter` is new: one matter across
  every branch that fixed it. The same roll-up lands on several release
  branches carrying different advisory sets, and only the full list shows
  that — the newest entry is not a safe stand-in for yours.
- **`check_stack` splits on `bucket`, not on severity.** That was already the
  behaviour; the briefing's own `hint` still described it as "critical/high",
  which is what an agent reads to decide what to do. It now says what the code
  does, and states that `severity` in a briefing is derived by this server
  (highest advisory, else a default from family and bucket) — good for ranking
  urgency, never for deciding who is affected.
- **Retired response fields are gone from everything that describes them**:
  `fixed_in`, `coverage`, `assessment`, `group_severity`, `mandatory`,
  `applies_if_target` (now plural). An entry's `version` is the release
  carrying the fix; `same_matter_also_addressed_in` names the other branches.
- **kagent manifests** (chart template and example) allow-listed two tools
  that no longer exist and therefore hid three that do; their system prompt
  still instructed the model about retired fields. Both are corrected. The
  triage procedure itself — the measured-run rules about reclassification,
  unresolved conditions and roster coverage — is unchanged.
- **Documentation** is rewritten from live responses: the tools reference in
  three languages, plus README, install guide and chart/example READMEs.

## [0.6.2] — 2026-07-31

### Tool annotations, and a cursor that says how it ends

- Every tool now carries a `title` and `readOnlyHint: true` — machine-readable
  metadata saying what a human can already infer: these tools only read.
  Clients that understand the hint can label the tools and skip write-style
  confirmation prompts.
- `list_facts` described its own pagination as "until it stops growing";
  the actual protocol is `next_since: null`. The description now says so,
  matching the docs and the web API's own wording.

## [0.6.1] — 2026-07-31

### Tool descriptions read true from the hosted endpoint

- ratatosk.io now runs this server as a public hosted endpoint
  (`https://ratatosk.io/mcp`), and two baked-in strings were written when
  "the process" could only mean the caller's own. `check_stack` no longer
  says versions "never leave this process" — it scopes the claim per mode:
  comparison inside the server process; self-hosted, versions never leave
  your infrastructure; hosted, they transit server memory only and are not
  logged. The server instructions now state the upstream rate limit per
  caller, noting a hosted endpoint shares one caller budget.

### Audit records carry the transport session id

- Fifth-campaign dogfooding reconstructed a full conversation from the audit
  stream alone, with one gap: every trace_id sat empty (the kagent Go ADK
  does not send `_meta` traceparent yet), so attribution relied on time
  windows — impossible with two concurrent conversations. Records now carry
  `session_id`, the transport session identifier: honest about its layer
  (a transport fact, not a conversation id) and enough to separate
  concurrent stateful callers until trace context arrives from upstream.

## [0.6.0] — 2026-07-30

### Audit stream (P3) — who did what, inside your own perimeter

- `MCP_AUDIT=metadata` (chart `auditMode`) emits one `event:"audit"` JSON
  line per tool call: tool, outcome (`ok`/`tool_error`/`error`), caller
  `clientInfo`, transport, argument *names*, and `trace_id` when the caller
  sent trace context. `full` adds argument values. Default **off**, and the
  off path is regression-tested to emit zero bytes.
- The stream lands in the operator's own collectors — the same property
  that keeps versions private (data stays home) is what makes calls
  auditable at home. Retention and tamper-evidence stay the log platform's
  job; route on `event=audit`.
- Layer honesty, in the docs (en/ko/ja): this server has no authentication,
  so a record attests which *client* called which tool with which
  arguments — which *human* prompted the agent is the agent layer's
  knowledge, and an MCP-layer record cannot manufacture it.

### Agent definition rev 15 — existence mandated, observability separated

- The fourth campaign's edge case: cluster_core told agents to check etcd,
  and where etcd lives outside the k8s API nine of twenty runs passed a
  guessed version (all confessed, all caught by the chain — no harm, but
  structural tension). `/v1/projects` cluster_core entries now carry a
  `visibility` hint stating the component's observation properties — never
  a distribution list; "true even for tomorrow's distro" is the admission
  test. The prompt answers the hint in one move: unreadable-by-design is
  this cluster being normal — report under "Could not check" citing the
  hint, never guess. `list_projects` description documents the field; one
  new guard clause in check-agent-prompt.sh.

## [0.5.1] — 2026-07-30

### Agent definition rev 14 — what-to-check becomes roster data

- The third campaign confirmed prompt attention is zero-sum: tightening
  envoy enumeration halved coredns coverage (90→45%), mirroring rev 12's
  opposite-direction seesaw. The cure that worked for name mapping
  (`image_aliases`) now applies to coverage: `/v1/projects` marks the
  cluster substrate with `cluster_core:true`, and the agent prompt reduces
  the what-to-check question to one data-driven line — every marked project
  present in the cluster goes into check_stack. The `list_projects` tool
  description documents both fields.
- The count-preservation rule moved INTO the response template (section 1
  is defined as "every action_required fact, one entry each") instead of
  competing with the template as a separate rule — the third campaign
  showed some runs dropping the 4-section format as enumeration rules
  grew louder. One clause guards the roster line in check-agent-prompt.sh.

## [0.5.0] — 2026-07-30

### MCP 2026-07-28 revision support (go-sdk v1.7.0)

- **SDK bump v1.6.1 → v1.7.0.** All five protocol revisions (2024-11-05 →
  2026-07-28) negotiate natively; the legacy `initialize` handshake keeps
  working through its deprecation window (wire-probed both transports).
- **Opt-in stateless HTTP** (`MCP_HTTP_STATELESS=1`, chart value
  `statelessHttp`): no `Mcp-Session-Id` round-trip, and — per SDK design —
  the only HTTP mode that speaks the 2026-07-28 revision. Default stays
  stateful, so existing installs see no transport change; stdio speaks every
  revision regardless.

### Operational error log (P1) + trace propagation (P2)

- The request path is no longer silent when sick: one-line JSON via `log/slog`
  to stderr (`service:"mcp"` house schema — collectors lift `level` into a
  severity label with no config). Levels are disciplined: upstream 5xx /
  timeout / connection refused = ERROR, 429 = WARN, **a client mistake is
  never an ERROR** (unknown slug or bad params surface at DEBUG only —
  `MCP_LOG=debug`, chart `logLevel`). Error fields are *reconstructed*
  (endpoint pattern, status, error kind), never copied — upstream URLs embed
  running versions, and the argument-free invariant now has a CI probe test
  watching text and JSON fields alike.
- W3C trace context, phase 0: a `traceparent` sent in `_meta` (MCP 2026-07-28
  convention) is validated, stamped as `trace_id` on every log line of that
  call, and forwarded as the standard header on the upstream `/v1` request —
  conversation → agent → MCP → upstream joins into one trace with no OTel
  SDK and no exporter. Malformed values are dropped, not forwarded.

### check_stack: self-nullifying target_version guard — from the 20-run M2 campaign

- A `target_version` at or below the running version defines the empty range
  `(running, target]`; 5 of 20 measured agent runs sent exactly
  running-as-target and read the guaranteed zero facts as "no issues". Such a
  target is now **ignored, with a note** that explains the empty range and
  teaches the intended use (version = running, target = destination). The
  schema description says the same. Replaying the self-nullifying call now
  returns every action_required fact (regression-tested).

### Agent definition rev 13 — bucket reclassification banned both ways

- The M2 (3.5-flash-lite) campaign surfaced the inverse of the known
  promotion failure: **silent demotion** — the server returned mandatory
  action_required facts and 7 of 15 answers still said "Applies: none". The
  placement rule now bans reclassification in both directions and adds a
  count check (N action_required ⇒ exactly N entries under "Applies");
  two new clauses guard it in `scripts/check-agent-prompt.sh`.

### Docs

- Install docs (en/ko/ja): on free model tiers prefer the flash-lite line —
  measured 2026-07, every full-flash free tier sits at 5 RPM (agent-inviable).

### Agent definition (Helm chart + kagent example) — from the 60-run evaluation

A controlled campaign (3 prompts × 20 runs, fixed rubric, hub-run against a
live cluster) put numbers on the failure modes and the prompt now answers
them:

- **Aliases are spelled out.** `cilium-envoy` was visible in all 60 runs and
  mapped to envoy in 17 — the cluster's only real criticals rode on that one
  mapping. The prompt points at `/v1/projects`' new `image_aliases` field.
- **Bucket placement is the default, promotion needs evidence.** ~30% of runs
  promoted conditional facts or the coverage note into "applies" regardless of
  prompt wording; the best-scoring run mirrored the server's buckets exactly.
  Now: action_required reports as applying, check_config moves only together
  with the configuration line that was actually read, the low-version note is
  never a finding.
- **The judgment rule is restated at the response boundary.** Repeating it in
  the user turn tripled config reads and envoy detection in the campaign
  (P3 effect); the same wording now sits directly before the response-format
  rules, with the note that claiming to have read a config does not count —
  runs were observed narrating reads that never happened.
- **Resource names come from listings.** Runs invented pod names
  (`kube-apiserver-minikube`) and burned turns on 404s.
- Install docs state the model floor: ~10 RPM, because one run is 6+ internal
  calls and the kagent Go ADK does not retry 429s.

### Added

- **`check_stack` flags self-confessed inferred versions.** Across 60 runs,
  every version whose `version_source` described an inference ("inferred from
  typical deployment versioning", "guessed from k8s 1.36 stack") was a
  hallucination — 8 of 8 — and every version citing a concrete read was
  correct (197 of 197). The model does not forge sources; it confesses. When
  a `version_source` reads as inference rather than a live read, the
  component's `note` now says so, in the same channel as the low-version
  warning. Detection is a vocabulary match on the caller's own words — the
  server still sees no cluster.

## [0.4.1] — 2026-07-29

Everything below came out of one week of dogfooding: a kagent agent ran
against a live cluster, answered wrongly three times in three different
ways, and each failure became a structural fix rather than a patch note.

### Changed

- **`check_stack` no longer calls a conditional finding an action.**
  `action_required` used to be chosen by severity alone, so a high-severity
  fact that only bites installs using a particular feature read as "upgrade
  now" to every caller. Critical/high facts now split: `action_required` holds
  what applies to every install of that version, and the new `check_config`
  holds what applies only if its `applies_if` condition holds — the caller
  resolves that against the running configuration before recommending
  anything. `other_facts` is unchanged.
- **Condition phrases are written for a reader.** The stored `(verb, kind,
  name)` triple was joined raw, which put the storage enum on display —
  `configures config_field per-upstream read timeout`. It now renders as
  `configures the per-upstream read timeout setting`; unknown kinds fall
  through to the bare name.

### Added

- **`fixed_in` (plus `removed_in` / `deprecated_in`) on every brief fact.**
  The REST API has always carried it and this server dropped it. It is the
  minimum version that closes an issue, which is what turns an unmet condition
  into useful advice: *before you enable X, be on at least this version.*
- **`applies_if_target`** — `{kind, name}` when the server stored the condition
  structurally, so an agent can look the thing up in a live configuration
  instead of parsing the sentence.

- **`version_source` on each component, echoed back.** Optional, and explicitly
  not a check: this server cannot see the caller's environment, so it cannot
  tell a real citation from an invented one. It exists so the claim is
  machine-readable afterwards — a comparison script can hold the reported
  version against the cluster without a person reading the answer.
- **A running version below every release on record is flagged in `note`.**
  The only cross-check available to a server that sees no configuration. It
  catches a genuinely ancient install (the briefing is then partial, not clean)
  and a version that was never read off a live resource at all.

### Agent definition (Helm chart + kagent example)

- Discovery covers the **whole cluster**: `all_namespaces`, the control plane
  (static pods), and node-level versions (kubelet, container runtime).
  A survey that silently omits Kubernetes itself is not a survey.
- Triage is condition-first: read the configuration, then classify each fact as
  *applies* / *does not apply* / *cannot tell*, and report an unmet condition
  forward as a precondition rather than dropping it.
- Response format has explicit sections, replacing "lead with severity" —
  severity describes the damage if a condition holds, not the odds it holds.
- **Where a version comes from is spelled out**, after a live run invented one.
  A pod listing carries no images, so a whole-cluster pod survey left the agent
  with nothing for cilium and it filled the gap with a plausible number
  (1.16.0 against an actual 1.19.5) — every conclusion downstream was then
  confidently wrong. The prompt now names the image-bearing listings, the
  static-pod and node paths, and forbids sending check_stack any version that
  was not read off a live resource.
- **The forward-looking form has to be earned.** Filing every condition under
  "before you enable this" without opening a ConfigMap skips the same judgment
  as recommending the upgrade outright.
- **An unverified condition is not an action.** With the rules above in place a
  live run still read cilium-config, found no ClusterMesh, and filed the
  ClusterMesh fact under "act now" anyway — severity broke the tie when the
  condition could not be resolved. Unresolved now belongs under "could not
  check", and the prompt says where to look for a condition: an annotation sits
  on the object it annotates, not in a ConfigMap. Searching the wrong object is
  "cannot tell", not "does not apply".
- **The response speaks declaratively — no-warranty made explicit.** The
  section names were instructions ("Act now", "before you enable this, upgrade
  first"), and a live run duly told an operator to upgrade over a fact that did
  not apply. Ratatosk provides facts without warranty; an agent built on it has
  no business issuing orders. Sections are now findings ("Applies to this
  cluster", "Conditions not met today"), the branch outcomes report rather than
  direct, and the prompt states the principle outright: the decision to upgrade
  stays with the operator. The `check_stack` hint says the same of its buckets —
  a data classification, not a recommendation.
- **CI asserts the prompt still carries the rules** (`scripts/check-agent-prompt.sh`).
  Every prompt regression so far was a deletion — a rule vanished while
  rewording something near it, and the next live run answered confidently and
  wrongly. The clause list doubles as the record of failures already paid for,
  and keeps the chart template and the kagent example from drifting apart.

## [0.4.0] — 2026-07-27

See the GitHub release notes.
