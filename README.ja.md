<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="巻物をかばんに入れて手を振るメッセンジャーリス、ラタトスク">

# ratatosk-mcp

**CNCF のリリースノートは、ラタトスクが毎時読んでいます。エージェントは結果を受け取るだけ。**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)
[![Glama score](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp/badges/score.svg)](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp)


</div>

---

北欧神話のラタトスクは、世界樹を駆け回って言葉を運ぶリスです。ここで運ぶのは
リリースの事実。[ratatosk.io](https://ratatosk.io) は 74 を超える CNCF
プロジェクトを見張り、リリースノートをセキュリティ修正、破壊的変更、機能削除、
非推奨化、デフォルト値の変更といったファクトに整理します。ただのバグ修正や
宣伝文句は除外。残るのは、運用者が対応すべきことだけです。

この MCP サーバーは、そのファクトをツールとしてエージェントに渡します。

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
| `list_projects` | 追跡プロジェクトの一覧。スラッグは推測せず、まずここで確認します |
| `get_release` | レビュー済みリリース 1 件のカバレッジ・評価・原文リンク・全ファクト。`version` を省略すると、そのプロジェクトの最新レビュー済みリリースを返します。`facts: []` かつ `coverage: full_reviewed` なら、読んだ上で平穏なリリースという意味です。`include_raw` でリリースノート原文(`raw_notes`)も — 分析が不十分、またはファクト 0 件なら自動で含まれます |
| `list_releases` | プロジェクト 1 件の最新リリース N 件のサマリー(深刻度別ファクト数、アドバイザリグループ最大深刻度)を新しい順で — 「X の最近のリリース」向けツール。各行は `get_release` で掘り下げられます |
| `check_stack` | 運用中のコンポーネントのバージョンを渡すと、アップグレード経路のブリーフィングを返します。critical/high は全文、残りは一行ずつ、複数ブランチで修正された同一イシューは一項目に畳みます。全件は `detail: "full"`、一段階のアップグレードだけなら `target_version`、深刻度フィルタは `severity_min` |

## バージョンは外に出ません

`check_stack` がサーバーへ送るのはプロジェクト名だけ。バージョンの比較はこの
プロセスの中で完結します。何を運用しているかが ratatosk.io に届くことは
ありません。サーバーはファクトを配るだけで、どれが自分に関係するかは
エージェントが判断します。バージョン正規化器(`internal/version`)を同梱して
いるので、範囲比較もクライアント側で済みます。

## クイックスタート

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

アカウントも API キーも不要です。Kubernetes でエージェントを運用している、
または kagent を使っている場合は、下のインストールガイドをどうぞ。

## ドキュメント

| | |
| --- | --- |
| **[インストールと使い方](docs/install.ja.md)** — ローカル stdio · クラスタ（Helm） · kagent | [English](docs/install.en.md) · [한국어](docs/install.ko.md) |
| **[Helm チャート](charts/ratatosk-mcp/README.ja.md)** — values、kagent トグル | [English](charts/ratatosk-mcp/README.md) · [한국어](charts/ratatosk-mcp/README.ko.md) |
| **[kagent サンプル](examples/kagent/README.ja.md)** — マニフェスト + ratatosk-agent | [English](examples/kagent/README.md) · [한국어](examples/kagent/README.ko.md) |
| **[コントリビュート](CONTRIBUTING.md)** · **[セキュリティポリシー](SECURITY.md)** | |

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
