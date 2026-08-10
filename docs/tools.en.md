# Tools reference

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

This page is the detailed reference for the seven tools ratatosk-mcp
provides — [`check_stack`](#check_stack), [`list_projects`](#list_projects),
[`list_releases`](#list_releases), [`get_release`](#get_release),
[`changes_by_entity`](#changes_by_entity), [`get_matter`](#get_matter), and
[`list_changes`](#list_changes). For each tool you will find when to reach
for it, every parameter, a real call and its response, and example questions
you can put to an agent. For connecting the server itself (hosted endpoint,
Docker, Helm, kagent), see the [installation guide](install.en.md).

> **Measured 2026-08-10**: every call and response below was made by running
> this server against the public API at `https://ratatosk.io` and is
> reproduced from what came back. The stack used in the `check_stack`
> example — the five components and their `version_source` strings —
> originates from a real cluster (anonymized). Release data accrues hourly,
> so the numbers and lists will differ if you call today; the shape of the
> responses will not. Long responses are trimmed with `…` for space.

## The change model in one page

Every tool here returns the same unit: a **change**. One change is one thing
that happened in one release — a fix with a CVE behind it, a removed flag, a
renamed config field, a default that flipped — carried together with the
verbatim sentence from the release note it was read out of.

Three axes describe every change, and they are independent:

| Axis | Values | The question it answers |
|---|---|---|
| `family` | `security` · `breaking` · `deprecated` | What kind of thing is this? |
| `bucket` | `action` · `check` · `plan` | How do I act on it *now*? |
| `applies_if` | a condition, structured where possible | Is it mine to act on at all? |

**Read `bucket` before severity.** An `action` entry applies to every
install passing through that version. A `check` entry applies only where its
`applies_if` holds, so it is a question to resolve against the running
configuration, not a task. A `plan` entry is an announcement about a future
release. The website, the weekly email, and `check_stack` all split on this
same field, so the three agree by construction.

Four more fields carry most of the weight:

- **`matter_key`** — the identity of the underlying matter across releases.
  The same advisory fixed on five branches has one `matter_key` and five
  changes. [`get_matter`](#get_matter) expands it.
- **`applies_if.clauses`** — when the condition could be structured, each
  clause names a `kind` (`api`, `crd`, `feature_gate`, `flag`, `config_field`,
  `extension`, `subsystem`, `dependency`, …), a `name`, a `verb`, and a
  `polarity`, combined by `mode` (`all_of`, `any_of`, `universal`). Look the
  names up in the running configuration instead of parsing the sentence.
- **`advisories`** — cited CVE/GHSA ids with their *current* severity, which
  is not necessarily the severity the release note claimed at the time.
- **`quote`** — the verbatim sentence, so a judgment can always be checked
  against `source_url`.

`changes: []` on a release means the note was read and nothing
operator-facing was recorded — auditable silence, not a gap. The
`notes_total` field counts the routine record (bot dependency bumps and the
like) that is deliberately not surfaced one by one.

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
`image_aliases` field resolves the cases where they differ: the
`cilium-envoy` daemonset in the cluster is the `envoy` project.

**Stage 2 — one `check_stack` call for the whole stack.** Five components go
out together rather than five separate lookups, and each one carries a
`version_source` saying where the version was read.

**Stage 3 — resolve, then drill down.** The briefing splits into what
applies to everyone and what depends on the configuration, and the agent
worked both sides: `get_release` for the release behind an
`action_required` entry, and a read of `cilium-config` to settle an
`applies_if` it could not answer from the briefing alone.

The shape of the answer is the point. The agent did not report "there are
updates available"; it reported which entries applied unconditionally, which
ones it had verified against the running configuration, and which it could
not check.

## How to read a check_stack briefing

The default `check_stack` response (`detail:"brief"`) has the same structure
for every component. Keep the measured response in the
[check_stack section](#check_stack) at hand while reading.

1. **`changes_scanned` and `summary` — the counts.** `changes_scanned` is
   how many changes the project has on record at all; `summary.new_changes`
   is how many of those sit above the running version. `distinct_matters` is
   what remains after merging repeats of one `matter_key`. In the measured
   response, containerd scans 35, of which 11 are new, collapsing to 9
   distinct matters. The `by_severity`, `by_family`, and `by_bucket`
   breakdowns all count the distinct matters, not the raw changes.
2. **The three lists split strictly by `bucket`** — `action` goes to
   `action_required`, `check` to `check_config`, `plan` to `other_changes`.
   The split is *not* by severity: containerd's `check_config` holds five
   `low` entries, and coredns has a `medium` in `action_required`. Severity
   ranks urgency inside a list; the bucket decides which list.
3. **`action_required` — act on this regardless of configuration.**
   containerd's first entry is the security roll-up carrying ten advisory
   ids, `critical` because the worst of them is. Most entries here have no
   `applies_if` at all; a few do, and carry it because the condition is
   near-universal for that project (an envoy entry conditioned on "runs
   http2", for instance).
4. **`check_config` — resolve the condition before you recommend anything.**
   Every entry here has an `applies_if`. Until you have checked it against
   the actual configuration, the entry is not an action, and an unmet
   condition is not a reason to upgrade. Read it forward instead: the
   entry's `version` is the minimum to be on *before* enabling what
   `applies_if` describes. `applies_if_targets` lists the things to grep the
   live configuration for — containerd's `["CreateContainer", "sandbox"]`,
   cilium's `["ipBlock"]`. When `applies_if` is only a sentence, the targets
   field is absent.
5. **`other_changes` — the `plan` bucket, one line each.** Deprecations
   announced for a future release. containerd's two entries both carry
   `window.deprecated_in`, which is when the clock started.
6. **`severity` here is computed by this server, not by ratatosk.** It is
   the highest severity among the entry's `advisories`; with no advisories,
   `security` family maps to `high`, and otherwise the bucket decides —
   `action` → `medium`, `check` → `low`, `plan` → `info`. That is why
   cilium's dependency bumps tagged `[security]` show as `high` with no
   `advisories` array. To judge from the advisories themselves, use
   [`get_release`](#get_release) or
   [`changes_by_entity`](#changes_by_entity), which return them with their
   own severities.
7. **Two merge rules, both visible in the measured response.** Changes
   sharing a `matter_key` across release branches collapse into the earliest
   fix, with every other release that carried it — inside the window or not —
   listed in `same_matter_also_addressed_in`. An operator upgrades once, so
   the nearest release that closes the matter is the actionable one.
   Separately, changes sharing one quoted sentence *within* a release merge
   into a single entry with their ids listed together, and with
   `applies_if_any` in place of `applies_if` when their conditions differ.
   Merged entries take the highest severity and the most urgent bucket of
   their members.
8. **The briefing is branch-aware, and has to be.** Maintainers backport one
   fix onto every supported branch, so the same `matter_key` lands on v2.2.4
   and v2.3.1 on the same day. Reading only "releases newer than the running
   version" would show an operator on 2.2.5 the v2.3.1 occurrence and call it
   `action_required` — work on something their own branch closed a release
   ago. A matter already fixed **at or below the running version on the
   running branch** is therefore excluded, and `note` says how many were.
   Branch equality is what makes that safe: a backport to v2.1.9 says nothing
   about v2.2.4, which was cut from a different branch and may never have
   received it, so only the running version's own branch counts as proof.
9. **`note` — per-component state markers.** Two kinds appear:
   `tracked:false` with "NOT tracked by ratatosk — zero changes means no
   coverage here, not safety", and "running version … is older than every
   release on record". The second is the only cross-check available on a
   version claim: this server cannot see your environment, so a version
   older than the whole record is a signal to re-read it off a live
   resource. Absence of both markers, with `new_changes: 0`, is the quiet
   case — tracked, scanned, nothing above the running version. kubernetes in
   the measured response is that: 45 scanned, 0 new.
10. **`hint` and `privacy`.** The `hint` restates the briefing as a data
   classification rather than a recommendation and points to the next tools.
   The `privacy` line records how far your versions travelled in this call.

## check_stack

Takes the list of component versions you are running and returns, per
component, the changes on its upgrade path — the releases newer than the
running version. It is the first tool to reach for when the question is
"anything to do before we upgrade?". Because one call covers several
projects, it answers stack-wide questions better than calling the other
tools project by project.

The version comparison happens **inside this server process**. Only project
slugs are sent upstream, and this tool never calls the server-side
`/v1/upgrade` endpoint. Run the server yourself and running versions never
leave your infrastructure; on the hosted endpoint they transit server memory
only and are not logged.

To go deep on one release, switch to `get_release`; to trace one CVE or
flag, switch to `changes_by_entity`; to see every branch that fixed one
matter, switch to `get_matter`.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `components` | yes | array | — | The running stack to check. Item shape below |
| `components[].project` | yes | string | — | Project slug (e.g. `envoy`). The canonical source is `slug` in `list_projects` |
| `components[].version` | yes | string | — | The version currently running (e.g. `v1.36.8`) |
| `components[].target_version` | no | string | *(none = everything up to the newest)* | Upgrade destination, strictly above the running version: only changes with `running < version <= target` are returned. A target at or below the running version would make the range empty, so it is ignored with a `note` |
| `components[].version_source` | no | string | *(none)* | Where the running version was read (e.g. `daemonset/cilium image tag`, or that the user stated it). Echoed back verbatim so the claim can be audited later. The server cannot see your environment and cannot verify a version — the `note` marker described above is the only cross-check available |
| `detail` | no | string | `brief` | `brief`: `summary` plus the three bucket lists, merged and one line each. `full`: every matching change verbatim in `relevant_changes`, unmerged and with no summary — capped at 50 per component, the overflow counted in `relevant_changes_omitted` |
| `severity_min` | no | string | *(none = all)* | Only changes at or above this severity: `info`, `low`, `medium`, `high`, `critical` |

In `brief` mode the `other_changes` tail is capped at 100 per component,
with the overflow counted in `other_changes_omitted`. Nothing is ever
dropped silently.

### Example call

This is the stack from the scenario above. The JSON below is the tool's
`arguments` object only — the JSON-RPC envelope it travels in is covered in
the [installation guide](install.en.md):

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
      "changes_scanned": 45,
      "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
    },
    {
      "project": "containerd",
      "running_version": "2.2.4",
      "version_source": "node status containerRuntimeVersion",
      "changes_scanned": 35,
      "summary": {
        "new_changes": 11, "distinct_matters": 9,
        "by_severity": { "critical": 1, "high": 1, "low": 5, "info": 2 },
        "by_family": { "breaking": 5, "deprecated": 2, "security": 2 },
        "by_bucket": { "action": 2, "check": 5, "plan": 2 }
      },
      "action_required": [
        {
          "change_id": "containerd:v2.2.5:a74c2b48",
          "matter_key": "containerd/advisory:cve-2026-47262",
          "version": "v2.2.5",
          "kind": "defect_corrected",
          "family": "security",
          "bucket": "action",
          "severity": "critical",
          "quote": "CVE-2026-50195",
          "advisories": [
            "CVE-2026-47262", "CVE-2026-50195", "CVE-2026-53488", "CVE-2026-53489",
            "CVE-2026-53492", "GHSA-33vj-92qq-66hc", "GHSA-cvxm-645q-p574",
            "GHSA-jpcc-p29g-p8mq", "GHSA-rgh6-rfwx-v388", "GHSA-xhf5-7wjv-pqxp"
          ],
          "same_matter_also_addressed_in": [ "v2.3.2" ]
        },
        { "change_id": "containerd:v2.3.1:040369eb", "version": "v2.3.1", "severity": "high",
          "family": "security", "bucket": "action", "quote": "* [**CVE-2026-46680**]",
          "advisories": [ "CVE-2026-46680", "GHSA-fqw6-gf59-qr4w" ] }
      ],
      "check_config": [
        {
          "change_id": "containerd:v2.2.6:b8305908",
          "matter_key": "containerd/api/createcontainer#constraint_changed",
          "version": "v2.2.6",
          "kind": "constraint_changed",
          "family": "breaking",
          "bucket": "check",
          "severity": "low",
          "applies_if": "uses the CreateContainer API and runs sandbox",
          "applies_if_targets": [ "CreateContainer", "sandbox" ],
          "quote": "* cri: reject CreateContainer when sandbox is not running (#13669)",
          "same_matter_also_addressed_in": [ "v2.3.3" ]
        },
        { "change_id": "containerd:v2.3.1:4dff4eb6", "version": "v2.3.1", "severity": "low",
          "kind": "removed", "family": "breaking", "bucket": "check",
          "applies_if": "runs user namespace", "applies_if_targets": [ "user namespace" ],
          "window": { "removed_in": "v2.3.1" },
          "quote": "* Disable overlayfs \"rebase\" capability when running in user namespace" },
        …3 entries omitted…
      ],
      "other_changes": [
        { "change_id": "containerd:v2.3.0:a4f90153",
          "matter_key": "containerd/api/shim.command#deprecated",
          "version": "v2.3.0", "kind": "deprecated", "family": "deprecated",
          "bucket": "plan", "severity": "info",
          "applies_if": "uses the shim.Command API",
          "applies_if_targets": [ "shim.Command" ],
          "window": { "deprecated_in": "2.3" },
          "quote": "* Deprecate shim.Command" },
        …1 entry omitted…
      ]
    },
    {
      "project": "cilium",
      "running_version": "v1.19.4",
      "version_source": "daemonset/cilium image tag",
      "changes_scanned": 122,
      "summary": {
        "new_changes": 51, "distinct_matters": 48,
        "by_severity": { "high": 7, "medium": 3, "low": 32, "info": 6 },
        "by_family": { "breaking": 35, "security": 7, "deprecated": 6 },
        "by_bucket": { "action": 8, "check": 34, "plan": 6 }
      },
      "action_required": [
        { "change_id": "cilium:v1.20.0:20adda37",
          "matter_key": "cilium/config_field/cni configuration version#default_changed",
          "version": "v1.20.0", "kind": "default_changed", "family": "breaking",
          "bucket": "action", "severity": "medium",
          "window": { "introduced_in": "v1.20.0" },
          "quote": "the default CNI configuration version moves from 0.3.1 to 1.0.0." },
        { "change_id": "cilium:v1.20.0:4bb6c02d", "version": "v1.20.0", "severity": "high",
          "family": "security", "bucket": "action",
          "quote": "fix(deps): update module google.golang.org/grpc to v1.82.1 [security] (v1.20)" },
        …6 entries omitted…
      ],
      "check_config": [
        { "change_id": "cilium:v1.19.5:41a4ef5b",
          "matter_key": "cilium/config_field/l2podannouncements.interface#renamed",
          "version": "v1.19.5", "kind": "renamed", "family": "breaking",
          "bucket": "check", "severity": "low",
          "applies_if": "enables L2 pod announcements",
          "applies_if_targets": [ "L2 pod announcements" ],
          "window": { "removed_in": "v1.19.5" },
          "quote": "Remove defunct `l2podAnnouncements.interface` Helm value that rendered a configmap key the agent no longer recognises, causing crash-loops when L2 pod announcements were enabled. Users must use `l2podAnnouncements.interfacePattern` instead." },
        { "change_id": "cilium:v1.19.5:9caac3a9",
          "matter_key": "cilium/api/ipblock#defect_corrected",
          "version": "v1.19.5", "kind": "defect_corrected", "family": "security",
          "bucket": "check", "severity": "high",
          "applies_if": "configures the ipBlock API",
          "applies_if_targets": [ "ipBlock" ],
          "quote": "Fix wildcard namespace bypass for selectorless ipBlock rules" },
        …32 entries omitted…
      ],
      "other_changes": [ …6 entries… ]
    },
    {
      "project": "envoy",
      "running_version": "v1.36.7",
      "version_source": "daemonset/cilium-envoy image tag",
      "changes_scanned": 124,
      "summary": {
        "new_changes": 90, "distinct_matters": 45,
        "by_severity": { "high": 12, "medium": 15, "low": 15, "info": 3 },
        "by_family": { "security": 26, "breaking": 16, "deprecated": 3 },
        "by_bucket": { "action": 4, "check": 38, "plan": 3 }
      },
      "action_required": [
        { "change_id": "envoy:v1.36.9:d69a1e5f",
          "matter_key": "envoy/advisory:cve-2026-47261",
          "version": "v1.36.9", "family": "security", "bucket": "action", "severity": "high",
          "quote": "wasm: bumped `com_github_wasmtime` to resolve CVE-2026-47261.",
          "advisories": [ "CVE-2026-47261" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …3 entries omitted…
      ],
      "check_config": [
        { "change_id": "envoy:v1.36.9:7ccbb3ff",
          "matter_key": "envoy/advisory:cve-2026-48743",
          "version": "v1.36.9", "family": "security", "bucket": "check", "severity": "high",
          "applies_if": "uses HTTP/3 and uses HTTP/1 and uses headers-only request and configures the Content-Length setting",
          "applies_if_targets": [ "HTTP/3", "HTTP/1", "headers-only request", "Content-Length" ],
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "advisories": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …37 entries omitted…
      ],
      "other_changes": [ …3 entries… ]
    },
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "summary": {
        "new_changes": 8, "distinct_matters": 8,
        "by_severity": { "critical": 1, "high": 3, "medium": 1, "low": 3 },
        "by_family": { "breaking": 4, "security": 4 },
        "by_bucket": { "action": 3, "check": 5 }
      },
      "action_required": [ …3 entries… ],
      "check_config": [ …5 entries… ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation — changes are provided without warranty and the decision stays with the operator): …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

Four states coexist in this one response:

- **kubernetes** — 45 changes on record, none of them above v1.36.1. Tracked,
  scanned, quiet. No `note`, because there is nothing to warn about.
- **containerd** — the compact case: 11 new changes, 9 distinct matters,
  split 2 / 5 / 2 across the buckets. The first `action_required` entry
  carries ten advisory ids from one roll-up.
- **cilium and envoy** — the crowded case, and the reason the bucket split
  matters. envoy has 90 new changes, but only 4 land in `action_required`;
  38 are questions about the configuration, not tasks.
- **coredns** — a `critical` sitting in a stack whose other components are
  quiet, which is exactly what a stack-wide call is for.

An untracked slug looks different again. Asking about `nginx-ingress`, which
ratatosk does not cover, returns:

```json
{
  "project": "nginx-ingress",
  "running_version": "v1.14.0",
  "version_source": "user-stated",
  "tracked": false,
  "note": "NOT tracked by ratatosk — zero changes means no coverage here, not safety",
  "changes_scanned": 0,
  "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
}
```

That is why a guessed slug is worth avoiding: it comes back as an absence of
findings, which reads like safety. Call `list_projects` when unsure.

And a running version below the whole record returns the coverage marker:

```json
{
  "project": "coredns",
  "running_version": "v1.9.0",
  "note": "running version v1.9.0 is older than every release on record (earliest on record: v1.14.0) — this covers the reviewed window only, so treat it as partial, and re-check that the running version was read off a live resource",
  "changes_scanned": 11,
  "summary": { "new_changes": 11, "distinct_matters": 11, … }
}
```

### `detail:"full"`

`full` is a different response, not a longer one. The component drops
`summary` and the three bucket lists and returns `relevant_changes`: the
matching changes in full server shape — `applies_if` as a structured object,
`advisories` with their own severities, `subjects`, `seq` — unmerged, so
repeats of one `matter_key` all appear.

```json
{ "detail": "full", "severity_min": "high",
  "components": [ { "project": "coredns", "version": "v1.14.1", "version_source": "deployment/coredns image tag" } ] }
```

```json
{
  "components": [
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "relevant_changes": [
        {
          "change_id": "coredns:v1.14.2:83ed5f33",
          "matter_key": "coredns/advisory:cve-2026-25679",
          "project": "coredns", "version": "v1.14.2", "version_rank": [ 1, 14, 2 ],
          "released_at": "2026-03-06T06:34:58.000Z",
          "family": "security", "actionability": "act", "bucket": "action",
          "kind": "value_changed",
          "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
          "advisories": [
            { "id": "CVE-2026-25679", "severity": "high" },
            { "id": "CVE-2026-27137", "severity": "high" },
            { "id": "CVE-2026-27138", "severity": "medium" },
            { "id": "CVE-2026-27139", "severity": "low" },
            { "id": "CVE-2026-27142", "severity": "medium" }
          ],
          "subjects": [ { "kind": "dependency", "name": "go 1.26.1", "name_full": "Go 1.26.1", "role": "changed" } ],
          "window": { "introduced_in": "v1.14.2" },
          "quote": "In addition, the release updates the build to Go 1.26.1, which include security fixes addressing CVE-2026-27137, CVE-2026-27138, CVE-2026-27139, CVE-2026-25679, and CVE-2026-27142.",
          "disclosure": "described",
          "source_url": "https://github.com/coredns/coredns/releases/tag/v1.14.2",
          "release_url": "https://ratatosk.io/en/releases/coredns/v1.14.2",
          "seq": 3431
        },
        …3 entries omitted…
      ]
    }
  ],
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

Note that the severities differ between the two modes for the same change:
`brief` reports one `severity` (`high`, the maximum of the group), `full`
reports all five advisories with their own. Narrow `full` with
`severity_min` or `target_version` — an unfiltered `full` on a busy project
is large.

### Example prompts

> "We run envoy v1.36.8 — anything to take care of before moving to
> v1.37.0?"

> "Our stack is Kubernetes 1.31, Cilium 1.16, and CoreDNS 1.11 — anything we
> should look at before upgrading?"

An agent answers both with a single `check_stack` call — the first with
`target_version` filled in, the second with three components in one call. An
agent that can read the cluster directly can also be handed the version
gathering itself, as in the scenario above.

## list_projects

The roster of every tracked project. It takes no arguments and the response
is small, so call it before guessing a slug from a project name. A wrong
slug is not an error: it comes back as `tracked:false` in `check_stack`, so
a guess turns silently into "no coverage". The `slug` field here is the
canonical source for the `project` argument every other tool takes.

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
    { "slug": "argo", "name": "Argo", "tier": "graduated", "category": "cicd", "analyzed_releases": 32 },
    …
    { "slug": "cilium", "name": "Cilium", "tier": "graduated", "category": "networking",
      "analyzed_releases": 21, "cluster_core": true },
    { "slug": "containerd", "name": "containerd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 22, "cluster_core": true,
      "visibility": "observed via node status (nodeInfo.containerRuntimeVersion), never via pods — a workload listing cannot show the runtime" },
    { "slug": "coredns", "name": "CoreDNS", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 7, "cluster_core": true },
    { "slug": "envoy", "name": "Envoy", "tier": "graduated", "category": "networking",
      "analyzed_releases": 23, "image_aliases": [ "cilium-envoy" ], "cluster_core": true },
    { "slug": "etcd", "name": "etcd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 21, "cluster_core": true,
      "visibility": "may live outside the k8s API, be hidden by a managed control plane, or be replaced entirely — a missing pod is not a missing component; when unreadable, report it under Could not check instead of guessing a version" },
    { "slug": "kubernetes", "name": "Kubernetes", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 26, "cluster_core": true,
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
  legitimately be unreadable. containerd's says to read it off node status
  rather than from a pod listing; etcd's warns that it may live outside the
  k8s API entirely. An unreadable component should be reported as unchecked,
  never guessed.

### Example prompts

> "Our stack is Kubernetes 1.31, Cilium 1.16, and CoreDNS 1.11 — anything we
> should look at before upgrading?"

The same question as `check_stack` — as the scenario above showed, the agent
calls this tool first to pin down the slugs, then moves on to `check_stack`.

## list_releases

The newest N releases of one project as light summaries, newest first. It is
the tool for "what happened in X lately?". Contrast it with `list_changes`,
which serves the same data in the opposite order: that one is a sync feed,
oldest-analyzed first, for keeping a local copy current. Recent-news
questions belong here instead. When a row catches your eye, drill into it
with `get_release`.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | yes | string | — | Project slug (e.g. `istio`) |
| `limit` | no | integer | `5` | How many recent releases. Maximum `20` |

### Example call

```json
{ "project": "cilium", "limit": 5 }
```

### Measured response (trimmed)

```json
{
  "project": "cilium",
  "count": 5,
  "releases": [
    {
      "version": "v1.20.0", "version_rank": [ 1, 20, 0 ],
      "released_at": "2026-07-29T15:00:29.000Z", "reviewed_at": "2026-08-09T04:33:14.949Z",
      "changes_total": 45,
      "by_bucket": { "action": 9, "check": 30, "plan": 6 },
      "by_family": { "breaking": 32, "security": 7, "deprecated": 6 },
      "max_severity": null,
      "notes_total": 478,
      "api_url": "https://ratatosk.io/v1/releases/cilium/v1.20.0",
      "release_url": "https://ratatosk.io/en/releases/cilium/v1.20.0"
    },
    {
      "version": "v1.19.6", "released_at": "2026-07-16T22:52:21.000Z",
      "changes_total": 1, "by_bucket": { "check": 1 }, "by_family": { "breaking": 1 },
      "max_severity": null, "notes_total": 46, …
    },
    {
      "version": "v1.18.12", "released_at": "2026-07-16T22:47:50.000Z",
      "changes_total": 0, "by_bucket": {}, "by_family": {},
      "max_severity": null, "notes_total": 20, …
    },
    { "version": "v1.17.18", "changes_total": 0, "notes_total": 10, … },
    { "version": "v1.19.5", "changes_total": 3, "by_bucket": { "check": 3 },
      "by_family": { "breaking": 2, "security": 1 }, "notes_total": 48, … }
  ],
  "hint": "summaries only — fetch /v1/releases/{project}/{version} for a release's full changes"
}
```

Three reading notes:

- **`changes_total: 0`** — as on v1.18.12 and v1.17.18 — means the note was
  read in full and the release was routine. Auditable silence, distinct from
  having no data. `notes_total` shows the note was not empty: 20 and 10
  routine lines respectively.
- **`max_severity` is the highest *advisory* severity in the release**, and
  it is `null` when no change in it cites an advisory. v1.20.0 has seven
  `security`-family changes and still reports `null`: they are dependency
  bumps whose notes carry no CVE. `null` here means "no advisory ids", not
  "nothing serious".
- **`notes_total` dwarfs `changes_total`** on big releases — 478 against 45
  on v1.20.0. That gap is the routine record, deliberately not surfaced one
  by one.

### Example prompts

> "What happened in Cilium lately?"

The agent calls `list_releases(project: "cilium")` and answers from the
summaries — noting that the changes cluster in v1.20.0, and that the two
quiet releases were read rather than skipped.

## get_release

One reviewed release in full: the envelope (summary, source URL, release
URL, counts) plus all of its changes. This is the tool for drilling into a
release that `check_stack` or `list_releases` surfaced, and for verifying a
judgment against the original note via `source_url`. Omitting `version`
returns the project's latest reviewed release, so "how does the latest X
release look?" also lands here.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | yes | string | — | Project slug (e.g. `envoy`) |
| `version` | no | string | *(none = the latest reviewed release)* | Release tag as published (e.g. `v1.38.3`). Accepted with or without the leading `v` — projects disagree on the spelling |
| `include_raw` | no | boolean | `false` | Also return the original release note body as `raw_notes` (capped; `raw_notes_truncated: true` when cut) |

A wrong tag is a `404` that names the project's recent reviewed tags, so a
caller can self-correct in one retry:

```
/v1/releases/cilium/v9.9.9: HTTP 404: {"error":"no reviewed release 'v9.9.9' for
project 'cilium'. Recent reviewed versions: v1.20.0, v1.19.6, v1.18.12, v1.17.18,
v1.19.5 — retry with one of these exact tags."}
```

### Example call

```json
{ "project": "envoy", "version": "v1.38.3" }
```

### Measured response (trimmed)

```json
{
  "project": "envoy",
  "version": "v1.38.3",
  "version_rank": [ 1, 38, 3 ],
  "released_at": "2026-06-23T23:28:25.000Z",
  "reviewed_at": "2026-08-09T04:33:14.780Z",
  "summary": "Envoy v1.38.3 is a maintenance release with multiple disclosed security fixes and a security-related dependency update. It also disables a broken extension and changes the default for TLS certificate compression, so the release concerns deployments using those features.",
  "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
  "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
  "by_bucket": { "action": 1, "check": 17 },
  "by_family": { "security": 16, "breaking": 2 },
  "max_severity": "high",
  "notes_total": 0,
  "changes": [
    {
      "change_id": "envoy:v1.38.3:54ea382c",
      "matter_key": "envoy/advisory:cve-2026-47261",
      "project": "envoy", "version": "v1.38.3", "version_rank": [ 1, 38, 3 ],
      "released_at": "2026-06-23T23:28:25.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [ { "id": "CVE-2026-47261", "severity": "high" } ],
      "subjects": [
        { "kind": "dependency", "name": "com_github_wasmtime", "name_full": "com_github_wasmtime", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-47261", "name_full": "CVE-2026-47261", "role": "changed" }
      ],
      "window": null,
      "transition": null,
      "remedy": null,
      "symptom": [],
      "quote": "wasm: bumped ``com_github_wasmtime`` to resolve CVE-2026-47261.",
      "disclosure": "described",
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
      "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
      "seq": 13911
    },
    …17 entries omitted…
  ]
}
```

The 18 changes against `by_bucket` of 1 / 17 is the shape of a security
point release: one thing everybody must take, seventeen that depend on which
extensions you run.

### Auditable silence ships its own evidence

When a release has no operator-facing changes, `get_release` includes
`raw_notes` **without** being asked, so silence can be judged from the
source rather than trusted:

```json
{
  "project": "cilium",
  "version": "v1.18.12",
  "summary": "Cilium v1.18.12 adds Gateway access-log configuration and BYOCNI loopback support. It also fixes policy, startup, Gateway validation, IPAM, and metric-label defects, while updating shipped images and dependencies; no security advisories or security-specific flaws are disclosed.",
  "by_bucket": {}, "by_family": {}, "max_severity": null,
  "notes_total": 20,
  "changes": [],
  "raw_notes": "# 1.18.12\n\nSummary of Changes\n------------------\n\n**Minor Changes:**\n* gateway-api: add support for configuring Gateway access logs …",
  "raw_notes_truncated": false
}
```

### Example prompts

> "How does the latest Istio release look — what did the review find?"

The agent calls `get_release(project: "istio")` with no `version` and
answers from `summary` and the bucket counts.

## changes_by_entity

The reverse index: every change touching one exact identifier — a CVE id, a
CRD, a feature gate, a flag, a metric, a config field, a dependency —
collected across projects and releases. Lookup is case-insensitive. Reach
for this when you have one identifier from a manifest or an advisory and
want to know what happened around it. It works in the opposite direction
from `get_release` and `list_releases`, which start from a project.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `name` | yes | string | — | The exact identifier: a CVE id, CRD, feature gate, flag, metric, config field, extension, subsystem, or dependency |
| `kind` | no | string | *(none = all kinds)* | Restrict to one identifier kind: `api`, `crd`, `feature_gate`, `flag`, `metric`, `config_field`, `extension`, `dependency`, `cve`, `advisory`, `subsystem` |

The index is built from the `subjects` a change carries. The match is exact
on the stored `name` (case-insensitive), not a substring search, and at most
200 changes come back.

`{ "changes": [] }` means no change on record names that identifier — not
that nothing happened around it. Widen before concluding: drop `kind` first,
since it must equal the subject's stored kind exactly and one identifier is
not always filed under the kind you would guess. Then try the other
identifier a change might be indexed under — an advisory cited only by its
GHSA id is not reachable by its CVE id, and the reverse.

### Example call

```json
{ "name": "CVE-2026-41178" }
```

### Measured response

```json
{
  "changes": [
    {
      "change_id": "buildpacks:v0.40.9:83576398",
      "matter_key": "buildpacks/advisory:cve-2026-41178",
      "project": "buildpacks", "version": "v0.40.9", "version_rank": [ 0, 40, 9 ],
      "released_at": "2026-08-09T17:12:10.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [
        { "id": "CVE-2026-41178", "severity": "medium" },
        { "id": "GO-2026-5158", "severity": "medium" }
      ],
      "subjects": [
        { "kind": "dependency", "name": "go.opentelemetry.io/otel", "name_full": "go.opentelemetry.io/otel", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-41178", "name_full": "CVE-2026-41178", "role": "changed" },
        { "kind": "advisory", "name": "go-2026-5158", "name_full": "GO-2026-5158", "role": "changed" }
      ],
      "window": { "introduced_in": "v0.40.9" },
      "quote": "`go.opentelemetry.io/otel` | v1.43.0 → v1.44.0 | GO-2026-5158 / CVE-2026-41178 — baggage header not length-capped",
      "disclosure": "described",
      "source_url": "https://github.com/buildpacks/pack/releases/tag/v0.40.9",
      "release_url": "https://ratatosk.io/en/releases/buildpacks/v0.40.9",
      "seq": 18932
    }
  ]
}
```

Three subjects are indexed on this one change — the dependency, the CVE, and
the Go advisory — so the same entry is reachable by
`go.opentelemetry.io/otel`, `CVE-2026-41178`, or `GO-2026-5158`.
`window.introduced_in` says the fix landed in v0.40.9, which is the version
to be at or above.

### Example prompts

> "CVE-2026-41178 — which releases fixed it, and does it affect what we
> run?"

The agent calls `changes_by_entity(name: "CVE-2026-41178")`, reads
`window.introduced_in` per project, and compares against the running
versions. For security checks, query both the CVE id and the advisory id —
a change whose note cites only one of the two is indexed only under that
one.

## get_matter

One matter across every release it appeared in, oldest first. Take
`matter_key` verbatim from a change — it is case-sensitive and contains `/`
and `:`. This answers "which version fixes this for *my* branch" and "have I
already handled this".

Why every occurrence and not just the newest: a maintainer fixing one issue
on five supported branches produces five changes, and the branches do not
always carry the same set of advisories. Told only about the newest, someone
on an older branch would draw the wrong conclusion about their own coverage.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `matter_key` | yes | string | — | The `matter_key` copied verbatim from a change |
| `include_all` | no | boolean | `false` | Also include the routine record (mostly bot dependency bumps) |

### Example call

```json
{ "matter_key": "containerd/advisory:cve-2026-47262" }
```

### Measured response (trimmed)

```json
{
  "matter_key": "containerd/advisory:cve-2026-47262",
  "project": "containerd",
  "family": "security",
  "occurrences": [
    {
      "change_id": "containerd:v2.2.5:a74c2b48",
      "version": "v2.2.5", "version_rank": [ 2, 2, 5 ],
      "released_at": "2026-06-18T23:11:33.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "defect_corrected",
      "advisories": [
        { "id": "CVE-2026-47262", "severity": "medium" },
        { "id": "CVE-2026-50195", "severity": "medium" },
        { "id": "CVE-2026-53488", "severity": "critical" },
        { "id": "CVE-2026-53489", "severity": "high" },
        { "id": "CVE-2026-53492", "severity": "high" },
        { "id": "GHSA-33vj-92qq-66hc", "severity": "high" },
        …4 more…
      ],
      "source_url": "https://github.com/containerd/containerd/releases/tag/v2.2.5", …
    },
    { "version": "v2.3.2",  "advisories": [ …10 ids… ], … },
    { "version": "v2.0.10", "advisories": [ "CVE-2026-47262", "CVE-2026-53488", "GHSA-jpcc-p29g-p8mq", "GHSA-xhf5-7wjv-pqxp" ], … },
    { "version": "v1.7.33", "advisories": [ …4 ids… ], … },
    { "version": "v2.1.9",  "advisories": [ "CVE-2026-47262", "GHSA-jpcc-p29g-p8mq" ], … }
  ],
  "includes_notes": false
}
```

This is the case the tool exists for. One containerd roll-up landed on five
branches, and the branches carry **ten, ten, four, four, and two** advisory
ids respectively. Someone on v2.1.x reading only the v2.2.5 entry would
credit their branch with eight fixes it did not receive. `occurrences` is
ordered oldest-first, so the entry whose `version` is the nearest one above
what you run is the one to upgrade to.

`includes_notes` echoes whether the routine record was included, so a caller
can tell an empty tail from an omitted one.

### Example prompts

> "We're on containerd 2.1.8 — is that containerd CVE roll-up fixed for us,
> and in which release?"

The agent takes the `matter_key` from whatever surfaced the issue —
`check_stack` or `changes_by_entity` — calls `get_matter`, and answers from
the occurrence on the 2.1 branch rather than from the newest one.

## list_changes

The incremental sync feed. It flows in ascending `seq` order —
oldest-analyzed first — so the first page is **not** the newest data. The
tool exists to keep a local copy up to date. For "what is the latest release
of X" or "recent releases of X", use `list_releases` or `get_release`
instead.

The routine record is excluded; the feed carries only operator-facing
changes.

### Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `project` | no | string | *(none = all)* | Project slug filter (e.g. `envoy`) |
| `family` | no | string | *(none = all)* | `security`, `breaking`, or `deprecated` |
| `bucket` | no | string | *(none = all)* | `action`, `check`, or `plan` |
| `since` | no | integer | *(none = from the beginning)* | Cursor: only changes with a `seq` greater than this. Pass the previous response's `next_since` |
| `limit` | no | integer | `50` | Page size. Maximum `200` |

### Example call

```json
{ "family": "security", "bucket": "action", "limit": 3 }
```

### Measured response (trimmed)

```json
{
  "changes": [
    {
      "change_id": "argo:v3.5.0:c140f331",
      "matter_key": "argo/dependency/formidable#value_changed",
      "project": "argo", "version": "v3.5.0", "version_rank": [ 3, 5, 0 ],
      "released_at": "2026-08-04T08:35:57.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [],
      "subjects": [ { "kind": "dependency", "name": "formidable", "name_full": "formidable", "role": "changed" } ],
      "window": null, "transition": null, "remedy": null, "symptom": [],
      "quote": "chore(deps): update dependency formidable to v2.1.3 [security]",
      "disclosure": "undisclosed",
      "source_url": "https://github.com/argoproj/argo-cd/releases/tag/v3.5.0",
      "release_url": "https://ratatosk.io/en/releases/argo/v3.5.0",
      "seq": 1415
    },
    { "change_id": "backstage:v1.50.0:22fe2851", "project": "backstage", "version": "v1.50.0",
      "released_at": "2026-04-14T17:49:58.000Z", "family": "security", "bucket": "action",
      "disclosure": "described", "seq": 1464, … },
    { "change_id": "backstage:v1.50.0:fc3cb011", "project": "backstage", "version": "v1.50.0", "seq": 1465, … }
  ],
  "next_since": 1465
}
```

The next page is
`{ "family": "security", "bucket": "action", "limit": 3, "since": 1465 }`.
Repeat until `next_since` comes back `null`, which means you are caught up —
do not send `since: null`, that is a `400`. A short page always ends the
walk: `next_since` is only non-null when the page came back full.

Note that `seq` is an analysis cursor, not a timeline. The first page above
holds an argo release from August and a backstage release from April,
because they were analyzed in that order. Sort by `released_at` if you want
chronology.

`disclosure: "undisclosed"` on the argo entry says the note flagged the bump
as security-relevant without naming an advisory — which is why `advisories`
is empty on a `security` / `action` change.

### Example prompts

> "Keep our local copy of ratatosk's data current."

The agent stores the last `next_since` it saw and resumes from there,
walking pages until `next_since` is `null`.

## Privacy and rate limits

In short: the `check_stack` version comparison happens inside the server
process, so if you self-host, running versions never leave your
infrastructure. The full boundary and log handling are covered in
["How your component versions are handled" in the README](../README.md#how-your-component-versions-are-handled).

Two limits apply, in different units. The public API allows **1200 requests
per minute per IP** — that is what a self-hosted server draws on. The hosted
endpoint additionally allows **60 tool calls per minute per caller**, counted
per caller rather than pooled.

One tool call is not one request: every tool here costs a single upstream
request except `check_stack`, which costs one per component (plus one more for
each component that turns out to have no changes). A fifteen-component
`check_stack` is therefore one tool call and roughly fifteen requests — still
far cheaper than fifteen separate `list_changes` calls, which is the reason to
bundle stack questions into one call rather than polling per project. For
heavy or automated use, self-host.

The data is AI-extracted from official release notes. Verify critical
decisions against the `source_url` every change carries
([terms](https://ratatosk.io/terms)).
