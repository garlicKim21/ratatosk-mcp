# インストールと使い方

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

このページでは、Ratatosk のリリース change を AI エージェントから使う 4 つの
方法を説明します。何もインストールせずホスト版エンドポイントに接続する
方法、手元の PC で stdio サーバーを動かす方法、Kubernetes クラスタに Helm
でデプロイする方法、そして kagent と統合する方法です。

**MCP（Model Context Protocol）** は、AI エージェントが外部ツールを呼び出す
ときに使う標準プロトコルです。ratatosk-mcp はこのプロトコルで 7 つの
ツールを提供するサーバーです。`check_stack`（稼働中のバージョン一覧を受け取り、
アップグレード経路上の change を層別に分けて返す）、
`list_changes`（増分 change フィード）、`changes_by_entity`（CVE・フラグなど
識別子からの逆引き）、`get_matter`（一つの事案が登場したリリースすべて）、
`get_release`（リリース 1 件の全体）、
`list_releases`（プロジェクトごとの最新リリース要約）、
`list_projects`（追跡プロジェクトと正規スラッグ）です。Claude Code、
Claude Desktop、kagent、自作の SDK エージェントなど、MCP に対応した
クライアントであればどれからでも接続できます。

## 実行方法を選ぶ

| 利用シーン | 方法 | セクション |
| --- | --- | --- |
| まず試したい — 何もインストールせずに | ホスト版エンドポイントの URL を登録 | [ホスト版](#ホスト版エンドポイント) |
| 手元の PC で Claude Code / Claude Desktop など stdio MCP クライアントを使う | `docker run`（stdio） | [ローカル](#ローカルstdio) |
| Kubernetes で独自エージェントを運用（フレームワーク不問、CI ジョブ、SDK クライアント） | Helm チャート | [クラスタ内デプロイ](#クラスタ内への単独デプロイhelm) |
| [kagent](https://kagent.dev) を使用中 | `kagent.enabled=true` の Helm オプション、またはマニフェスト | [kagent 統合](#kagent-統合) |

4 つの方法はいずれも同じ 7 つのツールで同じ公開データを提供し、どのモード
でもアカウントも API キーも不要です。違いは 3 つあります。

- **`check_stack` に渡した稼働中バージョンがどこまで行くか。** セルフホスト
  ではバージョン比較が利用者のプロセス内で完結し、そのバージョンはインフラ
  を離れません。ホスト版ではバージョンがサーバーメモリを通過しますが、
  どのログにも記録されません。
- **アップストリームのリクエスト上限を誰と分け合うか。** ホスト版は共有
  バケットを使い、セルフホストは自分の IP の枠を使います。
- **監査ストリームを残せるか。** セルフホストのみ可能です。

共通点も 1 つあります。`get_release` のように特定バージョンを引数に取る
ツールは、どの方法でもそのバージョンをアップストリームのリクエストパスに
載せて取得します。取得対象を名指しする値だからです。ただしそのパスも
サーバー側のログには残りません。アクセスログが記録前にクエリ文字列を
除去し、`/v1/releases/…`・`/v1/upgrade/…` のパスをプレフィックスだけに
縮めるためです。詳細は各セクションで説明します。

## 前提条件

- **ホスト版**：リモート MCP サーバー（Streamable HTTP — ストリーミング
  方式の HTTP トランスポート）に対応したクライアントがあれば十分です。
  ほかに準備するものはありません。
- **ローカル（stdio）**：Docker。ソースからビルドするなら Go 1.26 以上。
- **例に出てくる `claude mcp add` コマンド**：[Claude Code](https://claude.com/claude-code)
  CLI（`claude` コマンド）がインストールされている必要があります。別の MCP
  クライアントを使う場合はそれぞれの登録方法に従えばよいので、必須では
  ありません。
- **クラスタ（Helm）**：Kubernetes クラスタと Helm 3、そしてこのリポジトリの
  クローン。チャートは別のチャートリポジトリや OCI レジストリには公開されて
  おらず、リポジトリの中にだけあります。
- **アウトバウンド HTTPS（egress）**：セルフホストではどの方法でも、サーバー
  プロセスが `ratatosk.io:443` へのアウトバウンド HTTPS 接続を開ける必要が
  あります。NetworkPolicy や egress プロキシでアウトバウンドを制限している
  クラスタでは、この宛先を許可リストに追加してください。プロキシやミラーを
  経由する必要があるなら、`RATATOSK_URL`（チャート値：`ratatoskUrl`）で
  アップストリームのアドレスを変更できます。遮断されたまま実行するとツールが
  エラーを返すか、結果が空になります（`check_stack` はエラーの代わりに
  コンポーネントごとの `note` フィールドに `fetch failed: …` を入れて
  返します）— [トラブルシューティング](#トラブルシューティング)参照。
- **kagent 統合**：kagent がすでにインストールされたクラスタ（kagent CRD を
  含む）。

## ホスト版エンドポイント

`https://ratatosk.io/mcp` — インストールなしですぐ使えるリモート MCP
エンドポイントです。Streamable HTTP で提供され、ステートレス
（stateless）です。セッションがないため、`Mcp-Session-Id` の往復なしに
リクエスト 1 つひとつが独立して処理されます。

### クライアントへのエンドポイント登録

Claude Code：

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

Claude（ウェブ・デスクトップ）では、リモートコネクタ設定の URL 欄に
`https://ratatosk.io/mcp` を入力します。ほかのクライアントはそれぞれの
リモート MCP サーバー登録方法に従ってください。

### 確認

クライアントがなくても、curl でエンドポイントが生きているか確認できます：

```bash
curl -s -X POST https://ratatosk.io/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}'
```

応答は SSE（Server-Sent Events）フレーミングで返ってきます —
`Content-Type` が `text/event-stream` でも失敗ではありません。`data:` 行の
JSON に `"serverInfo"` と `"name":"ratatosk"` が見えれば正常です：

```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{…,"serverInfo":{"name":"ratatosk","version":"0.7.0"}}}
```

Claude Code なら `claude mcp list` の出力で確認してください：

```
ratatosk: https://ratatosk.io/mcp (HTTP) - ✔ Connected
```

### 既知の挙動と制限

- **SSE の GET は 405**：ステートレスモードはサーバー発の通知ストリーム
  （GET で開く SSE）を提供しないため、405 応答が正常な動作です。ブラウザの
  アドレスバーで開くと案内ページ（`/docs/mcp`）にリダイレクトされます。
- **適正利用について**：ホスト版エンドポイントは**呼び出し元ごとに毎分
  60 回のツール呼び出し**を許可します。全体で分け合う枠ではなく呼び出し元の
  アドレスごとに数えるので、誰か 1 人が忙しくても他の利用者が枯渇しません。
  超えると `Retry-After` 付きの `429` が返ります。ツール呼び出し 1 回は
  API リクエスト 1 回ではありません — `check_stack` はコンポーネント 1 つ
  につきアップストリームへ 1 リクエストを使います。上限をツール呼び出し
  単位で表しているのはそのためです。直接 `/v1` を呼ぶ場合は自分の IP の枠
  として毎分 1200 リクエストが使えます。ポーリングのような負荷の高い使い方
  が必要なら、セルフホストに切り替えてください。

### プライバシー — ホスト版エンドポイントが記録するもの

ホスト版では、`check_stack` の引数（稼働中のバージョン一覧）がサーバー
メモリを通過しますが、リクエスト内容はどこにも記録されません：

- MCP サーバーのログには起動行とアップストリームのエラー行だけが残り、
  正常なリクエストは記録されません。エラー行にもリクエスト引数は
  含まれません。
- アップストリーム API のリクエストログは、呼び出し側が `traceparent` を
  送った場合にのみ 1 行書かれ、その行にあるのも正規化されたエンドポイント
  ラベルと trace_id だけです — パス・クエリは記録されません。
- サーバー前段のアクセスログはクエリ文字列を除去し、スラッグ・バージョンが
  パスに載る `/v1/releases/…`・`/v1/upgrade/…` のパスはプレフィックスだけを
  残し、IP をマスクし、リクエストボディの記録項目自体がありません。外部には
  送信されず、サーバーローカルでサイズ基準のローテーションが行われます
  （現在のトラフィックで数日分）。
- 後述の監査ストリームはホスト版では無効で、有効にすることはありません。

ただし、管理の及ばない境界が 1 つあります。CDN 区間の接続
メタデータは CDN 事業者のポリシーに従い、これは ratatosk.io が制御できない
領域です。この境界が要件に合わない場合はセルフホストを使ってください —
`check_stack` に渡したバージョンがプロセスの外に出なくなります。

## ローカル（stdio）

stdio は、MCP クライアントがサーバーを子プロセスとして起動し、標準入出力で
やり取りする方式です。手元の PC で Claude Code や Claude Desktop と一緒に
使うときの基本の選び方です。

### Claude Code

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.0
```

`0.7.0` の部分には任意の
[リリースタグ](https://github.com/garlicKim21/ratatosk-mcp/releases)を
指定できます。常に最新を追うなら `latest` を使ってください —
[バージョン固定](#バージョン固定)参照。

### Claude Desktop

Claude Desktop は CLI の代わりに設定ファイル
（`claude_desktop_config.json`）で MCP サーバーを登録します。`mcpServers`
に次を追加してください：

```json
{
  "mcpServers": {
    "ratatosk": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "ghcr.io/garlickim21/ratatosk-mcp:0.7.0"]
    }
  }
}
```

設定ファイルの場所は OS ごとに異なるため、Claude Desktop のドキュメントを
参照してください。インストールなしで使うなら、上の
[ホスト版エンドポイント](#ホスト版エンドポイント)をリモート
コネクタとして登録しても構いません。

### ソースからビルド

Docker なしでバイナリとして使うには：

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

stdio に対応した MCP クライアントであれば、いずれも同じ手順です。

### 確認

```bash
claude mcp list
# ratatosk: docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.0 - ✔ Connected
```

そのうえで、エージェントにこう聞いてみてください：

> 「envoy v1.36.8 と istio 1.30.1 を運用中。アップグレード前に対応すべき
> ことはある？」

エージェントが `check_stack` を呼び、現行バージョン以降のセキュリティ修正・
削除・デフォルト値の変更を、根拠の引用とともに挙げて答えれば成功です。

## クラスタ内への単独デプロイ（Helm）

`MCP_HTTP_ADDR` を設定すると、同じバイナリが `/mcp` で Streamable HTTP の
MCP を提供します（`/healthz` ヘルスチェック付き）。チャートはこれを
ClusterIP Service としてデプロイし、`MCP_HTTP_ADDR` を `service.port`
（デフォルト 8080）に合わせて自動設定します。クラスタ内のどのクライアントも、
プロセスを起動する代わりに URL で接続できます。SDK で作った独自
エージェント、ほかのエージェントフレームワーク、アップグレード前に
`check_stack` でゲートをかける CI ジョブなど、いずれも同じです。

### インストール

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

RBAC もシークレットも不要で、Pod Security Standards（PSS）の `restricted`
プロファイルでも警告なしに動作します。チャートオプションの全体は
[チャート README](../charts/ratatosk-mcp/README.ja.md) にまとまっています。
MCP 仕様 2026-07-28 リビジョンを使う最新クライアント
（`Mcp-Protocol-Version: 2026-07-28` ヘッダーを送るクライアント）を HTTP
で受け付ける場合や、レプリカを 2 つ以上に増やす予定がある場合は、
`--set statelessHttp=true` を有効にしてください —
[設定リファレンス](#設定リファレンス)参照。

### 確認

Pod が起動しているか：

```bash
kubectl get pods -l app.kubernetes.io/name=ratatosk-mcp
# NAME                            READY   STATUS    RESTARTS   AGE
# ratatosk-mcp-6f7b9c8d4-x2m5q    1/1     Running   0          30s
```

起動ログの 1 行に、待ち受けアドレスとアップストリームが出力されます：

```bash
kubectl logs deploy/ratatosk-mcp
# {"time":"…","level":"INFO","msg":"listening","service":"mcp","transport":"http","addr":":8080/mcp","mode":"stateful","upstream":"https://ratatosk.io","version":"0.7.0"}
```

ヘルスチェックまで確認するなら：

```bash
kubectl port-forward svc/ratatosk-mcp 8080:8080 &
sleep 2   # port-forward の準備が整う前に curl を実行すると connection refused になります
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
```

確認が終わったら、バックグラウンドの port-forward を `kill %1` で終了して
ください。

### 接続

クラスタ内の MCP クライアントには、次の URL を指定してください：

```
http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

### アップグレード

```bash
git -C ratatosk-mcp pull
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

インストール時に `--set` で値を変えていた場合は、アップグレードでも同じ値を
再指定するか `--reuse-values` を付けてください。

## kagent 統合

次の 2 つはどちらを選んでも結果は同じです。いずれか一方を実行してください。

**A. Helm トグル** — インストール 1 回で、サーバー、kagent への登録
（RemoteMCPServer）、サンプルエージェント `ratatosk-agent` までデプロイ
されます：

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` のデフォルトは `default-model-config` です。環境が
異なる場合は上書きし、サンプルエージェントなしで登録だけしたい場合は
`kagent.agent.enabled=false` を設定してください。

**B. マニフェスト** — 同じ 3 つのリソースを、Helm を使わずコピー＆ペースト
で適用します（[詳細](../examples/kagent/README.ja.md)）：

```bash
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml
kubectl apply -f $BASE/ratatosk-agent.yaml
```

この経路にクローンは不要です。マニフェストには `namespace: kagent` が
入っています。

### 確認

どちらの経路でも、kagent UI に `ratatosk-agent` が現れます。こう聞いて
みてください：

> 「このクラスタで、アップグレード前に対応すべきことはありますか？」

エージェントは kagent 内蔵の読み取り専用クラスタツールで、稼働中の
バージョンを自分で調べます（`kagent.agent.k8sTools=false` で無効化
できます）。質問にバージョンを直接書いても構いません。エージェントが UI に
現れない場合は[トラブルシューティング](#トラブルシューティング)を見て
ください。

> **モデルの最低要件**：エージェントを 1 回実行すると、内部でモデル
> 呼び出しが 6 回以上発生します。一方で kagent の Go ADK は、
> 429（リクエスト上限超過）応答をリトライしません。したがって毎分およそ
> 10 リクエスト以上を許容するモデルティアが必要で、それより低いと実行の
> たびに失敗します。たとえば Gemini 無料ティア（2026-07 時点）では、毎分
> 5 リクエストの full-flash 系では実行が完走せず、flash-lite 系だけが
> エージェントを動かせる上限を備えています。

## 設定リファレンス

サーバーの動作を変える設定はすべて環境変数で、Helm チャートは各変数に
対応する値を提供します。チャートにはこのほかに、レプリカ数・リソース・
サービスタイプのようにデプロイの形を決める値もあります —
[チャート README](../charts/ratatosk-mcp/README.ja.md) 参照。

| 環境変数 | チャート値 | デフォルト | 説明 |
|---|---|---|---|
| `RATATOSK_URL` | `ratatoskUrl` | `https://ratatosk.io` | アップストリームの changes API（`/v1`、公開、読み取り専用）。egress をプロキシ・ミラー経由にするとき変更 |
| `MCP_HTTP_ADDR` | *（チャートが `service.port` から自動設定）* | *（未設定 = stdio）* | 設定時（例：`:8080`）は stdio の代わりに `/mcp` の Streamable HTTP で提供、`/healthz` 付き |
| `MCP_HTTP_STATELESS` | `statelessHttp` | *（オフ）* | `1` でセッション状態なしの HTTP：`Mcp-Session-Id` の往復がなく、MCP 仕様 2026-07-28 リビジョンを使う最新クライアントを HTTP で受けるには必須です。レプリカ 2 つ以上への水平スケールにも推奨。旧来のクライアントはどちらでも動きます |
| `MCP_LOG` | `logLevel` | `info` | 許容値は `info`（デフォルト）・`debug`・`warn`・`error` — 認識できない値は警告なしに `info` として扱われます。`debug` はアップストリーム呼び出しごとの所要時間を追加し、`warn`・`error` はログを減らします。どのレベルでもリクエスト引数は記録されません — [ログと監査ストリーム](#ログと監査ストリーム)参照 |
| `MCP_AUDIT` | `auditMode` | *（オフ）* | `metadata` または `full` — ツール呼び出しの監査ストリーム。[監査ストリームを有効にする](#監査ストリームを有効にする)参照 |
| `MCP_RATE_LIMIT_PER_MIN` | `rateLimitPerMin` | *（オフ）* | 呼び出し元ごとの毎分ツール呼び出し上限。自分が管理していない呼び出し元を受けるサーバー向けです。呼び出し元アドレスごとに枠を分けるため、誰か 1 人が忙しくても他が枯渇しません。超えると `Retry-After` 付きの `429` を返します。アドレスはウィンドウの間だけのメモリ上のバケットキーであり、記録もアップストリームへの転送もしません。前段のリバースプロキシが `X-Forwarded-For` を渡す必要があります。単一利用者のインストールではオフのままに |

docker で HTTP モードを直接起動する例：

```bash
docker run --rm -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:0.7.0
```

## ログと監査ストリーム

### 運用ログ

ログは stderr に JSON 1 行ずつ書かれます（stdout は stdio トランスポート
専用）。デフォルトレベル（`info`）で残るのは、ライフサイクル行（起動時の
`listening` 1 行、stdio セッション終了の 1 行）と、アップストリーム接続が
不調なときの警告・エラー行だけです。正常なリクエストは 1 行も残しません。
なお、stdio セッション終了の行には 2 つの形があります。クライアントが
正常に終了処理を行って切断した場合は `"msg":"stdio session ended"`（INFO）、
終了処理を行わずに切断された場合は
`"msg":"stdio session ended with error"`（ERROR）です。後者も
障害ではなく、終了のしかたの記録です。エラーが起きても元のエラーメッセージは
コピーしません（リクエスト URL に稼働中のバージョンが含まれるためです）。
代わりに、エンドポイントのパターンとエラーの種類だけからログの文言を
作り直して記録します：

```json
{"time":"…","level":"ERROR","msg":"upstream fetch failed","service":"mcp","upstream":"/v1/changes","kind":"connection_refused","tool":"check_stack"}
```

`MCP_LOG=debug` に上げると、これに加えてアップストリーム呼び出しごとに
エンドポイントのパターン・ステータスコード・所要時間が 1 行ずつ付きます。
**どのレベルでも、リクエスト引数（バージョンなど）はログに含まれません。**

### 監査ストリームを有効にする

監査ストリームは「どのクライアントがどのツールを呼んだか」を残す別枠の
記録です。**デフォルトはオフで、オフの間は何も出力されません** —
ストリーム自体が存在しません。

有効にするには `MCP_AUDIT` を設定します：

```bash
# docker
docker run -i --rm -e MCP_AUDIT=metadata ghcr.io/garlickim21/ratatosk-mcp:0.7.0

# Helm
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp --set auditMode=metadata
```

有効にすると、ツール呼び出しごとに `event:"audit"` の JSON 1 行が運用ログと
同じストリーム（stderr）に残ります：

```json
{"argument_names":["components","detail"],"client_name":"my-agent","client_version":"1.0.0","event":"audit","level":"INFO","msg":"audit","outcome":"ok","service":"mcp","time":"2026-07-31T02:54:47.029169122Z","tool":"check_stack","transport":"stdio"}
```

0.6.2 からは、サーバーをステートフル（状態保持）HTTP モード（チャートの
デフォルト）で動かしているとき、監査レコードにトランスポートセッション
識別子である `session_id` フィールドが加わります（例：
`"session_id":"T3E77BYZFDDA33SIUSORQ365ZL"`）。同時に接続している
呼び出し元をセッション単位で区別するための値です。`statelessHttp`
（環境変数 `MCP_HTTP_STATELESS`）を有効にすると、特定できるセッションが
存在しないため、`session_id` もクライアントが自己申告した `clientInfo` も
レコードから消え、呼び出し単位をつなぐキーは後述の `trace_id` だけに
なります。監査ストリームで呼び出し元を区別する必要があるなら、ステートフル
モードで動かすか、呼び出し元に `traceparent` を送らせてください。上の例の
ような stdio のレコードにも `session_id` はありません。プロセス 1 つに
呼び出し元は 1 つだけで、区別するものがないためです。

2 つのモードの違い：

- **`metadata`** — ツール名、結果（`ok`・`error`・`tool_error`）、
  トランスポート、クライアントが自己申告した `clientInfo`（stdio・
  ステートフル HTTP のとき）、そして引数の**名前**だけ。引数の値はこの
  モードでは決して記録されません。
- **`full`** — 上記に加えて引数の値の全体（`arguments` フィールド）。
  `check_stack` に渡したバージョン一覧もここに入るため、その値がログ
  システムに残ってよいか判断してから有効にしてください。

このストリームは利用者のインフラ内で生成され、自前のログコレクタに
蓄積されます。保持期間と改ざん防止はログプラットフォーム側で決めればよく、
`event` フィールドが `audit` のレコードを別のシンクにルーティングすれば、
運用ログとは別の保持ポリシーを与えられます。

限界も知っておいてください。このサーバーには認証がないため、記録上の
「呼び出し元」は、クライアントが自己申告した `clientInfo` と、トランス
ポート層から得られる情報までです。どのユーザーがエージェントにその呼び出し
を行わせたかは、エージェント層の記録から探す必要があります。

ホスト版エンドポイントに監査ストリームはありません — リクエスト内容を
保持しないという運用原則に従い、常にオフです。監査要件があるなら
セルフホストを使ってください。

### トレースの相関付け（traceparent）

エージェントフレームワークが W3C trace context（分散トレーシング標準の
リクエスト識別ヘッダー）に対応している場合、ツール呼び出しの `_meta` に
`traceparent` を載せて送れます：

```json
{"method":"tools/call","params":{"name":"check_stack","arguments":{"components":[…]},"_meta":{"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}}}
```

すると、その呼び出しが残すログ行（エラー・`debug` レベル）と監査レコードに
`trace_id` フィールドが付与され、アップストリームの `/v1` リクエストにも
同じ `traceparent` ヘッダーが転送されて、エージェント → MCP サーバー →
アップストリームを 1 つの `trace_id` でつなげられます。先に述べた
「監査記録だけでは呼び出し元をたどりきれない」という限界を、層をまたいで
補うための結合キーがこれです。形式が不正な値は破棄されて転送されず、
送らなければ何も記録されません。

## トラブルシューティング

| 症状 | 確認 | 原因と対処 |
|---|---|---|
| ツール呼び出しがエラーを返し（`check_stack` はエラーの代わりにコンポーネントごとの `note` に `fetch failed: …`）、ログに `"msg":"upstream fetch failed"` と `kind: connection_refused`・`dns`・`timeout` | `kubectl logs deploy/ratatosk-mcp`（ローカルはクライアントの MCP ログ） | egress の遮断。`ratatosk.io:443` へのアウトバウンド HTTPS を許可するか、ミラーを立てて `RATATOSK_URL`（チャート：`ratatoskUrl`）で指定 |
| ログに `"msg":"upstream rate limited"`、`status: 429` | 上と同じ | アップストリーム上限（毎分 1200 リクエスト/IP）の超過。プロジェクトごとの `list_changes` ポーリングを `check_stack` 1 回にまとめ、しばらく待って再試行 |
| HTTP クライアントが `400 Bad Request: protocol version "2026-07-28" is only supported on stateless HTTP servers` を受け取る | クライアントが送る `Mcp-Protocol-Version` ヘッダー | クライアントが MCP 2026-07-28 リビジョンを使っており、ステートフル HTTP モードはこれを拒否します。`--set statelessHttp=true`（環境変数：`MCP_HTTP_STATELESS=1`）を有効にしてください — エラー文の `StreamableHTTPOptions.Stateless` は同じスイッチの Go SDK 側の名前です |
| kagent UI に `ratatosk-agent` が現れない | `kubectl api-resources \| grep kagent` · `kubectl get pods -n kagent` | kagent CRD のないクラスタでは `kagent.enabled=true` のインストールが失敗します — 先に kagent をインストールしてください。マニフェスト経路は `namespace: kagent` がハードコードされているため、kagent を別のネームスペースに入れている場合はマニフェストの修正が必要です |
| kagent でエージェントランタイムを Go ADK に変えると `ImagePullBackOff` | `kubectl describe pod` | このチャートとは無関係な、kagent 0.9.12 自体の既知の問題（[#2247]、0.9.12 以降で修正）：Go ADK イメージは `ghcr.io` にのみ公開されているのに、コントローラのデフォルトが廃止済みの `cr.kagent.dev` を指しています。回避策：kagent のインストールに `--set controller.agentImage.registry=ghcr.io` |
| kagent エージェントが実行のたびに失敗 | エージェントログの 429 | モデルティアのリクエスト上限不足 — [kagent 統合](#kagent-統合)のモデル最低要件を参照 |

[#2247]: https://github.com/kagent-dev/kagent/issues/2247

## バージョン固定

- **コンテナイメージ**：リリースごとにバージョンタグ（例：`0.7.0`）で自動
  ビルドされ、マルチアーキテクチャ（`linux/amd64`、`linux/arm64`）です。
  `latest` は最新リリースを追います — 再現可能なデプロイにはバージョンタグを
  固定してください。
- **Helm**：チャートはチャートリポジトリに公開されず、リポジトリの
  クローンからのみインストールします。特定バージョンに固定するには、
  クローン後にリリースタグをチェックアウトしてください：

  ```bash
  git -C ratatosk-mcp checkout v0.7.0
  ```

  イメージタグはデフォルトでチャートの `appVersion` に従い、`image.tag`
  値で上書きできます。

## アップストリーム API

このサーバーは公開 REST API の薄いクライアントです。直接呼びたい場合は、
ratatosk.io の `GET /v1` を呼ぶと API 自身の説明が返ります。API キーは
不要で、IP ごとに毎分 1200 リクエストの制限があります。

## 次のステップ

- 7 つのツールのパラメータ・呼び出し例・実測レスポンス：[ツールリファレンス](tools.ja.md)
- ツール一覧表（早見）：[README のツール表](../README.ja.md#ツール)
- チャート値の全体と kagent トグル：[チャート README](../charts/ratatosk-mcp/README.ja.md)
- kagent のマニフェストとサンプルエージェント：[kagent の例](../examples/kagent/README.ja.md)
