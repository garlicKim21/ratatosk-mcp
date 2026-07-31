# Install & usage

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

This page covers the four ways to use Ratatosk's release facts from an AI
agent — from connecting to the hosted endpoint with nothing to install, to a
stdio server on your laptop, a Helm deployment inside a Kubernetes cluster,
and the kagent integration.

**MCP (Model Context Protocol)** is the standard protocol AI agents use to
call external tools. ratatosk-mcp is a server that speaks this protocol and
provides six release-fact tools — `check_stack` (takes the component versions
you run and returns the facts on your upgrade path), `list_facts` (the
incremental fact feed), `facts_by_entity` (reverse lookup by identifier —
CVE ids, flags, and so on), `get_release` (one release in full),
`list_releases` (recent releases of one project as summaries),
`list_projects` (tracked projects and their canonical slugs) — and any
MCP-capable client can connect: Claude Code, Claude Desktop, kagent, your
own SDK agent.

## Choose how to run it

| Your situation | Use | Section |
| --- | --- | --- |
| Just want to try it — installing nothing | Register the hosted endpoint URL | [Hosted](#hosted-endpoint) |
| On a laptop with Claude Code / Claude Desktop / any stdio MCP client | `docker run` (stdio) | [Local](#local-stdio) |
| Running your own agents in Kubernetes (any framework, CI jobs, SDK clients) | Helm chart | [In-cluster, standalone](#in-cluster-standalone-helm) |
| Using [kagent](https://kagent.dev) | `kagent.enabled=true` Helm toggle, or plain manifests | [With kagent](#with-kagent) |

All four modes serve the same public data through the same six tools, and no
mode needs an account or an API key. Three things differ: how far the running
versions you pass to `check_stack` travel (when self-hosted, the version
comparison finishes inside your own process, so the versions never leave your
infrastructure; when hosted, they pass through server memory but are written
to no log), who you share the upstream request limit with (hosted uses a shared
bucket; self-hosted gets its own IP's allowance), and whether you can keep an
audit stream (self-hosted only).

One thing to know is common to all modes:
tools that take a specific version as an argument, like `get_release`, put
that version in the upstream request path in every mode — it is the value
that names what to fetch. Even that path does not land in server-side logs,
though: the access log strips query strings and reduces `/v1/releases/…` and
`/v1/upgrade/…` paths to their prefix before writing. Each section below has
the details.

## Prerequisites

- **Hosted**: any client that supports remote MCP servers (streamable HTTP).
  Nothing else.
- **Local (stdio)**: Docker. Go 1.26 or later to build from source.
- **The `claude mcp add` commands in the examples**: the
  [Claude Code](https://claude.com/claude-code) CLI (the `claude` command)
  must be installed. Not required if you use another MCP client — follow its
  own registration method instead.
- **In-cluster (Helm)**: a Kubernetes cluster, Helm 3, and a clone of this
  repository. The chart is not published to a chart repository or an OCI
  registry; it lives only in the repo.
- **Outbound HTTPS (egress)**: in any self-hosted mode, the server process
  must be able to open outbound HTTPS connections to `ratatosk.io:443`. If
  your cluster restricts egress with NetworkPolicy or an egress proxy, add
  this destination to the allowlist. If you have to go through a proxy or a
  mirror, `RATATOSK_URL` (chart value: `ratatoskUrl`) changes the upstream
  address. Running while blocked makes tools return errors or empty results
  (`check_stack` reports `fetch failed: …` in each component's `note` field
  instead of erroring) — see [Troubleshooting](#troubleshooting).
- **kagent integration**: a cluster with kagent already installed (kagent
  CRDs included).

## Hosted endpoint

`https://ratatosk.io/mcp` — a remote MCP endpoint you use right away, with
nothing to install. It is served over streamable HTTP and is stateless: there
are no sessions, so each request is handled independently, without an
`Mcp-Session-Id` round trip.

### Register

Claude Code:

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

In Claude (web and desktop), paste `https://ratatosk.io/mcp` into the URL
field of the remote connector settings. For other clients, follow their own
way of registering a remote MCP server.

### Verify

You can check that the endpoint is alive with curl, no client needed:

```bash
curl -s -X POST https://ratatosk.io/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}'
```

The response comes with SSE (Server-Sent Events) framing — a `Content-Type`
of `text/event-stream` is not a failure. If the JSON on the `data:` line
shows `"serverInfo"` with `"name":"ratatosk"`, all is well:

```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{…,"serverInfo":{"name":"ratatosk","version":"0.6.1"}}}
```

With Claude Code, check the `claude mcp list` output:

```
ratatosk: https://ratatosk.io/mcp (HTTP) - ✔ Connected
```

### Good to know

- **GET for SSE returns 405**: stateless mode serves no server-initiated
  notification stream (the SSE opened with GET), so a 405 response is normal
  behavior. Opening the URL in a browser redirects to an explainer page
  (`/docs/mcp`).
- **Fair use**: the hosted path shares the upstream public API's rate-limit
  bucket (60 requests/minute per IP) with its other users (a separate bucket
  from the per-IP allowance you get when calling `/v1` directly). If you need
  heavy use like polling, switch to self-hosting.

### Privacy — where request content goes on the hosted endpoint

On the hosted endpoint, your `check_stack` arguments (the list of versions
you run) pass through server memory. In return, request content is recorded
nowhere:

- The MCP server log keeps only the startup line and upstream error lines;
  normal requests are not logged. Error lines carry no request arguments
  either.
- The upstream API's request log writes one line only when the caller sends a
  `traceparent`, and even that line holds only a normalized endpoint label
  and the trace_id — paths and queries are not recorded.
- The access log in front of the server strips query strings, reduces the
  `/v1/releases/…` and `/v1/upgrade/…` paths (where slugs and versions ride)
  to their prefix, masks IPs, and has no field for request bodies at all. It
  is not shipped anywhere and rotates locally by size (a few days' worth at
  current traffic).
- The audit stream described below is off on the hosted endpoint, and stays
  off.

One boundary is out of my control: connection metadata on the CDN
(Cloudflare) leg follows Cloudflare's policy, which ratatosk.io cannot
control. If that boundary does not meet your requirements, self-host — the
versions you pass to `check_stack` then never leave your process.

## Local (stdio)

stdio is the mode where an MCP client spawns the server as a child process
and talks to it over standard input/output. It is the default path on a
laptop with Claude Code or Claude Desktop.

### Claude Code

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.6.1
```

In place of `0.6.1` you can use any
[release tag](https://github.com/garlicKim21/ratatosk-mcp/releases), or
`latest` to always track the newest release — see
[Version pinning](#version-pinning).

### Claude Desktop

Claude Desktop registers MCP servers through a config file
(`claude_desktop_config.json`) instead of a CLI. Add this under
`mcpServers`:

```json
{
  "mcpServers": {
    "ratatosk": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "ghcr.io/garlickim21/ratatosk-mcp:0.6.1"]
    }
  }
}
```

The config file's location differs per operating system; see the Claude
Desktop documentation. To use it with no install at all, you can also
register the [hosted endpoint](#hosted-endpoint) above as a remote connector.

### Build from source

To use a binary without Docker:

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

Any MCP client that speaks stdio works the same way.

### Verify

```bash
claude mcp list
# ratatosk: docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.6.1 - ✔ Connected
```

Then ask your agent:

> "We run envoy v1.36.8 and istio 1.30.1 — anything we should act on before
> upgrading?"

Success looks like the agent calling `check_stack` and answering with the
security fixes, removals, and changed defaults past your versions, evidence
quotes included.

## In-cluster, standalone (Helm)

Set `MCP_HTTP_ADDR` and the same binary serves MCP over streamable HTTP at
`/mcp` (with a `/healthz` health check). The chart deploys this as a
ClusterIP Service and sets `MCP_HTTP_ADDR` automatically to match
`service.port` (default 8080). Anything in the cluster connects to a URL
instead of spawning a process — your own SDK-built agents, other agent
frameworks, even a CI job that gates upgrades on `check_stack`.

### Install

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

No RBAC, no secrets, and it runs without warnings under the Pod Security
Standards (PSS) `restricted` profile. All chart options are covered in the
[chart README](../charts/ratatosk-mcp/README.md). Turn on
`--set statelessHttp=true` in two cases: to serve current clients that use
the 2026-07-28 revision of the MCP spec over HTTP (clients that send
`Mcp-Protocol-Version: 2026-07-28`), and before scaling to two or more
replicas — see the [Configuration reference](#configuration-reference).

### Verify

Check that the pod is up:

```bash
kubectl get pods -l app.kubernetes.io/name=ratatosk-mcp
# NAME                            READY   STATUS    RESTARTS   AGE
# ratatosk-mcp-6f7b9c8d4-x2m5q    1/1     Running   0          30s
```

One startup log line tells you the serving address and the upstream:

```bash
kubectl logs deploy/ratatosk-mcp
# {"time":"…","level":"INFO","msg":"listening","service":"mcp","transport":"http","addr":":8080/mcp","mode":"stateful","upstream":"https://ratatosk.io","version":"0.6.1"}
```

To check health as well:

```bash
kubectl port-forward svc/ratatosk-mcp 8080:8080 &
sleep 2   # curl gets connection refused if it runs before the forward is ready
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
```

When you are done, clean up the backgrounded port-forward with `kill %1`.

### Connect

Point MCP clients inside the cluster at:

```
http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

### Upgrade

```bash
git -C ratatosk-mcp pull
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

If you changed values with `--set` at install time, pass the same values
again on upgrade, or add `--reuse-values`.

## With kagent

Two equivalent routes — pick exactly one:

**A. Helm toggle** — one install brings the server, the kagent registration
(RemoteMCPServer), and the example agent `ratatosk-agent`:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` defaults to `default-model-config`. Override it if your
environment differs; if you want the registration without the example agent,
set `kagent.agent.enabled=false`.

**B. Plain manifests** — the same three pieces as copy-paste files, no Helm
([details](../examples/kagent/README.md)):

```bash
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml
kubectl apply -f $BASE/ratatosk-agent.yaml
```

No clone is needed on this route. The manifests carry `namespace: kagent`.

### Verify

Either way, `ratatosk-agent` appears in the kagent UI. Ask it:

> "Anything in this cluster that needs action before we upgrade?"

The agent finds the running versions itself through kagent's built-in
read-only cluster tools (disable with `kagent.agent.k8sTools=false`). You can
also name the versions in the question. If the agent does not appear in the
UI, see [Troubleshooting](#troubleshooting).

> **Model minimum**: one agent run makes six or more internal model calls,
> and the kagent Go ADK does not retry 429 (rate-limited) responses. You
> therefore need a model tier that allows roughly 10+ requests per minute;
> below that, every run fails. For example, on the Gemini free tier (as of
> 2026-07) the full-flash lines at 5 requests per minute never complete a
> run — only the flash-lite lines carry limits an agent can live with.

## Configuration reference

Every setting that changes server behavior is an environment variable, and
the Helm chart provides a value for each. The chart also has values that
shape the deployment itself — replica count, resources, service type — see
the [chart README](../charts/ratatosk-mcp/README.md).

| Env | Chart value | Default | What it does |
|---|---|---|---|
| `RATATOSK_URL` | `ratatoskUrl` | `https://ratatosk.io` | Upstream facts API (`/v1`, public, read-only). Change it to route egress through a proxy or a mirror |
| `MCP_HTTP_ADDR` | *(set automatically by the chart from `service.port`)* | *(empty = stdio)* | When set (e.g. `:8080`), serve streamable HTTP at `/mcp` instead of stdio, with `/healthz` |
| `MCP_HTTP_STATELESS` | `statelessHttp` | *(off)* | `1` serves HTTP without session state: no `Mcp-Session-Id` round trip, and required to serve current clients that use the 2026-07-28 MCP spec revision over HTTP. Also recommended for horizontal scaling to two or more replicas. Older clients work either way |
| `MCP_LOG` | `logLevel` | `info` | Accepted values `info` (default), `debug`, `warn`, `error` — unrecognized values fall back to `info` without a warning. `debug` adds per-call upstream timing; `warn` and `error` reduce output. No level records request arguments — see [Logs and the audit stream](#logs-and-the-audit-stream) |
| `MCP_AUDIT` | `auditMode` | *(off)* | `metadata` or `full` — the tool-call audit stream. See [Enabling the audit stream](#enabling-the-audit-stream) |

Running HTTP mode directly with Docker:

```bash
docker run --rm -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:0.6.1
```

## Logs and the audit stream

### Operational logs

Logs are one-line JSON on stderr (stdout is reserved for the stdio
transport). At the default level (`info`) you get lifecycle lines (one
`listening` line at start, one line when a stdio session ends) plus
warn/error lines when the upstream connection is unhealthy. A normal request
logs nothing at all. Note that the stdio session end line comes in two
shapes: `"msg":"stdio session ended"` (INFO) when the client cleans up and
disconnects, `"msg":"stdio session ended with error"` (ERROR) when it drops
without cleanup — the latter records how the session ended, not a failure.
On errors, the original error message is never copied — request URLs can
carry the running versions — so the log text is reconstructed from just the
endpoint pattern and the kind of error:

```json
{"time":"…","level":"ERROR","msg":"upstream fetch failed","service":"mcp","upstream":"/v1/facts","kind":"connection_refused","tool":"check_stack"}
```

Raising to `MCP_LOG=debug` adds one line per upstream call with the endpoint
pattern, status code, and duration. **No level puts request arguments
(versions and the like) in the log.**

### Enabling the audit stream

The audit stream is a separate record of which client called which tool.
**It is off by default, and while off, audit records are zero bytes** — the
stream does not exist.

Enable it with `MCP_AUDIT`:

```bash
# docker
docker run -i --rm -e MCP_AUDIT=metadata ghcr.io/garlickim21/ratatosk-mcp:0.6.1

# Helm
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp --set auditMode=metadata
```

When on, every tool call emits one `event:"audit"` JSON line on the same
stream as the operational logs (stderr):

```json
{"argument_names":["components","detail"],"client_name":"my-agent","client_version":"1.0.0","event":"audit","level":"INFO","msg":"audit","outcome":"ok","service":"mcp","time":"2026-07-31T02:54:47.029169122Z","tool":"check_stack","transport":"stdio"}
```

Since 0.6.1, audit records on the HTTP transport also carry a `session_id`
field — the transport session identifier (e.g.
`"session_id":"T3E77BYZFDDA33SIUSORQ365ZL"`) — which separates concurrent
callers by session, when the server runs in stateful HTTP mode (the chart
default). With `statelessHttp` (env `MCP_HTTP_STATELESS`) on there is no
session to name, so `session_id` and the client's self-reported `clientInfo`
are absent from the record, and `trace_id` (below) is the only per-call join
key in that mode — if the audit stream must separate callers, run stateful,
or have callers send `traceparent`. stdio records like the example above have
no `session_id` either: one process serves one caller, so there is nothing
to separate.

The two modes differ as follows:

- **`metadata`** — the tool name, the outcome (`ok`, `error`, `tool_error`),
  the transport, the client's self-reported `clientInfo` (on stdio and
  stateful HTTP), and the **names** of the arguments only. Argument values
  are never recorded in this mode.
- **`full`** — all of the above plus the full argument values (the
  `arguments` field). The version list you pass to `check_stack` lands here
  too, so decide whether that value may live in your log system before
  turning it on.

The stream is produced inside your infrastructure and lands in your own log
collectors. Retention and tamper-evidence are your log platform's job;
routing records whose `event` field is `audit` to a separate sink gives them
a retention policy of their own.

Know the limit, too: this server has no authentication, so the "caller" in a
record goes as far as the client's self-reported `clientInfo` and whatever
the transport shows. Which human prompted the agent to make that call is
knowledge you have to find in the agent layer's records.

The hosted endpoint has no audit stream — it is always off, in line with the
operating stance of not retaining request content. If you have audit
requirements, self-host.

### Trace correlation (traceparent)

If your agent framework supports W3C trace context (the request-identity
header of the distributed-tracing standard), it can send `traceparent` in
the `_meta` of a tool call:

```json
{"method":"tools/call","params":{"name":"check_stack","arguments":{"components":[…]},"_meta":{"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}}}
```

The log lines that call produces (errors, `debug` level) and its audit
record are then stamped with a `trace_id` field, and the same `traceparent`
header is forwarded on the upstream `/v1` request — one `trace_id` connects
agent → MCP server → upstream. This is the join key across the layer limit
noted above — the link an audit record alone cannot make. Malformed values
are dropped and not forwarded; send nothing and nothing is recorded.

## Troubleshooting

| Symptom | Check | Cause and fix |
|---|---|---|
| Tool calls return errors (`check_stack` instead reports `fetch failed: …` in each component's `note`); the log shows `"msg":"upstream fetch failed"` with `kind: connection_refused`, `dns`, or `timeout` | `kubectl logs deploy/ratatosk-mcp` (locally: your client's MCP logs) | Egress is blocked. Allow outbound HTTPS to `ratatosk.io:443`, or set up a mirror and point `RATATOSK_URL` (chart: `ratatoskUrl`) at it |
| The log shows `"msg":"upstream rate limited"`, `status: 429` | Same as above | Upstream limit exceeded (60 requests/minute per IP). Replace per-project `list_facts` polling with a single `check_stack` call, and retry after a pause |
| An HTTP client gets `400 Bad Request: protocol version "2026-07-28" is only supported on stateless HTTP servers` | The `Mcp-Protocol-Version` header the client sends | The client speaks the 2026-07-28 MCP revision, which stateful HTTP mode rejects. Turn on `--set statelessHttp=true` (env: `MCP_HTTP_STATELESS=1`) — the `StreamableHTTPOptions.Stateless` named in the error is the Go SDK's name for the same switch |
| `ratatosk-agent` does not appear in the kagent UI | `kubectl api-resources \| grep kagent` · `kubectl get pods -n kagent` | Installing with `kagent.enabled=true` fails on a cluster without the kagent CRDs — install kagent first. The manifest route hardcodes `namespace: kagent`, so if kagent lives in another namespace, edit the manifests |
| `ImagePullBackOff` after switching the kagent agent runtime to the Go ADK | `kubectl describe pod` | A known issue in kagent 0.9.12 itself, unrelated to this chart ([#2247], fixed after 0.9.12): the Go ADK image is published only to `ghcr.io` while the controller default points at the retired `cr.kagent.dev`. Workaround: `--set controller.agentImage.registry=ghcr.io` on the kagent install |
| The kagent agent fails every run | 429 in the agent logs | The model tier's request limit is too low — see the model minimum under [With kagent](#with-kagent) |

[#2247]: https://github.com/kagent-dev/kagent/issues/2247

## Version pinning

- **Container image**: built automatically for every release under its
  version tag (e.g. `0.6.1`), multi-arch (`linux/amd64`, `linux/arm64`).
  `latest` tracks the newest release — pin a version tag for reproducible
  deployments.
- **Helm**: the chart is not published to a chart repository; you install
  from a clone. To pin a specific version, check out the release tag after
  cloning:

  ```bash
  git -C ratatosk-mcp checkout v0.6.1
  ```

  The image tag follows the chart's `appVersion` by default and can be
  overridden with the `image.tag` value.

## Upstream API

This server is a thin client over the public REST API. If you would rather
call it directly, `GET /v1` on ratatosk.io describes itself. No API key; rate
limited at 60 requests per minute per IP.

## Next steps

- The six tools in detail: [the tools table in the README](../README.md#tools)
- Full chart values and the kagent toggle: [chart README](../charts/ratatosk-mcp/README.md)
- kagent manifests and the example agent: [kagent example](../examples/kagent/README.md)
