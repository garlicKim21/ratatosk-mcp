# ratatosk-mcp

MCP server for [Ratatosk](https://ratatosk.io) — release intelligence for the
CNCF ecosystem. Ratatosk reads the release notes of 74+ CNCF projects every
hour and extracts typed, entity-level change facts: security fixes, breaking
changes, removals, deprecations, renames, default changes. This server exposes
those facts as MCP tools, so your agent can answer "what do I need to do
before upgrading?" from data instead of guesswork.

## Tools

| Tool | What it does |
|---|---|
| `list_facts` | Incremental fact feed — filter by `project`, `type`, `severity`; cursor with `since` |
| `facts_by_entity` | Reverse index: every fact touching one identifier (CVE id, CRD, feature gate, flag, config field, dependency) |
| `get_release` | One reviewed release: coverage, assessment, source, and all its facts. `facts: []` with `coverage: full_reviewed` means the release was read and is routine |
| `check_stack` | Give it your running component versions; returns the facts from releases **newer** than what you run — your upgrade path |

### Privacy: versions never leave your side

`check_stack` fetches facts by **project slug only** and compares version keys
locally, inside this process. What you run never reaches ratatosk.io — the
server broadcasts facts, your agent decides relevance. The version normalizer
is bundled (`internal/version`) so range comparison happens client-side.

## Quick start (stdio)

```bash
go build -o ratatosk-mcp .

# Claude Code
claude mcp add ratatosk -- /path/to/ratatosk-mcp

# or any MCP client over stdio
```

## In-cluster (streamable HTTP + Helm)

Set `MCP_HTTP_ADDR` and the same binary serves MCP over streamable HTTP at
`/mcp` (plus `/healthz`):

```bash
MCP_HTTP_ADDR=:8080 ./ratatosk-mcp
```

The Helm chart deploys it as a ClusterIP Service, so in-cluster agents
(kagent, custom operators) connect to a URL instead of spawning a process:

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Configuration

| Env | Default | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | Upstream facts API (`/v1`, public, read-only) |
| `MCP_HTTP_ADDR` | *(empty)* | When set (e.g. `:8080`), serve streamable HTTP instead of stdio |

## Container image

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

Images are built for `linux/amd64` and `linux/arm64` on every release tag.

## Upstream API

Everything here is a thin client over the public REST API — `GET /v1` on
ratatosk.io is self-describing if you'd rather call it directly. No API key,
rate limited at 60 requests/minute per IP.

## License

[Apache-2.0](LICENSE)
