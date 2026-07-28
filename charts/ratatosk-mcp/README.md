# ratatosk-mcp Helm chart

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

Deploys the ratatosk MCP server in streamable-HTTP mode as a ClusterIP
Service. No RBAC, no secrets; runs clean under PSS `restricted`.

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → MCP endpoint: http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Values

| Key | Default | Meaning |
| --- | --- | --- |
| `image.repository` | `ghcr.io/garlickim21/ratatosk-mcp` | Image |
| `image.tag` | chart `appVersion` | Pin a specific server version |
| `ratatoskUrl` | `https://ratatosk.io` | Upstream facts API |
| `service.port` | `8080` | Service / MCP endpoint port |
| `resources` | small requests/limits | Adjust for busy clusters |
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
Prefer plain manifests? See [`examples/kagent/`](../../examples/kagent/).

## Upgrades

```bash
helm upgrade ratatosk-mcp ./charts/ratatosk-mcp
```

Your values are kept; the image follows the chart `appVersion` unless
`image.tag` pins it.
