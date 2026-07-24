# インストールと使い方

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

## 実行方法を選ぶ

| あなたは… | 方法 | セクション |
| --- | --- | --- |
| ノートPCで Claude Code / Claude Desktop など stdio MCP クライアントを使う | `docker run`（stdio） | [ローカル](#ローカルstdio) |
| Kubernetes で独自エージェントを運用（フレームワーク不問、CI ジョブ、SDK クライアント） | Helm チャート | [クラスタ内・単独](#クラスタ内単独helm) |
| [kagent](https://kagent.dev) を使用中 | `kagent.enabled=true` の Helm オプション、またはマニフェスト | [kagent と一緒に](#kagent-と一緒に) |

3 つの方法はすべて同じバイナリで同じ公開 API を使います。どのモードでも
アカウントも API キーも不要です。

## ローカル（stdio）

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

ソースからビルドしてバイナリを登録することもできます:

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

stdio 対応の MCP クライアントならどれでも同じ方法です。

## クラスタ内・単独（Helm）

`MCP_HTTP_ADDR` を設定すると、同じバイナリが `/mcp` でストリーミング HTTP の
MCP を提供します（`/healthz` プローブ付き）。チャートは ClusterIP Service
として配備するため、クラスタ内の何からでもプロセス起動なしに URL で接続
できます — SDK で作った独自エージェント、他のエージェントフレームワーク、
アップグレード前に `check_stack` でゲートする CI ジョブなど:

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
# → MCP クライアントを http://ratatosk-mcp.<namespace>.svc:8080/mcp へ
```

アップグレードは値を保ったまま `helm upgrade` 一行です。RBAC もシークレットも
不要で、PSS `restricted` でも警告なしで動きます。チャートのオプションは
[チャート README](../charts/ratatosk-mcp/README.ja.md) を参照してください。

## kagent と一緒に

同等の 2 つの経路から 1 つだけ選んでください:

**A. Helm トグル** — 1 回のインストールでサーバー、kagent 登録
（RemoteMCPServer）、サンプルの `release-triage-agent` まで:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` の既定は `default-model-config` です。環境が違う場合は
上書きし、サンプルエージェントなしで登録だけしたい場合は
`kagent.agent.enabled=false` にします。

**B. マニフェスト** — 同じ 3 つの部品を Helm なしでそのまま適用
（[詳細](../examples/kagent/README.ja.md)）:

```bash
kubectl apply -f examples/kagent/ratatosk-deploy.yaml
kubectl apply -f examples/kagent/ratatosk-remote-mcpserver.yaml
kubectl apply -f examples/kagent/ratatosk-agent.yaml
```

どちらでも kagent UI に `release-triage-agent` が現れます。こう聞いてみて
ください:

> 「kubernetes v1.36.0、cilium v1.17.18、envoy v1.38.3 を運用中です。
> アップグレード前に対応が必要なものはありますか？」

## 設定

| 環境変数 | 既定値 | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | 上流の facts API（`/v1`、公開、読み取り専用） |
| `MCP_HTTP_ADDR` | *(なし)* | 設定すると（例: `:8080`）stdio の代わりにストリーミング HTTP で提供 |

## コンテナイメージ

マルチアーキテクチャ（`linux/amd64`, `linux/arm64`）、リリースタグごとに自動ビルド:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 上流 API

このサーバーは公開 REST API の薄いクライアントです。直接呼びたい場合は
ratatosk.io の `GET /v1` が自己記述します。API キー不要、IP あたり毎分 60
リクエストの制限があります。
