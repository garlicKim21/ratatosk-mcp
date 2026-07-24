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

## Quick start

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

No account, no API key. Running agents in Kubernetes, or using kagent?
See the install guide below.

## Documentation

| | |
| --- | --- |
| **[Install & usage](docs/install.en.md)** — local stdio · in-cluster (Helm) · kagent | [한국어](docs/install.ko.md) · [日本語](docs/install.ja.md) |
| **[Helm chart](charts/ratatosk-mcp/README.md)** — values, kagent toggle | |
| **[kagent example](examples/kagent/README.md)** — manifests + release-triage agent | |
| **[Contributing](CONTRIBUTING.md)** · **[Security policy](SECURITY.md)** | |

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
