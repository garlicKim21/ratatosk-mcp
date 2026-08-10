# 설치와 사용법

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

이 페이지는 Ratatosk의 릴리스 변경(change)을 AI 에이전트에서 쓰는 네 가지 방법을
설명합니다. 설치 없이 호스팅 엔드포인트에 연결하기, 노트북에서 stdio 서버
띄우기, 쿠버네티스 클러스터에 Helm으로 배포하기, 그리고 kagent와
연동하기입니다.

**MCP(Model Context Protocol)**는 AI 에이전트가 외부 도구를 호출할 때 쓰는
표준 프로토콜입니다. ratatosk-mcp는 이 프로토콜로 도구 7개를 제공하는
서버입니다. `check_stack`(실행 중인 버전 목록을 받아 업그레이드 경로의 변경을
층별로 갈라 반환), `list_changes`(증분 변경 피드), `changes_by_entity`(CVE·플래그
등 식별자 역조회), `get_matter`(한 사안이 등장한 릴리스 전부),
`get_release`(릴리스 하나 전체),
`list_releases`(프로젝트별 최신 릴리스 요약), `list_projects`(추적 프로젝트와
정식 슬러그)입니다. Claude Code, Claude Desktop, kagent, 직접 만든 SDK
에이전트처럼 MCP를 지원하는 클라이언트라면 무엇이든 연결할 수 있습니다.

## 실행 방식 고르기

