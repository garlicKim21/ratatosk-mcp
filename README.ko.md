<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="라타토스크 — 두루마리를 가방에 넣고 손을 흔드는 배달부 다람쥐">

# ratatosk-mcp

**라타토스크가 매시간 CNCF 릴리스 노트를 읽습니다. 당신의 에이전트가 읽을 필요 없도록.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

</div>

---

북유럽 신화에서 라타토스크는 세계수를 오르내리며 소식을 전하는 다람쥐입니다.
이 라타토스크가 나르는 것은 릴리스 인텔리전스입니다.
[ratatosk.io](https://ratatosk.io)는 74개 이상의 CNCF 프로젝트를 지켜보며 모든
릴리스 노트를 타입이 붙은 엔티티 단위 사실(facts)로 바꿉니다 — 보안 패치,
브레이킹 체인지, 기능 제거, 지원 중단, 기본값 변경. 단순 버그 픽스와 홍보성
문구는 걸러지고, 운영자가 실제로 조치할 것만 남습니다.

이 MCP 서버는 그 사실들을 4개의 도구로 에이전트에게 건네줍니다.

## 이런 경험입니다

> **당신:** "우리는 envoy v1.36.8과 istio 1.30.1을 돌리고 있어. 업그레이드 전에 꼭 해야 할 게 있을까?"
>
> **에이전트**가 `check_stack`을 호출해 사실에 근거해 답합니다: 지금 버전 이후에
> 수정된 CVE, 업그레이드 경로에서 제거되는 API, 바뀐 기본값 — 각각 릴리스
> 노트 원문 인용을 증거로 곁들여서.

## 도구

| 도구 | 하는 일 |
|---|---|
| `list_facts` | 증분 사실 피드 — `project`·`type`·`severity`로 필터, `since` 커서로 폴링 |
| `facts_by_entity` | 역인덱스: 하나의 식별자(CVE, CRD, 피처 게이트, 플래그, 설정 필드, 의존성)를 건드리는 모든 사실 |
| `get_release` | 검토된 릴리스 1건: 커버리지·평가·원문 링크·사실 전부. `facts: []` + `coverage: full_reviewed`는 "읽었는데 평이한 릴리스"라는 뜻 |
| `check_stack` | 돌리고 있는 컴포넌트 버전을 주면, 그보다 **새로운** 릴리스의 사실들 — 즉 업그레이드 경로를 돌려줍니다 |

## <img src="docs/assets/ratatosk-face.png" width="26" alt="" align="top"> 버전은 당신 곁을 떠나지 않습니다

`check_stack`은 **프로젝트 이름만** 서버에 보내 사실을 받아오고, 버전 비교는
이 프로세스 안에서 로컬로 합니다. 무엇을 돌리는지는 ratatosk.io에 절대 전달되지
않습니다 — 서버는 사실을 방송하고, 관련성 판단은 에이전트가 합니다. 버전
정규화기(`internal/version`)가 동봉되어 있어 범위 비교가 클라이언트 쪽에서
이루어집니다.

## 빠른 시작 (stdio)

```bash
go build -o ratatosk-mcp .

# Claude Code
claude mcp add ratatosk -- /path/to/ratatosk-mcp
```

빌드 없이 컨테이너 이미지로:

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 클러스터 안에서 (스트리밍 HTTP + Helm)

`MCP_HTTP_ADDR`를 설정하면 같은 바이너리가 `/mcp` 경로에서 스트리밍 HTTP로
동작합니다(`/healthz` 포함):

```bash
MCP_HTTP_ADDR=:8080 ./ratatosk-mcp
```

Helm 차트는 이를 ClusterIP 서비스로 배포합니다. 클러스터 안 에이전트(kagent,
커스텀 오퍼레이터)는 프로세스를 띄우는 대신 URL로 접속합니다:

```bash
helm install ratatosk-mcp ./charts/ratatosk-mcp
# → http://ratatosk-mcp.<namespace>.svc:8080/mcp
```

## 설정

| 환경 변수 | 기본값 | |
|---|---|---|
| `RATATOSK_URL` | `https://ratatosk.io` | 업스트림 사실 API (`/v1`, 공개, 읽기 전용) |
| `MCP_HTTP_ADDR` | *(비어 있음)* | 설정 시(예: `:8080`) stdio 대신 스트리밍 HTTP로 동작 |

## 컨테이너 이미지

멀티 아키텍처(`linux/amd64`, `linux/arm64`), 릴리스 태그마다 빌드:

```bash
docker run -i ghcr.io/garlickim21/ratatosk-mcp:latest            # stdio
docker run -p 8080:8080 -e MCP_HTTP_ADDR=:8080 ghcr.io/garlickim21/ratatosk-mcp:latest
```

## 업스트림 API

이 서버는 공개 REST API 위의 얇은 클라이언트입니다 — 직접 호출하고 싶다면
ratatosk.io의 `GET /v1`이 스스로를 설명합니다. API 키 없음, IP당 분당 60회 제한.

## 라이선스

[Apache-2.0](LICENSE)
