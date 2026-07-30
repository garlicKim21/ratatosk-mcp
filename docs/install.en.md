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
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml
kubectl apply -f $BASE/ratatosk-agent.yaml
```

No clone needed for this route. The manifests carry `namespace: kagent`.

Either way, `ratatosk-agent` appears in the kagent UI. Ask it:

> "Anything in this cluster that needs action before we upgrade?"

The agent finds running versions itself via kagent's built-in read-only
cluster tools (disable with `kagent.agent.k8sTools=false`), or you can name
the versions in the question.

> **Model minimum**: one agent run makes 6+ internal model calls and the
> kagent Go ADK does not retry on 429, so a model tier below ~10 requests
> per minute fails every run (measured: gemini free tiers at 5 RPM never
> completed a single run). On free tiers, prefer the **flash-lite line** —
> as of 2026-07 every full-flash gemini free tier sits at 5 RPM while the
> flash-lite tiers carry agent-viable rate limits (measured across a
> 60-run campaign).

> **Known issue in kagent 0.9.12 itself** (not this chart): switching the
> agent runtime to the Go ADK hits ImagePullBackOff, because the Go ADK image
> is published to `ghcr.io` only while the controller still defaults to the
> retired `cr.kagent.dev` registry (kagent [#2247], fixed after 0.9.12).
> Workaround on the kagent install:
> `--set controller.agentImage.registry=ghcr.io`.

[#2247]: https://github.com/kagent-dev/kagent/issues/2247

## Configuration

| Env | Default | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | Upstream facts API (`/v1`, public, read-only) |
| `MCP_HTTP_ADDR` | *(empty)* | When set (e.g. `:8080`), serve streamable HTTP instead of stdio |
| `MCP_HTTP_STATELESS` | *(off)* | `1` serves HTTP without per-session state — no `Mcp-Session-Id`, and the only HTTP mode that speaks the 2026-07-28 protocol revision (chart: `statelessHttp`) |
| `MCP_LOG` | `info` | `debug` adds per-call timing. Logs are one-line JSON on stderr and carry **no request arguments at any level** — error text is reconstructed, never copied (chart: `logLevel`) |
| `MCP_AUDIT` | *(off)* | `metadata` emits one `event:"audit"` JSON line per tool call (tool, outcome, caller clientInfo, argument *names*); `full` adds argument values (chart: `auditMode`) |

**Audit stream, plainly**: it runs inside your cluster and lands in your
collectors — the data never leaves your perimeter, which is the point.
Retention and tamper-evidence are your log platform's job; route on
`event=audit`. And one honest boundary: this server has no authentication,
so "caller" means the self-reported `clientInfo` plus what the transport
shows. The record attests *which client called which tool with which
arguments* — which human prompted the agent is knowledge only the agent
layer holds, and no MCP-layer record can manufacture it. A caller-sent
`traceparent` is stamped on the record as `trace_id`, the join key across
those layers.

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
