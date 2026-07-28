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

### Agent definition (Helm chart + kagent example)

- Discovery covers the **whole cluster**: `all_namespaces`, the control plane
  (static pods), and node-level versions (kubelet, container runtime).
  A survey that silently omits Kubernetes itself is not a survey.
- Triage is condition-first: read the configuration, then classify each fact as
  *applies* / *does not apply* / *cannot tell*, and report an unmet condition
  forward as a precondition rather than dropping it.
- Response format has explicit sections, replacing "lead with severity" —
  severity describes the damage if a condition holds, not the odds it holds.

## [0.4.0] — 2026-07-27

See the GitHub release notes.
