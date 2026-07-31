# ratatosk-mcp Helm チャート

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

ratatosk MCP サーバーを Streamable HTTP モードの ClusterIP Service として
配備します。RBAC もシークレットも不要で、PSS `restricted` でも警告なしで
動きます。

```bash
# リポジトリのルートで実行
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → MCP エンドポイント: http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Values

| キー | 既定値 | 意味 |
| --- | --- | --- |
| `replicaCount` | `1` | レプリカ数 — 2 以上にする前に `statelessHttp` を有効化してください |
| `image.repository` | `ghcr.io/garlickim21/ratatosk-mcp` | イメージ |
| `image.tag` | チャートの `appVersion` | 特定バージョンの固定 |
| `ratatoskUrl` | `https://ratatosk.io` | 上流の facts API — egress をプロキシ・ミラー経由にする場合は変更 |
| `statelessHttp` | `false` | セッション状態なしの HTTP 配信 — MCP 仕様 2026-07-28 リビジョンのクライアントと複数レプリカに必要 |
| `logLevel` | `""` (= info) | `MCP_LOG`: `debug`・`warn`・`error` も受け付け、それ以外の値は警告なしに info 扱い。どのレベルでもリクエスト引数は記録されません |
| `auditMode` | `""` (= 無効) | `MCP_AUDIT`: `metadata` はツール呼び出しごとに 1 行(引数名のみ)、`full` は引数値も記録 |
| `service.port` | `8080` | Service / MCP エンドポイントのポート |
| `resources` | 小さめの requests/limits | 必要に応じて調整 |
| `nodeSelector` / `tolerations` / `affinity` | `{}` / `[]` / `{}` | 標準的なスケジューリング制御 |
| `kagent.enabled` | `false` | kagent 統合も一緒にインストール（下記） |
| `kagent.modelConfig` | `default-model-config` | kagent 側の ModelConfig 名 |
| `kagent.agent.enabled` | `true` | サンプルエージェントの導入（kagent.enabled 時） |
| `kagent.agent.name` | `ratatosk-agent` | サンプルエージェント名 |
| `kagent.agent.k8sTools` | `true` | kagent 内蔵の読み取り専用クラスタツールをエージェントに付与し、稼働バージョンを自力で把握 |

## kagent 統合

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`RemoteMCPServer`（kagent がサーバーのツールをすべて自動検出）と、準備済みの
`ratatosk-agent` が追加されます。エージェントには kagent 内蔵の
読み取り専用クラスタツール（`k8s_get_resources`、`k8s_get_resource_yaml`）
も付き、稼働中のバージョンを自力で見つけます — 無効化は
`kagent.agent.k8sTools=false`。kagent CRD のあるクラスタでのみ
有効化してください。Helm を使わない場合は
[`examples/kagent/`](../../examples/kagent/) を参照。

## アップグレード

```bash
helm upgrade ratatosk-mcp ./charts/ratatosk-mcp
```

`--set`・`-f` で指定した値は自動では引き継がれません — 同じ値を再指定するか
`--reuse-values` を付けてください。`image.tag` で固定しない限りイメージは
チャートの `appVersion` に従います。
