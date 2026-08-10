# Ratatosk MCP server

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[Ratatosk](https://ratatosk.io) reads every CNCF graduated/incubating project's
release notes daily and extracts typed changes — security fixes, breaking
changes, removals, deprecations, default changes — each with a verbatim quote
and a source URL. This MCP server exposes those changes to in-cluster agents.

Privacy: the `check_stack` tool compares your running versions locally, inside
this process. Only project slugs are sent to the ratatosk server; versions
never leave your cluster. The upstream API is public, read-only, and needs no
credentials (rate limit 60 req/min per IP).

## Tools

| Tool | Purpose |
| --- | --- |
| `list_projects` | The tracked-project roster — resolve slugs here first |
| `check_stack` | Compare running versions against known changes (local comparison), split by how to act |
| `get_release` | One analyzed release with all its changes and the source URL |
| `list_releases` | The newest N releases of one project as light summaries, newest first |
| `changes_by_entity` | Reverse index: every change touching one CVE/flag/CRD |
| `get_matter` | Every release in which one matter appeared |
| `list_changes` | Incremental change feed with a `since` cursor |

## Install

```bash
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml            # Deployment + Service (no RBAC needed)
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml  # register with kagent
kubectl apply -f $BASE/ratatosk-agent.yaml             # optional: the ratatosk-agent example agent
```

The manifests carry `namespace: kagent`. Applying local copies works the same
from this directory.

Or with the Helm chart from the repository:

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp -n kagent
```

Ask the example agent things like:

> "Anything in this cluster that needs action before we upgrade?"

The agent discovers running versions itself through kagent's built-in
read-only cluster tools (`k8s_get_resources`, `k8s_get_resource_yaml` from
`kagent-tool-server`, present in a default kagent install). You can also just
tell it what you run:

> "We run kubernetes v1.36.0, cilium v1.17.18 and envoy v1.38.3 — anything
> that needs action before we upgrade?"

The agent answers with severity-ranked changes, each backed by a quote from the
release note and a link to the source.