| 이런 상황이라면 | 방법 | 섹션 |
| --- | --- | --- |
| 아무것도 설치하지 않고 일단 써 보고 싶다 | 호스팅 엔드포인트 URL 등록 | [호스팅](#호스팅-엔드포인트) |
| 노트북에서 Claude Code / Claude Desktop 등 stdio MCP 클라이언트 사용 | `docker run` (stdio) | [로컬](#로컬-stdio) |
| 쿠버네티스에서 자체 에이전트 운용 (프레임워크 무관, CI 잡, SDK 클라이언트) | Helm 차트 | [클러스터 내 단독 배포](#클러스터-내-단독-배포-helm) |
| [kagent](https://kagent.dev) 사용 중 | `kagent.enabled=true` Helm 옵션 또는 매니페스트 | [kagent 연동](#kagent-연동) |

네 방식 모두 같은 도구 7개로 같은 공개 데이터를 제공하며, 어느 모드든 계정도
API 키도 필요 없습니다. 차이는 셋입니다.

- **`check_stack`에 넘긴 실행 중인 버전이 어디까지 가는가.** 설치형은 버전
  비교가 사용자 프로세스 안에서 끝나 그 버전이 인프라를 떠나지 않습니다.
  호스팅은 버전이 서버 메모리를 지나가지만 어떤 로그에도 기록되지 않습니다.
- **업스트림 요청 한도를 누구와 나누는가.** 호스팅은 공유 버킷을 쓰고,
  설치형은 자기 IP 몫을 받습니다.
- **감사 스트림을 남길 수 있는가.** 설치형만 가능합니다.

한 가지 공통점도 알아두세요. `get_release`처럼 특정 버전을 인자로 받는
도구는 어느 방식에서든 그 버전을 업스트림 요청 경로에 담아 조회합니다.
조회 대상을 지목하는 값이기 때문입니다. 다만 그 경로도 서버 쪽 로그에는
남지 않습니다. 액세스 로그가 기록 전에 쿼리스트링을 제거하고
`/v1/releases/…`·`/v1/upgrade/…` 경로를 접두사만 남기기 때문입니다. 각
섹션에서 자세히 설명합니다.

## 사전 준비

- **호스팅**: 원격 MCP 서버(Streamable HTTP — 스트리밍 방식의 HTTP
  트랜스포트)를 지원하는 클라이언트만 있으면 됩니다. 그 밖에 준비할 것은
  없습니다.
- **로컬(stdio)**: Docker. 소스에서 빌드하려면 Go 1.26 이상.
- **예시의 `claude mcp add` 명령**: [Claude Code](https://claude.com/claude-code)
  CLI(`claude` 명령)가 설치되어 있어야 합니다. 다른 MCP 클라이언트를 쓴다면
  각자의 등록 방법을 따르면 되므로 필수는 아닙니다.
- **클러스터(Helm)**: 쿠버네티스 클러스터, Helm 3, 그리고 이 저장소를
  클론한 사본이 필요합니다. 차트는 별도 차트 저장소나 OCI 레지스트리로
  발행되지 않고 이 저장소 안에만 있습니다.
- **아웃바운드 HTTPS(egress)**: 설치형은 어느 방식이든 서버 프로세스가
  `ratatosk.io:443`으로 나가는 HTTPS 연결을 열 수 있어야 합니다.
  NetworkPolicy나 egress 프록시로 아웃바운드를 제한하는 클러스터라면 이
  목적지를 허용 목록에 추가하세요. 프록시나 미러를 경유해야 한다면
  `RATATOSK_URL`(차트 값: `ratatoskUrl`)로 업스트림 주소를 바꿀 수
  있습니다. 차단된 채로 실행하면 도구가 오류를 돌려주거나 빈 결과가
  돌아옵니다(`check_stack`은 오류 대신 컴포넌트별 `note` 필드에
  `fetch failed: …`를 담아 돌려줍니다) — [문제 해결](#문제-해결) 참조.
- **kagent 통합**: kagent가 이미 설치된 클러스터(kagent CRD 포함).

## 호스팅 엔드포인트

`https://ratatosk.io/mcp` — 설치 없이 바로 쓰는 원격 MCP 엔드포인트입니다.
Streamable HTTP로 동작하며 무상태(stateless)입니다: 세션이 없어
`Mcp-Session-Id` 왕복 없이 요청 하나하나가 독립적으로 처리됩니다.

### 클라이언트에 엔드포인트 등록하기

Claude Code:

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

Claude(웹·데스크톱)에서는 원격 커넥터 설정의 URL 입력란에
`https://ratatosk.io/mcp`를 넣으면 됩니다. 다른 클라이언트는 각자의 원격
MCP 서버 등록 방법을 따르세요.

### 확인

클라이언트 없이도 엔드포인트가 살아 있는지 curl로 확인할 수 있습니다:

```bash
curl -s -X POST https://ratatosk.io/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}'
```

응답은 SSE(Server-Sent Events) 프레이밍으로 옵니다 — `Content-Type`이
`text/event-stream`이어도 실패가 아닙니다. `data:` 줄의 JSON에
`"serverInfo"`와 `"name":"ratatosk"`가 보이면 정상입니다:

```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{…,"serverInfo":{"name":"ratatosk","version":"0.7.4"}}}
```

Claude Code라면 `claude mcp list` 출력에서 확인하세요:

```
ratatosk: https://ratatosk.io/mcp (HTTP) - ✔ Connected
```

### 알려진 동작과 제한

- **SSE GET은 405**: 무상태 모드는 서버가 먼저 보내는 알림 스트림(GET으로
  여는 SSE)을 제공하지 않으므로 405 응답이 정상 동작입니다. 브라우저
  주소창으로 열면 안내 페이지(`/docs/mcp`)로 리다이렉트됩니다.
- **적정 사용**: 호스팅 엔드포인트는 **호출자당 분당 도구 호출 60회**를
  허용합니다. 전체가 나눠 쓰는 몫이 아니라 호출자 주소별로 세므로, 한 명이
  바쁘다고 나머지가 굶지 않습니다. 넘기면 `Retry-After`가 붙은 `429`가
  돌아옵니다. 도구 호출 1회가 API 요청 1회는 아닙니다 — `check_stack`은
  컴포넌트 하나당 업스트림 요청 1회를 씁니다. 한도를 도구 호출 단위로
  표현한 이유가 그것입니다. 직접 `/v1`을 호출하면 자기 IP 몫으로 분당
  1200회를 받습니다. 폴링처럼 사용량이 많은 용도라면 설치형으로 전환하세요.

### 프라이버시 — 호스팅 엔드포인트가 기록하는 것

호스팅에서는 `check_stack` 인자(실행 중인 버전 목록)가 서버 메모리를
지나가지만, 요청 내용은 어디에도 기록되지 않습니다:

- MCP 서버 로그에는 시작 줄과 업스트림 오류 줄만 남고, 정상 요청은
  기록되지 않습니다. 오류 줄에도 요청 인자는 담기지 않습니다.
- 업스트림 API 요청 로그는 호출자가 `traceparent`를 보낸 경우에만 한 줄
  남으며, 그 줄에도 정규화된 엔드포인트 라벨과 trace_id뿐입니다 — 경로·쿼리는
  기록되지 않습니다.
- 서버 앞단의 액세스 로그는 쿼리스트링을 제거하고, 슬러그·버전이 경로에
  실리는 `/v1/releases/…`·`/v1/upgrade/…` 경로는 접두사만 남기며, IP를
  마스킹하고 요청 바디는 기록 항목에 아예 없습니다. 외부로 전송되지 않고
  서버 안에서 크기를 기준으로 순환 삭제됩니다(현재 트래픽 기준 며칠 분량).
- 아래에서 설명할 감사 스트림은 호스팅에서 일부러 꺼 두었고, 앞으로도
  켜지 않습니다.

다만 통제 밖의 경계가 하나 있습니다: CDN 구간의 접속
메타데이터는 CDN 사업자의 정책을 따르며, 이는 ratatosk.io가 관여할 수 없는
구간입니다. 이 경계가 요건에 맞지 않으면 설치형을 쓰세요 — `check_stack`에
넘긴 버전이 프로세스 밖으로 나가지 않습니다.

## 로컬 (stdio)

stdio는 MCP 클라이언트가 서버를 자식 프로세스로 띄우고 표준 입출력으로
대화하는 방식입니다. 노트북에서 Claude Code나 Claude Desktop과 함께 쓸 때의
기본 경로입니다.

### Claude Code

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.4
```

`0.7.4` 자리에는 원하는 [릴리스 태그](https://github.com/garlicKim21/ratatosk-mcp/releases)를
쓰면 되고, 항상 최신을 따라가려면 `latest`를 쓰세요 —
[버전 고정](#버전-고정) 참조.

### Claude Desktop

Claude Desktop은 CLI 대신 설정 파일(`claude_desktop_config.json`)로 MCP
서버를 등록합니다. `mcpServers`에 다음을 추가하세요:

```json
{
  "mcpServers": {
    "ratatosk": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "ghcr.io/garlickim21/ratatosk-mcp:0.7.4"]
    }
  }
}
```

설정 파일 위치는 운영체제마다 다르므로 Claude Desktop 문서를 참조하세요.
설치 없이 쓰려면 위 [호스팅 엔드포인트](#호스팅-엔드포인트)를 원격
커넥터로 등록해도 됩니다.

### 소스 빌드

Docker 없이 바이너리로 쓰려면:

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

stdio를 지원하는 MCP 클라이언트라면 무엇이든 같은 방식입니다.

### 확인

```bash
claude mcp list
# ratatosk: docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.4 - ✔ Connected
```

그리고 에이전트에게 이렇게 물어보세요:

> "우리는 envoy v1.36.8과 istio 1.30.1을 쓰는데, 업그레이드 전에 조치할 게
> 있어?"

에이전트가 `check_stack`을 호출해, 지금 버전 이후의 보안 수정·기능
제거·기본값 변경을 근거 인용과 함께 알려 주면 성공입니다.

## 클러스터 내 단독 배포 (Helm)

`MCP_HTTP_ADDR`를 설정하면 같은 바이너리가 `/mcp`에서 Streamable HTTP로
MCP를 제공합니다(`/healthz` 헬스 체크 포함). 차트는 이를 ClusterIP Service로
배포하고 `MCP_HTTP_ADDR`를 `service.port`(기본 8080)에 맞춰 자동
설정합니다. 클러스터 안에서는 프로세스를 따로 띄울 필요 없이 URL로
접속하면 됩니다. SDK로 만든 자체 에이전트, 다른 에이전트 프레임워크,
업그레이드 전에 `check_stack`으로 배포를 막아 세우는 CI 잡까지 모두
마찬가지입니다.

### 설치

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

RBAC도 시크릿도 필요 없고, Pod Security Standards(PSS)의 `restricted`
프로파일에서도 경고 없이 동작합니다. 차트 옵션 전체는
[차트 README](../charts/ratatosk-mcp/README.ko.md)에 정리되어 있습니다.
다음 두 경우에는 `--set statelessHttp=true`를 켜세요. MCP 스펙 2026-07-28
리비전을 쓰는 최신 클라이언트(`Mcp-Protocol-Version: 2026-07-28` 헤더를
보내는 클라이언트)를 HTTP로 받을 때, 그리고 레플리카를 2개 이상으로 늘릴
때입니다 — [설정 레퍼런스](#설정-레퍼런스) 참조.

### 확인

파드가 떠 있는지:

```bash
kubectl get pods -l app.kubernetes.io/name=ratatosk-mcp
# NAME                            READY   STATUS    RESTARTS   AGE
# ratatosk-mcp-6f7b9c8d4-x2m5q    1/1     Running   0          30s
```

시작 로그 한 줄에 수신 주소와 업스트림이 표시됩니다:

```bash
kubectl logs deploy/ratatosk-mcp
# {"time":"…","level":"INFO","msg":"listening","service":"mcp","transport":"http","addr":":8080/mcp","mode":"stateful","upstream":"https://ratatosk.io","version":"0.7.4"}
```

헬스 체크까지 확인하려면:

```bash
kubectl port-forward svc/ratatosk-mcp 8080:8080 &
sleep 2   # 포워드가 준비되기 전에 curl이 돌면 connection refused가 납니다
curl -i http://localhost:8080/healthz
# HTTP/1.1 200 OK
```

확인이 끝나면 백그라운드로 띄운 port-forward를 `kill %1`로 정리하세요.

### 접속

클러스터 안의 MCP 클라이언트를 다음 URL로 연결하세요:

```
http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

### 업그레이드

```bash
git -C ratatosk-mcp pull
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
```

설치할 때 `--set`으로 값을 바꿨다면 업그레이드에도 같은 값을 다시
지정하거나 `--reuse-values`를 붙이세요.

## kagent 연동

결과가 같은 두 가지 방법입니다. 하나만 고르세요:

**A. Helm 토글** — 설치 한 번에 서버, kagent 등록(RemoteMCPServer),
예제 에이전트 `ratatosk-agent`까지 설치됩니다:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` 기본값은 `default-model-config`입니다. 환경이 다르면
덮어쓰고, 예제 에이전트 없이 등록만 원하면 `kagent.agent.enabled=false`로
설정하세요.

**B. 매니페스트** — 같은 세 조각을 Helm 없이 복사해 붙여 넣는 방식으로
설치합니다([상세](../examples/kagent/README.ko.md)):

```bash
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml
kubectl apply -f $BASE/ratatosk-agent.yaml
```

이 경로는 클론이 필요 없습니다. 매니페스트에는 `namespace: kagent`가 들어 있습니다.

### 확인

어느 쪽이든 kagent UI에 `ratatosk-agent`가 나타납니다. 이렇게 물어보세요:

> "이 클러스터에서 업그레이드 전에 조치할 게 있나요?"

에이전트는 kagent 내장 읽기 전용 클러스터 도구로 실행 중인 버전을 스스로
알아냅니다(`kagent.agent.k8sTools=false`로 끌 수 있음). 질문에 버전을
직접 적어도 됩니다. 에이전트가 UI에 나타나지 않으면
[문제 해결](#문제-해결)을 보세요.

> **모델 최소 요건**: 에이전트를 한 번 실행하면 내부 모델 호출이 6회 이상
> 발생하고, kagent Go ADK는 429(요청 한도 초과) 응답을 재시도하지
> 않습니다. 따라서 분당 약 10요청 이상을 허용하는 모델 티어가 필요하며,
> 그보다 낮으면 실행할 때마다 실패합니다. 예를 들어 Gemini 무료
> 티어(2026-07 기준)에서는 분당 5요청인 full-flash 계열로는 실행이 끝까지
> 가지 못하고, flash-lite 계열이라야 에이전트를 돌릴 수 있는 한도가
> 나옵니다.

## 설정 레퍼런스

서버 동작을 바꾸는 설정은 모두 환경 변수이고, Helm 차트는 각 변수에 대응하는
값을 제공합니다. 차트에는 이 밖에 레플리카 수·리소스·서비스 타입처럼 배포
형태를 정하는 값도 있습니다 —
[차트 README](../charts/ratatosk-mcp/README.ko.md) 참조.

| 환경 변수 | 차트 값 | 기본값 | 설명 |
|---|---|---|---|
| `RATATOSK_URL` | `ratatoskUrl` | `https://ratatosk.io` | 업스트림 changes API(`/v1`, 공개, 읽기 전용). egress를 프록시·미러로 우회할 때 변경 |
| `MCP_HTTP_ADDR` | *(차트가 `service.port`로 자동 설정)* | *(없음 = stdio)* | 설정 시(예: `:8080`) stdio 대신 `/mcp` Streamable HTTP로 동작, `/healthz` 포함 |
| `MCP_HTTP_STATELESS` | `statelessHttp` | *(꺼짐)* | `1`이면 세션 상태 없는 HTTP: `Mcp-Session-Id` 왕복이 없고, MCP 스펙 2026-07-28 리비전을 쓰는 최신 클라이언트를 HTTP로 받으려면 필요합니다. 레플리카 2개 이상 수평 확장에도 권장. 구형 클라이언트는 어느 쪽이든 동작합니다 |
| `MCP_LOG` | `logLevel` | `info` | 허용값 `info`(기본)·`debug`·`warn`·`error` — 인식하지 못한 값은 경고 없이 `info`로 처리됩니다. `debug`는 업스트림 호출별 소요 시간을 추가하고, `warn`·`error`는 로그를 줄입니다. 어느 레벨에서도 요청 인자는 기록되지 않습니다 — [로그와 감사 스트림](#로그와-감사-스트림) 참조 |
| `MCP_AUDIT` | `auditMode` | *(꺼짐)* | `metadata` 또는 `full` — 도구 호출 감사 스트림. [감사 스트림 켜기](#감사-스트림-켜기) 참조 |
| `MCP_RATE_LIMIT_PER_MIN` | `rateLimitPerMin` | *(꺼짐)* | 호출자당 분당 도구 호출 상한. 내가 통제하지 않는 호출자를 대신 받는 서버용입니다. 호출자 주소마다 몫을 따로 주므로 한 명이 바쁘다고 나머지가 굶지 않고, 넘기면 `Retry-After`가 붙은 `429`가 돌아옵니다. 주소는 창이 유지되는 동안의 메모리 버킷 키일 뿐이고 기록하지도, 상류로 보내지도 않습니다. `CF-Connecting-IP` → `X-Forwarded-For` → `X-Real-IP` → 피어 주소 순으로 읽으므로, 앞단 프록시가 그중 하나를 채우고 **클라이언트가 보낸 값은 지워야** 합니다. 호출자가 조작할 수 있는 값을 키로 쓰면 제한이 아니니까요. 단일 이용자 설치라면 꺼 두세요 |

docker로 HTTP 모드를 직접 띄우는 예:

```bash
docker run --rm -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:0.7.4
```

## 로그와 감사 스트림

### 운영 로그

로그는 stderr에 JSON 한 줄씩 남습니다(stdout은 stdio 전송 전용). 기본
레벨(`info`)에서 남는 것은 수명주기 줄(시작 시 `listening` 한 줄, stdio
세션 종료 한 줄)과, 업스트림 연결에 문제가 생겼을 때 남는 경고·오류
줄뿐입니다. 정상 요청은 한 줄도 남기지 않습니다. 참고로 stdio 세션 종료
줄은 두 모양입니다: 클라이언트가 정리하고 끊으면
`"msg":"stdio session ended"`(INFO), 정리 없이 갑자기 끊으면
`"msg":"stdio session ended with error"`(ERROR) — 후자도 장애가 아니라 종료
방식의 기록입니다. 오류가 나도 원래 오류 메시지를 그대로 옮기지 않습니다.
요청 URL에 실행 중인 버전이 들어 있기 때문입니다. 대신 엔드포인트 패턴과
오류 종류만으로 로그 문구를 새로 만들어 기록합니다:

```json
{"time":"…","level":"ERROR","msg":"upstream fetch failed","service":"mcp","upstream":"/v1/changes","kind":"connection_refused","tool":"check_stack"}
```

`MCP_LOG=debug`로 올리면 여기에 더해 업스트림 호출마다 엔드포인트 패턴·상태
코드·소요 시간이 한 줄씩 붙습니다. **어느 레벨에서도 요청 인자(버전 등)는
로그에 담기지 않습니다.**

### 감사 스트림 켜기

감사 스트림은 "어느 클라이언트가 어떤 도구를 불렀는가"를 남기는 별도
기록입니다. **기본은 꺼짐이고, 꺼진 동안에는 아무것도 기록되지 않습니다**
— 스트림 자체가 존재하지 않습니다.

켜려면 `MCP_AUDIT`를 설정합니다:

```bash
# docker
docker run -i --rm -e MCP_AUDIT=metadata ghcr.io/garlickim21/ratatosk-mcp:0.7.4

# Helm
helm upgrade ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp --set auditMode=metadata
```

켜면 도구 호출마다 `event:"audit"` JSON 한 줄이 운영 로그와 같은
스트림(stderr)에 남습니다:

```json
{"argument_names":["components","detail"],"client_name":"my-agent","client_version":"1.0.0","event":"audit","level":"INFO","msg":"audit","outcome":"ok","service":"mcp","time":"2026-07-31T02:54:47.029169122Z","tool":"check_stack","transport":"stdio"}
```

0.6.2부터, 서버가 상태 유지(stateful) HTTP 모드(차트 기본값)로 돌아갈
때는 HTTP 감사 레코드에 전송 세션 식별자인 `session_id` 필드가 함께
담깁니다(예: `"session_id":"T3E77BYZFDDA33SIUSORQ365ZL"`). 동시에 접속한
호출자들을 세션 단위로 구분하는 값입니다. `statelessHttp`(환경 변수
`MCP_HTTP_STATELESS`)를 켜면 이름 붙일 세션이 없으므로 `session_id`도
클라이언트가 보고한 `clientInfo`도 레코드에서 빠지고, 호출 단위를 이어
주는 키는 아래의 `trace_id`뿐입니다. 감사 스트림에서 호출자를 구분해야
한다면 상태 유지 모드로 돌리거나 호출자가 `traceparent`를 보내게 하세요.
위 예시 같은 stdio 레코드에도 `session_id`는 없습니다. 프로세스 하나에
호출자가 하나뿐이라 구분할 것이 없기 때문입니다.

두 모드의 차이:

- **`metadata`** — 도구 이름, 결과(`ok`·`error`·`tool_error`), 전송 방식,
  클라이언트가 보고한 `clientInfo`(stdio·상태 유지 HTTP에서), 그리고
  인자의 **이름**만. 인자 값은 이 모드에서 절대 기록되지 않습니다.
- **`full`** — 위에 더해 인자 값 전체(`arguments` 필드). `check_stack`에
  넘긴 버전 목록도 여기 담기므로, 그 값이 로그 시스템에 남아도 되는지
  판단한 뒤 켜세요.

이 스트림은 사용자 인프라 안에서 만들어져 자체 로그 콜렉터에 쌓입니다.
보존 기간과 위변조 방지는 로그 플랫폼에서 정하면 되고, `event` 필드가
`audit`인 레코드를 별도 싱크로 라우팅하면 운영 로그와 다른 보존 정책을
적용할 수 있습니다.

한계도 알아두세요. 이 서버에는 인증이 없으므로, 기록에 남는 "호출자"는
클라이언트가 스스로 보고한 `clientInfo`와 전송 계층이 드러내는 정보가
전부입니다. 어떤 사람이 에이전트에게 그 호출을 시켰는지는 에이전트 계층의
기록에서 찾아야 합니다.

호스팅 엔드포인트에는 감사 스트림이 없습니다 — 요청 내용을 보관하지
않는다는 운영 원칙에 따라 항상 꺼져 있습니다. 감사 요건이 있다면 설치형을
쓰세요.

### 트레이스 연계 (traceparent)

에이전트 프레임워크가 W3C trace context(분산 트레이싱 표준의 요청 식별
헤더)를 지원하는 경우, 도구 호출의 `_meta`에 `traceparent`를 실어 보낼 수
있습니다:

```json
{"method":"tools/call","params":{"name":"check_stack","arguments":{"components":[…]},"_meta":{"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}}}
```

그러면 그 호출이 남기는 로그 줄(오류·`debug` 레벨)과 감사 레코드에
`trace_id` 필드가 찍히고, 업스트림
`/v1` 요청에도 같은 `traceparent` 헤더가 전달되어 에이전트 → MCP 서버 →
업스트림까지 하나의 `trace_id`로 이을 수 있습니다. 감사 레코드 혼자서는
잇지 못하는 계층을 이어 주는 조인 키가 바로 이것입니다. 에이전트 플랫폼의
프롬프트와 이곳의 도구 호출을 연결해 줍니다. 형식이 잘못된 값은 버려지고
전달되지 않으며, 보내지 않으면 아무것도 기록되지 않습니다.

## 문제 해결

| 증상 | 확인 | 원인과 해결 |
|---|---|---|
| 도구 호출이 오류를 반환하고(`check_stack`은 오류 대신 컴포넌트별 `note`에 `fetch failed: …`), 로그에 `"msg":"upstream fetch failed"`와 `kind: connection_refused`·`dns`·`timeout` | `kubectl logs deploy/ratatosk-mcp` (로컬은 클라이언트의 MCP 로그) | egress 차단. `ratatosk.io:443` 아웃바운드 HTTPS를 허용하거나, 미러를 두고 `RATATOSK_URL`(차트: `ratatoskUrl`)로 지정 |
| 로그에 `"msg":"upstream rate limited"`, `status: 429` | 위와 동일 | 업스트림 한도(분당 1200요청/IP) 초과. 프로젝트별 `list_changes` 폴링 대신 `check_stack` 하나로 묶어 호출하고, 잠시 후 재시도 |
| HTTP 클라이언트가 `400 Bad Request: protocol version "2026-07-28" is only supported on stateless HTTP servers`를 받음 | 클라이언트가 보내는 `Mcp-Protocol-Version` 헤더 | 클라이언트가 MCP 2026-07-28 리비전을 쓰는데 상태 유지 HTTP 모드는 이를 거절합니다. `--set statelessHttp=true`(환경 변수: `MCP_HTTP_STATELESS=1`)를 켜세요 — 오류 메시지의 `StreamableHTTPOptions.Stateless`는 같은 스위치의 Go SDK 쪽 이름입니다 |
| kagent UI에 `ratatosk-agent`가 나타나지 않음 | `kubectl api-resources \| grep kagent` · `kubectl get pods -n kagent` | kagent CRD가 없는 클러스터에서는 `kagent.enabled=true` 설치가 실패합니다 — kagent를 먼저 설치하세요. 매니페스트 경로는 `namespace: kagent`가 하드코딩되어 있으므로 kagent를 다른 네임스페이스에 설치했다면 매니페스트를 수정해야 합니다 |
| kagent에서 에이전트 런타임을 Go ADK로 바꾸면 `ImagePullBackOff` | `kubectl describe pod` | 이 차트와 무관한 kagent 0.9.12 자체의 알려진 문제([#2247], 0.9.12 이후 수정): Go ADK 이미지는 `ghcr.io`에만 발행되는데 컨트롤러 기본값이 폐기된 `cr.kagent.dev`를 가리킵니다. 우회: kagent 설치에 `--set controller.agentImage.registry=ghcr.io` |
| kagent 에이전트가 실행할 때마다 실패 | 에이전트 로그의 429 | 모델 티어의 요청 한도 부족 — [kagent 연동](#kagent-연동)의 모델 최소 요건 참조 |

[#2247]: https://github.com/kagent-dev/kagent/issues/2247

## 버전 고정

- **컨테이너 이미지**: 릴리스마다 버전 태그(예: `0.7.4`)로 자동 빌드되며
  멀티 아치(`linux/amd64`, `linux/arm64`)입니다. `latest`는 최신 릴리스를
  따라갑니다 — 재현 가능한 배포에는 버전 태그를 고정하세요.
- **Helm**: 차트는 차트 저장소로 발행되지 않고 저장소 클론으로만
  설치합니다. 특정 버전에 고정하려면 클론 후 릴리스 태그를 체크아웃하세요:

  ```bash
  git -C ratatosk-mcp checkout v0.7.4
  ```

  이미지 태그는 기본적으로 차트의 `appVersion`을 따르고, `image.tag` 값으로
  덮어쓸 수 있습니다.

## 업스트림 API

이 서버는 공개 REST API의 얇은 클라이언트입니다. 직접 호출하고 싶다면
ratatosk.io의 `GET /v1`을 열어 보세요. 인덱스가 스스로를 설명합니다. API
키는 없고, IP당 분당 1200회 제한이 있습니다.

## 다음 단계

- 도구 7개의 파라미터·예시 호출·실제 응답: [도구 레퍼런스](tools.ko.md)
- 한눈에 보는 도구 표: [README의 도구 표](../README.ko.md#도구)
- 차트 값 전체와 kagent 토글: [차트 README](../charts/ratatosk-mcp/README.ko.md)
- kagent 매니페스트와 예제 에이전트: [kagent 예제](../examples/kagent/README.ko.md)
