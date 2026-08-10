<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="두루마리를 가방에 넣고 손을 흔드는 배달부 다람쥐, 라타토스크">

# ratatosk-mcp

**CNCF 릴리스 노트, 라타토스크가 매시간 대신 읽습니다. 에이전트는 변경(change)을 받습니다.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)
[![Glama score](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp/badges/score.svg)](https://glama.ai/mcp/servers/garlicKim21/ratatosk-mcp)


</div>

---

북유럽 신화의 라타토스크는 세계수를 오르내리며 소식을 나르는 다람쥐입니다.
여기서 나르는 것은 릴리스 정보입니다. [ratatosk.io](https://ratatosk.io)가
CNCF 프로젝트 76개를 지켜보며 릴리스 노트 하나하나를 유형이 정해진
변경(change)으로 바꿉니다. 보안 수정, 호환성이 깨지는 변경(breaking change),
기능 제거, 지원 중단, 기본값 변경 같은 것들이고, 변경마다 영향을 받는
식별자와 **지금 어떻게 행동할 것인가**까지 짚습니다. 일상적인 줄도 기록하되
앞을 가리지 않게 접어 둡니다.

이 저장소는 그 변경을 도구로 에이전트에게 건네는 MCP 서버입니다.
MCP(Model Context Protocol)는 AI 에이전트가 외부 도구를 호출할 때 쓰는
공개 표준입니다. Claude Code, Claude Desktop, kagent, 직접 만든 SDK
에이전트처럼 MCP를 지원하는 클라이언트라면 무엇이든 연결할 수 있습니다.
계정도 API 키도 필요 없습니다.

## 쓰는 방법 두 가지

**호스팅 — 설치할 것이 없습니다.** 원격 커넥터를 지원하는 클라이언트에
`https://ratatosk.io/mcp`를 원격 MCP 서버로 등록하면 됩니다. 호스팅
엔드포인트는 호출자마다 분당 60회의 도구 호출을 허용합니다(다른 이용자와
나눠 쓰는 몫이 아니라 호출자별로 셉니다) — 폴링이나 CI 워크로드라면
설치형으로 쓰세요. Claude Code CLI 기준:

```bash
claude mcp add --transport http ratatosk https://ratatosk.io/mcp
```

**설치형 — 같은 서버를 내 프로세스로 돌립니다.** Docker, Helm 차트, 소스
빌드 가운데 어느 방식이든 됩니다. Claude Code와 Docker가 설치되어 있다면:

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:0.7.1
```

`0.7.1`이 현재 릴리스이고, 새 릴리스를 계속 따라가려면 `latest`를 쓰세요.

어느 쪽이든 연결을 확인하세요:

```bash
claude mcp list
# ratatosk: … - ✔ Connected
```

그리고 도구가 답할 수 있는 질문을 에이전트에게 던져 보세요:

> **질문:** "envoy v1.36.8하고 istio 1.30.1을 쓰는데, 업그레이드 전에 꼭 해야 할 게 있을까?"
>
> **에이전트**가 `check_stack`을 호출해 기록으로 답합니다. 지금 버전 이후에
> 고쳐진 CVE, 업그레이드 경로에서 사라지는 API, 달라진 기본값 같은
> 것들을 **모두에게 해당하는 것**과 **내 설정이 맞을 때만 해당하는 것**으로
> 갈라서 줍니다. 각 변경에는 릴리스 노트 원문 인용이 근거로 붙습니다.

다른 클라이언트(Claude Desktop, kagent, 클러스터 내 에이전트)와 전체 설정
레퍼런스는 [설치 가이드](docs/install.ko.md)를 보세요.

## 도구

도구가 쓰는 말 둘을 먼저 정의합니다. **변경(change)**은 릴리스가 한 일
하나입니다. 공식 릴리스 노트에서 뽑아내고, 그 변경이 건드리는 정확한
식별자(CVE id, 플래그, CRD, 설정 필드)와 근거가 되는 원문 인용이 붙습니다.
변경마다 축 세 개를 답니다.

- **family** — `security`·`breaking`·`deprecated`: 어떤 종류인가.
- **bucket** — `action`(모두에게 해당)·`check`(`applies_if`가 내 설정과 맞을
  때만)·`plan`(앞으로 예고됨)·`other`(전수 기록).
- **applies_if** — 읽어야 할 문장이 아니라, 내 매니페스트에 대고 평가할 수
  있는 불리언 식.

**사안(matter)**은 그 밑에 깔린 문제이고 `matter_key`로 식별합니다. 릴리스와
갈래를 가로질러 유지되므로, 같은 보안 롤업이 다섯 갈래에 착지해도 키는
하나입니다. 심각도는 인용된 권고에 붙고, 분석 시점에 굳은 값이 아니라 대장의
현재 값을 읽습니다.

| 도구 | 하는 일 |
|---|---|
| `check_stack` | 지금 실행 중인 컴포넌트 버전을 받아 업그레이드 경로의 변경을 층별로 갈라 돌려줍니다 — `action_required`는 모두에게 해당하고, `check_config`는 `applies_if`가 맞을 때만 해당합니다. 비교는 서버 프로세스(설치형이라면 내 프로세스) 안에서 일어납니다([실행 중인 컴포넌트 버전의 처리 방식](#실행-중인-컴포넌트-버전의-처리-방식)) |
| `list_changes` | 증분 변경 피드, 분석이 오래된 순. 프로젝트·가족·층으로 거르고 `since` 커서로 페이지를 넘겨 로컬 사본을 동기화합니다 |
| `changes_by_entity` | 역조회: 정확한 식별자 하나(CVE id, CRD, 피처 게이트, 플래그, 설정 필드, 의존성 등)를 건드린 변경 전부 |
| `get_matter` | 한 사안이 등장한 릴리스 전부. 같은 롤업이 갈래마다 다른 권고를 달고 착지하므로, 최신 하나만 보면 다 덮인 줄 알게 됩니다 |
| `get_release` | 릴리스 한 건 전체: 변경, 요약, 원문 노트 링크. 변경이 0건이면 읽어 봤지만 평이한 릴리스였다는 뜻입니다 |
| `list_releases` | 프로젝트 하나의 최신 릴리스들을 한 줄 요약(날짜, 층별·가족별 건수, 최고 권고 등급)으로 최신순 정렬 — "X에 최근 무슨 일이 있었나" 전용 도구 |
| `list_projects` | 추적 프로젝트 명단과 정식 슬러그(다른 모든 도구가 받는 짧은 프로젝트 id) — 이름은 짐작하지 말고 여기서 찾으세요 |

도구별 파라미터 전체와 예시 호출·실제 응답은 [도구 레퍼런스](docs/tools.ko.md)에 있습니다.

## 실행 중인 컴포넌트 버전의 처리 방식

**설치형:** `check_stack`이 서버로 보내는 것은 프로젝트 슬러그뿐이고,
버전 비교는 이 프로세스 안, 곧 로컬에서 끝냅니다. `check_stack`에 넘긴
버전은 ratatosk.io에 닿지 않습니다. 서버는 변경을 내보내고, 무엇이
해당하는지는 에이전트가 판단합니다. 버전 정규화기(`internal/version`)도
함께 들어 있어 범위 비교까지 클라이언트 쪽에서 끝납니다. 업그레이드
질문에도 똑같이 적용됩니다: 업스트림 API에는 호출자가 버전을 보내는 편의
엔드포인트(`/v1/upgrade/{project}`)가 있지만 `check_stack`은 이를 호출하지
않습니다 — 비교는 직접 읽을 수 있는 소스 코드 안에 있습니다.

다만 이 보증의 범위는 짚어 두겠습니다. 지금까지의 이야기는
`check_stack`에 한정된 보증입니다.
`get_release(project, version)`처럼 버전을 인자로 받는 도구는 그 버전을
업스트림 요청 경로에 담습니다 — 특정 릴리스를 가져오려면 지목해야 하기
때문입니다. 다만 그 지목된 경로도 서버 쪽 로그에는 남지 않습니다: 로그 한 줄이
쓰이기 전에 쿼리스트링이 제거되고 `/v1/releases/…`·`/v1/upgrade/…` 경로는
접두사만 남으므로, 슬러그도 버전도 로그에 실리지 않습니다.

**호스팅:** `check_stack` 인자(실행 중인 버전 목록)는 같은 답을 만들기
위해 서버 메모리를 지나가고, 어디에도 기록되지 않습니다. 경로의 각 계층이
남기는 것은 다음과 같습니다:

- 호스팅 MCP 프로세스 자체는 시작 줄 하나만 남깁니다 — 정상 요청은
  아무것도 추가하지 않습니다.
- 업스트림 API 요청 로그는 호출자가 `traceparent`를 보낼 때만 한 줄 쓰며,
  그 줄에 담기는 것은 정규화된 엔드포인트 라벨과 trace id뿐입니다 —
  경로도 쿼리도 본문도 담기지 않습니다.
- 앞단 액세스 로그는 쿼리스트링을 제거하고 `/v1/releases/…`·`/v1/upgrade/…`
  경로를 접두사만 남기며 호출자 IP를 마스킹하고, 요청 바디 항목 자체가
  없습니다.

호스팅 엔드포인트는 감사 스트림을 꺼 둔 채로 돌아가고, 앞으로도 켜지
않습니다 — 요청 내용을 기록하지 않는 것이 이 엔드포인트의 운영
원칙입니다. 통제 밖의 경계 하나: CDN 계층의 접속 메타데이터는
CDN 사업자 자체 정책을 따릅니다. 그 구간이 요건에 맞지 않으면 설치형을
쓰세요 — 그러면 `check_stack` 호출에서 인프라를 떠나는 것은 프로젝트
슬러그뿐입니다.

설치형에는 반대 방향의 선택지도 있습니다. 누가 어떤 도구를 불렀는지
기록하는 옵트인 감사 스트림(`MCP_AUDIT=metadata` 또는 `full`)이 사용자
인프라 안에서 만들어져 자체 콜렉터에 쌓입니다. 호스팅 엔드포인트에는
설계상 없습니다. 자세한 내용은 [설치 가이드](docs/install.ko.md)에
있습니다.

## 문서

- **[설치와 사용법](docs/install.ko.md)** — 호스팅 엔드포인트 · 로컬 stdio · 클러스터(Helm) · kagent ([English](docs/install.en.md) · [日本語](docs/install.ja.md))
- **[Helm 차트](charts/ratatosk-mcp/README.ko.md)** — values, kagent 토글 ([English](charts/ratatosk-mcp/README.md) · [日本語](charts/ratatosk-mcp/README.ja.md))
- **[kagent 예제](examples/kagent/README.ko.md)** — 매니페스트 + ratatosk-agent ([English](examples/kagent/README.md) · [日本語](examples/kagent/README.ja.md))
- **[기여 안내](CONTRIBUTING.md)** · **[보안 정책](SECURITY.md)**

## 업스트림 API

이 서버는 공개 REST API를 얇게 감싼 클라이언트입니다. 직접 호출하고
싶다면 ratatosk.io의 `GET /v1`을 열어 보세요. 인덱스가 스스로를
설명합니다. API 키는 없고, IP당 분당 1200회 제한이 있습니다.

## 데이터와 약관

데이터는 ratatosk.io가 [이용약관](https://ratatosk.io/terms)에 따라 무료로
제공합니다(변경될 수 있으며, 변경 시 사전 공지). 분석은 AI가 생성한 참고
정보이며 보증이 없습니다 — 조치 전에 원문 릴리스 노트를 확인하세요.
에이전트가 대신 작업하는 경우라면 더욱 그렇습니다. 원문의 저작권은 각
프로젝트에 있으며, 원문 전체가 실리는 응답에는 출처 고지
(`raw_notes_notice`)가 함께 담깁니다.

## 라이선스

이 저장소의 코드는 [Apache-2.0](LICENSE)로 배포됩니다.
