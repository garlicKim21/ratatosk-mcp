# ratatosk-mcp Helm チャート

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

ratatosk MCP サーバーをストリーミング HTTP モードの ClusterIP Service として
配備します。RBAC もシークレットも不要で、PSS `restricted` でも警告なしで
動きます。

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → MCP エンドポイント: http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Values

| キー | 既定値 | 意味 |
| --- | --- | --- |
| `image.repository` | `ghcr.io/garlickim21/ratatosk-mcp` | イメージ |
| `image.tag` | チャートの `appVersion` | 特定バージョンの固定 |
| `ratatoskUrl` | `https://ratatosk.io` | 上流の facts API |
| `service.port` | `8080` | Service / MCP エンドポイントのポート |
| `resources` | 小さめの requests/limits | 必要に応じて調整 |
| `kagent.enabled` | `false` | kagent 統合も一緒にインストール（下記） |
| `kagent.modelConfig` | `default-model-config` | kagent 側の ModelConfig 名 |
| `kagent.agent.enabled` | `true` | サンプルエージェントの導入（kagent.enabled 時） |
| `kagent.agent.name` | `ratatosk-agent` | サンプルエージェント名 |

## kagent 統合

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`RemoteMCPServer`（kagent が 5 つのツールを自動発見）と、準備済みの
`ratatosk-agent` が追加されます。kagent CRD のあるクラスタでのみ
有効化してください。Helm を使わない場合は
[`examples/kagent/`](../../examples/kagent/) を参照。

## アップグレード

```bash
helm upgrade ratatosk-mcp ./charts/ratatosk-mcp
```

設定値は保持され、`image.tag` で固定しない限りイメージはチャートの
`appVersion` に従います。
