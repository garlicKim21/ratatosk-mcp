# Ratatosk MCP 서버

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[Ratatosk](https://ratatosk.io)는 CNCF graduated/incubating 프로젝트 전체의
릴리스 노트를 매일 읽고 타입 있는 팩트 — 보안 수정, 브레이킹 체인지, 제거,
지원 중단, 기본값 변경 — 를 추출합니다. 팩트마다 원문 인용과 소스 URL이
붙습니다. 이 MCP 서버가 그 팩트를 클러스터 내 에이전트에게 제공합니다.

프라이버시: `check_stack` 툴은 실행 중인 버전을 이 프로세스 안에서 로컬로
비교합니다. ratatosk 서버로는 프로젝트 슬러그만 전송되고, 버전은 클러스터
밖으로 나가지 않습니다. 업스트림 API는 공개·읽기 전용·무자격증명입니다
(IP당 분당 60회 제한).

## 도구

| 도구 | 용도 |
| --- | --- |
| `list_projects` | 추적 프로젝트 전목록 — 슬러그는 먼저 여기서 확인 |
| `check_stack` | 실행 중 버전을 알려진 팩트와 대조 (로컬 비교) |
| `get_release` | 리뷰된 릴리스 하나: 전체 팩트와 소스 URL |
| `facts_by_entity` | 역인덱스: 하나의 CVE/플래그/CRD를 건드린 모든 팩트 |
| `list_facts` | `since` 커서로 증분 팩트 피드 |

## 설치

```bash
kubectl apply -f ratatosk-deploy.yaml            # Deployment + Service (RBAC 불필요)
kubectl apply -f ratatosk-remote-mcpserver.yaml  # kagent에 등록
kubectl apply -f ratatosk-agent.yaml             # 선택: 예제 에이전트 ratatosk-agent
```

헬름을 쓴다면 차트의 kagent 토글 하나로도 같은 구성이 됩니다:

```bash
helm install ratatosk-mcp ../../charts/ratatosk-mcp -n kagent --set kagent.enabled=true
```

예제 에이전트에게 이렇게 물어보세요:

> "이 클러스터에서 업그레이드 전에 조치할 게 있나요?"

에이전트는 kagent 내장 읽기 전용 클러스터 도구(`kagent-tool-server`의
`k8s_get_resources`, `k8s_get_resource_yaml` — 기본 kagent 설치에 포함)로
구동 중인 버전을 스스로 알아냅니다. 버전을 직접 알려줘도 됩니다:

> "kubernetes v1.36.0, cilium v1.17.18, envoy v1.38.3을 돌리는데,
> 업그레이드 전에 조치할 게 있나요?"

에이전트는 심각도 순으로 정리된 팩트에 릴리스 노트 원문 인용과 소스 링크를
붙여 답합니다.
