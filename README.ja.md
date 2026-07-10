<div align="center">

<img src="docs/assets/ratatosk.png" width="380" alt="ラタトスク — 巻物をかばんに入れ、手を振るメッセンジャーリス">

# ratatosk-mcp

**ラタトスクが毎時 CNCF のリリースノートを読みます。あなたのエージェントが読まなくて済むように。**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

</div>

---

北欧神話のラタトスクは、世界樹を駆け上り駆け下りて言葉を運ぶリスです。
このラタトスクが運ぶのはリリースインテリジェンス。
[ratatosk.io](https://ratatosk.io) は 74 以上の CNCF プロジェクトを見守り、
すべてのリリースノートを型付きのエンティティ単位のファクトに変換します —
セキュリティ修正、破壊的変更、機能削除、非推奨化、デフォルト値の変更。
ただのバグ修正や宣伝文句は取り除かれ、運用者が実際に対応すべきものだけが残ります。

この MCP サーバーは、それらのファクトを 4 つのツールとしてエージェントに渡します。

## こんな体験です

> **あなた:** 「envoy v1.36.8 と istio 1.30.1 を運用中。アップグレード前に必ずやるべきことは?」
>
> **エージェント**が `check_stack` を呼び、ファクトに基づいて答えます:
> 現行バージョン以降に修正された CVE、アップグレード経路で削除される API、
> 変更されたデフォルト値 — それぞれリリースノート原文の引用を証拠として添えて。

## ツール

| ツール | 役割 |
|---|---|
| `list_facts` | 増分ファクトフィード — `project`・`type`・`severity` でフィルタ、`since` カーソルでポーリング |
| `facts_by_entity` | 逆引きインデックス: 1 つの識別子(CVE、CRD、フィーチャーゲート、フラグ、設定フィールド、依存関係)に触れるすべてのファクト |
| `get_release` | レビュー済みリリース 1 件: カバレッジ・評価・原文リンク・全ファクト。`facts: []` + `coverage: full_reviewed` は「読んだ上で平穏なリリース」の意味 |
| `check_stack` | 運用中のコンポーネントのバージョンを渡すと、それより**新しい**リリースのファクト — つまりアップグレード経路を返します |

## <img src="docs/assets/ratatosk-face.png" width="26" alt="" align="top"> バージョンはあなたの元を離れません

`check_stack` がサーバーに送るのは**プロジェクト名だけ**。バージョンの比較は
このプロセスの中でローカルに行われます。何を運用しているかが ratatosk.io に
届くことはありません — サーバーはファクトを配信し、関連性の判断はエージェントが
行います。バージョン正規化器(`internal/version`)が同梱されているため、範囲比較は
クライアント側で完結します。

## クイックスタート (stdio)

```bash
go build -o ratatosk-mcp .

# Claude Code
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

ビルドせずコンテナイメージで:

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

## クラスター内 (ストリーミング HTTP + Helm)

`MCP_HTTP_ADDR` を設定すると、同じバイナリが `/mcp` でストリーミング HTTP
サーバーとして動作します(`/healthz` 付き):

```bash
MCP_HTTP_ADDR=:8080 ./ratatosk-mcp
```

Helm チャートはこれを ClusterIP Service としてデプロイします。クラスター内の
エージェント(kagent、カスタムオペレーター)はプロセスを起動する代わりに URL に
接続します:

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## 設定

| 環境変数 | デフォルト | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | 上流ファクト API (`/v1`、公開、読み取り専用) |
| `MCP_HTTP_ADDR` | *(空)* | 設定時(例: `:8080`)は stdio の代わりにストリーミング HTTP で動作 |

## コンテナイメージ

マルチアーキテクチャ(`linux/amd64`、`linux/arm64`)、リリースタグごとにビルド:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 上流 API

このサーバーは公開 REST API の薄いクライアントです — 直接呼びたい場合は
ratatosk.io の `GET /v1` が自己記述的です。API キー不要、IP あたり毎分 60
リクエストの制限。

## ライセンス

[Apache-2.0](LICENSE)
