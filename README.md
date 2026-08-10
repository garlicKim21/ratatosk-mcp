<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="Ratatosk, a messenger squirrel with a scroll in its bag, waving hello">

# ratatosk-mcp

**Ratatosk reads CNCF release notes every hour. Your agents get the changes.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)
[![Glama score](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp/badges/score.svg)](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp)


</div>

---

In Norse myth, Ratatoskr is the squirrel that carries messages up and down the
world tree. This one carries release intelligence. [ratatosk.io](https://ratatosk.io)
watches 76 CNCF projects and turns every release note into typed, entity-level
changes: security fixes, breaking changes, removals, deprecations, changed
defaults — each one classified by how you should act on it now. Routine lines
are recorded too, but kept out of the way.

This repository is the MCP server that hands those changes to your agent as
tools. MCP (Model Context Protocol) is the open standard AI agents use to call
external tools; any MCP-capable client — Claude Code, Claude Desktop, kagent,
your own SDK agent — can connect. No account, no API key.

## Two ways to connect

**Hosted — nothing to install.** Register `https://ratatosk.io/mcp` as a
remote MCP server in any client that supports remote connectors. The hosted
endpoint allows each caller 60 tool calls a minute, counted per caller rather
than pooled — for polling or CI workloads, self-host. With the Claude Code
CLI:

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

**Self-hosted — the same server, running as your own process.** Run it with
Docker, the Helm chart, or a source build. With Claude Code and Docker
installed:

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.3
```

`0.7.3` is the current release; use `latest` to follow new ones.

Either way, verify the connection:

```bash
claude mcp list
# ratatosk: … - ✔ Connected
```

Then ask your agent a question the tools can answer:

> **You:** "We run envoy v1.36.8 and istio 1.30.1. Anything we must do before upgrading?"
>
> **Your agent** calls `check_stack` and answers from the record: the CVEs
> fixed after your version, the APIs removed on your upgrade path, the defaults
> that changed — separated into what applies to everyone and what applies only
> if your configuration matches. Each change carries a verbatim quote from the
> release notes as evidence.

Other clients (Claude Desktop, kagent, in-cluster agents) and the full setup
reference: see the [install guide](docs/install.en.md).

## Tools

Two things the tools speak in. A **change** is one thing a release did, taken
from an official release note and tied to the exact identifiers it touches
(a CVE id, a flag, a CRD, a config field), with a verbatim quote as evidence.
Every change carries three axes:

- **family** — `security`, `breaking`, or `deprecated`: what kind of thing it is.
- **bucket** — `action` (applies to everyone), `check` (only if `applies_if`
  matches your setup), `plan` (announced for later), `other` (the full record).
- **applies_if** — a boolean expression you can evaluate against your own
  manifests, not prose to read.

A **matter** is the issue underneath, identified by `matter_key` and stable
across releases and branches: the same security roll-up landing on five
branches shares one key. Severity lives on the cited advisories and is read
from the ledger's current value, not frozen at analysis time.

| Tool | What it does |
|---|---|
| `check_stack` | Takes the component versions you run and returns the changes on your upgrade path, split by bucket: `action_required` applies to everyone, `check_config` only if its `applies_if` holds. The comparison happens inside the server process — which is your own process when you self-host ([how your component versions are handled](#how-your-component-versions-are-handled)) |
| `list_changes` | The incremental change feed, oldest-analyzed first. Filter by project, family, or bucket; page with the `since` cursor to keep a local copy in sync |
| `changes_by_entity` | Reverse lookup: every change touching one exact identifier — such as a CVE id, CRD, feature gate, flag, config field, or dependency |
| `get_matter` | Every release in which one matter appeared. The same roll-up lands on several branches carrying different advisories — told only the newest, you would assume you were covered |
| `get_release` | One release in full: its changes, a summary, and the link to the original note. A release with zero changes means it was read and found routine |
| `list_releases` | The newest releases of one project as one-line summaries (dates, counts by bucket and family, highest advisory severity), newest first — the tool for "what changed in X lately" |
| `list_projects` | The roster of tracked projects and their canonical slugs (the short project id every other tool takes) — look names up here instead of guessing |

Full per-tool parameters, example calls, and measured responses live in the [tools reference](docs/tools.en.md).

## How your component versions are handled

**Self-hosted:** `check_stack` sends only project slugs to the server and
compares versions locally, inside this process — the versions you pass it
never reach ratatosk.io. The server publishes changes; your agent decides what
applies. The version normalizer is bundled (`internal/version`), so range
comparison happens client-side too. This holds for upgrade questions as well:
the upstream API has a convenience endpoint (`/v1/upgrade/{project}`) that
receives caller-supplied versions — `check_stack` does not call it; the
comparison is in the source you can read.

One limit on that guarantee: it covers `check_stack`. Tools that take a
version as an argument — `get_release(project, version)` — put that version
in the upstream request path, because fetching a specific release means
naming it. That named path is not kept on my side, though: before a log line
is written, query strings are stripped and `/v1/releases/…` and
`/v1/upgrade/…` paths are reduced to their prefix, so neither the slug nor
the version lands in a log.

**Hosted:** your `check_stack` arguments (the versions you run) pass through
the server's memory to produce the same answer, and are not written down.
Here is what each layer on the way keeps:

- The hosted MCP process itself logs only its startup line — a normal
  request adds nothing.
- The upstream API's request log writes one line only when the caller sends a
  `traceparent`, and that line carries a normalized endpoint label and the
  trace id — never a path, query, or body.
- The front-door access log strips query strings, reduces `/v1/releases/…`
  and `/v1/upgrade/…` paths to their prefix, masks caller IPs, and has no
  field for request bodies.

The hosted endpoint runs with its audit stream off, and I keep it off — not
recording request content is the operating stance for that endpoint. One
boundary I do not control: connection metadata on the CDN leg
follows the CDN provider's own policy. If your requirements rule out that transit,
self-host: then only project slugs leave your infrastructure on a
`check_stack` call.

Self-hosting adds the opposite capability: an opt-in audit stream
(`MCP_AUDIT=metadata` or `full`) that records who called which tool, emitted
inside your own infrastructure into your own collectors. The hosted endpoint
has none by design. Details in the [install guide](docs/install.en.md).

## Documentation

- **[Install & usage](docs/install.en.md)** — hosted endpoint · local stdio · in-cluster (Helm) · kagent ([한국어](docs/install.ko.md) · [日本語](docs/install.ja.md))
- **[Helm chart](charts/ratatosk-mcp/README.md)** — values, kagent toggle ([한국어](charts/ratatosk-mcp/README.ko.md) · [日本語](charts/ratatosk-mcp/README.ja.md))
- **[kagent example](examples/kagent/README.md)** — manifests + ratatosk-agent ([한국어](examples/kagent/README.ko.md) · [日本語](examples/kagent/README.ja.md))
- **[Contributing](CONTRIBUTING.md)** · **[Security policy](SECURITY.md)**

## Upstream API

This server is a thin client over the public REST API. If you would rather
call it directly, `GET /v1` on ratatosk.io describes itself. No API key; rate
limited at 1200 requests per minute per IP.

## Data & terms

The data is served free of charge by ratatosk.io — a term that may change,
with advance notice — under its [terms of service](https://ratatosk.io/terms).
Analyses are AI-generated reference information with no warranty — check the
original release notes before acting, especially when an agent acts on your
behalf. Original notes belong to their respective projects; responses that
carry a full note include an attribution notice (`raw_notes_notice`).

## License

The code in this repository is licensed under [Apache-2.0](LICENSE).
