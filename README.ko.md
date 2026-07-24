<div align="center">

<img src="docs/assets/ratatosk-hero.webp" width="280" alt="두루마리를 가방에 넣고 손을 흔드는 배달부 다람쥐, 라타토스크">

# ratatosk-mcp

**CNCF 릴리스 노트, 라타토스크가 매시간 대신 읽습니다.**

[English](README.md) · [한국어](README.ko.md) · [日本語](README.ja.md)

[![MCP Registry](https://img.shields.io/badge/MCP_Registry-listed-blue)](https://registry.modelcontextprotocol.io/?q=ratatosk&all=1)
[![Release](https://img.shields.io/github/v/release/garlicKim21/ratatosk-mcp)](https://github.com/garlicKim21/ratatosk-mcp/releases)
[![License](https://img.shields.io/github/license/garlicKim21/ratatosk-mcp)](LICENSE)


</div>

---

북유럽 신화의 라타토스크는 세계수를 오르내리며 소식을 나르는 다람쥐입니다.
여기서 나르는 것은 릴리스 소식입니다. [ratatosk.io](https://ratatosk.io)가
74개가 넘는 CNCF 프로젝트를 지켜보다가, 릴리스 노트를 보안 패치, 브레이킹
체인지, 기능 제거, 지원 중단, 기본값 변경 같은 사실(fact) 단위로 정리합니다.
단순 버그 수정과 홍보 문구는 걸러냅니다. 남는 것은 운영자가 조치할 내용뿐입니다.

이 MCP 서버는 그 사실들을 도구로 에이전트에게 건넵니다.

## 이런 식으로 씁니다

> **질문:** "envoy v1.36.8하고 istio 1.30.1을 쓰고 있는데, 업그레이드 전에 꼭 챙길 게 있을까?"
>
> **에이전트**가 `check_stack`을 호출해 사실만으로 답합니다. 지금 버전 이후에
> 고쳐진 CVE, 업그레이드 경로에서 사라지는 API, 달라진 기본값을 릴리스 노트
> 원문 인용과 함께 보여줍니다.

## 도구

| 도구 | 하는 일 |
|---|---|
| `list_facts` | 증분 사실 피드. `project`·`type`·`severity`로 거르고 `since` 커서로 이어받습니다 |
| `facts_by_entity` | 역인덱스. CVE, CRD, 피처 게이트, 플래그, 설정 필드, 의존성 등 식별자 하나를 건드린 사실을 전부 찾습니다 |
| `list_projects` | 추적 중인 프로젝트 전목록. 슬러그는 짐작하지 말고 먼저 여기서 확인합니다 |
| `get_release` | 검토된 릴리스 한 건의 커버리지·평가·원문 링크·사실 전체. `version`을 생략하면 그 프로젝트의 최신 검토 릴리스를 돌려줍니다. `facts: []`에 `coverage: full_reviewed`면 읽어봤지만 평이한 릴리스라는 뜻입니다. `include_raw`면 패치노트 원문(`raw_notes`)까지 — 분석이 불충분하거나 사실이 0건이면 자동 포함됩니다 |
| `check_stack` | 지금 쓰는 컴포넌트 버전을 주면 업그레이드 경로를 브리핑으로 돌려줍니다. critical/high는 전문, 나머지는 한 줄씩, 여러 브랜치에서 고쳐진 같은 이슈는 한 항목으로 접습니다. 전문 전체는 `detail: "full"`, 한 단계 업그레이드만은 `target_version`, 등급 필터는 `severity_min` |

## 버전은 밖으로 나가지 않습니다

`check_stack`이 서버에 보내는 것은 프로젝트 이름뿐입니다. 버전 비교는 이
프로세스 안에서 끝납니다. 무엇을 운영 중인지는 ratatosk.io에 닿지 않습니다.
서버는 사실을 내보내기만 하고, 무엇이 해당되는지는 에이전트가 판단합니다.
버전 정규화기(`internal/version`)를 함께 담아 범위 비교까지 클라이언트 쪽에서
처리합니다.

## 빠른 시작

```bash
claude mcp add ratatosk -- docker run -i --rm ghcr.io/garlickim21/ratatosk-mcp:latest
```

계정도 API 키도 필요 없습니다. 쿠버네티스에서 에이전트를 돌리거나 kagent를
쓴다면 아래 설치 가이드를 보세요.

## 문서

| | |
| --- | --- |
| **[설치와 사용법](docs/install.ko.md)** — 로컬 stdio · 클러스터(Helm) · kagent | [English](docs/install.en.md) · [日本語](docs/install.ja.md) |
| **[헬름 차트](charts/ratatosk-mcp/README.ko.md)** — values, kagent 토글 | |
| **[kagent 예제](examples/kagent/README.ko.md)** — 매니페스트 + ratatosk-agent | |
| **[기여 안내](CONTRIBUTING.md)** · **[보안 정책](SECURITY.md)** | |

## 업스트림 API

이 서버는 공개 REST API를 얇게 감싼 클라이언트입니다. 직접 부르고 싶다면
ratatosk.io의 `GET /v1`이 스스로를 설명합니다. API 키는 없고, IP당 분당
60회 제한만 있습니다.

## 데이터와 약관

데이터는 ratatosk.io가 [이용약관](https://ratatosk.io/terms)에 따라 무료로
제공합니다(변경될 수 있으며, 변경 시 사전 공지). 분석은 AI가 생성한 참고
정보이며 보증이 없습니다 — 조치 전에 원문 릴리즈 노트를 확인하세요. 에이전트가
대신 작업하는 경우에도 마찬가지입니다. 원문의 저작권은 각 프로젝트에 있으며,
원문 전체가 실리는 응답에는 출처 고지(`raw_notes_notice`)가 함께 담깁니다.

## 라이선스

이 저장소의 코드는 [Apache-2.0](LICENSE)로 배포됩니다.
