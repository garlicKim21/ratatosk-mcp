# Install & usage

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

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
No RBAC, no secrets; runs clean under PSS `restricted`. Chart options are
documented in the [chart README](../charts/ratatosk-mcp/README.md).

## With kagent

Two equivalent routes — pick one, not both:

**A. Helm toggle** — one install brings the server, the kagent registration
(RemoteMCPServer), and a ready-made `ratatosk-agent`:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` defaults to `default-model-config`; override it if your
kagent install names it differently. Set `kagent.agent.enabled=false` to
register the server without the example agent.

**B. Plain manifests** — the same three pieces as copy-paste files, no Helm
([details](../examples/kagent/README.md)):

```bash
kubectl apply -f examples/kagent/ratatosk-deploy.yaml
kubectl apply -f examples/kagent/ratatosk-remote-mcpserver.yaml
kubectl apply -f examples/kagent/ratatosk-agent.yaml
```

Either way, `ratatosk-agent` appears in the kagent UI. Ask it:

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
