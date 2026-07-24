<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="巻物をかばんに入れて手を振るメッセンジャーリス、ラタトスク">

# ratatosk-mcp

**CNCF のリリースノートは、ラタトスクが毎時読んでいます。エージェントは結果を受け取るだけ。**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)


</div>

---

北欧神話のラタトスクは、世界樹を駆け回って言葉を運ぶリスです。ここで運ぶのは
リリースの事実。[ratatosk.io](https://ratatosk.io) は 74 を超える CNCF
プロジェクトを見張り、リリースノートをセキュリティ修正、破壊的変更、機能削除、
非推奨化、デフォルト値の変更といったファクトに整理します。ただのバグ修正や
宣伝文句は除外。残るのは、運用者が対応すべきことだけです。

この MCP サーバーは、そのファクトを 4 つのツールとしてエージェントに渡します。

## こんな使い方です

> **質問:** 「envoy v1.36.8 と istio 1.30.1 を運用中。アップグレード前に必ずやることは?」
>
> **エージェント**が `check_stack` を呼び、ファクトだけで答えます。現行
> バージョン以降に修正された CVE、アップグレード経路で消える API、変わった
> デフォルト値を、リリースノート原文の引用つきで示します。

## ツール

| ツール | 役割 |
|---|---|
| `list_facts` | 増分ファクトフィード。`project`・`type`・`severity` で絞り、`since` カーソルで続きを取得します |
| `facts_by_entity` | 逆引きインデックス。CVE、CRD、フィーチャーゲート、フラグ、設定フィールド、依存関係など、識別子ひとつに触れるファクトをすべて返します |
| `get_release` | レビュー済みリリース 1 件のカバレッジ・評価・原文リンク・全ファクト。`version` を省略すると、そのプロジェクトの最新レビュー済みリリースを返します。`facts: []` かつ `coverage: full_reviewed` なら、読んだ上で平穏なリリースという意味です。`include_raw` でリリースノート原文(`raw_notes`)も — 分析が不十分、またはファクト 0 件なら自動で含まれます |
| `check_stack` | 運用中のコンポーネントのバージョンを渡すと、アップグレード経路のブリーフィングを返します。critical/high は全文、残りは一行ずつ、複数ブランチで修正された同一イシューは一項目に畳みます。全件は `detail: "full"`、一段階のアップグレードだけなら `target_version`、深刻度フィルタは `severity_min` |

## <img src="docs/assets/ratatosk-face.png" width="26" alt="" align="top"> バージョンは外に出ません

`check_stack` がサーバーへ送るのはプロジェクト名だけ。バージョンの比較はこの
プロセスの中で完結します。何を運用しているかが ratatosk.io に届くことは
ありません。サーバーはファクトを配るだけで、どれが自分に関係するかは
エージェントが判断します。バージョン正規化器(`internal/version`)を同梱して
いるので、範囲比較もクライアント側で済みます。

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
不要で、PSS `restricted` でも警告なしで動きます。

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

**B. マニフェスト** — 同じ 3 つの部品を Helm なしでそのまま適用:

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

| 環境変数 | デフォルト | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | 上流ファクト API (`/v1`、公開、読み取り専用) |
| `MCP_HTTP_ADDR` | *(空)* | 設定すると(例: `:8080`)stdio の代わりにストリーミング HTTP で動作 |

## コンテナイメージ

マルチアーキテクチャ(`linux/amd64`、`linux/arm64`)、リリースタグごとにビルドします:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 上流 API

このサーバーは公開 REST API を薄く包んだクライアントです。直接呼びたければ、
ratatosk.io の `GET /v1` が自分自身を説明します。API キーは不要。制限は IP
あたり毎分 60 リクエストだけです。

## データと利用規約

データは ratatosk.io が[利用規約](https://ratatosk.io/terms)に基づき無料で
提供します（変更の可能性あり、変更時は事前告知）。分析は AI が生成した参考
情報であり、保証はありません — 対応する前に原文のリリースノートを確認して
ください。エージェントが代わりに作業する場合も同様です。原文の著作権は各
プロジェクトにあり、原文全体を含むレスポンスには出典表示（`raw_notes_notice`）
が付きます。

## ライセンス

このリポジトリのコードは [Apache-2.0](LICENSE) で配布されます。
