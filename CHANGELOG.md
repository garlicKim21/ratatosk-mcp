# Changelog

Notable changes to ratatosk-mcp. Versions follow the container tag and the
Helm chart `appVersion`; see `docs/` for the release procedure.

## [Unreleased]

Changes are collected here and shipped together at the next version bump.

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
