# 설치와 사용법

[English](install.en.md) · [한국어](install.ko.md) · [日本語](install.ja.md)

## 실행 방식 고르기

| 당신은… | 방법 | 섹션 |
| --- | --- | --- |
| 노트북에서 Claude Code / Claude Desktop 등 stdio MCP 클라이언트 사용 | `docker run` (stdio) | [로컬](#로컬-stdio) |
| 쿠버네티스에서 자체 에이전트 운용 (프레임워크 무관, CI 잡, SDK 클라이언트) | 헬름 차트 | [클러스터 단독](#클러스터-단독-helm) |
| [kagent](https://kagent.dev) 사용 중 | `kagent.enabled=true` 헬름 옵션 또는 매니페스트 | [kagent와 함께](#kagent와-함께) |

세 방식 모두 같은 바이너리로 같은 공개 API를 사용합니다. 어느 모드든 계정도
API 키도 필요 없습니다.

## 로컬 (stdio)

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

소스에서 빌드해 바이너리를 등록해도 됩니다:

```bash
go build -o ratatosk-mcp .
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

stdio를 지원하는 MCP 클라이언트라면 무엇이든 같은 방식입니다.

## 클러스터 단독 (Helm)

`MCP_HTTP_ADDR`를 설정하면 같은 바이너리가 `/mcp`에서 스트리밍 HTTP로 MCP를
서빙합니다(`/healthz` 프로브 포함). 차트는 이를 ClusterIP Service로 배포해,
클러스터 안의 무엇이든 프로세스를 띄우는 대신 URL로 접속합니다 — SDK로 만든
자체 에이전트, 다른 에이전트 프레임워크, 업그레이드 전에 `check_stack`으로
게이트를 거는 CI 잡까지:

```bash
git clone https://github.com/garlicKim21/ratatosk-mcp
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp
# → MCP 클라이언트를 http://ratatosk-mcp.<namespace>.svc:8080/mcp 로
```

업그레이드는 값을 유지한 채 `helm upgrade` 한 줄입니다. RBAC도 시크릿도
필요 없고 PSS `restricted`에서 무경고로 돕니다. 차트 옵션은
[차트 README](../charts/ratatosk-mcp/README.ko.md)에 정리되어 있습니다.

## kagent와 함께

동등한 두 경로 중 하나만 고르세요:

**A. 헬름 토글** — 설치 한 번에 서버, kagent 등록(RemoteMCPServer),
예제 에이전트 `ratatosk-agent`까지:

```bash
helm install ratatosk-mcp ./ratatosk-mcp/charts/ratatosk-mcp \
  --namespace kagent --set kagent.enabled=true
```

`kagent.modelConfig` 기본값은 `default-model-config`입니다. 환경이 다르면
덮어쓰고, 예제 에이전트 없이 등록만 원하면 `kagent.agent.enabled=false`.

**B. 매니페스트** — 같은 세 조각을 헬름 없이 복붙으로
([상세](../examples/kagent/README.ko.md)):

```bash
BASE=https://raw.githubusercontent.com/garlicKim21/ratatosk-mcp/main/examples/kagent
kubectl apply -f $BASE/ratatosk-deploy.yaml
kubectl apply -f $BASE/ratatosk-remote-mcpserver.yaml
kubectl apply -f $BASE/ratatosk-agent.yaml
```

이 경로는 클론이 필요 없습니다. 매니페스트에는 `namespace: kagent`가 들어 있습니다.

어느 쪽이든 kagent UI에 `ratatosk-agent`가 나타납니다. 이렇게 물어보세요:

> "이 클러스터에서 업그레이드 전에 조치할 게 있나요?"

에이전트는 kagent 내장 읽기 전용 클러스터 도구로 구동 버전을 스스로
알아냅니다(`kagent.agent.k8sTools=false`로 끌 수 있음). 질문에 버전을
직접 적어도 됩니다.

> **모델 최소 요건**: 에이전트 런 1회가 내부 모델 호출 6회 이상을 만들고
> kagent Go ADK는 429 재시도를 하지 않으므로, 분당 약 10요청 미만의 모델
> 티어에서는 매 런이 실패합니다(실측: 5 RPM인 gemini 무료 티어는 단 한
> 런도 완주하지 못함). 무료 티어에서는 **flash-lite 계열을 권장**합니다 —
> 2026-07 기준 full-flash 계열 무료 티어는 전부 5 RPM이고, flash-lite
> 티어만 에이전트 구동이 가능한 한도를 제공합니다(60런 캠페인 실측).

> **kagent 0.9.12 자체의 알려진 문제**(이 차트와 무관): 에이전트 런타임을
> Go ADK로 바꾸면 ImagePullBackOff가 납니다. Go ADK 이미지는 `ghcr.io`에만
> 발행되는데 컨트롤러 기본값이 폐기된 `cr.kagent.dev`를 가리키기 때문입니다
> (kagent [#2247], 0.9.12 이후 수정). 우회: kagent 설치에
> `--set controller.agentImage.registry=ghcr.io`.

[#2247]: https://github.com/kagent-dev/kagent/issues/2247

## 설정

| 환경변수 | 기본값 | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | 업스트림 facts API (`/v1`, 공개, 읽기 전용) |
| `MCP_HTTP_ADDR` | *(없음)* | 설정 시(예: `:8080`) stdio 대신 스트리밍 HTTP로 서빙 |
| `MCP_HTTP_STATELESS` | *(꺼짐)* | `1`이면 세션 상태 없는 HTTP — `Mcp-Session-Id` 왕복이 없고, HTTP에서 2026-07-28 프로토콜 리비전을 말하는 유일한 모드 (차트: `statelessHttp`) |
| `MCP_LOG` | `info` | `debug`는 호출별 시간을 추가. 로그는 stderr에 JSON 한 줄이며 **어느 레벨에서도 요청 인자를 담지 않음** — 오류문은 복사가 아니라 재구성 (차트: `logLevel`) |
| `MCP_AUDIT` | *(꺼짐)* | `metadata`는 도구 호출마다 `event:"audit"` JSON 한 줄(도구·결과·호출자 clientInfo·인자 *이름*), `full`은 인자 값까지 (차트: `auditMode`) |

**감사 스트림을 있는 그대로 말하면**: 이 스트림은 당신의 클러스터 안에서
나와 당신의 콜렉터에 쌓입니다 — 데이터가 경계를 안 벗어나는 것이 핵심입니다.
보존 기간과 위변조 방지는 로그 플랫폼의 몫이며, `event=audit`으로 라우팅하면
됩니다. 정직한 경계 하나: 이 서버에는 인증이 없으므로 "호출자"란 자가 보고된
`clientInfo`와 전송 계층이 보여주는 것까지입니다. 기록이 증명하는 것은
*어느 클라이언트가 어떤 도구를 어떤 인자로 불렀는가*까지이고 — 어떤 사람이
에이전트에게 시켰는가는 에이전트 계층만 아는 지식이라 MCP 계층의 기록이
만들어낼 수 없습니다. 호출자가 보낸 `traceparent`는 기록에 `trace_id`로
찍혀 그 계층들을 잇는 조인 키가 됩니다.

## 컨테이너 이미지

멀티 아치(`linux/amd64`, `linux/arm64`), 릴리스 태그마다 자동 빌드:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 업스트림 API

이 서버는 공개 REST API의 얇은 클라이언트입니다. 직접 호출하고 싶다면
ratatosk.io의 `GET /v1`이 스스로를 설명합니다. API 키 없음, IP당 분당 60회
제한.
