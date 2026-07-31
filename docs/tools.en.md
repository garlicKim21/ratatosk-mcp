# Tools reference

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

This page is the detailed reference for the six tools ratatosk-mcp provides —
[`check_stack`](#check_stack), [`list_projects`](#list_projects),
[`list_releases`](#list_releases), [`get_release`](#get_release),
[`facts_by_entity`](#facts_by_entity), and [`list_facts`](#list_facts). For
each tool you will find when to reach for it, every parameter, a real call
and its response, and example questions you can put to an agent. For connecting the server itself
(hosted endpoint, Docker, Helm, kagent), see the
[installation guide](install.en.md).

Two terms first. A **fact** is one extracted change from an official release
note — a security fix, a removal, a deprecation, a changed default — tied to
the exact identifiers it touches (a CVE id, a flag, a CRD, a config field),
with a verbatim quote from the note as evidence. Every fact carries a
**severity** from `info` to `critical`.

> **Measured as of 2026-07-31**: every call and response in this document was
> sent to the hosted endpoint `https://ratatosk.io/mcp` (server
> 0.6.2) and is reproduced from what came back. The stack used in the
> `check_stack` example — the five components and their `version_source`
> strings — originates from a real cluster (anonymized). Release data accrues
> hourly, so the numbers and lists will differ if you call today — the shape
> of the responses will not. Long responses are trimmed with `…` for space.

## From a question to a tool call — a worked example

Suppose you ask an agent:

> "Please check the state of our cluster."

This phrasing has worked well in real use (translated here from the original
question). In one real run by an agent that can read the cluster directly —
for example, kagent with both ratatosk-mcp and cluster read tools attached —
the tools were called in this order:

```
list_projects
k8s_get_resources  (pod, all_namespaces)
k8s_get_resources  (daemonset, all_namespaces)
k8s_get_resources  (deployment, all_namespaces)
k8s_get_resources  (node)
check_stack        (components ×5)
get_release        (cilium, v1.20.0)               ← action_required drill-down
k8s_get_resources  (customresourcedefinition)
k8s_get_resources  (ciliumnodeconfig, all_namespaces)
k8s_get_resources  (configmap, all_namespaces)
k8s_get_resource_yaml (kube-system/cilium-config)  ← resolving applies_if
```

A note on the names: `k8s_*` are cluster read tools from kagent-tools (a
separate MCP server); the ratatosk tools in this sequence are
`list_projects`, `check_stack`, and `get_release`.

That path has three stages.

**Stage 1 — pin down the roster with `list_projects`.** This maps the names
seen in the cluster to the canonical slugs the other tools take, and the
`cluster_core:true` marker (the cluster substrate — control plane, runtime,
DNS, CNI, and so on) selects the candidates worth checking.

**Stage 2 — read the versions off live resources.** The `k8s_*` tools pull
the running versions and their provenance from pods, daemonsets, and node
status. The result is the example call in the check_stack section below —
the five components (kubernetes/containerd/cilium/envoy/coredns) with their
`version_source` strings.

**Stage 3 — run the `check_stack` briefing, then resolve the conditions.** The
briefing surfaces cilium's two `action_required` entries (the docker
libnetwork plugin removal and the proxylib/Kafka policy removal in v1.20.0),
and `get_release(cilium, v1.20.0)` drills down to the source evidence. The
`check_config` condition "uses the v2alpha1 CiliumNodeConfig CRD" is then
judged applies/does-not-apply by actually reading the CRD list and
`kube-system/cilium-config`. In this run, the tool was used exactly as
designed: take the briefing, then check each condition against the live
configuration.

The response is reproduced in the check_stack section below and in the next
section, "How to read a check_stack briefing".

To push the agent harder on conditions, state the rule in the question
itself — this phrasing has worked well too:

> "Read the versions of the components actually running in the cluster —
> control plane, node runtime, networking (the CNI and its companions), and
> DNS — from the live resources, and check them against ratatosk's release
> information for issues that apply to us right now. For conditional issues,
> judge applies/does-not-apply only where you have actually read the
> relevant configuration, and classify any condition you could not verify as
> 'could not check'."

One anti-pattern to know: always pair a question that asks for broad
enumeration with a rule for unverified items ("anything unverified goes to
could-not-check"). Pressure alone makes the model produce plausible-looking
values instead of reading the cluster.

## How to read a check_stack briefing

The default `check_stack` response (`detail:"brief"`) has the same structure
for every component. Keep the measured response in the check_stack section
below at hand while reading.

1. **`summary` — the counts.** Facts newer than the running version
   (`new_facts`), distinct issues after merging (`distinct_issues`), facts
   that apply unconditionally (`mandatory`), and the breakdown by severity
   and by type. The total number of facts scanned is `facts_scanned`. The
   envoy component in the measured response is a good example: 75 new facts
   merge down to 40 distinct issues. Note that `distinct_issues` is counted
   before the quote-sharing merge (rule 5 below), so it can exceed the
   number of entries across the three lists.
2. **`action_required` — critical/high that applies to every install passing
   through this version range.** Each entry
   carries the verbatim release-note quote (`quote`) and its CVE/advisory
   ids (`ids`). In the measured response, envoy has five entries here
   (including fact 211, the critical TLS SAN auth bypass) and cilium two
   (the libnetwork and proxylib removals).
3. **`check_config` — critical/high that needs a config check.** These apply
   only if the `applies_if` condition holds. Until you have verified the
   condition against the actual configuration, the entry is not an action —
   and an unmet condition is not a reason to upgrade. Read `fixed_in` in
   that case as a precondition: the minimum version you must be running
   before you enable what `applies_if` describes. When the server stored the condition's
   subject structurally, `applies_if_target` (kind and name) names the thing
   to look for. Cilium's fact 614 in the measured response is an example of
   that shape —
   `applies_if` "uses the cilium.io/v2alpha1 CiliumNodeConfig CRD" with
   `applies_if_target` `{ "kind": "crd", … }`, which is exactly why the
   agent in the scenario above went to read the CRD list. On the envoy side,
   eleven entries sit here, including fact 215 (critical, "if you proxy
   HTTP/3 to HTTP/1 backends").
4. **`other_facts` — everything else, one line each.** Entries at medium and
   below appear as one line per fact. The coredns component in the
   measured response — five facts, all medium or below — appears only in
   this list.
5. **The merge rules.** Facts sharing one quoted sentence are merged
   into a single entry with their ids listed together (`applies_if_any` when
   their conditions differ) — envoy's fact 466 in the measured response is
   that shape: five CVE ids in one entry, with the three differing
   conditions listed in `applies_if_any`. The same advisory fixed on several release
   branches is also one entry — that is the
   `same_issue_also_addressed_in: ["v1.37.5", "v1.38.3"]` actually attached
   to envoy's fact 211 in the measured response. The severity shown is the
   advisory group's maximum.
6. **`note` — per-component state markers.** The measured response contains
   both kinds. On kubernetes and containerd, "tracked by ratatosk; no facts
   on record — releases so far were routine" is the tracked-but-quiet state,
   distinct from not-tracked (`tracked:false` — where an absence of facts is
   not safety). On cilium and coredns, "older than every release on record …
   treat it as partial, and re-check" warns that the running version is
   older than the oldest release on record — the server cannot see your
   environment, so this flag is the only cross-check on a version claim, and
   a signal to re-read the version off a live resource.
7. **`hint` and `privacy`.** The `hint` states that the briefing is a data
   classification, not a recommendation, and points to the next tools
   (`detail:"full"` for everything verbatim; `get_release` and
   `facts_by_entity` to drill down). The `privacy` line records how far your
   versions traveled in this call.

## check_stack

Takes the list of component versions you are running and returns, per
component, the facts on its upgrade path — the releases newer than the
running version. It is the first tool to reach for when the question is
"anything to do before we upgrade?". Because one call covers several
projects, it answers stack-wide questions better than calling the other
tools project by project. The version
comparison happens inside the server process, so if you self-host, running
versions never leave your infrastructure. To go deep on one release, switch
to `get_release`; to trace one CVE or flag, switch to `facts_by_entity`.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `components` | yes | array | — | The running stack to check. Item shape below |
| `components[].project` | yes | string | — | Project slug (e.g. `envoy`). The canonical source is `slug` in `list_projects` |
| `components[].version` | yes | string | — | The version currently running (e.g. `v1.36.8`) |
| `components[].target_version` | no | string | *(none = everything up to the newest)* | Upgrade destination, strictly above the running version: only facts with `running < version <= target` are returned. A target at or below the running version would make the range empty, so it is ignored with a `note` |
| `components[].version_source` | no | string | *(none)* | Where the running version was read (e.g. `daemonset/cilium image tag`, or that the user stated it). Echoed back verbatim so the claim can be audited later. The server cannot see your environment and cannot verify a version — the `note` marker described in "How to read a check_stack briefing" is the only cross-check available |
| `detail` | no | string | `brief` | `brief`: summary + critical/high + one line each for the rest. `full`: every fact verbatim — capped at 50 per component, with the overflow flagged as `relevant_facts_omitted`; narrow with `severity_min` or `target_version` |
| `severity_min` | no | string | *(none = all)* | Only facts at or above this severity: `info`, `low`, `medium`, `high`, `critical` |

### Example call

This is the stack from the scenario above, sent to the hosted endpoint as
shown. The JSON below is the tool's `arguments` object only — the JSON-RPC
envelope it travels in is covered in the
[installation guide](install.en.md):

```json
{
  "components": [
    { "project": "kubernetes", "version": "v1.36.1", "version_source": "kubectl version (server)" },
    { "project": "containerd", "version": "2.2.4",   "version_source": "node status containerRuntimeVersion" },
    { "project": "cilium",     "version": "v1.19.4", "version_source": "daemonset/cilium image tag" },
    { "project": "envoy",      "version": "v1.36.7", "version_source": "daemonset/cilium-envoy image tag" },
    { "project": "coredns",    "version": "v1.14.1", "version_source": "deployment/coredns image tag" }
  ]
}
```

### Measured response (trimmed)

```json
{
  "components": [
    {
      "project": "kubernetes",
      "running_version": "v1.36.1",
      "version_source": "kubectl version (server)",
      "tracked": true,
      "note": "tracked by ratatosk; no facts on record — releases so far were routine",
      "facts_scanned": 0,
      "summary": { "new_facts": 0, "distinct_issues": 0, "mandatory": 0, "by_severity": {}, "by_type": {} }
    },
    {
      "project": "containerd",
      "running_version": "2.2.4",
      "version_source": "node status containerRuntimeVersion",
      "tracked": true,
      "note": "tracked by ratatosk; no facts on record — releases so far were routine",
      "facts_scanned": 0,
      "summary": { … }
    },
    {
      "project": "cilium",
      "running_version": "v1.19.4",
      "version_source": "daemonset/cilium image tag",
      "note": "running version v1.19.4 is older than every release on record (earliest with facts: v1.19.6) — this covers the reviewed window only, so treat it as partial, and re-check that the running version was read off a live resource",
      "facts_scanned": 24,
      "summary": {
        "new_facts": 24, "distinct_issues": 24, "mandatory": 19,
        "by_severity": { "high": 4, "medium": 12, "low": 8 },
        "by_type": { "capability_removed": 12, "capability_deprecated": 5, "behavior_changed": 3, … }
      },
      "action_required": [
        {
          "fact_id": 612, "version": "v1.20.0", "fact_type": "capability_removed",
          "severity": "high", "mandatory": true, "removed_in": "v1.20.0",
          "quote": "As previously announced, docker libnetwork plugin as been sunset and is no longer available."
        },
        …1 entry omitted…
      ],
      "check_config": [
        …1 entry omitted…
        {
          "fact_id": 614, "version": "v1.20.0", "fact_type": "api_version_changed",
          "severity": "high", "mandatory": true,
          "applies_if": "uses the cilium.io/v2alpha1 CiliumNodeConfig CRD",
          "applies_if_target": { "kind": "crd", "name": "cilium.io/v2alpha1 CiliumNodeConfig" },
          "removed_in": "v1.20.0", "deprecated_in": "v1.16",
          "quote": "Remove deprecated `v2alpha1` `CiliumNodeConfig` API that was promoted to `v2` in cilium 1.16."
        }
      ],
      "other_facts": [ …20 entries omitted… ]
    },
    {
      "project": "envoy",
      "running_version": "v1.36.7",
      "version_source": "daemonset/cilium-envoy image tag",
      "facts_scanned": 90,
      "summary": {
        "new_facts": 75, "distinct_issues": 40, "mandatory": 38,
        "by_severity": { "critical": 3, "high": 13, "medium": 22, "low": 2 },
        "by_type": { "security_fix": 30, "behavior_changed": 3, "default_changed": 3, … }
      },
      "action_required": [
        …1 entry omitted…
        {
          "fact_id": 211, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true, "fixed_in": "v1.36.9",
          "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass",
          "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2 entries omitted…
        {
          "fact_id": 347, "version": "v1.37.5", "fact_type": "security_fix",
          "severity": "high", "mandatory": true, "fixed_in": "v1.37.5",
          "quote": "CVE-2026-47220: REQUESTED_SERVER_NAME crash",
          "ids": [ "CVE-2026-47220", "GHSA-j9wh-4qfm-wf2v" ],
          "same_issue_also_addressed_in": [ "v1.38.3" ]
        }
      ],
      "check_config": [
        …8 entries omitted…
        {
          "fact_id": 215, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true,
          "applies_if": "if you proxy HTTP/3 to HTTP/1 backends",
          "fixed_in": "v1.36.9",
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "ids": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2 entries omitted…
      ],
      "other_facts": [
        …7 entries omitted…
        {
          "fact_id": 466, "version": "v1.39.0", "fact_type": "security_fix",
          "severity": "medium", "mandatory": true,
          "applies_if_any": [
            "uses the ext_authz extension",
            "uses the ext_proc extension",
            "uses the OAuth2 extension"
          ],
          "applies_if_target": { "kind": "extension", "name": "ext_authz" },
          "fixed_in": "v1.39.0",
          "quote": "Security fixes were added for ext_authz (**CVE-2026-47205**), ext_proc (**CVE-2026-47207**), gRPC stats (**CVE-2026-47204**), internal redirects (**CVE-2026-47221**), and OAuth2 lifecycle handling (**CVE-2026-48090**).",
          "ids": [ "CVE-2026-47205", "CVE-2026-47207", "CVE-2026-47204", "CVE-2026-47221", "CVE-2026-48090" ]
        },
        …6 entries omitted…
      ]
    },
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "note": "running version v1.14.1 is older than every release on record (earliest with facts: v1.14.5) — …",
      "facts_scanned": 5,
      "summary": {
        "new_facts": 5, "distinct_issues": 5, "mandatory": 2,
        "by_severity": { "medium": 2, "low": 3 }, "by_type": { "behavior_changed": 3, "default_changed": 2 }
      },
      "other_facts": [
        { "fact_id": 432, "version": "v1.14.5", "fact_type": "default_changed", "severity": "medium",
          "mandatory": true, "fixed_in": "v1.14.5", "quote": "core: Use Go TLS defaults" },
        …4 entries omitted…
      ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation …): action_required = critical/high that applies to every install of this version. check_config = critical/high that applies ONLY IF applies_if holds — …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

Note how three different states coexist in one response:

- **kubernetes and containerd** — zero facts, but the `note` says the
  projects are tracked and their releases so far were routine. A different
  state from not-tracked (`tracked:false`).
- **cilium and coredns** — the running version is older than every release
  on record, so the `note` warns that the reviewed window is partial — a
  signal to re-check the version against a live resource.
- **envoy** — 75 new facts merged down to 40 issues, because the same
  advisories were fixed on the v1.36.9, v1.37.5, and v1.38.3 branches and
  merged into single entries via `same_issue_also_addressed_in`.

### Example prompts

> "We run envoy v1.36.8 — anything to take care of before moving to
> v1.37.0?"

> "Our stack is Kubernetes 1.31, Cilium 1.16, and CoreDNS 1.11 — anything we
> should look at before upgrading?"

An agent answers both with a single `check_stack` call — the first with
`target_version` filled in, the second with three components in one call. An
agent that can read the cluster directly can also be handed the version
gathering itself, as with the two phrasings in the scenario above.

## list_projects

The roster of every tracked project. It takes no arguments and the response
is small, so call it before guessing a slug from a project name. A wrong
slug is not an error: it comes back as `tracked:false` in `check_stack`, so
a guess turns silently into "no coverage". The `slug` field in this response is the canonical source for the
`project` argument every other tool takes.

### Parameters

None. Call it with empty arguments.

### Example call

```json
{}
```

### Measured response (trimmed)

```json
{
  "projects": [
    { "slug": "argo", "name": "Argo", "tier": "graduated", "category": "cicd", "analyzed_releases": 25 },
    …
    { "slug": "coredns", "name": "CoreDNS", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 5, "cluster_core": true },
    { "slug": "envoy", "name": "Envoy", "tier": "graduated", "category": "networking",
      "analyzed_releases": 22, "image_aliases": [ "cilium-envoy" ], "cluster_core": true },
    { "slug": "etcd", "name": "etcd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 18, "cluster_core": true,
      "visibility": "may live outside the k8s API, be hidden by a managed control plane, or be replaced entirely — a missing pod is not a missing component; when unreadable, report it under Could not check instead of guessing a version" },
    { "slug": "kubernetes", "name": "Kubernetes", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 22, "cluster_core": true,
      "image_aliases": [ "kube-apiserver", "kube-controller-manager", "kube-scheduler", "kubelet", "kube-proxy" ] },
    …
  ],
  "count": 76
}
```

Beyond the base fields (`slug`, `name`, `tier`, `category`,
`analyzed_releases`), three markers appear on some entries:

- `image_aliases` — names the project runs under in clusters. An image or
  workload matching an alias belongs to that project, at the version in its
  tag.
- `cluster_core:true` — the cluster substrate (control plane, datastore,
  DNS, runtime, CNI/dataplane). Every `cluster_core` project present in a
  cluster is a candidate for the `check_stack` call.
- `visibility` — a hint about how the component is observed and where it can
  legitimately be unreadable (etcd, for instance, may live outside the k8s
  API). An unreadable component should be reported as unchecked, never
  guessed.

### Example prompts

> "Our stack is Kubernetes 1.31, Cilium 1.16, and CoreDNS 1.11 — anything we
> should look at before upgrading?"

The same question as `check_stack` — as the scenario above showed, the agent
calls this tool first to pin down the slugs, then moves on to `check_stack`.

## list_releases

Returns the newest N releases of one project as one-line summaries (version,
date, coverage, fact counts by severity, and the advisory group's maximum
severity — `max_group_severity`, `null` when no fact in the release belongs
to an advisory group), newest first. It is the tool for "what happened in X
lately?". Contrast it with `list_facts`, which serves the same data in the
opposite order: `list_facts` is a sync feed, oldest-analyzed first, for
keeping a local copy current. Recent-news questions belong here instead.
When a row catches your eye, drill into it with `get_release`.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | yes | string | — | Project slug (e.g. `istio`) |
| `limit` | no | integer | `5` | How many recent releases. Maximum `20` |

### Example call

```json
{ "project": "cilium", "limit": 3 }
```

### Measured response (trimmed)

```json
{
  "project": "cilium",
  "count": 3,
  "releases": [
    {
      "version": "v1.20.0", "version_rank": [ 1, 20, 0 ],
      "released_at": "2026-07-29T15:00:29.000Z", "reviewed_at": "2026-07-29T15:49:54.048Z",
      "coverage": "full_reviewed",
      "facts_total": 22, "facts_mandatory": 17,
      "facts_by_severity": { "high": 3, "medium": 11, "low": 8 },
      "max_group_severity": null,
      "api_url": "https://ratatosk.io/v1/releases/cilium/v1.20.0",
      "release_url": "https://ratatosk.io/releases/cilium/v1.20.0"
    },
    {
      "version": "v1.19.6", …, "facts_total": 2, "facts_mandatory": 2,
      "facts_by_severity": { "high": 1, "medium": 1 }, …
    },
    {
      "version": "v1.18.12", …, "coverage": "full_reviewed",
      "facts_total": 0, "facts_mandatory": 0, "facts_by_severity": {}, …
    }
  ],
  "hint": "summaries only — fetch /v1/releases/{project}/{version} for a release's full facts"
}
```

A row like `v1.18.12` — `facts_total` of 0 with `coverage` of
`full_reviewed` — means the note was read in full and the release was
routine, distinct from having no data.

### Example prompts

> "What happened in Cilium lately?"

The agent calls `list_releases(project: "cilium")` and answers from the
summaries above — noting that the facts cluster in v1.20.0.

## get_release

One reviewed release in full: the release-level fields (coverage, overall
assessment, link to the original note) plus all of its facts. This is the
tool for drilling into one release that `check_stack` or `list_releases`
surfaced, and for verifying a judgment against the original note via
`source_url`. Omitting `version` returns the project's latest reviewed
release, so "how does the latest X release look?" also lands here. One
privacy note: unlike `check_stack`, this tool takes a version as an
argument, and that version travels in the upstream request path — see
["How your component versions are handled" in the README](../README.md#how-your-component-versions-are-handled).

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | yes | string | — | Project slug (e.g. `envoy`) |
| `version` | no | string | *(none = latest reviewed release)* | The release tag exactly as published (e.g. `v1.38.3`). The leading `v` is accepted either way — projects disagree on the spelling. A tag that does not exist returns an error listing the project's recent reviewed tags; retry with one of those |
| `include_raw` | no | boolean | `false` | Also return the original release note body as `raw_notes` — judge from the source instead of the extracted facts. Included automatically whenever the review is not the full story (coverage `insufficient`, or zero facts) |

### Example call

Omitting the version to get the latest reviewed release:

```json
{ "project": "istio" }
```

### Measured response (trimmed)

```json
{
  "project": "istio",
  "version": "1.29.6",
  "released_at": "2026-07-16T16:51:06.000Z",
  "reviewed_at": "2026-07-16T17:27:06.386Z",
  "source_url": "https://github.com/istio/istio/releases/tag/1.29.6",
  "coverage": "full_reviewed",
  "assessment": "Routine patch release focused on bug fixes for ambient mesh components (pilot-agent drain logic, WorkloadEntry HBONE capability propagation, ztunnel CNI deadlock, Istiod memory leak, and east-west gateway RBAC filtering); no new breaking changes, API changes, or CVEs, though one fix has an operator-visible transitional caveat for auto-registered WorkloadEntry resources.",
  "release_url": "https://ratatosk.io/releases/istio/1.29.6",
  "facts": [
    {
      "fact_id": 490,
      "fact_type": "behavior_changed",
      "severity": "medium",
      "mandatory": true,
      "confidence": 0.85,
      "applies_if": {
        "status": "degraded",
        "fallback": "workloads auto-registered before upgrading continue to be reached over plaintext until they either re-register or the networking.istio.io/tunnel=http label is added to their existing WorkloadEntry"
      },
      "affected": { "fixed_in": "1.29.6", "removed_in": null, "deprecated_in": null },
      "entities": [ { "kind": "config_field", "name": "networking.istio.io/tunnel", … } ],
      "references": {
        "ids": [],
        "quote": "workloads auto-registered before upgrading continue to be reached over plaintext until …"
      },
      …
    }
  ]
}
```

Read the release-level fields first: `coverage: "full_reviewed"` means the
note was read in full, and `assessment` is a one-paragraph judgment of the
whole release.
An empty `facts` array under `full_reviewed` coverage means the release was
read and found routine. Verify critical decisions against the original at
`source_url`.

### Example prompts

> "How does the latest Istio release look — what did the review find?"

The agent calls `get_release(project: "istio")` and answers from the
`assessment` above.

## facts_by_entity

The reverse index: every fact touching one exact identifier — a CVE id, a
CRD, a feature gate, a flag, a metric, a config field, a dependency —
collected across projects and releases. Lookup is case-insensitive. Reach
for this tool when you have one identifier from a manifest or an advisory
and want to know what happened around it. It works in the opposite
direction from `get_release` and `list_releases`, which start from a
project.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `name` | yes | string | — | The exact identifier to look up: a CVE id, CRD, feature gate, flag, metric, config field, or dependency |
| `kind` | no | string | *(none = all kinds)* | Restrict to one identifier kind: `api`, `crd`, `feature_gate`, `flag`, `metric`, `config_field`, `extension`, `dependency`, `cve`, `advisory`, `subsystem` |

### Example call

```json
{ "name": "CVE-2026-47778" }
```

### Measured response (trimmed)

Eight facts came back — five envoy release branches, plus three istio
releases whose notes covered the same CVE:

```json
{
  "facts": [
    {
      "fact_id": 211, "project": "envoy", "version": "v1.36.9",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-f8x4-rw5x-f3r7", "group_severity": "critical",
      "affected": { "fixed_in": "v1.36.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
        "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass"
      },
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.36.9", …
    },
    {
      "fact_id": 239, "project": "istio", "version": "1.28.9",
      "fact_type": "security_fix", "severity": "medium", "mandatory": true,
      "advisory_group_key": "adv:istio-security-2026-005", "group_severity": "high",
      "affected": { "fixed_in": "1.28.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "ISTIO-SECURITY-2026-005" ],
        "quote": "Envoy could fail to validate the Subject Alternative Name (SAN) of a peer certificate if the SAN contained an embedded NUL byte"
      },
      "source_url": "https://github.com/istio/istio/releases/tag/1.28.9", …
    },
    { "fact_id": 268, "project": "envoy", "version": "v1.35.13", "severity": "critical", … },
    { "fact_id": 285, "project": "envoy", "version": "v1.38.3", "severity": "critical", … },
    { "fact_id": 328, "project": "istio", "version": "1.30.2", "severity": "low",
      "advisory_group_key": "adv:istio-security-2026-005", "group_severity": "high", … },
    …3 entries omitted…
  ]
}
```

One reading note: every fact carries two severities. `severity` is what that
release's note said; `group_severity` is the maximum across the whole
advisory group (`advisory_group_key`). Judge urgency by `group_severity` —
the istio 1.30.2 fact above is `low` per-release but `high` as a group.

### Example prompts

> "CVE-2026-47778 — which releases fixed it, and does it affect the envoy we
> run?"

The agent calls `facts_by_entity(name: "CVE-2026-47778")` and answers from
the per-branch `fixed_in` values (envoy v1.35.13, v1.36.9, v1.37.5, v1.38.3,
and v1.39.0). For security checks, query both the CVE id and the advisory id
(GHSA) — a fact whose note cites only one of the two is indexed only under
that one.

## list_facts

The incremental sync feed of facts. It flows in ascending `fact_id` order —
oldest-analyzed first — so the first page is not the newest data. Pass the
response's `next_since` as the next call's `since` to page through; the tool
exists to keep a local copy up to date. For "latest release of X" questions,
use `list_releases` or `get_release` instead. Filters for project, type, and
severity narrow the feed.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | no | string | *(none = all)* | Project slug filter (e.g. `envoy`) |
| `type` | no | string | *(none = all)* | Fact type filter: `security_fix`, `dependency_bump`, `capability_removed`, `capability_deprecated`, `api_version_changed`, `identifier_renamed`, `validation_tightened`, `default_changed`, `behavior_changed` |
| `severity` | no | string | *(none = all)* | Severity filter: `info`, `low`, `medium`, `high`, `critical` (exactly that severity) |
| `since` | no | integer | *(none = from the beginning)* | Cursor: only facts with a `fact_id` greater than this. Pass the previous response's `next_since` |
| `limit` | no | integer | `50` | Page size. Maximum `200` |

### Example call

```json
{ "type": "security_fix", "severity": "critical", "limit": 2 }
```

### Measured response (trimmed)

```json
{
  "facts": [
    {
      "fact_id": 211, "project": "envoy", "version": "v1.36.9",
      "released_at": "2026-06-23T20:22:33.000Z",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-f8x4-rw5x-f3r7", "group_severity": "critical",
      "affected": { "fixed_in": "v1.36.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
        "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass"
      },
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.36.9", …
    },
    {
      "fact_id": 215, "project": "envoy", "version": "v1.36.9",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-8phg-2h2q-jgxf", "group_severity": "critical",
      "applies_if": { "status": "degraded", "fallback": "if you proxy HTTP/3 to HTTP/1 backends" },
      "references": {
        "ids": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
        "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length"
      }, …
    }
  ],
  "next_since": 215
}
```

The next page is
`{ "type": "security_fix", "severity": "critical", "since": 215 }`. Repeat
until `next_since` comes back `null` — `null` means there is nothing more to
fetch (do not send `since=null`; that is a `400`).

### Example prompts

> "Walk me through the critical security fixes on record."

The agent calls `list_facts(type: "security_fix", severity: "critical")` and
works through the feed page by page.

## Privacy and rate limits

In short: the `check_stack` version comparison happens inside the server
process, so if you self-host, running versions never leave your
infrastructure. The full boundary and log handling are covered in
["How your component versions are handled" in the README](../README.md#how-your-component-versions-are-handled).
The upstream public API is limited to 60 requests per minute per IP, and the
hosted endpoint shares that bucket with other users — bundle stack questions
into one `check_stack` call instead of polling per project, and switch to
self-hosting for heavy use.
