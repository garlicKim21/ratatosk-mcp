<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="Ratatosk, a messenger squirrel with a scroll in its bag, waving hello">

# ratatosk-mcp

**Ratatosk reads CNCF release notes every hour. Your agents get the facts.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)


</div>

---

In Norse myth, Ratatosk is the squirrel that carries messages up and down the
world tree. This one carries release intelligence. [ratatosk.io](https://ratatosk.io)
watches 74+ CNCF projects and turns every release note into typed, entity-level
facts: security fixes, breaking changes, removals, deprecations, changed
defaults. Plain bug fixes and marketing copy are filtered out. What remains is
what an operator acts on.

This MCP server hands those facts to your agent as four tools.

## What it feels like

> **You:** "We run envoy v1.36.8 and istio 1.30.1. Anything we must do before upgrading?"
>
> **Your agent** calls `check_stack` and answers from facts: the CVEs fixed
> after your version, the APIs removed on your upgrade path, the defaults that
> changed. Each fact carries a verbatim quote from the release notes as
> evidence.

## Tools

| Tool | What it does |
|---|---|
| `list_facts` | Incremental fact feed. Filter by `project`, `type`, `severity`; poll with the `since` cursor |
| `facts_by_entity` | Reverse index: every fact touching one identifier (CVE id, CRD, feature gate, flag, config field, dependency) |
| `get_release` | One reviewed release: coverage, assessment, source, and all its facts. Omit `version` for the latest reviewed release of the project. `facts: []` with `coverage: full_reviewed` means the release was read and is routine. `include_raw` adds the original note body (`raw_notes`) — automatic when the review is not the full story |
| `check_stack` | Takes the component versions you run, returns a briefing on your upgrade path: critical/high facts in full, one line each for the rest, the same advisory across release branches folded into one entry. `detail: "full"` for everything verbatim, `target_version` for one upgrade hop, `severity_min` to filter |

## <img src="docs/assets/ratatosk-face.png" width="26" alt="" align="top"> Your versions stay on your side

`check_stack` sends only project slugs to the server and compares version keys
locally, inside this process. What you run never reaches ratatosk.io. The
server publishes facts; your agent decides what applies. The version
normalizer is bundled (`internal/version`), so range comparison happens
client-side too.

## Choose how to run it

| You are… | Use | Section |
| --- | --- | --- |
| On a laptop, with Claude Code / Claude Desktop / any stdio MCP client | `docker run` (stdio) | [Local](#local-stdio) |
| Running your own agents in Kubernetes (any framework, CI jobs, SDK clients) | Helm chart | [In-cluster, standalone](#in-cluster-standalone-helm) |
| Using [kagent](https://kagent.dev) | Helm chart with `kagent.enabled=true`, or plain manifests | [With kagent](#with-kagent) |

All three run the same binary against the same public API. No account, no
API key, in any mode.

## Local (stdio)

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

Or build from source and register the binary:

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

Any MCP client that speaks stdio works the same way.

## In-cluster, standalone (Helm)

Set `MCP_HTTP_ADDR` and the same binary serves MCP over streamable HTTP at
`/mcp`, with `/healthz` for probes. The chart deploys it as a ClusterIP
Service so anything in the cluster connects to a URL instead of spawning a
process — custom agents built on an SDK, other agent frameworks, or a CI job
that gates upgrades on `check_stack`:

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
# → point MCP clients at http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

Upgrades keep your values: `helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp`.
No RBAC, no secrets; runs clean under PSS `restricted`.

## With kagent

Two equivalent routes — pick one, not both:

**A. Helm toggle** — one install brings the server, the kagent registration
(RemoteMCPServer), and a ready-made `release-triage-agent`:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` defaults to `default-model-config`; override it if your
kagent install names it differently. Set `kagent.agent.enabled=false` to
register the server without the example agent.

**B. Plain manifests** — the same three pieces as copy-paste files, no Helm:

```bash
kubectl apply -f examples/kagent/ratatosk-deploy.yaml
kubectl apply -f examples/kagent/ratatosk-remote-mcpserver.yaml
kubectl apply -f examples/kagent/ratatosk-agent.yaml
```

Either way, `release-triage-agent` appears in the kagent UI. Ask it:

> "We run kubernetes v1.36.0, cilium v1.17.18 and envoy v1.38.3 — anything
> that needs action before we upgrade?"

## Configuration

| Env | Default | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | Upstream facts API (`/v1`, public, read-only) |
| `MCP_HTTP_ADDR` | *(empty)* | When set (e.g. `:8080`), serve streamable HTTP instead of stdio |

## Container image

Multi-arch (`linux/amd64`, `linux/arm64`), built on every release tag:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## Upstream API

This server is a thin client over the public REST API. If you would rather
call it directly, `GET /v1` on ratatosk.io describes itself. No API key; rate
limited at 60 requests per minute per IP.

## Data & terms

The data is served by ratatosk.io free of charge (subject to change, with
advance notice) under its [terms of service](https://ratatosk.io/terms).
Analyses are AI-generated reference information with no warranty — check the
original release notes before acting, especially when an agent acts on your
behalf. Original notes belong to their respective projects; responses that
carry a full note include an attribution notice (`raw_notes_notice`).

## License

The code in this repository is licensed under [Apache-2.0](LICENSE).
