# ツールリファレンス

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

このページは、ratatosk-mcp が提供する 7 つの
ツール（[`check_stack`](#check_stack)、[`list_projects`](#list_projects)、
[`list_releases`](#list_releases)、[`get_release`](#get_release)、
[`changes_by_entity`](#changes_by_entity)、[`get_matter`](#get_matter)、
[`list_changes`](#list_changes)）の詳細リファレンスです。ツールごとに、
用途、パラメータ一覧、実際の呼び出しとレスポンス、そしてエージェントに
尋ねる質問の例をまとめました。サーバーへの接続方法（ホスト版
エンドポイント、Docker、Helm、kagent）は
[インストールと使い方](install.ja.md)を見てください。

> **2026-08-10 時点の実測**：この文書のすべての呼び出しとレスポンスは、
> このリポジトリのサーバーをビルドし、公開 API `https://ratatosk.io` に
> 接続して受け取ったものをそのまま載せています。`check_stack` の例に
> 使ったスタック（コンポーネント 5 種と `version_source` の文言）は、
> 匿名化した実クラスタのものです。リリースデータは毎時増え続けるため、
> いま呼び出すと数値やリストは変わり得ますが、レスポンスの形は同じです。
> 長いレスポンスは表示の都合で一部を `…` に省略しています。

## 変更モデルを 1 ページで

ここにあるツールはすべて同じ単位を返します。**変更（change）** 1 件とは、
あるリリースで起きた出来事 1 つです。CVE を伴う修正、削除されたフラグ、
名前が変わった設定フィールド、反転したデフォルト値 — そして、それを
読み取った元のリリースノートの一文が必ず一緒に付いてきます。

すべての変更は 3 つの軸で説明され、3 つの軸は互いに独立です。

| 軸 | 値 | 答える問い |
|---|---|---|
| `family` | `security` · `breaking` · `deprecated` | これはどんな種類の出来事か？ |
| `bucket` | `action` · `check` · `plan` | *いま* どう動けばよいか？ |
| `applies_if` | 条件。可能なら構造化される | そもそも自分に関係あるか？ |

**深刻度より先に `bucket` を読んでください。** `action` の項目は、その
バージョン帯を通過するすべてのインストールに当てはまります。`check` の
項目は `applies_if` が成り立つ環境にだけ当てはまるので、作業ではなく、
実際の設定に照らして判定すべき問いです。`plan` の項目は将来のリリースに
向けた予告です。ウェブサイトも、週次メールも、`check_stack` も同じこの
フィールドで振り分けるため、3 つの表面は偶然ではなく構造として一致します。

残りの 4 つのフィールドが大半の重みを担います。

- **`matter_key`** — リリースをまたいだ「案件」の同一性。同じアドバイザリを
  5 つのブランチで直したなら、`matter_key` は 1 つ、変更は 5 件です。
  [`get_matter`](#get_matter) がこれを展開します。
- **`applies_if.clauses`** — 条件を構造化できた場合、節ごとに
  `kind`（`api`、`crd`、`feature_gate`、`flag`、`config_field`、`extension`、
  `subsystem`、`dependency` など）と `name`、`verb`、`polarity` が付き、
  `mode`（`all_of`、`any_of`、`universal`）でまとめられます。文章を解釈する
  のではなく、これらの名前を実際の設定から探してください。
- **`advisories`** — 引用された CVE・GHSA id と、その **現在の** 深刻度。
  リリースノートが当時主張した深刻度とは異なることがあります。
- **`quote`** — 根拠となった原文の一文。判断はいつでも `source_url` に
  照らして確かめられるべきだからです。

リリースの `changes` が `[]` なら、ノートは読まれ、運用者が対処すべきものは
なかったという意味です。欠落ではなく **監査可能な沈黙** です。
`notes_total` は、1 件ずつ表に出さないと決めた日常的な記録（ボットの
依存関係更新など）の数です。

## 質問からツール呼び出しまで — 実例で追う

エージェントにこう尋ねたとします：

> 「うちのクラスタの状態を確認してください。」

実際に使ってうまくいった言い回しをそのまま載せています。クラスタを直接
読めるエージェント、たとえば kagent に ratatosk-mcp とクラスタ読み取り
ツールを併せて接続した構成で実行したとき、ツールはこの順に呼ばれました：

```
list_projects
k8s_get_resources  (pod, all_namespaces)
k8s_get_resources  (daemonset, all_namespaces)
k8s_get_resources  (deployment, all_namespaces)
k8s_get_resources  (node)
check_stack        (components ×5)
get_release        (cilium, v1.20.0)               ← action_required の深掘り
k8s_get_resources  (customresourcedefinition)
k8s_get_resources  (ciliumnodeconfig, all_namespaces)
k8s_get_resources  (configmap, all_namespaces)
k8s_get_resource_yaml (kube-system/cilium-config)  ← applies_if の判定
```

名前について一言：`k8s_*` は kagent-tools（別の MCP サーバー）が提供する
クラスタ読み取りツールです。この並びのうち ratatosk のツールは
`list_projects`、`check_stack`、`get_release` の 3 つです。

この経路は 3 段階に分かれます。

**第 1 段階 — `list_projects` で名簿を確定する。** クラスタで見えた名前を、
他のツールが受け取る正式なスラグに対応づける段階です。名前が異なる場合は
`image_aliases` が解決します。クラスタの `cilium-envoy` デーモンセットは
`envoy` プロジェクトです。

**第 2 段階 — スタック全体を `check_stack` 1 回で。** 5 回に分けて尋ねる
代わりにコンポーネント 5 つをまとめて送り、それぞれにバージョンをどこで
読んだかを `version_source` として添えます。

**第 3 段階 — 判定し、深掘りする。** ブリーフィングは全員に当てはまるものと
設定次第のものに分かれ、エージェントは両側を処理しました。
`action_required` の項目のリリースは `get_release` で覗き、ブリーフィング
だけでは答えられない `applies_if` は `cilium-config` を読んで判定しています。

大事なのは答えの形です。エージェントは「アップデートがあります」とは
言いませんでした。無条件に当てはまる項目はどれか、実際の設定に照らして
確認したものはどれか、確認できなかったものはどれかを分けて報告しています。

## check_stack ブリーフィングの読み方

既定の `check_stack` レスポンス（`detail:"brief"`）は、コンポーネントごとに
同じ構造です。下の [check_stack の節](#check_stack)にある実測レスポンスを
手元に置いて読んでください。

1. **`changes_scanned` と `summary` — 数字。** `changes_scanned` はその
   プロジェクトに記録されている変更の総数、`summary.new_changes` はその
   うち実行中のバージョンより上にあるものの数です。`distinct_matters` は
   同じ `matter_key` の繰り返しをまとめた後に残る数です。実測レスポンスの
   containerd がその例で、35 件を走査して 11 件が新しく、案件としては 9 件に
   なります。`by_severity`・`by_family`・`by_bucket` はいずれも、生の変更
   ではなくまとめた後の案件を数えます。
2. **3 つのリストは `bucket` だけで分かれます** — `action` は
   `action_required` へ、`check` は `check_config` へ、`plan` は
   `other_changes` へ。深刻度で分けているのでは **ありません**。実測
   レスポンスでは containerd の `check_config` に `low` が 5 件あり、coredns の
   `action_required` には `medium` が 1 件あります。深刻度はリストの
   **なかで** 緊急度を並べるもので、どのリストに入るかを決めるのはバケットです。
3. **`action_required` — 設定に関わらず対処するもの。** containerd の最初の
   項目はアドバイザリ id を 10 個抱えたセキュリティのロールアップで、その
   最悪のものが `critical` なので `critical` です。ここに並ぶ項目はたいてい
   `applies_if` を持ちません。いくつかは条件付きですが、それはそのプロジェクト
   では事実上普遍的な条件だからです（例：「runs http2」が付いた envoy の項目）。
4. **`check_config` — 勧める前に条件を判定すること。** ここに並ぶ項目は
   すべて `applies_if` を持ちます。実際の設定に照らして確認するまで、これは
   作業ではありませんし、条件が成り立たないことはアップグレードの理由には
   なりません。代わりに前向きに読んでください。その項目の `version` は、
   `applies_if` が言うものを **有効にする前に** 到達しているべき最小バージョン
   です。`applies_if_targets` は実際の設定から探す名前を並べます —
   containerd の `["CreateContainer", "sandbox"]`、cilium の `["ipBlock"]` の
   ように。`applies_if` が文章だけのときはこのフィールドはありません。
5. **`other_changes` — `plan` バケットを 1 行ずつ。** 将来のリリースに向けた
   非推奨の予告です。containerd の 2 件はどちらも `window.deprecated_in` を
   持ち、それが時計の動き始めた時点です。
6. **ここでの `severity` は ratatosk ではなくこのサーバーが計算します。**
   その項目の `advisories` のうち最も高い深刻度で、アドバイザリがなければ
   `security` ファミリーは `high`、それ以外はバケットが決めます —
   `action` は `medium`、`check` は `low`、`plan` は `info`。cilium の
   `[security]` タグ付き依存関係更新が `advisories` なしで `high` に見えるのは
   このためです。アドバイザリそのもので判断したいなら
   [`get_release`](#get_release) か
   [`changes_by_entity`](#changes_by_entity) を使ってください。そちらは
   アドバイザリをそれぞれの深刻度とともに返します。
7. **マージ規則が 2 つ、どちらも実測レスポンスに現れています。** ブランチを
   またいで `matter_key` が同じ変更は最も早い修正に畳まれ、その案件を載せた
   **他のすべてのリリース**が（返却ウィンドウの内外を問わず）
   `same_matter_also_addressed_in` に並びます。運用者は一度上げるのだから、
   その案件を閉じる最も近いリリースこそが実行可能な答えだからです。これとは
   別に、**1 つのリリースのなかで** 引用文が同じ変更は id をまとめて 1 項目に
   統合され、条件が異なる場合は `applies_if` の代わりに `applies_if_any` が
   付きます。統合された項目は、構成員のうち最も高い深刻度と最も急ぐバケットを
   採ります。
8. **ブリーフィングはブランチを見ます。見なければなりません。** メンテナは
   1 つの修正をサポート中のすべてのブランチにバックポートするので、同じ
   `matter_key` が同じ日に v2.2.4 と v2.3.1 に着地します。「実行中の
   バージョンより新しいリリース」だけを読むと、2.2.5 を使う運用者に v2.3.1 の
   項目を見せて `action_required` と呼ぶことになります — 自分のブランチが
   1 リリース前に閉じた仕事をやれ、という意味です。そこで、**実行中の
   バージョンのブランチで、実行中のバージョン以下にすでに修正済み**の案件は
   除外し、何件除いたかを `note` に記します。ブランチが同じであることが、
   これを安全にします。v2.1.9 へのバックポートは v2.2.4 について何も語りません
   — 別のブランチから切られ、その修正を受け取っていない可能性があるからです。
   証拠になるのは実行中のバージョン自身のブランチだけです。
9. **`note` — コンポーネントごとの状態マーカー。** 2 種類が現れます。
   `tracked:false` に伴う "NOT tracked by ratatosk — zero changes means no
   coverage here, not safety" と、"running version … is older than every
   release on record" です。後者は、バージョンの申告に対してここで可能な
   唯一の照合です。このサーバーはあなたの環境を見られないので、記録全体より
   古いバージョンは、生きたリソースから読み直せという合図です。どちらの
   マーカーもなく `new_changes: 0` なら静かなケース — 追跡していて、走査
   していて、実行中のバージョンより上に何もない、ということです。実測
   レスポンスの kubernetes がそれで、45 件を走査して新規 0 件です。
10. **`hint` と `privacy`。** `hint` はこのブリーフィングが推奨ではなく
   データの分類であることを改めて述べ、次に使うツールを指し示します。
   `privacy` の行は、この呼び出しであなたのバージョンがどこまで行ったかを
   記録します。

## check_stack

実行中のコンポーネントのバージョン一覧を受け取り、コンポーネントごとに
アップグレード経路上の変更 — 実行中のバージョンより新しいリリース群 — を
返します。「上げる前に対処すべきものはあるか？」が問いなら、まず手に取る
ツールです。1 回の呼び出しが複数のプロジェクトを覆うので、スタック全体を
問う質問には、他のツールをプロジェクトごとに呼ぶよりこちらが適します。

バージョンの比較は **このサーバープロセスのなか** で行われます。上流へ出て
いくのはプロジェクトのスラグだけで、このツールはサーバー側の
`/v1/upgrade` エンドポイントを決して呼びません。サーバーを自分で動かせば
実行中のバージョンはあなたのインフラから出ませんし、ホスト版
エンドポイントではサーバーのメモリを通るだけで記録されません。

1 つのリリースを深く見るなら `get_release`、CVE やフラグ 1 つを追うなら
`changes_by_entity`、1 つの案件を直したすべてのブランチを見るなら
`get_matter` へ進んでください。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `components` | はい | array | — | 確認する実行中スタック。項目の形は下記 |
| `components[].project` | はい | string | — | プロジェクトスラグ（例：`envoy`）。正典は `list_projects` の `slug` |
| `components[].version` | はい | string | — | 現在実行中のバージョン（例：`v1.36.8`） |
| `components[].target_version` | いいえ | string | *(なし = 最新まですべて)* | アップグレード先。実行中バージョンより厳密に上である必要があり、`running < version <= target` の変更だけが返ります。実行中バージョン以下を指定すると範囲が空になるため無視され、`note` が付きます |
| `components[].version_source` | いいえ | string | *(なし)* | そのバージョンをどこで読んだか（例：`daemonset/cilium image tag`、あるいはユーザーが申告したという事実）。後から監査できるようそのまま返されます。サーバーはあなたの環境を見られず、バージョンを検証できません — 上で説明した `note` マーカーが、ここで可能な唯一の照合です |
| `detail` | いいえ | string | `brief` | `brief`：`summary` とバケット別の 3 リスト（マージ済み・1 行ずつ）。`full`：該当する変更を全文で `relevant_changes` に（マージなし・サマリなし） — コンポーネントあたり 50 件で打ち切り、切られた数は `relevant_changes_omitted` に |
| `severity_min` | いいえ | string | *(なし = すべて)* | この深刻度以上のみ：`info`、`low`、`medium`、`high`、`critical` |

`brief` では `other_changes` の末尾はコンポーネントあたり 100 件で打ち切られ、
切られた数は `other_changes_omitted` に載ります。黙って捨てることはありません。

### 呼び出し例

上のシナリオに出てきたスタックです。以下の JSON はツールの `arguments`
オブジェクトだけで、これが載る JSON-RPC のエンベロープは
[インストールと使い方](install.ja.md)で扱います。

```json
{
  "components": [
    { "project": "kubernetes", "version": "v1.36.1", "version_source": "kubectl version (server)" },
    { "project": "containerd", "version": "2.2.4",   "version_source": "node status containerRuntimeVersion" },
    { "project": "cilium",     "version": "v1.19.4", "version_source": "daemonset/cilium image tag" },
    { "project": "envoy",      "version": "v1.36.7", "version_source": "daemonset/cilium-envoy image tag" },
    { "project": "coredns",    "version": "v1.14.1", "version_source": "deployment/coredns image tag" }
  ]
}
```

### 実測レスポンス（一部省略）

```json
{
  "components": [
    {
      "project": "kubernetes",
      "running_version": "v1.36.1",
      "version_source": "kubectl version (server)",
      "changes_scanned": 45,
      "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
    },
    {
      "project": "containerd",
      "running_version": "2.2.4",
      "version_source": "node status containerRuntimeVersion",
      "changes_scanned": 35,
      "summary": {
        "new_changes": 11, "distinct_matters": 9,
        "by_severity": { "critical": 1, "high": 1, "low": 5, "info": 2 },
        "by_family": { "breaking": 5, "deprecated": 2, "security": 2 },
        "by_bucket": { "action": 2, "check": 5, "plan": 2 }
      },
      "action_required": [
        {
          "change_id": "containerd:v2.2.5:a74c2b48",
          "matter_key": "containerd/advisory:cve-2026-47262",
          "version": "v2.2.5",
          "kind": "defect_corrected",
          "family": "security",
          "bucket": "action",
          "severity": "critical",
          "quote": "CVE-2026-50195",
          "advisories": [
            "CVE-2026-47262", "CVE-2026-50195", "CVE-2026-53488", "CVE-2026-53489",
            "CVE-2026-53492", "GHSA-33vj-92qq-66hc", "GHSA-cvxm-645q-p574",
            "GHSA-jpcc-p29g-p8mq", "GHSA-rgh6-rfwx-v388", "GHSA-xhf5-7wjv-pqxp"
          ],
          "same_matter_also_addressed_in": [ "v2.3.2" ]
        },
        { "change_id": "containerd:v2.3.1:040369eb", "version": "v2.3.1", "severity": "high",
          "family": "security", "bucket": "action", "quote": "* [**CVE-2026-46680**]",
          "advisories": [ "CVE-2026-46680", "GHSA-fqw6-gf59-qr4w" ] }
      ],
      "check_config": [
        {
          "change_id": "containerd:v2.2.6:b8305908",
          "matter_key": "containerd/api/createcontainer#constraint_changed",
          "version": "v2.2.6",
          "kind": "constraint_changed",
          "family": "breaking",
          "bucket": "check",
          "severity": "low",
          "applies_if": "uses the CreateContainer API and runs sandbox",
          "applies_if_targets": [ "CreateContainer", "sandbox" ],
          "quote": "* cri: reject CreateContainer when sandbox is not running (#13669)",
          "same_matter_also_addressed_in": [ "v2.3.3" ]
        },
        { "change_id": "containerd:v2.3.1:4dff4eb6", "version": "v2.3.1", "severity": "low",
          "kind": "removed", "family": "breaking", "bucket": "check",
          "applies_if": "runs user namespace", "applies_if_targets": [ "user namespace" ],
          "window": { "removed_in": "v2.3.1" },
          "quote": "* Disable overlayfs \"rebase\" capability when running in user namespace" },
        …3 件省略…
      ],
      "other_changes": [
        { "change_id": "containerd:v2.3.0:a4f90153",
          "matter_key": "containerd/api/shim.command#deprecated",
          "version": "v2.3.0", "kind": "deprecated", "family": "deprecated",
          "bucket": "plan", "severity": "info",
          "applies_if": "uses the shim.Command API",
          "applies_if_targets": [ "shim.Command" ],
          "window": { "deprecated_in": "2.3" },
          "quote": "* Deprecate shim.Command" },
        …1 件省略…
      ]
    },
    {
      "project": "cilium",
      "running_version": "v1.19.4",
      "version_source": "daemonset/cilium image tag",
      "changes_scanned": 122,
      "summary": {
        "new_changes": 51, "distinct_matters": 48,
        "by_severity": { "high": 7, "medium": 3, "low": 32, "info": 6 },
        "by_family": { "breaking": 35, "security": 7, "deprecated": 6 },
        "by_bucket": { "action": 8, "check": 34, "plan": 6 }
      },
      "action_required": [
        { "change_id": "cilium:v1.20.0:20adda37",
          "matter_key": "cilium/config_field/cni configuration version#default_changed",
          "version": "v1.20.0", "kind": "default_changed", "family": "breaking",
          "bucket": "action", "severity": "medium",
          "window": { "introduced_in": "v1.20.0" },
          "quote": "the default CNI configuration version moves from 0.3.1 to 1.0.0." },
        { "change_id": "cilium:v1.20.0:4bb6c02d", "version": "v1.20.0", "severity": "high",
          "family": "security", "bucket": "action",
          "quote": "fix(deps): update module google.golang.org/grpc to v1.82.1 [security] (v1.20)" },
        …6 件省略…
      ],
      "check_config": [
        { "change_id": "cilium:v1.19.5:41a4ef5b",
          "matter_key": "cilium/config_field/l2podannouncements.interface#renamed",
          "version": "v1.19.5", "kind": "renamed", "family": "breaking",
          "bucket": "check", "severity": "low",
          "applies_if": "enables L2 pod announcements",
          "applies_if_targets": [ "L2 pod announcements" ],
          "window": { "removed_in": "v1.19.5" },
          "quote": "Remove defunct `l2podAnnouncements.interface` Helm value that rendered a configmap key the agent no longer recognises, causing crash-loops when L2 pod announcements were enabled. Users must use `l2podAnnouncements.interfacePattern` instead." },
        { "change_id": "cilium:v1.19.5:9caac3a9",
          "matter_key": "cilium/api/ipblock#defect_corrected",
          "version": "v1.19.5", "kind": "defect_corrected", "family": "security",
          "bucket": "check", "severity": "high",
          "applies_if": "configures the ipBlock API",
          "applies_if_targets": [ "ipBlock" ],
          "quote": "Fix wildcard namespace bypass for selectorless ipBlock rules" },
        …32 件省略…
      ],
      "other_changes": [ …6 件… ]
    },
    {
      "project": "envoy",
      "running_version": "v1.36.7",
      "version_source": "daemonset/cilium-envoy image tag",
      "changes_scanned": 124,
      "summary": {
        "new_changes": 90, "distinct_matters": 45,
        "by_severity": { "high": 12, "medium": 15, "low": 15, "info": 3 },
        "by_family": { "security": 26, "breaking": 16, "deprecated": 3 },
        "by_bucket": { "action": 4, "check": 38, "plan": 3 }
      },
      "action_required": [
        { "change_id": "envoy:v1.36.9:d69a1e5f",
          "matter_key": "envoy/advisory:cve-2026-47261",
          "version": "v1.36.9", "family": "security", "bucket": "action", "severity": "high",
          "quote": "wasm: bumped `com_github_wasmtime` to resolve CVE-2026-47261.",
          "advisories": [ "CVE-2026-47261" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …3 件省略…
      ],
      "check_config": [
        { "change_id": "envoy:v1.36.9:7ccbb3ff",
          "matter_key": "envoy/advisory:cve-2026-48743",
          "version": "v1.36.9", "family": "security", "bucket": "check", "severity": "high",
          "applies_if": "uses HTTP/3 and uses HTTP/1 and uses headers-only request and configures the Content-Length setting",
          "applies_if_targets": [ "HTTP/3", "HTTP/1", "headers-only request", "Content-Length" ],
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "advisories": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …37 件省略…
      ],
      "other_changes": [ …3 件… ]
    },
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "summary": {
        "new_changes": 8, "distinct_matters": 8,
        "by_severity": { "critical": 1, "high": 3, "medium": 1, "low": 3 },
        "by_family": { "breaking": 4, "security": 4 },
        "by_bucket": { "action": 3, "check": 5 }
      },
      "action_required": [ …3 件… ],
      "check_config": [ …5 件… ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation — changes are provided without warranty and the decision stays with the operator): …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

このレスポンス 1 つに 4 つの状態が同居しています。

- **kubernetes** — 記録されている変更は 45 件ですが、v1.36.1 より上にある
  ものはありません。追跡していて、走査していて、静かです。警告することが
  ないので `note` もありません。
- **containerd** — こぢんまりしたケース。新しい変更 11 件が案件 9 件になり、
  バケット別に 2 / 5 / 2 に分かれます。`action_required` の最初の項目が、
  1 つのロールアップから出たアドバイザリ id 10 個を抱えています。
- **cilium と envoy** — 混み合ったケースであり、バケットで分ける理由でも
  あります。envoy は新しい変更が 90 件ありますが、`action_required` に
  落ちるのは 4 件で、38 件は作業ではなく設定についての問いです。
- **coredns** — 他のコンポーネントが静かなスタックに `critical` が 1 つ
  座っています。スタック全体をまとめて問う意味はまさにここにあります。

追跡していないスラグはまた違って見えます。ratatosk が扱っていない
`nginx-ingress` を尋ねると：

```json
{
  "project": "nginx-ingress",
  "running_version": "v1.14.0",
  "version_source": "user-stated",
  "tracked": false,
  "note": "NOT tracked by ratatosk — zero changes means no coverage here, not safety",
  "changes_scanned": 0,
  "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
}
```

スラグを推測すべきでない理由がこれです。結果が「該当なし」として返り、それが
安全のように読めてしまいます。確信がなければ `list_projects` を呼んでください。

記録全体より低いバージョンを実行中なら、カバレッジのマーカーが付きます：

```json
{
  "project": "coredns",
  "running_version": "v1.9.0",
  "note": "running version v1.9.0 is older than every release on record (earliest on record: v1.14.0) — this covers the reviewed window only, so treat it as partial, and re-check that the running version was read off a live resource",
  "changes_scanned": 11,
  "summary": { "new_changes": 11, "distinct_matters": 11, … }
}
```

### `detail:"full"`

`full` は長いレスポンスではなく **別の** レスポンスです。コンポーネントから
`summary` とバケット別の 3 リストが消え、`relevant_changes` が入ります。
該当する変更をサーバーの素の形のまま — `applies_if` は構造化された
オブジェクト、`advisories` はそれぞれの深刻度付き、`subjects` と `seq` まで —
マージせずに返すので、同じ `matter_key` の繰り返しもすべて現れます。

```json
{ "detail": "full", "severity_min": "high",
  "components": [ { "project": "coredns", "version": "v1.14.1", "version_source": "deployment/coredns image tag" } ] }
```

```json
{
  "components": [
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "relevant_changes": [
        {
          "change_id": "coredns:v1.14.2:83ed5f33",
          "matter_key": "coredns/advisory:cve-2026-25679",
          "project": "coredns", "version": "v1.14.2", "version_rank": [ 1, 14, 2 ],
          "released_at": "2026-03-06T06:34:58.000Z",
          "family": "security", "actionability": "act", "bucket": "action",
          "kind": "value_changed",
          "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
          "advisories": [
            { "id": "CVE-2026-25679", "severity": "high" },
            { "id": "CVE-2026-27137", "severity": "high" },
            { "id": "CVE-2026-27138", "severity": "medium" },
            { "id": "CVE-2026-27139", "severity": "low" },
            { "id": "CVE-2026-27142", "severity": "medium" }
          ],
          "subjects": [ { "kind": "dependency", "name": "go 1.26.1", "name_full": "Go 1.26.1", "role": "changed" } ],
          "window": { "introduced_in": "v1.14.2" },
          "quote": "In addition, the release updates the build to Go 1.26.1, which include security fixes addressing CVE-2026-27137, CVE-2026-27138, CVE-2026-27139, CVE-2026-25679, and CVE-2026-27142.",
          "disclosure": "described",
          "source_url": "https://github.com/coredns/coredns/releases/tag/v1.14.2",
          "release_url": "https://ratatosk.io/en/releases/coredns/v1.14.2",
          "seq": 3431
        },
        …3 件省略…
      ]
    }
  ],
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

同じ変更でも 2 つのモードで深刻度が違う点に注目してください。`brief` は
`severity` を 1 つ（グループの最大値である `high`）返し、`full` は
アドバイザリ 5 件をそれぞれの深刻度とともに返します。`full` は
`severity_min` か `target_version` で絞って使ってください。忙しい
プロジェクトに絞り込みなしで当てると、レスポンスは大きくなります。

### 質問例

> 「うちの envoy は v1.36.8 ですが、v1.37.0 に上げる前に対処すべきことは
> ありますか？」

> 「うちのスタックは Kubernetes 1.31、Cilium 1.16、CoreDNS 1.11 です。
> アップグレード前に見ておくべきことはありますか？」

エージェントはどちらも `check_stack` 1 回で答えます。前者は
`target_version` を埋めて、後者はコンポーネント 3 つを 1 回の呼び出しに
まとめて。上のシナリオのようにクラスタを直接読めるエージェントなら、
バージョンを集める作業そのものも任せられます。

## list_projects

追跡しているプロジェクトの全名簿です。引数がなくレスポンスも小さいので、
プロジェクト名からスラグを推測する前にこれを呼んでください。誤ったスラグは
エラーにならず、`check_stack` で `tracked:false` として返ります。つまり
推測ひとつが静かに「カバレッジなし」に変わります。ここの `slug` が、他の
すべてのツールが受け取る `project` 引数の正典です。

### パラメータ

ありません。空の引数で呼んでください。

### 呼び出し例

```json
{}
```

### 実測レスポンス（一部省略）

```json
{
  "projects": [
    { "slug": "argo", "name": "Argo", "tier": "graduated", "category": "cicd", "analyzed_releases": 32 },
    …
    { "slug": "cilium", "name": "Cilium", "tier": "graduated", "category": "networking",
      "analyzed_releases": 21, "cluster_core": true },
    { "slug": "containerd", "name": "containerd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 22, "cluster_core": true,
      "visibility": "observed via node status (nodeInfo.containerRuntimeVersion), never via pods — a workload listing cannot show the runtime" },
    { "slug": "coredns", "name": "CoreDNS", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 7, "cluster_core": true },
    { "slug": "envoy", "name": "Envoy", "tier": "graduated", "category": "networking",
      "analyzed_releases": 23, "image_aliases": [ "cilium-envoy" ], "cluster_core": true },
    { "slug": "etcd", "name": "etcd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 21, "cluster_core": true,
      "visibility": "may live outside the k8s API, be hidden by a managed control plane, or be replaced entirely — a missing pod is not a missing component; when unreadable, report it under Could not check instead of guessing a version" },
    { "slug": "kubernetes", "name": "Kubernetes", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 26, "cluster_core": true,
      "image_aliases": [ "kube-apiserver", "kube-controller-manager", "kube-scheduler", "kubelet", "kube-proxy" ] },
    …
  ],
  "count": 76
}
```

基本フィールド（`slug`、`name`、`tier`、`category`、`analyzed_releases`）の
ほかに、一部の項目には 3 つのマーカーが付きます。

- `image_aliases` — そのプロジェクトがクラスタ内で名乗る別名。別名に合致する
  イメージやワークロードはそのプロジェクトであり、バージョンはタグが示す
  ものです。
- `cluster_core:true` — クラスタの土台（コントロールプレーン、データストア、
  DNS、ランタイム、CNI・データプレーン）。クラスタに存在する
  `cluster_core` プロジェクトはすべて `check_stack` 呼び出しの候補です。
- `visibility` — そのコンポーネントをどう観測するか、そしてどこでなら読めなくても
  正常かのヒント。containerd はポッド一覧ではなくノードの状態から読めと言い、
  etcd はそもそも k8s API の外にあり得ると警告します。読めないコンポーネントは
  推測せず「確認できず」として報告すべきです。

### 質問例

> 「うちのスタックは Kubernetes 1.31、Cilium 1.16、CoreDNS 1.11 です。
> アップグレード前に見ておくべきことはありますか？」

`check_stack` と同じ質問です。上のシナリオで見たとおり、エージェントは
まずこのツールでスラグを確定してから `check_stack` へ進みます。

## list_releases

1 つのプロジェクトの最新 N 件のリリースを、軽いサマリとして新しい順に
返します。「最近 X に何があった？」に答えるツールです。`list_changes` と
対比してください。あちらは同じデータを逆順に — 分析の古いものから —
流す同期フィードで、ローカルの写しを最新に保つためのものです。「最近の
できごと」を問う質問はこちらです。気になる行があれば `get_release` で
深掘りしてください。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `project` | はい | string | — | プロジェクトスラグ（例：`istio`） |
| `limit` | いいえ | integer | `5` | 最近のリリースを何件。最大 `20` |

### 呼び出し例

```json
{ "project": "cilium", "limit": 5 }
```

### 実測レスポンス（一部省略）

```json
{
  "project": "cilium",
  "count": 5,
  "releases": [
    {
      "version": "v1.20.0", "version_rank": [ 1, 20, 0 ],
      "released_at": "2026-07-29T15:00:29.000Z", "reviewed_at": "2026-08-09T04:33:14.949Z",
      "changes_total": 45,
      "by_bucket": { "action": 9, "check": 30, "plan": 6 },
      "by_family": { "breaking": 32, "security": 7, "deprecated": 6 },
      "max_severity": null,
      "notes_total": 478,
      "api_url": "https://ratatosk.io/v1/releases/cilium/v1.20.0",
      "release_url": "https://ratatosk.io/en/releases/cilium/v1.20.0"
    },
    {
      "version": "v1.19.6", "released_at": "2026-07-16T22:52:21.000Z",
      "changes_total": 1, "by_bucket": { "check": 1 }, "by_family": { "breaking": 1 },
      "max_severity": null, "notes_total": 46, …
    },
    {
      "version": "v1.18.12", "released_at": "2026-07-16T22:47:50.000Z",
      "changes_total": 0, "by_bucket": {}, "by_family": {},
      "max_severity": null, "notes_total": 20, …
    },
    { "version": "v1.17.18", "changes_total": 0, "notes_total": 10, … },
    { "version": "v1.19.5", "changes_total": 3, "by_bucket": { "check": 3 },
      "by_family": { "breaking": 2, "security": 1 }, "notes_total": 48, … }
  ],
  "hint": "summaries only — fetch /v1/releases/{project}/{version} for a release's full changes"
}
```

読むときの注意が 3 つあります。

- **`changes_total: 0`** — v1.18.12 と v1.17.18 がそうです — は、ノートを
  最後まで読み、そのリリースが日常的だったという意味です。データがないのとは
  異なる、監査可能な沈黙です。`notes_total` はノートが空でなかったことを
  示します。それぞれ日常的な記録が 20 行と 10 行ありました。
- **`max_severity` はそのリリースの最高 *アドバイザリ* 深刻度** であり、
  どの変更もアドバイザリを引用していなければ `null` です。v1.20.0 は
  `security` ファミリーの変更が 7 件あるのに `null` ですが、これは CVE の
  付かない依存関係更新だからです。ここでの `null` は「深刻なものがない」では
  なく「アドバイザリ id がない」という意味です。
- **大きなリリースでは `notes_total` が `changes_total` を圧倒します** —
  v1.20.0 では 45 に対して 478。その差が、1 件ずつ表に出さないと決めた
  日常的な記録です。

### 質問例

> 「最近 Cilium に何がありましたか？」

エージェントは `list_releases(project: "cilium")` を呼び、サマリだけで
答えます。変更が v1.20.0 に集中していること、そして静かな 2 件のリリースは
飛ばしたのではなく読んだ結果であることまで含めて。

## get_release

1 つのリリースの全体です。エンベロープ（要約、原文 URL、リリース URL、
集計）と、そのリリースのすべての変更が一緒に返ります。`check_stack` や
`list_releases` が浮かび上がらせたリリースを深掘りするとき、そして
`source_url` で原文に照らして判断を検証するときのツールです。`version` を
省略するとそのプロジェクトの最新リリースが返るので、「X の最新リリースは
どんな感じ？」もここに来ます。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `project` | はい | string | — | プロジェクトスラグ（例：`envoy`） |
| `version` | いいえ | string | *(なし = 最新リリース)* | 公開されたままのリリースタグ（例：`v1.38.3`）。先頭の `v` はあってもなくても構いません — プロジェクトごとに表記が違うためです |
| `include_raw` | いいえ | boolean | `false` | リリースノートの原文を `raw_notes` として併せて返す（上限あり。切られた場合は `raw_notes_truncated: true`） |

誤ったタグは、そのプロジェクトの最近のタグを教える `404` になるので、
呼び出した側が 1 回の再試行で自力で直せます：

```
/v1/releases/cilium/v9.9.9: HTTP 404: {"error":"no reviewed release 'v9.9.9' for
project 'cilium'. Recent reviewed versions: v1.20.0, v1.19.6, v1.18.12, v1.17.18,
v1.19.5 — retry with one of these exact tags."}
```

### 呼び出し例

```json
{ "project": "envoy", "version": "v1.38.3" }
```

### 実測レスポンス（一部省略）

```json
{
  "project": "envoy",
  "version": "v1.38.3",
  "version_rank": [ 1, 38, 3 ],
  "released_at": "2026-06-23T23:28:25.000Z",
  "reviewed_at": "2026-08-09T04:33:14.780Z",
  "summary": "Envoy v1.38.3 is a maintenance release with multiple disclosed security fixes and a security-related dependency update. It also disables a broken extension and changes the default for TLS certificate compression, so the release concerns deployments using those features.",
  "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
  "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
  "by_bucket": { "action": 1, "check": 17 },
  "by_family": { "security": 16, "breaking": 2 },
  "max_severity": "high",
  "notes_total": 0,
  "changes": [
    {
      "change_id": "envoy:v1.38.3:54ea382c",
      "matter_key": "envoy/advisory:cve-2026-47261",
      "project": "envoy", "version": "v1.38.3", "version_rank": [ 1, 38, 3 ],
      "released_at": "2026-06-23T23:28:25.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [ { "id": "CVE-2026-47261", "severity": "high" } ],
      "subjects": [
        { "kind": "dependency", "name": "com_github_wasmtime", "name_full": "com_github_wasmtime", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-47261", "name_full": "CVE-2026-47261", "role": "changed" }
      ],
      "window": null,
      "transition": null,
      "remedy": null,
      "symptom": [],
      "quote": "wasm: bumped ``com_github_wasmtime`` to resolve CVE-2026-47261.",
      "disclosure": "described",
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
      "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
      "seq": 13911
    },
    …17 件省略…
  ]
}
```

変更 18 件に対して `by_bucket` が 1 / 17 というのが、セキュリティ
ポイントリリースの典型的な形です。全員が取り込むべきものが 1 つ、どの拡張を
使っているかに依存するものが 17 つ。

### 監査可能な沈黙は、自ら根拠を携えてきます

運用者が対処すべき変更がないリリースでは、`get_release` は頼まれなくても
`raw_notes` を一緒に返します。沈黙を信じてくださいと言う代わりに、原文から
直接判断できるようにするためです。

```json
{
  "project": "cilium",
  "version": "v1.18.12",
  "summary": "Cilium v1.18.12 adds Gateway access-log configuration and BYOCNI loopback support. It also fixes policy, startup, Gateway validation, IPAM, and metric-label defects, while updating shipped images and dependencies; no security advisories or security-specific flaws are disclosed.",
  "by_bucket": {}, "by_family": {}, "max_severity": null,
  "notes_total": 20,
  "changes": [],
  "raw_notes": "# 1.18.12\n\nSummary of Changes\n------------------\n\n**Minor Changes:**\n* gateway-api: add support for configuring Gateway access logs …",
  "raw_notes_truncated": false
}
```

### 質問例

> 「Istio の最新リリースはどうですか？ 何が見つかりました？」

エージェントは `version` なしで `get_release(project: "istio")` を呼び、
`summary` とバケットの集計から答えます。

## changes_by_entity

逆引きインデックスです。識別子 1 つ — CVE id、CRD、フィーチャーゲート、
フラグ、メトリクス、設定フィールド、依存関係 — に触れるすべての変更を、
プロジェクトとリリースをまたいで集めてきます。大文字小文字は区別しません。
マニフェストやアドバイザリから識別子を 1 つ手にして、その周辺に何が
あったかを知りたいときに使ってください。プロジェクトから出発する
`get_release`・`list_releases` とは逆方向です。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `name` | はい | string | — | 探す正確な識別子：CVE id、CRD、フィーチャーゲート、フラグ、メトリクス、設定フィールド、拡張、サブシステム、依存関係 |
| `kind` | いいえ | string | *(なし = すべて)* | 識別子の種類を 1 つに限定：`api`、`crd`、`feature_gate`、`flag`、`metric`、`config_field`、`extension`、`dependency`、`cve`、`advisory`、`subsystem` |

インデックスは変更が持つ `subjects` から作られます。突き合わせは保存された
`name` への完全一致（大文字小文字は無視）であり、部分文字列検索ではありません。
返るのは最大 200 件です。

`{ "changes": [] }` は、その識別子を名前に持つ変更が記録にないという意味で
あって、その周辺に何も起きていないという意味ではありません。結論を出す前に
範囲を広げてください。まず `kind` を外します — 対象に保存された種類と完全に
一致する必要があり、1 つの識別子が必ずしも想像どおりの種類で分類されているとは
限らないからです。次に、その変更が索引されていそうな別の識別子で試します。
GHSA id だけで引用されたアドバイザリは CVE id では届きませんし、その逆も
同じです。

### 呼び出し例

```json
{ "name": "CVE-2026-41178" }
```

### 実測レスポンス

```json
{
  "changes": [
    {
      "change_id": "buildpacks:v0.40.9:83576398",
      "matter_key": "buildpacks/advisory:cve-2026-41178",
      "project": "buildpacks", "version": "v0.40.9", "version_rank": [ 0, 40, 9 ],
      "released_at": "2026-08-09T17:12:10.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [
        { "id": "CVE-2026-41178", "severity": "medium" },
        { "id": "GO-2026-5158", "severity": "medium" }
      ],
      "subjects": [
        { "kind": "dependency", "name": "go.opentelemetry.io/otel", "name_full": "go.opentelemetry.io/otel", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-41178", "name_full": "CVE-2026-41178", "role": "changed" },
        { "kind": "advisory", "name": "go-2026-5158", "name_full": "GO-2026-5158", "role": "changed" }
      ],
      "window": { "introduced_in": "v0.40.9" },
      "quote": "`go.opentelemetry.io/otel` | v1.43.0 → v1.44.0 | GO-2026-5158 / CVE-2026-41178 — baggage header not length-capped",
      "disclosure": "described",
      "source_url": "https://github.com/buildpacks/pack/releases/tag/v0.40.9",
      "release_url": "https://ratatosk.io/en/releases/buildpacks/v0.40.9",
      "seq": 18932
    }
  ]
}
```

この変更 1 件に索引された対象が 3 つ — 依存関係、CVE、Go のアドバイザリ —
あるので、同じ項目に `go.opentelemetry.io/otel` でも、`CVE-2026-41178` でも、
`GO-2026-5158` でも届きます。`window.introduced_in` は修正が v0.40.9 に
入ったと告げており、それが到達しているべきバージョンです。

### 質問例

> 「CVE-2026-41178 はどのリリースで直り、うちが使っているものに関係あり
> ますか？」

エージェントは `changes_by_entity(name: "CVE-2026-41178")` を呼び、
プロジェクトごとの `window.introduced_in` を読んで実行中のバージョンと
比べます。セキュリティの確認なら、CVE id とアドバイザリ id の両方で
問い合わせてください。ノートが片方しか引用していない変更は、その片方でしか
索引されません。

## get_matter

1 つの案件が現れたすべてのリリースを、古い順に返します。`matter_key` は
変更からそのままコピーして渡してください。大文字小文字を区別し、`/` と `:`
を含みます。「*自分の* ブランチではどのバージョンがこれを直すのか」
「もう対処済みか」に答えるツールです。

最新のものだけでなくすべての出現を返す理由はこうです。メンテナが 1 つの問題を
サポート中の 5 ブランチで直すと変更は 5 件生まれますが、ブランチごとに載る
アドバイザリの組が同じとは限りません。最新のものだけを聞いて判断すると、
古いブランチにいる人は自分のカバレッジを取り違えます。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `matter_key` | はい | string | — | 変更からそのままコピーした `matter_key` |
| `include_all` | いいえ | boolean | `false` | 日常的な記録（大半はボットの依存関係更新）も含める |

### 呼び出し例

```json
{ "matter_key": "containerd/advisory:cve-2026-47262" }
```

### 実測レスポンス（一部省略）

```json
{
  "matter_key": "containerd/advisory:cve-2026-47262",
  "project": "containerd",
  "family": "security",
  "occurrences": [
    {
      "change_id": "containerd:v2.2.5:a74c2b48",
      "version": "v2.2.5", "version_rank": [ 2, 2, 5 ],
      "released_at": "2026-06-18T23:11:33.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "defect_corrected",
      "advisories": [
        { "id": "CVE-2026-47262", "severity": "medium" },
        { "id": "CVE-2026-50195", "severity": "medium" },
        { "id": "CVE-2026-53488", "severity": "critical" },
        { "id": "CVE-2026-53489", "severity": "high" },
        { "id": "CVE-2026-53492", "severity": "high" },
        { "id": "GHSA-33vj-92qq-66hc", "severity": "high" },
        …ほか 4 件…
      ],
      "source_url": "https://github.com/containerd/containerd/releases/tag/v2.2.5", …
    },
    { "version": "v2.3.2",  "advisories": [ …id 10 件… ], … },
    { "version": "v2.0.10", "advisories": [ "CVE-2026-47262", "CVE-2026-53488", "GHSA-jpcc-p29g-p8mq", "GHSA-xhf5-7wjv-pqxp" ], … },
    { "version": "v1.7.33", "advisories": [ …id 4 件… ], … },
    { "version": "v2.1.9",  "advisories": [ "CVE-2026-47262", "GHSA-jpcc-p29g-p8mq" ], … }
  ],
  "includes_notes": false
}
```

このツールが存在する理由がまさにこのケースです。containerd のロールアップ
1 つが 5 つのブランチに降り、ブランチごとに載るアドバイザリ id が
**10 件、10 件、4 件、4 件、2 件** です。v2.1.x にいる人が v2.2.5 の項目だけを
読むと、自分のブランチが受け取ってもいない修正 8 件を受け取ったと思い込みます。
`occurrences` は古い順なので、`version` がいま動かしているものの直上にある
項目が、上げるべき先です。

`includes_notes` は日常的な記録を含めたかどうかを返すので、末尾が空なのか
外したのかを区別できます。

### 質問例

> 「うちは containerd 2.1.8 ですが、あの containerd の CVE ロールアップは
> うちでは直っていますか？ どのリリースで？」

エージェントは、その案件を浮かび上がらせたツール — `check_stack` でも
`changes_by_entity` でも — から `matter_key` を取って `get_matter` を呼び、
最新の項目ではなく 2.1 ブランチの項目から答えます。

## list_changes

増分同期フィードです。`seq` の昇順 — 分析の古いものから — で流れるため、
最初のページは最新のデータでは **ありません**。ローカルの写しを最新に保つ
ためにあるツールです。「X の最新リリースは？」や「X の最近のリリース」には
`list_releases` か `get_release` を使ってください。

日常的な記録は除かれます。このフィードには運用者が対処すべき変更だけが
流れます。

### パラメータ

| パラメータ | 必須 | 型 | 既定値 | 説明 |
|---|---|---|---|---|
| `project` | いいえ | string | *(なし = すべて)* | プロジェクトスラグでの絞り込み（例：`envoy`） |
| `family` | いいえ | string | *(なし = すべて)* | `security`、`breaking`、`deprecated` |
| `bucket` | いいえ | string | *(なし = すべて)* | `action`、`check`、`plan` |
| `since` | いいえ | integer | *(なし = 最初から)* | カーソル。`seq` がこの値より大きい変更のみ。直前のレスポンスの `next_since` を渡します |
| `limit` | いいえ | integer | `50` | ページサイズ。最大 `200` |

### 呼び出し例

```json
{ "family": "security", "bucket": "action", "limit": 3 }
```

### 実測レスポンス（一部省略）

```json
{
  "changes": [
    {
      "change_id": "argo:v3.5.0:c140f331",
      "matter_key": "argo/dependency/formidable#value_changed",
      "project": "argo", "version": "v3.5.0", "version_rank": [ 3, 5, 0 ],
      "released_at": "2026-08-04T08:35:57.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [],
      "subjects": [ { "kind": "dependency", "name": "formidable", "name_full": "formidable", "role": "changed" } ],
      "window": null, "transition": null, "remedy": null, "symptom": [],
      "quote": "chore(deps): update dependency formidable to v2.1.3 [security]",
      "disclosure": "undisclosed",
      "source_url": "https://github.com/argoproj/argo-cd/releases/tag/v3.5.0",
      "release_url": "https://ratatosk.io/en/releases/argo/v3.5.0",
      "seq": 1415
    },
    { "change_id": "backstage:v1.50.0:22fe2851", "project": "backstage", "version": "v1.50.0",
      "released_at": "2026-04-14T17:49:58.000Z", "family": "security", "bucket": "action",
      "disclosure": "described", "seq": 1464, … },
    { "change_id": "backstage:v1.50.0:fc3cb011", "project": "backstage", "version": "v1.50.0", "seq": 1465, … }
  ],
  "next_since": 1465
}
```

次のページは
`{ "family": "security", "bucket": "action", "limit": 3, "since": 1465 }`
です。`next_since` が `null` で返るまで繰り返してください。`null` は追いつい
たという意味です。`since: null` は送らないでください — `400` になります。
ページが短く返ってきたら、そこで巡回は終わりです。`next_since` はページが
いっぱいだったときにだけ値を持ちます。

`seq` は分析のカーソルであって時間軸ではない点に注意してください。上の最初の
ページには 8 月の argo リリースと 4 月の backstage リリースが同居していますが、
分析された順序がそうだったからです。時系列が必要なら `released_at` で並べ替えて
ください。

argo の項目の `disclosure: "undisclosed"` は、ノートがその更新をセキュリティ
関連と示しただけで、アドバイザリを名指ししてはいないという意味です。
`security` / `action` の変更なのに `advisories` が空なのはこのためです。

### 質問例

> 「ratatosk のデータのローカルコピーを最新に保って。」

エージェントは最後に見た `next_since` を保存しておいてそこから再開し、
`next_since` が `null` になるまでページを歩きます。

## プライバシーとレート制限

要約すると、`check_stack` のバージョン比較はサーバープロセスのなかで
行われるので、自分でホストすれば実行中のバージョンはあなたのインフラから
出ません。境界とログの扱いの全体像は
[README の「実行中のコンポーネントバージョンの取り扱い」](../README.ja.md#実行中のコンポーネントバージョンの取り扱い)を
見てください。

制限は 2 つあり、単位が異なります。公開 API は **IP あたり毎分 1200
リクエスト**を許可します — セルフホストしたサーバーが使うのはこちらです。
ホスト版エンドポイントはこれに加えて**呼び出し元ごとに毎分 60 回のツール
呼び出し**を許可し、全体で分け合う枠ではなく呼び出し元ごとに数えます。

ツール呼び出し 1 回はリクエスト 1 回ではありません。ここにあるツールは
`check_stack` を除いてすべて上流リクエスト 1 回で済み、`check_stack` は
コンポーネント 1 つにつき 1 回（変更がないと分かったコンポーネントごとに
さらに 1 回）使います。コンポーネント 15 個の `check_stack` はツール呼び出し
1 回でリクエスト約 15 回ですが、`list_changes` を 15 回別々に呼ぶよりはるかに
安く済みます。プロジェクトごとにポーリングする代わりにスタックの質問を 1 回に
まとめる理由がこれです。多用する場合や自動化する場合は自前ホストへ移って
ください。

このデータは公式リリースノートから AI が抽出したものです。重要な判断は、
すべての変更が持つ `source_url` に照らして確認してください
（[利用規約](https://ratatosk.io/terms)）。
