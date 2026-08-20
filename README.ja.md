<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="巻物をかばんに入れて手を振るメッセンジャーリス、ラタトスク">

# ratatosk-mcp

**CNCF のリリースノートは、ラタトスクが毎時読んでいます。エージェントは change を受け取ります。**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)
[![Glama score](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp/badges/score.svg)](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp)


</div>

---

北欧神話のラタトスクは、世界樹を駆け回って言葉を運ぶリスです。ここで
運ぶのは、リリースの情報です。[ratatosk.io](https://ratatosk.io) は 76 の
CNCF プロジェクトを追跡し、リリースノート 1 つひとつを型付きの change に
変換します。対象はセキュリティ修正、破壊的変更、機能削除、非推奨化、
デフォルト値の変更で、それぞれに**いまどう動くべきか**の区分が付きます。
定型的な行も記録しますが、前面には出しません。

このリポジトリは、その change をツールとしてエージェントに渡す MCP
サーバーです。MCP（Model Context Protocol）は AI エージェントが外部
ツールを呼び出すときに使うオープン標準です。Claude Code、Claude Desktop、
kagent、自作の SDK エージェントなど、MCP に対応したクライアントであれば
どれからでも接続できます。アカウントも API キーも不要です。

## 二つの使い方

**ホスト版 — インストールは不要です。** リモートコネクタに
対応したクライアントに `https://ratatosk.io/mcp` をリモート MCP サーバー
として登録するだけです。ホスト版エンドポイントは呼び出し元ごとに毎分 60 回
のツール呼び出しを許可します（全体で分け合う枠ではなく、呼び出し元ごとに
数えます）— ポーリングや CI ワークロードならセルフホストしてください。
Claude Code CLI では：

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

**セルフホスト — 同じサーバーを、自分のプロセスとして動かします。**
Docker、Helm チャート、ソースビルドのいずれでも構いません。Claude Code と
Docker がインストール済みなら：

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.5
```

`0.7.5` が現行リリースです。新しいリリースを追い続けるなら `latest` を
使ってください。

どちらの場合も、接続を確認してください：

```bash
claude mcp list
# ratatosk: … - ✔ Connected
```

そのうえで、ツールが答えられる質問をエージェントに投げてみてください：

> **質問：**「envoy v1.36.8 と istio 1.30.1 を運用中。アップグレード前に必ずやることは？」
>
> **エージェント**が `check_stack` を呼び、記録で答えます。現行
> バージョン以降に修正された CVE、アップグレード経路で削除される API、
> 変更されたデフォルト値を、**全員に該当するもの**と**自分の構成が一致する
> 場合だけ該当するもの**に分けて示します。各 change には、根拠としてリリース
> ノート原文の引用が付きます。

実際の `check_stack` 応答から 1 項目をそのまま載せます
(istio、2026-08-20 実測):

```json
{
  "severity": "high",
  "family": "security",
  "bucket": "action",
  "applies_if": "uses JWKS Resolver",
  "quote": "- CVE-2026-31837 / GHSA-v75c-crr9-733c : (CVSS score 8.7, High): JWKS Resolver Failure May Allow Authentication Bypass Using Known Default Keys.",
  "same_matter_also_addressed_in": ["1.28.5", "1.29.1"]
}
```

`applies_if` は読むための散文ではなく、エージェントが評価する条件式です —
JWKS を使わないスタックなら、エージェントはこの項目を自分で読み飛ばします。
応答には何が送信されたかも明記されます: *"versions were compared locally;
only project slugs were sent to the server."*

ほかのクライアント（Claude Desktop、kagent、クラスタ内エージェント）での
設定方法と、設定項目の完全なリファレンスは、
[インストールガイド](docs/install.ja.md)を参照してください。

## ツール

先にツールが使う言葉を 2 つ定義します。**change** とは、リリースが行った
こと 1 件です。公式リリースノートから取り出し、その変更が対象とする正確な
識別子（CVE id、フラグ、CRD、設定フィールド）と、根拠となる原文の引用が
付きます。各 change は 3 つの軸を持ちます。

- **family** — `security`・`breaking`・`deprecated`: どの種類か。
- **bucket** — `action`（全員に該当）・`check`（`applies_if` が自分の構成と
  一致する場合のみ）・`plan`（将来の予告）・`other`（全件記録）。
- **applies_if** — 読むための文章ではなく、自分のマニフェストに対して評価
  できる真偽式。

**matter（事案）** は根にある問題で、`matter_key` で識別します。リリースや
ブランチをまたいでも変わらないので、同じセキュリティロールアップが 5 つの
ブランチに着地してもキーは 1 つです。深刻度は引用されたアドバイザリに付き、
分析時点で固まった値ではなく台帳の現在値を読みます。

| ツール | 役割 |
|---|---|
| `check_stack` | 稼働中のコンポーネントバージョンを受け取り、アップグレード経路上の change を層別に分けて返します — `action_required` は全員に該当し、`check_config` は `applies_if` が一致する場合のみ該当します。比較はサーバープロセス（セルフホストなら自分のプロセス）の中で行われます（[実行中のコンポーネントバージョンの取り扱い](#実行中のコンポーネントバージョンの取り扱い)） |
| `list_changes` | 増分 change フィード、分析の古い順。プロジェクト・family・bucket で絞り、`since` カーソルでページ送りしてローカルコピーを同期できます |
| `changes_by_entity` | 逆引き：正確な識別子 1 つ（CVE id、CRD、フィーチャーゲート、フラグ、設定フィールド、依存関係など）を扱った change のすべて |
| `get_matter` | 一つの事案が登場したリリースすべて。同じロールアップがブランチごとに違うアドバイザリを伴って着地するため、最新の 1 件だけを見ると全部カバーされたと思ってしまいます |
| `get_release` | リリース 1 件の全体：change、サマリー、原文ノートへのリンク。change が 0 件なら、読んだうえで特記事項がなかったという意味です |
| `list_releases` | 1 プロジェクトの最新リリースを 1 行要約（日付、層別・family 別の件数、最高のアドバイザリ深刻度）で新しい順に — 「X で最近何があった？」のためのツール |
| `list_projects` | 追跡プロジェクトの一覧と正規スラッグ（ほかのすべてのツールが受け取る短いプロジェクト id）— 名前は推測せず、ここで調べてください |

ツールごとのパラメータ全体と呼び出し例・実測レスポンスは[ツールリファレンス](docs/tools.ja.md)にあります。

## 実行中のコンポーネントバージョンの取り扱い

**セルフホスト：** `check_stack` がサーバーへ送るのはプロジェクトスラッグ
だけで、バージョンの比較はローカル（このプロセスの中）で行います。
`check_stack` に渡したバージョンが ratatosk.io に届くことはありません。
サーバーは change を公開し、何が当てはまるかはエージェントが判断します。
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
エンドポイントの運用方針です。管理の及ばない境界が 1 つあります。CDN 層の
接続メタデータは CDN 事業者自身のポリシーに従います。
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
不要で、IP ごとに毎分 1200 リクエストの制限があります。

## データと利用規約

データは ratatosk.io が[利用規約](https://ratatosk.io/terms)のもとで無料で
提供します（変更される可能性があり、変更時は事前に告知します）。分析は AI
が生成した参考情報であり、保証はありません — 対応する前に原文のリリース
ノートを確認してください。エージェントが代わりに作業する場合はなおさら
です。原文の権利は各プロジェクトに帰属し、ノート全文を含む応答には出典の
告知（`raw_notes_notice`）が付きます。

## ライセンス

このリポジトリのコードは [Apache-2.0](LICENSE) でライセンスされています。
