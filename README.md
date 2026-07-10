<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="Ratatosk, a messenger squirrel with a scroll in its bag, waving hello">

# ratatosk-mcp

**Ratatosk reads CNCF release notes every hour. Your agents get the facts.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

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
| `get_release` | One reviewed release: coverage, assessment, source, and all its facts. `facts: []` with `coverage: full_reviewed` means the release was read and is routine. `include_raw` adds the original note body (`raw_notes`) — automatic when the review is not the full story |
| `check_stack` | Takes the component versions you run, returns a briefing on your upgrade path: critical/high facts in full, one line each for the rest, the same advisory across release branches folded into one entry. `detail: "full"` for everything verbatim, `target_version` for one upgrade hop, `severity_min` to filter |

## <img src="docs/assets/ratatosk-face.png" width="26" alt="" align="top"> Your versions stay on your side

`check_stack` sends only project slugs to the server and compares version keys
locally, inside this process. What you run never reaches ratatosk.io. The
server publishes facts; your agent decides what applies. The version
normalizer is bundled (`internal/version`), so range comparison happens
client-side too.

## Quick start (stdio)

```bash
go build -o ratatosk-mcp .

# Claude Code
claude mcp add ratatosk -- /path/to/ratatosk-mcp

# or any MCP client over stdio
```

Or skip the build and use the container image:

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

## In-cluster (streamable HTTP + Helm)

Set `MCP_HTTP_ADDR` and the same binary serves MCP over streamable HTTP at
`/mcp`, with `/healthz` for probes:

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

Multi-arch (`linux/amd64`, `linux/arm64`), built on every release tag:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## Upstream API

This server is a thin client over the public REST API. If you would rather
call it directly, `GET /v1` on ratatosk.io describes itself. No API key; rate
limited at 60 requests per minute per IP.

## License

[Apache-2.0](LICENSE)
