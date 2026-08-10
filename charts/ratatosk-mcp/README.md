# ratatosk-mcp Helm chart

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

Deploys the ratatosk MCP server in streamable-HTTP mode as a ClusterIP
Service. No RBAC, no secrets; runs cleanly under the Pod Security Standards
(PSS) `restricted` profile.

```bash
# from the repo root
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → MCP endpoint: http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Values

| Key | Default | Meaning |
| --- | --- | --- |
| `replicaCount` | `1` | Number of replicas; turn on `statelessHttp` before going above 1 |
| `image.repository` | `ghcr.io/garlickim21/ratatosk-mcp` | Image |
| `image.tag` | chart `appVersion` | Pin a specific server version |
| `ratatoskUrl` | `https://ratatosk.io` | Upstream changes API; point at a mirror if you proxy egress |
| `statelessHttp` | `false` | Serve HTTP without per-session state — needed for clients that use the 2026-07-28 MCP revision and for more than one replica |
| `logLevel` | `""` (= info) | `MCP_LOG`: `debug`, `warn`, `error` also accepted; any other value silently falls back to info, and no level records request arguments |
| `auditMode` | `""` (= off) | `MCP_AUDIT`: `metadata` records one line per tool call (argument names only), `full` adds argument values |
| `service.port` | `8080` | Service / MCP endpoint port |
| `resources` | small requests/limits | Adjust for busy clusters |
| `nodeSelector` / `tolerations` / `affinity` | `{}` / `[]` / `{}` | Standard scheduling controls |
| `kagent.enabled` | `false` | Also install the kagent integration (below) |
| `kagent.modelConfig` | `default-model-config` | ModelConfig name in your kagent install |
| `kagent.agent.enabled` | `true` | Install the example agent (when kagent.enabled) |
| `kagent.agent.name` | `ratatosk-agent` | Example agent name |
| `kagent.agent.k8sTools` | `true` | Give the agent kagent's read-only cluster tools so it discovers running versions itself |

## kagent integration

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

Adds a `RemoteMCPServer` (kagent discovers every tool the server exposes) and a
ready-made `ratatosk-agent`. The agent also gets kagent's built-in read-only
cluster tools (`k8s_get_resources`, `k8s_get_resource_yaml`) so it can find
running versions on its own — turn this off with `kagent.agent.k8sTools=false`.
Enable only where the kagent CRDs exist.
For plain manifests, see [`examples/kagent/`](../../examples/kagent/).

## Upgrades

```bash
helm upgrade ratatosk-mcp ./charts/ratatosk-mcp
```

Values you set with `--set`/`-f` are not carried over automatically — repeat
them, or add `--reuse-values`. The image follows the chart `appVersion`
unless `image.tag` pins it.
