<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="巻物をかばんに入れて手を振るメッセンジャーリス、ラタトスク">

# ratatosk-mcp

**CNCF のリリースノートは、ラタトスクが毎時読んでいます。エージェントは fact を受け取ります。**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)
[![Glama score](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp/badges/score.svg)](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp)


</div>

---

北欧神話のラタトスクは、世界樹を駆け回って言葉を運ぶリスです。ここで
運ぶのは、リリースの情報です。[ratatosk.io](https://ratatosk.io) は 76 の
CNCF プロジェクトを追跡し、リリースノート 1 つひとつを型付きの fact に
変換します。対象はセキュリティ修正、破壊的変更、機能削除、非推奨化、
デフォルト値の変更です。ただのバグ修正や宣伝文句は除外します。残るのは、
運用者が対応すべきことです。

このリポジトリは、その fact をツールとしてエージェントに渡す MCP
サーバーです。MCP（Model Context Protocol）は AI エージェントが外部
ツールを呼び出すときに使うオープン標準です。Claude Code、Claude Desktop、
kagent、自作の SDK エージェントなど、MCP に対応したクライアントであれば
どれからでも接続できます。アカウントも API キーも不要です。

## 二つの使い方

**ホスト版 — インストールは不要です。** リモートコネクタに
対応したクライアントに `https://ratatosk.io/mcp` をリモート MCP サーバー
として登録するだけです。ホスト版エンドポイントはアップストリームの
レート制限バケット 1 つ（毎分 60 リクエスト）を全利用者で共有します —
ポーリングや CI ワークロードならセルフホストしてください。Claude Code CLI
では：

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

**セルフホスト — 同じサーバーを、自分のプロセスとして動かします。**
Docker、Helm チャート、ソースビルドのいずれでも構いません。Claude Code と
Docker がインストール済みなら：

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.6.1
```

`0.6.1` が現行リリースです。新しいリリースを追い続けるなら `latest` を
使ってください。

どちらの場合も、接続を確認してください：

```bash
claude mcp list
# ratatosk: … - ✔ Connected
```

そのうえで、ツールが答えられる質問をエージェントに投げてみてください：

> **質問：**「envoy v1.36.8 と istio 1.30.1 を運用中。アップグレード前に必ずやることは？」
>
> **エージェント**が `check_stack` を呼び、fact で答えます。現行
> バージョン以降に修正された CVE、アップグレード経路で削除される API、
> 変更されたデフォルト値です。各 fact には、根拠としてリリースノート原文の
> 引用が付きます。

ほかのクライアント（Claude Desktop、kagent、クラスタ内エージェント）での
設定方法と、設定項目の完全なリファレンスは、
[インストールガイド](docs/install.ja.md)を参照してください。

## ツール

先にツールが使う用語を 2 つ定義します。**fact** とは、公式リリースノート
から抽出した変更 1 件（セキュリティ修正、削除、非推奨化、デフォルト値の
変更）のことです。各 fact には、その変更が対象とする正確な識別子（CVE
id、フラグ、CRD、設定フィールド）と、根拠となるノート原文の引用が
付きます。すべての fact には `info` から `critical` までの
**深刻度（severity）**が付きます。

| ツール | 役割 |
|---|---|
| `check_stack` | 稼働中のコンポーネントバージョンを受け取り、アップグレード経路上の fact を返します — critical/high はそれ以外と分け、アドバイザリごとに 1 項目、それぞれに引用と id 付き。比較はサーバープロセス（セルフホストなら自分のプロセス）の中で行われます（[実行中のコンポーネントバージョンの取り扱い](#実行中のコンポーネントバージョンの取り扱い)） |
| `list_facts` | 増分 fact フィード、分析の古い順。プロジェクト・タイプ・深刻度で絞り、`since` カーソルでページ送りしてローカルコピーを同期できます |
| `facts_by_entity` | 逆引き：正確な識別子 1 つ（CVE id、CRD、フィーチャーゲート、フラグ、設定フィールド、依存関係など）を扱った fact のすべて |
| `get_release` | リリース 1 件の全体：fact、総合評価、原文ノートへのリンク。完全にレビュー済みで fact が 0 件なら、読んだうえで特記事項がなかったという意味です |
| `list_releases` | 1 プロジェクトの最新リリースを 1 行要約（日付、深刻度別の fact 数）で新しい順に — 「X で最近何があった？」のためのツール |
| `list_projects` | 追跡プロジェクトの一覧と正規スラッグ（ほかのすべてのツールが受け取る短いプロジェクト id）— 名前は推測せず、ここで調べてください |

ツールごとのパラメータ全体と呼び出し例・実測レスポンスは[ツールリファレンス](docs/tools.ja.md)にあります。

## 実行中のコンポーネントバージョンの取り扱い

**セルフホスト：** `check_stack` がサーバーへ送るのはプロジェクトスラッグ
だけで、バージョンの比較はローカル（このプロセスの中）で行います。
`check_stack` に渡したバージョンが ratatosk.io に届くことはありません。
サーバーは fact を公開し、何が当てはまるかはエージェントが判断します。
バージョンを正規化する処理（`internal/version`）も同梱しており、範囲比較
までクライアント側で完結します。アップグレードの質問でも同じです。
アップストリーム API には、呼び出し側がバージョンを送る、利便性のための
エンドポイント（`/v1/upgrade/{project}`）がありますが、`check_stack` は
それを呼びません。比較のロジックは、誰でも読めるソースコードとして
公開されています。

ただし、この保証が及ぶ範囲は `check_stack` に限られます。
`get_release(project, version)` のようにバージョンを引数に取るツールは、
そのバージョンをアップストリームのリクエストパスに載せます — 特定の
リリースを取得するには名指しが必要だからです。ただし、その名指しされた
パスもサーバー側のログには残りません。ログが 1 行書かれる前にクエリ文字列
が除去され、`/v1/releases/…`・`/v1/upgrade/…` のパスはプレフィックスだけ
に縮められるため、スラッグもバージョンもログには載りません。

**ホスト版：** `check_stack` の引数（稼働中のバージョン）は、答えを
組み立てるためにサーバーのメモリを通過しますが、どこにも書き残されません。
経路上の各層が残すものは次のとおりです。

- ホスト版の MCP プロセス自体は起動行 1 つだけを残します — 正常な
  リクエストは何も追加しません。
- アップストリーム API のリクエストログは、呼び出し側が `traceparent` を
  送った場合にのみ 1 行書かれ、その行にあるのは正規化されたエンドポイント
  ラベルと trace id だけです — パス・クエリ・ボディは決して載りません。
- 前段のアクセスログはクエリ文字列を除去し、`/v1/releases/…`・
  `/v1/upgrade/…` のパスをプレフィックスだけに縮め、呼び出し元の IP を
  マスクします。リクエストボディを記録する項目自体がありません。

ホスト版エンドポイントは監査ストリームを無効にしたまま動いており、
有効にすることはありません — リクエスト内容を記録しないことが、この
エンドポイントの運用方針です。管理の及ばない境界が 1 つあります。CDN 層
（Cloudflare）の接続メタデータは Cloudflare 自身のポリシーに従います。
その区間が要件に合わないならセルフホストしてください — そうすれば
`check_stack` の呼び出しでインフラを離れるのはプロジェクトスラッグだけに
なります。

セルフホストには、もう一つの利点があります。誰がどのツールを呼んだかを
記録するオプトインの監査ストリーム（`MCP_AUDIT=metadata` または `full`）
を、利用者のインフラ内で生成し、自前のコレクタに蓄積できます。ホスト版
エンドポイントには設計上ありません。詳細は
[インストールガイド](docs/install.ja.md)へ。

## ドキュメント

- **[インストールと使い方](docs/install.ja.md)** — ホスト版エンドポイント · ローカル stdio · クラスタ内（Helm） · kagent（[English](docs/install.en.md) · [한국어](docs/install.ko.md)）
- **[Helm チャート](charts/ratatosk-mcp/README.ja.md)** — values、kagent トグル（[English](charts/ratatosk-mcp/README.md) · [한국어](charts/ratatosk-mcp/README.ko.md)）
- **[kagent の例](examples/kagent/README.ja.md)** — マニフェスト + ratatosk-agent（[English](examples/kagent/README.md) · [한국어](examples/kagent/README.ko.md)）
- **[コントリビューション](CONTRIBUTING.md)** · **[セキュリティポリシー](SECURITY.md)**

## アップストリーム API

このサーバーは公開 REST API の薄いクライアントです。直接呼びたい場合は、
ratatosk.io の `GET /v1` を呼ぶと API 自身の説明が返ります。API キーは
不要で、IP ごとに毎分 60 リクエストの制限があります。

## データと利用規約

データは ratatosk.io が[利用規約](https://ratatosk.io/terms)のもとで無料で
提供します（変更される可能性があり、変更時は事前に告知します）。分析は AI
が生成した参考情報であり、保証はありません — 対応する前に原文のリリース
ノートを確認してください。エージェントが代わりに作業する場合はなおさら
です。原文の権利は各プロジェクトに帰属し、ノート全文を含む応答には出典の
告知（`raw_notes_notice`）が付きます。

## ライセンス

このリポジトリのコードは [Apache-2.0](LICENSE) でライセンスされています。
