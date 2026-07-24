# Ratatosk MCP サーバー

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[Ratatosk](https://ratatosk.io) は CNCF graduated/incubating 全プロジェクトの
リリースノートを毎日読み、型付きファクト — セキュリティ修正、破壊的変更、
削除、非推奨化、既定値の変更 — を抽出します。各ファクトには原文の引用と
ソース URL が付きます。この MCP サーバーがそのファクトをクラスタ内の
エージェントに提供します。

プライバシー: `check_stack` ツールは稼働中のバージョンをこのプロセス内で
ローカルに比較します。ratatosk サーバーに送られるのはプロジェクトのスラッグ
だけで、バージョンがクラスタの外に出ることはありません。上流 API は公開・
読み取り専用・認証不要です（IP あたり毎分 60 リクエスト制限）。

## ツール

| ツール | 用途 |
| --- | --- |
| `list_projects` | 追跡プロジェクトの一覧 — スラッグはまずここで確認 |
| `check_stack` | 稼働中バージョンを既知のファクトと照合（ローカル比較） |
| `get_release` | レビュー済みリリース 1 件: 全ファクトとソース URL |
| `facts_by_entity` | 逆引き: 1 つの CVE/フラグ/CRD に触れた全ファクト |
| `list_facts` | `since` カーソルでの増分ファクトフィード |

## インストール

```bash
kubectl apply -f ratatosk-deploy.yaml            # Deployment + Service（RBAC 不要）
kubectl apply -f ratatosk-remote-mcpserver.yaml  # kagent への登録
kubectl apply -f ratatosk-agent.yaml             # 任意: release-triage サンプルエージェント
```

Helm を使う場合は、チャートの kagent トグル 1 つで同じ構成になります:

```bash
helm install ratatosk-mcp ../../charts/ratatosk-mcp -n kagent --set kagent.enabled=true
```

サンプルエージェントにこう聞いてみてください:

> 「kubernetes v1.36.0、cilium v1.17.18、envoy v1.38.3 を運用中です。
> アップグレード前に対応が必要なものはありますか？」

エージェントは深刻度順に整理したファクトに、リリースノート原文の引用と
ソースリンクを添えて答えます。
