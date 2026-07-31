# ツールリファレンス

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

このページは、ratatosk-mcp が提供する 6 つの
ツール（[`check_stack`](#check_stack)、[`list_projects`](#list_projects)、
[`list_releases`](#list_releases)、[`get_release`](#get_release)、
[`facts_by_entity`](#facts_by_entity)、[`list_facts`](#list_facts)）の
詳細リファレンスです。ツールごとに、用途、パラメータ一覧、実際の呼び出しと
レスポンス、そしてエージェントに尋ねる質問の例をまとめました。サーバーへの
接続方法（ホスト版エンドポイント、Docker、Helm、kagent）は
[インストールと使い方](install.ja.md)を見てください。

先に用語を 2 つだけ定義します。**fact** とは、公式リリースノートから
抽出した変更 1 件（セキュリティ修正、削除、非推奨化、デフォルト値の変更）
のことです。各 fact には、その変更が対象とする正確な識別子（CVE id、
フラグ、CRD、設定フィールド）と、根拠となるノート原文の引用が付きます。
すべての fact には `info` から `critical` までの**深刻度（severity）**が
付きます。

> **2026-07-31 時点の実測**：この文書のすべての呼び出しとレスポンスは、
> 2026-07-31 にホスト版エンドポイント `https://ratatosk.io/mcp`
> （サーバー 0.6.1）へ実際に送って受け取ったものです。`check_stack` の例に
> 使ったスタック（コンポーネント 5 種と `version_source` の文言）は、
> 匿名化した実クラスタのものです。リリースデータは毎時増え続けるため、
> いま呼び出すと数値やリストは変わり得ますが、レスポンスの形は同じです。
> 長いレスポンスは表示の都合で一部を `…` に省略しています。

## 質問からツール呼び出しまで — 実例で追う

エージェントにこう聞いたとします：

> 「うちのクラスタの状態を確認してください。」

実際に使ってみて効果が高かった言い回しを、日本語に訳して載せています。
クラスタを直接読めるエージェント（たとえば kagent 上で ratatosk-mcp と
クラスタ読み取りツールを併用した場合）の実際の実行では、ツールはこの
順序で呼ばれました：

```
list_projects
k8s_get_resources  (pod, all_namespaces)
k8s_get_resources  (daemonset, all_namespaces)
k8s_get_resources  (deployment, all_namespaces)
k8s_get_resources  (node)
check_stack        (components ×5)
get_release        (cilium, v1.20.0)               ← action_required の掘り下げ
k8s_get_resources  (customresourcedefinition)
k8s_get_resources  (ciliumnodeconfig, all_namespaces)
k8s_get_resources  (configmap, all_namespaces)
k8s_get_resource_yaml (kube-system/cilium-config)  ← applies_if の切り分け
```

表記の注意：`k8s_*` は kagent-tools（別の MCP）のクラスタ読み取りツール
で、この中の ratatosk ツールは `list_projects`・`check_stack`・
`get_release` です。

流れを整理すると 3 段階です。

**段階 1 — `list_projects` で一覧を確定。** クラスタで見た名前を、ほかの
ツールが受け取る正規スラッグに変換し、`cluster_core:true`（クラスタの
基盤レイヤー。コントロールプレーン、ランタイム、DNS、CNI など）の印で
点検に含める候補を選びます。

**段階 2 — バージョンを実際のリソースから読み取る。** `k8s_*` ツールで
Pod・DaemonSet・Node の status から、稼働中のバージョンとその出どころを
把握します。そうして出来上がったのが、下の check_stack 節の呼び出し例
です。コンポーネント 5 種（kubernetes/containerd/cilium/envoy/coredns）と
その `version_source` の文言です。

**段階 3 — `check_stack` のブリーフィング、そして条件の判定。**
ブリーフィングで cilium の `action_required` 2 件（v1.20.0 の docker
libnetwork プラグイン削除、proxylib/Kafka ポリシー削除）を確認し、
`get_release(cilium, v1.20.0)` で根拠となる原文まで掘り下げます。
`check_config` の「v2alpha1 CiliumNodeConfig CRD を使っている場合」という
条件については、CRD 一覧と `kube-system/cilium-config` を実際に読み、
適用の有無を判定します。「ブリーフィングを受け取り、条件を実際のリソースで
判定する」という `check_stack` の設計意図が、そのまま実行の流れになった
例です。

何が返ってきたかは、下の check_stack 節の実測レスポンスと、次の節
「ブリーフィングの読み方」でそのまま見られます。

条件の判定をさらに確実に引き出したいなら、やはり実際に効果が確認された
次の文言のように、判定ルールを質問に含めてください：

> 「クラスタで実際に稼働しているコンポーネントのバージョンを、コントロール
> プレーン、ノードランタイム、ネットワーキング（CNI とその周辺）、DNS まで
> 含めて実際のリソースから読み、ratatosk のリリース情報と突き合わせて、
> いま私たちに適用される問題があるか点検してください。条件付きの問題は該当する
> 設定を実際に読んで確認できた場合のみ適用/非適用を判定し、確認できなかった
> 条件は『確認不可』に分類してください。」

実際に観測されたアンチパターンも 1 つ挙げておきます。網羅的な列挙を求める
質問は、必ず判定ルール（「確認できなかったものは確認不可へ」）と
組み合わせてください。網羅を求めるだけでは、モデルは実物を読まずに、
もっともらしい値で空欄を埋めてしまいます。

## check_stack のブリーフィングの読み方

`check_stack` のデフォルトレスポンス（`detail:"brief"`）は、どの
コンポーネントでも同じ構造で返ります。下の check_stack 節の
実測レスポンスを手元に置いて読んでください。

1. **`summary` — 集計。** 稼働バージョンより新しい fact 数
   （`new_facts`）、マージ後の重複を除いた項目数（`distinct_issues`）、
   条件なしで該当する fact 数（`mandatory`）、そして深刻度別・タイプ別の
   内訳。スキャンした全 fact 数は `facts_scanned` です。実測レスポンスの
   envoy が良い例です：新しい fact 75 件がマージを経て 40 件の項目に
   減っています。なお `distinct_issues` は引用文のマージ（下の 5 番）より
   前の基準で数えるため、3 つのリストの項目数の合計より大きくなることが
   あります。
2. **`action_required` — このバージョン範囲を通るすべてのインストールに
   該当する critical/high。** 各項目には
   リリースノート原文の引用（`quote`）と CVE・アドバイザリ id（`ids`）が
   付きます。実測レスポンスでは envoy に 5 件（fact 211 の TLS SAN 認証
   バイパス critical を含む）、cilium に 2 件（libnetwork・proxylib の
   削除）がここに入りました。
3. **`check_config` — 設定の確認が必要な critical/high。** `applies_if` の
   条件が成り立つときだけ該当する項目です。条件を実際の設定で確認する
   までは対応事項ではなく、条件が成り立たなければアップグレードの理由でも
   ありません — その場合の `fixed_in` は「後でその機能を有効にするなら、
   最低でもこのバージョン以上が必要」という前提条件として読みます。
   サーバーが条件の対象を構造化して保持している場合は
   `applies_if_target`（kind と name）が探すべきものを指し示します。
   実測レスポンスの cilium fact 614 が
   その形の例です — `applies_if`「uses the cilium.io/v2alpha1 CiliumNodeConfig
   CRD」に `applies_if_target` `{ "kind": "crd", … }` が付いていて、上の
   シナリオでエージェントが CRD 一覧を実際に読みに行った理由になって
   います。envoy 側には fact 215（critical、「if you proxy HTTP/3 to
   HTTP/1 backends」）など 11 件がここにあります。
4. **`other_facts` — 残りのすべてを 1 行ずつ。** medium 以下の項目が
   fact ごとに 1 行で表示されます。実測レスポンスの coredns は 5 件すべて
   medium 以下なので、この一覧にだけ現れます。
5. **マージのルール。** 同じ引用文を共有する fact は 1 項目に
   マージされ、id がまとめて並びます（条件が異なる場合は
   `applies_if_any`）。実測レスポンスの envoy fact 466 がその形で、
   CVE id 5 件が 1 項目にまとまり、異なる条件 3 つが `applies_if_any` に
   並んでいます。同じアドバイザリが複数のリリースブランチで修正
   された場合も 1 項目です — 実測レスポンスの envoy fact 211 に実際に
   付いている `same_issue_also_addressed_in: ["v1.37.5", "v1.38.3"]` が
   それです。このとき表示される深刻度はそのアドバイザリグループの最大
   深刻度です。
6. **`note` — コンポーネントごとの状態表示。** 実測レスポンスには両方の
   種類が入っています。kubernetes・containerd の「tracked by ratatosk; no
   facts on record — releases so far were routine」は、追跡中だが静かな
   状態で、追跡していない状態（`tracked:false` — fact がないことは、
   安全を意味しません）と区別されます。cilium・coredns の「older than
   every release on record … treat it as partial, and re-check」は、稼働
   バージョンが記録上の最も古いリリースよりも古いという警告です —
   サーバーはユーザーの環境を見られないため、申告されたバージョンを
   裏取りする唯一の手段がこの表示であり、バージョンを実際のリソースから
   読み直すべきというシグナルです。
7. **`hint` と `privacy`。** `hint` には、このブリーフィングが推奨ではなく
   データの分類であることと、次に使うツール（全文は `detail:"full"`、
   掘り下げは `get_release`・`facts_by_entity`）が示されます。`privacy` は
   この呼び出しでバージョンがどこまで送信されたかを明示します。

## check_stack

稼働中のコンポーネントバージョンの一覧を受け取り、コンポーネントごとに
アップグレード経路（稼働バージョンより新しいリリース群）にある fact を
返します。「アップグレード前にやることはある？」という質問の最初の
ツールで、複数プロジェクトを 1 回の呼び出しにまとめられるため、
プロジェクトごとにツールを個別に呼び出すよりスタック単位の質問に
向いています。
バージョン比較はサーバープロセスの中で行われるので、セルフホストすれば
稼働バージョンはインフラを離れません。特定のリリース 1 件を深く見るなら
`get_release`、特定の CVE・フラグ 1 つを追うなら `facts_by_entity` に
進んでください。

### パラメータ

| パラメータ | 必須 | 型 | デフォルト | 説明 |
|---|---|---|---|---|
| `components` | はい | array | — | 点検する稼働中のスタック。項目の形式は下記 |
| `components[].project` | はい | string | — | プロジェクトスラッグ（例：`envoy`）。正式な値は `list_projects` の `slug` |
| `components[].version` | はい | string | — | いま稼働中のバージョン（例：`v1.36.8`） |
| `components[].target_version` | いいえ | string | *（なし＝最新まで全部）* | アップグレード先のバージョン。稼働バージョンより新しい値である必要があり、`稼働バージョン < バージョン ≤ target` の fact だけが返ります。稼働バージョン以下の値は範囲が空になるため無視され、`note` で案内されます |
| `components[].version_source` | いいえ | string | *（なし）* | バージョンをどこで読んだか（例：`daemonset/cilium image tag`、ユーザーの申告）。レスポンスにそのまま返されるため、あとから出どころを検証できます。サーバーはユーザーの環境を見られないためバージョンを検証できません — 「ブリーフィングの読み方」で説明した `note` の表示が唯一のクロスチェックです |
| `detail` | いいえ | string | `brief` | `brief`：要約 + critical/high + 残りは 1 行ずつ。`full`：すべての fact を省略せずに返す — コンポーネントあたり 50 件の上限があり、超過は `relevant_facts_omitted` で示されるので、`severity_min` や `target_version` で絞ってください |
| `severity_min` | いいえ | string | *（なし＝全部）* | この深刻度以上のみ：`info`・`low`・`medium`・`high`・`critical` |

### 呼び出し例

上のシナリオのスタックを、ホスト版エンドポイントへそのまま送った
呼び出しです。下の JSON はツールの引数（`arguments`）だけを載せています。
これを包む JSON-RPC の送信形式は
[インストールと使い方](install.ja.md)にあります：

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
      "tracked": true,
      "note": "tracked by ratatosk; no facts on record — releases so far were routine",
      "facts_scanned": 0,
      "summary": { "new_facts": 0, "distinct_issues": 0, "mandatory": 0, "by_severity": {}, "by_type": {} }
    },
    {
      "project": "containerd",
      "running_version": "2.2.4",
      "version_source": "node status containerRuntimeVersion",
      "tracked": true,
      "note": "tracked by ratatosk; no facts on record — releases so far were routine",
      "facts_scanned": 0,
      "summary": { … }
    },
    {
      "project": "cilium",
      "running_version": "v1.19.4",
      "version_source": "daemonset/cilium image tag",
      "note": "running version v1.19.4 is older than every release on record (earliest with facts: v1.19.6) — this covers the reviewed window only, so treat it as partial, and re-check that the running version was read off a live resource",
      "facts_scanned": 24,
      "summary": {
        "new_facts": 24, "distinct_issues": 24, "mandatory": 19,
        "by_severity": { "high": 4, "medium": 12, "low": 8 },
        "by_type": { "capability_removed": 12, "capability_deprecated": 5, "behavior_changed": 3, … }
      },
      "action_required": [
        {
          "fact_id": 612, "version": "v1.20.0", "fact_type": "capability_removed",
          "severity": "high", "mandatory": true, "removed_in": "v1.20.0",
          "quote": "As previously announced, docker libnetwork plugin as been sunset and is no longer available."
        },
        …1 件省略…
      ],
      "check_config": [
        …1 件省略…
        {
          "fact_id": 614, "version": "v1.20.0", "fact_type": "api_version_changed",
          "severity": "high", "mandatory": true,
          "applies_if": "uses the cilium.io/v2alpha1 CiliumNodeConfig CRD",
          "applies_if_target": { "kind": "crd", "name": "cilium.io/v2alpha1 CiliumNodeConfig" },
          "removed_in": "v1.20.0", "deprecated_in": "v1.16",
          "quote": "Remove deprecated `v2alpha1` `CiliumNodeConfig` API that was promoted to `v2` in cilium 1.16."
        }
      ],
      "other_facts": [ …20 件省略… ]
    },
    {
      "project": "envoy",
      "running_version": "v1.36.7",
      "version_source": "daemonset/cilium-envoy image tag",
      "facts_scanned": 90,
      "summary": {
        "new_facts": 75, "distinct_issues": 40, "mandatory": 38,
        "by_severity": { "critical": 3, "high": 13, "medium": 22, "low": 2 },
        "by_type": { "security_fix": 30, "behavior_changed": 3, "default_changed": 3, … }
      },
      "action_required": [
        …1 件省略…
        {
          "fact_id": 211, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true, "fixed_in": "v1.36.9",
          "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass",
          "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2 件省略…
        {
          "fact_id": 347, "version": "v1.37.5", "fact_type": "security_fix",
          "severity": "high", "mandatory": true, "fixed_in": "v1.37.5",
          "quote": "CVE-2026-47220: REQUESTED_SERVER_NAME crash",
          "ids": [ "CVE-2026-47220", "GHSA-j9wh-4qfm-wf2v" ],
          "same_issue_also_addressed_in": [ "v1.38.3" ]
        }
      ],
      "check_config": [
        …8 件省略…
        {
          "fact_id": 215, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true,
          "applies_if": "if you proxy HTTP/3 to HTTP/1 backends",
          "fixed_in": "v1.36.9",
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "ids": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2 件省略…
      ],
      "other_facts": [
        …7 件省略…
        {
          "fact_id": 466, "version": "v1.39.0", "fact_type": "security_fix",
          "severity": "medium", "mandatory": true,
          "applies_if_any": [
            "uses the ext_authz extension",
            "uses the ext_proc extension",
            "uses the OAuth2 extension"
          ],
          "applies_if_target": { "kind": "extension", "name": "ext_authz" },
          "fixed_in": "v1.39.0",
          "quote": "Security fixes were added for ext_authz (**CVE-2026-47205**), ext_proc (**CVE-2026-47207**), gRPC stats (**CVE-2026-47204**), internal redirects (**CVE-2026-47221**), and OAuth2 lifecycle handling (**CVE-2026-48090**).",
          "ids": [ "CVE-2026-47205", "CVE-2026-47207", "CVE-2026-47204", "CVE-2026-47221", "CVE-2026-48090" ]
        },
        …6 件省略…
      ]
    },
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "note": "running version v1.14.1 is older than every release on record (earliest with facts: v1.14.5) — …",
      "facts_scanned": 5,
      "summary": {
        "new_facts": 5, "distinct_issues": 5, "mandatory": 2,
        "by_severity": { "medium": 2, "low": 3 }, "by_type": { "behavior_changed": 3, "default_changed": 2 }
      },
      "other_facts": [
        { "fact_id": 432, "version": "v1.14.5", "fact_type": "default_changed", "severity": "medium",
          "mandatory": true, "fixed_in": "v1.14.5", "quote": "core: Use Go TLS defaults" },
        …4 件省略…
      ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation …): action_required = critical/high that applies to every install of this version. check_config = critical/high that applies ONLY IF applies_if holds — …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

一つのレスポンスの中に、異なる状態が 3 つ共存していることに注目して
ください。

- **kubernetes・containerd** — fact 0 件ですが、`note` には「追跡中で、
  これまでのリリースには特記事項がなかった」と示されています。追跡して
  いない状態（`tracked:false`）とは別の状態です。
- **cilium・coredns** — 稼働バージョンが記録上の最も古いリリースよりも
  古く、`note` がレビュー範囲が部分的だと警告しています — バージョンが
  実際のリソースから読んだ値かどうか、確認し直すべきというシグナルです。
- **envoy** — 新しい fact 75 件が 40 件の項目にマージされました。
  同じアドバイザリが v1.36.9・v1.37.5・v1.38.3 の 3 ブランチで修正されて
  いますが、`same_issue_also_addressed_in` でマージされ、1 項目ずつだけ
  残ったためです。

### 質問例

> 「うちは envoy v1.36.8 なんだけど、v1.37.0 に上げる前に対応することは
> ある？」

> 「うちのスタックは Kubernetes 1.31、Cilium 1.16、CoreDNS 1.11 なんだけど、
> アップグレード前に見ておくべきことはある？」

どちらの質問もエージェントは `check_stack` 1 回で答えます。前者は
`target_version` を指定した形に、後者はコンポーネント 3 つを 1 回の
呼び出しにまとめた形になります。クラスタを直接読めるエージェントなら、
上のシナリオの 2 つの文言のように、バージョンの調査ごと任せることも
できます。

## list_projects

追跡中のすべてのプロジェクトの一覧です。引数がなくレスポンスも小さいので、
プロジェクト名からスラッグを推測する代わりに、まずこのツールを呼ぶのが
定石です — 間違ったスラッグはエラーにならず `check_stack` の
`tracked:false` として現れるため、推測は静かに「カバレッジなし」に
つながります。ほかの 5 つのツールが受け取る `project` 引数の正式な値が、
このレスポンスの `slug` です。

### パラメータ

ありません。空の引数で呼び出します。

### 呼び出し例

```json
{}
```

### 実測レスポンス（一部省略）

```json
{
  "projects": [
    { "slug": "argo", "name": "Argo", "tier": "graduated", "category": "cicd", "analyzed_releases": 25 },
    …
    { "slug": "coredns", "name": "CoreDNS", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 5, "cluster_core": true },
    { "slug": "envoy", "name": "Envoy", "tier": "graduated", "category": "networking",
      "analyzed_releases": 22, "image_aliases": [ "cilium-envoy" ], "cluster_core": true },
    { "slug": "etcd", "name": "etcd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 18, "cluster_core": true,
      "visibility": "may live outside the k8s API, be hidden by a managed control plane, or be replaced entirely — a missing pod is not a missing component; when unreadable, report it under Could not check instead of guessing a version" },
    { "slug": "kubernetes", "name": "Kubernetes", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 22, "cluster_core": true,
      "image_aliases": [ "kube-apiserver", "kube-controller-manager", "kube-scheduler", "kubelet", "kube-proxy" ] },
    …
  ],
  "count": 76
}
```

基本フィールド（`slug`・`name`・`tier`・`category`・`analyzed_releases`）の
ほかに、3 つの印が付く項目があります：

- `image_aliases` — そのプロジェクトがクラスタで別名で動いている場合の
  名前。この別名に一致するイメージやワークロードは、そのプロジェクトが
  タグの示すバージョンで動いているものとして扱えます。
- `cluster_core:true` — クラスタの基盤レイヤー（コントロールプレーン、
  データストア、DNS、ランタイム、CNI/データプレーン）。クラスタに存在する
  `cluster_core` プロジェクトはすべて `check_stack` 呼び出しに含める
  候補です。
- `visibility` — そのコンポーネントをどう観測するか、そして読み取れなくても
  異常ではない箇所を示すヒント（たとえば etcd は k8s API の外に
  あることがあります）。読めなかったコンポーネントは、バージョンを推測する
  代わりに「確認できなかった」と報告してください、という案内です。

### 質問例

> 「うちのスタックは Kubernetes 1.31、Cilium 1.16、CoreDNS 1.11 なんだけど、
> アップグレード前に見ておくべきことはある？」

`check_stack` と同じ質問です — 上のシナリオで見たとおり、エージェントは
スラッグを確定するためにまずこのツールを呼び、それから `check_stack` に
進みます。

## list_releases

1 プロジェクトの最新リリース N 件を、1 行サマリー（バージョン、日付、
カバレッジ、深刻度別の fact 数、そしてアドバイザリグループの最高深刻度
`max_group_severity` — グループに属する fact がなければ `null`）で
新しい順に返します。「X で最近何があった？」という質問専用のツールです。
同じ fact データを扱う `list_facts` とは並び順が逆です。`list_facts` は
ローカルコピーの同期のために分析の古い順に流れるフィードで、最新の話題を
聞くときはこのツールを使ってください。サマリーで目に留まったリリースが
あれば `get_release` で掘り下げます。

### パラメータ

| パラメータ | 必須 | 型 | デフォルト | 説明 |
|---|---|---|---|---|
| `project` | はい | string | — | プロジェクトスラッグ（例：`istio`） |
| `limit` | いいえ | integer | `5` | 最近のリリースを何件見るか。最大 `20` |

### 呼び出し例

```json
{ "project": "cilium", "limit": 3 }
```

### 実測レスポンス（一部省略）

```json
{
  "project": "cilium",
  "count": 3,
  "releases": [
    {
      "version": "v1.20.0", "version_rank": [ 1, 20, 0 ],
      "released_at": "2026-07-29T15:00:29.000Z", "reviewed_at": "2026-07-29T15:49:54.048Z",
      "coverage": "full_reviewed",
      "facts_total": 22, "facts_mandatory": 17,
      "facts_by_severity": { "high": 3, "medium": 11, "low": 8 },
      "max_group_severity": null,
      "api_url": "https://ratatosk.io/v1/releases/cilium/v1.20.0",
      "release_url": "https://ratatosk.io/releases/cilium/v1.20.0"
    },
    {
      "version": "v1.19.6", …, "facts_total": 2, "facts_mandatory": 2,
      "facts_by_severity": { "high": 1, "medium": 1 }, …
    },
    {
      "version": "v1.18.12", …, "coverage": "full_reviewed",
      "facts_total": 0, "facts_mandatory": 0, "facts_by_severity": {}, …
    }
  ],
  "hint": "summaries only — fetch /v1/releases/{project}/{version} for a release's full facts"
}
```

`v1.18.12` のように `facts_total` が 0 で `coverage` が `full_reviewed` の
行は、「ノートを最後まで読み、特記事項のないリリースだった」という
意味です — データがないことと区別できる、確認済みの「異常なし」です。

### 質問例

> 「Cilium で最近何があった？」

エージェントが `list_releases(project: "cilium")` を呼び、上のサマリーを
根拠に、fact が v1.20.0 に集中していると答えます。

## get_release

レビュー済みリリース 1 件の全体です。概要部分（カバレッジ、総合評価、
原文ノートへのリンク）と、そのリリースのすべての fact が返ります。
`check_stack` や `list_releases` で目に留まったリリース 1 件を根拠まで
掘り下げるとき、そして fact の原文（`source_url`）にあたって判断を
検証するときに使うツールです。`version` を省略するとそのプロジェクトの
最新レビュー済みリリースが返るので、「X の最新リリースはどう？」という
質問にもこのツールを使います。プライバシーについて 1 点：`check_stack` と
違い、このツールはバージョンを引数に取り、そのバージョンは
アップストリームへのリクエストパスに載ります —
[README の「実行中のコンポーネントバージョンの取り扱い」](../README.ja.md#実行中のコンポーネントバージョンの取り扱い)を
見てください。

### パラメータ

| パラメータ | 必須 | 型 | デフォルト | 説明 |
|---|---|---|---|---|
| `project` | はい | string | — | プロジェクトスラッグ（例：`envoy`） |
| `version` | いいえ | string | *（なし＝最新レビュー済みリリース）* | 公開されたとおりのリリースタグ（例：`v1.38.3`）。先頭の `v` はあってもなくても受け付けます — プロジェクトごとに表記が違うためです。存在しないタグを渡すと、そのプロジェクトの最近のレビュー済みタグ一覧を含むエラーが返るので、その中のどれかで呼び直してください |
| `include_raw` | いいえ | boolean | `false` | 原文リリースノート本文を `raw_notes` として一緒に受け取ります — 抽出された fact ではなく原文で判断したいときに使います。レビューがリリース全体をカバーできていない場合（カバレッジ `insufficient`、または fact 0 件）には自動的に含まれます |

### 呼び出し例

バージョンを省略して最新レビュー済みリリースを受け取る呼び出しです：

```json
{ "project": "istio" }
```

### 実測レスポンス（一部省略）

```json
{
  "project": "istio",
  "version": "1.29.6",
  "released_at": "2026-07-16T16:51:06.000Z",
  "reviewed_at": "2026-07-16T17:27:06.386Z",
  "source_url": "https://github.com/istio/istio/releases/tag/1.29.6",
  "coverage": "full_reviewed",
  "assessment": "Routine patch release focused on bug fixes for ambient mesh components (pilot-agent drain logic, WorkloadEntry HBONE capability propagation, ztunnel CNI deadlock, Istiod memory leak, and east-west gateway RBAC filtering); no new breaking changes, API changes, or CVEs, though one fix has an operator-visible transitional caveat for auto-registered WorkloadEntry resources.",
  "release_url": "https://ratatosk.io/releases/istio/1.29.6",
  "facts": [
    {
      "fact_id": 490,
      "fact_type": "behavior_changed",
      "severity": "medium",
      "mandatory": true,
      "confidence": 0.85,
      "applies_if": {
        "status": "degraded",
        "fallback": "workloads auto-registered before upgrading continue to be reached over plaintext until they either re-register or the networking.istio.io/tunnel=http label is added to their existing WorkloadEntry"
      },
      "affected": { "fixed_in": "1.29.6", "removed_in": null, "deprecated_in": null },
      "entities": [ { "kind": "config_field", "name": "networking.istio.io/tunnel", … } ],
      "references": {
        "ids": [],
        "quote": "workloads auto-registered before upgrading continue to be reached over plaintext until …"
      },
      …
    }
  ]
}
```

まず概要部分を読みます。`coverage: "full_reviewed"` はノートを全部読んだ
という意味で、`assessment` はリリース全体への 1 段落の評価です。
カバレッジが `full_reviewed` なのに `facts` が空配列なら、読んだうえで
特記事項がなかったという意味です。重大な決定を下す前に、`source_url` の
原文で検証してください。

### 質問例

> 「Istio の最新リリース、レビュー結果はどうだった？」

エージェントが `get_release(project: "istio")` を呼び、上の `assessment` を
根拠に要約して答えます。

## facts_by_entity

逆引きインデックスです。正確な識別子 1 つ（CVE id、CRD、フィーチャー
ゲート、フラグ、メトリック、設定フィールド、依存関係）を扱ったすべての
fact を、プロジェクトとリリースを横断して集めます。大文字小文字は
区別しません。マニフェストやセキュリティアドバイザリで得た識別子を 1 つ
手がかりに「この周りで何があった？」と聞くときのツールです。
プロジェクトから出発する `get_release`・`list_releases` とは向きが逆です。

### パラメータ

| パラメータ | 必須 | 型 | デフォルト | 説明 |
|---|---|---|---|---|
| `name` | はい | string | — | 調べる正確な識別子：CVE id、CRD、フィーチャーゲート、フラグ、メトリック、設定フィールド、依存関係 |
| `kind` | いいえ | string | *（なし＝全種類）* | 識別子の種類で限定：`api`・`crd`・`feature_gate`・`flag`・`metric`・`config_field`・`extension`・`dependency`・`cve`・`advisory`・`subsystem` |

### 呼び出し例

```json
{ "name": "CVE-2026-47778" }
```

### 実測レスポンス（一部省略）

8 件返ってきました — envoy のリリースブランチ 5 つと、同じ CVE を自らの
リリースノートで扱った istio の 3 件です：

```json
{
  "facts": [
    {
      "fact_id": 211, "project": "envoy", "version": "v1.36.9",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-f8x4-rw5x-f3r7", "group_severity": "critical",
      "affected": { "fixed_in": "v1.36.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
        "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass"
      },
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.36.9", …
    },
    {
      "fact_id": 239, "project": "istio", "version": "1.28.9",
      "fact_type": "security_fix", "severity": "medium", "mandatory": true,
      "advisory_group_key": "adv:istio-security-2026-005", "group_severity": "high",
      "affected": { "fixed_in": "1.28.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "ISTIO-SECURITY-2026-005" ],
        "quote": "Envoy could fail to validate the Subject Alternative Name (SAN) of a peer certificate if the SAN contained an embedded NUL byte"
      },
      "source_url": "https://github.com/istio/istio/releases/tag/1.28.9", …
    },
    { "fact_id": 268, "project": "envoy", "version": "v1.35.13", "severity": "critical", … },
    { "fact_id": 285, "project": "envoy", "version": "v1.38.3", "severity": "critical", … },
    { "fact_id": 328, "project": "istio", "version": "1.30.2", "severity": "low",
      "advisory_group_key": "adv:istio-security-2026-005", "group_severity": "high", … },
    …3 件省略…
  ]
}
```

読み方を 1 つ：fact には深刻度が二重に付いています。`severity` はその
リリースノートに記載されていた深刻度、`group_severity` は同じアドバイザリ
グループ（`advisory_group_key`）全体の最大深刻度です。緊急度の判断は
`group_severity` で行ってください — 上の istio 1.30.2 の fact は
`severity` こそ `low` ですが、グループとしては `high` です。

### 質問例

> 「CVE-2026-47778 はどのリリースで修正された？うちの envoy にも影響する？」

エージェントが `facts_by_entity(name: "CVE-2026-47778")` を呼び、
ブランチごとの `fixed_in`（envoy
v1.35.13・v1.36.9・v1.37.5・v1.38.3・v1.39.0）を根拠に答えます。
セキュリティの確認は、CVE id とアドバイザリ id（GHSA）の両方で
調べてください。ノートがどちらか一方しか引用していない fact は、その
識別子でしか引けません。

## list_facts

fact の増分同期フィードです。`fact_id` の昇順、つまり分析の古い順に
流れるため、最初のページが最新データではありません。レスポンスの
`next_since` を次の呼び出しの `since` に渡してページを送り、ローカル
コピーを最新に保つためのツールです。「X の最新リリース」のような質問には、このツール
ではなく `list_releases` や `get_release` を使ってください。プロジェクト・
タイプ・深刻度のフィルタでフィードを絞れます。

### パラメータ

| パラメータ | 必須 | 型 | デフォルト | 説明 |
|---|---|---|---|---|
| `project` | いいえ | string | *（なし＝全部）* | プロジェクトスラッグのフィルタ（例：`envoy`） |
| `type` | いいえ | string | *（なし＝全部）* | fact タイプのフィルタ：`security_fix`・`dependency_bump`・`capability_removed`・`capability_deprecated`・`api_version_changed`・`identifier_renamed`・`validation_tightened`・`default_changed`・`behavior_changed` |
| `severity` | いいえ | string | *（なし＝全部）* | 深刻度のフィルタ：`info`・`low`・`medium`・`high`・`critical`（ちょうどその深刻度のみ） |
| `since` | いいえ | integer | *（なし＝最初から）* | カーソル：この値より大きい `fact_id` だけが返ります。前のレスポンスの `next_since` を入れてください |
| `limit` | いいえ | integer | `50` | ページサイズ。最大 `200` |

### 呼び出し例

```json
{ "type": "security_fix", "severity": "critical", "limit": 2 }
```

### 実測レスポンス（一部省略）

```json
{
  "facts": [
    {
      "fact_id": 211, "project": "envoy", "version": "v1.36.9",
      "released_at": "2026-06-23T20:22:33.000Z",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-f8x4-rw5x-f3r7", "group_severity": "critical",
      "affected": { "fixed_in": "v1.36.9", … },
      "references": {
        "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
        "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass"
      },
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.36.9", …
    },
    {
      "fact_id": 215, "project": "envoy", "version": "v1.36.9",
      "fact_type": "security_fix", "severity": "critical", "mandatory": true,
      "advisory_group_key": "adv:ghsa-8phg-2h2q-jgxf", "group_severity": "critical",
      "applies_if": { "status": "degraded", "fallback": "if you proxy HTTP/3 to HTTP/1 backends" },
      "references": {
        "ids": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
        "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length"
      }, …
    }
  ],
  "next_since": 215
}
```

次のページは
`{ "type": "security_fix", "severity": "critical", "since": 215 }` です。
`next_since` が `null` で返ってくるまで繰り返してください。`null` は、
もう取得するものがないという意味です（`since=null` は送らないでください —
`400` になります）。

### 質問例

> 「記録されている critical のセキュリティ修正をひととおり見せて。」

エージェントが `list_facts(type: "security_fix", severity: "critical")` を
呼び、フィードをページ単位で読んで整理します。

## プライバシーとレート制限

一行の要旨：`check_stack` のバージョン比較はサーバープロセスの中で
行われるため、セルフホストすれば稼働中のバージョンはインフラを
離れません。どこまでが境界か、ログをどう扱っているかは
[README の「実行中のコンポーネントバージョンの取り扱い」](../README.ja.md#実行中のコンポーネントバージョンの取り扱い)を
見てください。アップストリーム公開 API の上限は IP あたり毎分 60
リクエストで、ホスト版エンドポイントはこのバケットをほかの利用者と
共有します。プロジェクトごとのポーリングの代わりに `check_stack` 1 回に
まとめ、負荷の高い使い方ならセルフホストに切り替えてください。
