# Changelog

Notable changes to ratatosk-mcp. Versions follow the container tag and the
Helm chart `appVersion`; see `docs/` for the release procedure.

## [Unreleased]

Changes are collected here and shipped together at the next version bump.

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
