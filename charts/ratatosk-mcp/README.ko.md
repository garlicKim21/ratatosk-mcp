# ratatosk-mcp 헬름 차트

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

ratatosk MCP 서버를 스트리밍 HTTP 모드의 ClusterIP Service로 배포합니다.
RBAC도 시크릿도 없고 PSS `restricted`에서 무경고로 돕니다.

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → MCP 엔드포인트: http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## Values

| 키 | 기본값 | 의미 |
| --- | --- | --- |
| `image.repository` | `ghcr.io/garlickim21/ratatosk-mcp` | 이미지 |
| `image.tag` | 차트 `appVersion` | 특정 서버 버전 고정 |
| `ratatoskUrl` | `https://ratatosk.io` | 업스트림 facts API |
| `service.port` | `8080` | Service / MCP 엔드포인트 포트 |
| `resources` | 소형 requests/limits | 바쁜 클러스터면 조정 |
| `kagent.enabled` | `false` | kagent 통합까지 함께 설치 (아래 참조) |
| `kagent.modelConfig` | `default-model-config` | kagent 설치의 ModelConfig 이름 |
| `kagent.agent.enabled` | `true` | 예제 에이전트 설치 (kagent.enabled일 때) |
| `kagent.agent.name` | `ratatosk-agent` | 예제 에이전트 이름 |
| `kagent.agent.k8sTools` | `true` | kagent 내장 읽기 전용 클러스터 도구를 에이전트에 부여해 구동 버전을 스스로 파악 |

## kagent 통합

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`RemoteMCPServer`(kagent가 서버의 툴을 모두 자동 발견)와 준비된
`ratatosk-agent`가 추가됩니다. 에이전트에는 kagent 내장 읽기 전용
클러스터 도구(`k8s_get_resources`, `k8s_get_resource_yaml`)도 붙어서
구동 중인 버전을 스스로 알아냅니다 — 끄려면
`kagent.agent.k8sTools=false`. kagent CRD가 있는 클러스터에서만
켜세요. 헬름 없이 가려면 [`examples/kagent/`](../../examples/kagent/)를
보세요.

## 업그레이드

```bash
helm upgrade ratatosk-mcp ./charts/ratatosk-mcp
```

설정한 값은 유지되고, `image.tag`로 고정하지 않았다면 이미지는 차트
`appVersion`을 따릅니다.
