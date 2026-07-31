# 도구 레퍼런스

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

이 페이지는 ratatosk-mcp가 제공하는 도구
6개([`check_stack`](#check_stack), [`list_projects`](#list_projects),
[`list_releases`](#list_releases), [`get_release`](#get_release),
[`facts_by_entity`](#facts_by_entity), [`list_facts`](#list_facts))의 상세
레퍼런스입니다. 도구마다 어떤 상황에서 쓰는지, 파라미터는 무엇이 있는지,
실제 호출과 응답은 어떤 모습인지, 그리고 에이전트에게 던질 질문 예시까지
정리했습니다. 서버를 연결하는 방법(호스팅 엔드포인트, Docker, Helm,
kagent)은 [설치와 사용법](install.ko.md)을 보세요.

용어 둘만 먼저 정의합니다. **팩트(fact)**는 공식 릴리스 노트에서 뽑아낸
변경 하나입니다. 보안 수정, 기능 제거, 지원 중단, 기본값 변경 같은
것들이고, 팩트마다 그 변경이 건드리는 정확한 식별자(CVE id, 플래그, CRD,
설정 필드)와 근거가 되는 노트 원문 인용이 붙습니다. 모든 팩트에는
`info`부터 `critical`까지의 **심각도(severity)**가 있습니다.

> **기준 시점 2026-07-31**: 이 문서의 모든 호출과 응답은 2026-07-31에
> 호스팅 엔드포인트 `https://ratatosk.io/mcp`(서버 0.6.2)를 직접 호출해
> 받은 것입니다. `check_stack` 예시에 쓴 스택(컴포넌트 5종과
> `version_source` 문구)은 실제 클러스터에서 가져온 구성입니다(익명 처리).
> 릴리스 데이터는 매시간 쌓이므로 지금 호출하면 숫자와 목록이 달라질 수
> 있지만, 응답의 형태는 같습니다. 긴 응답은 분량을 줄이려고 일부를 `…`로
> 생략했습니다.

## 질문 하나가 도구 호출로 이어지는 과정 (실제 사례)

에이전트에게 이렇게 물었다고 합시다:

> "우리 클러스터의 상태 정보를 확인해주세요."

실제로 써 보고 잘 동작한 문구를 그대로 실었습니다. 클러스터를 직접 읽을
수 있는 에이전트, 예를 들어 kagent에 ratatosk-mcp와 클러스터 읽기 도구를
함께 붙인 에이전트를 실행했을 때 도구는 이런 순서로 호출됐습니다:

```
list_projects
k8s_get_resources  (pod, all_namespaces)
k8s_get_resources  (daemonset, all_namespaces)
k8s_get_resources  (deployment, all_namespaces)
k8s_get_resources  (node)
check_stack        (components ×5)
get_release        (cilium, v1.20.0)               ← action_required 드릴다운
k8s_get_resources  (customresourcedefinition)
k8s_get_resources  (ciliumnodeconfig, all_namespaces)
k8s_get_resources  (configmap, all_namespaces)
k8s_get_resource_yaml (kube-system/cilium-config)  ← applies_if 조건 확인
```

표기 주의: `k8s_*`는 kagent-tools(별도 MCP)의 클러스터 읽기 도구이고, 이
가운데 ratatosk 도구는 `list_projects`·`check_stack`·`get_release`입니다.

흐름을 풀면 세 단계입니다.

**1단계 — `list_projects`로 명단 확정.** 클러스터에서 본 이름들을 다른
도구가 받는 정식 슬러그로 바꾸고, `cluster_core:true`(클러스터 기반 계층 —
컨트롤 플레인, 런타임, DNS, CNI 등) 표시로 점검에 담을 후보를 고릅니다.

**2단계 — 버전을 실제 리소스에서 읽기.** `k8s_*` 도구로 파드·데몬셋·노드
상태에서 실행 중인 버전과 그 출처를 확보합니다. 그렇게 만들어진 것이 아래
check_stack 절의 예시 호출입니다 — 컴포넌트
5종(kubernetes/containerd/cilium/envoy/coredns)과 그 `version_source`
문구입니다.

**3단계 — `check_stack` 브리핑, 그리고 조건 확인.** 브리핑에서 cilium의
`action_required` 2건(v1.20.0의 docker libnetwork 플러그인 제거,
proxylib/Kafka 정책 제거)을 확인하고 `get_release(cilium, v1.20.0)`로 근거
원문까지 내려갑니다. `check_config`의 "v2alpha1 CiliumNodeConfig CRD를 쓰는
경우" 조건은 CRD 목록과 `kube-system/cilium-config`를 실제로 읽어야 적용
여부를 가릴 수 있습니다. "브리핑을 받고, 조건을 실제 리소스에서
확인한다"는 `check_stack`의 설계 의도가 그대로 실행 흐름으로 나타난
사례입니다.

무엇이 돌아왔는지는 아래 check_stack 절의 실제 응답과 다음 절 "브리핑 읽는
법"에서 그대로 볼 수 있습니다.

조건 판정을 더 확실히 받아 내고 싶다면, 역시 실제로 효과가 확인된 다음
문구처럼 판정 규칙을 질문에 포함하세요:

> "클러스터에서 실제 실행 중인 컴포넌트들의 버전을 컨트롤 플레인, 노드
> 런타임, 네트워킹(CNI와 그 부속), DNS까지 포함해서 실제 리소스에서 읽고,
> ratatosk 릴리스 정보와 대조해 지금 우리에게 적용되는 이슈가 있는지
> 점검해주세요. 조건부 이슈는 해당 설정을 실제로 읽어 확인한 경우에만
> 적용/미적용을 판정하고, 확인하지 못한 조건은 '확인 불가'로 분류해주세요."

실제로 겪은 안티패턴도 하나 알아두세요. 많은 항목을 빠짐없이 나열하라고
몰아붙이는 질문에는 반드시 판정 규칙("확인하지 못한 것은 '확인 불가'로")을
함께 주세요. 압박만 있으면 모델은 읽는 대신 그럴싸한 값으로 빈칸을
채웁니다.

## check_stack 브리핑 읽는 법

`check_stack`의 기본 응답(`detail:"brief"`)은 컴포넌트마다 같은 구조로
옵니다. 아래 check_stack 절의 실제 응답을 곁에 두고 읽으세요.

1. **`summary` — 요약 수치.** 실행 버전 이후의 팩트 수(`new_facts`),
   병합 후의 서로 다른 이슈 수(`distinct_issues`), 조건 없이 해당하는 팩트
   수(`mandatory`), 그리고 심각도별·유형별 분포. 스캔한 전체 팩트 수는
   `facts_scanned`입니다. 실제 응답의 envoy가 좋은 예입니다: 새 팩트
   75건이 병합을 거쳐 40건의 서로 다른 이슈로 줄었습니다. 덧붙여
   `distinct_issues`는 인용 문장 병합(아래 5번) 이전 기준이라, 세 목록의
   항목 수 합보다 클 수 있습니다.
2. **`action_required` — 이 버전 경로를 지나는 모든 설치에 해당하는
   critical/high.** 각 항목에는 릴리스 노트 원문
   인용(`quote`)과 CVE·어드바이저리 id(`ids`)가 붙습니다. 실제 응답에서는
   envoy에 5건(fact 211의 TLS SAN 인증 우회 critical 포함), cilium에
   2건(libnetwork·proxylib 제거)이 여기 담겼습니다.
3. **`check_config` — 조건을 확인해야 하는 critical/high.** `applies_if`
   조건이 성립할 때만 해당하는 항목입니다. 조건을 실제 설정에서 확인하기
   전에는 조치 대상이 아니고, 조건이 성립하지 않으면 업그레이드 사유도
   아닙니다 — 그때 `fixed_in`은 "나중에 그 기능을 켜려면 최소한 이 버전은
   되어야 한다"는 전제 조건으로 읽습니다. 서버가 조건의 대상을 구조적으로
   저장한 경우 `applies_if_target`(kind와 name)이 무엇을 찾아야 하는지
   지목합니다.
   실제 응답의 cilium fact 614가 바로 그런 경우입니다 — `applies_if` "uses the
   cilium.io/v2alpha1 CiliumNodeConfig CRD"에 `applies_if_target`
   `{ "kind": "crd", … }`가 붙어, 위 시나리오에서 에이전트가 CRD 목록을
   실제로 읽으러 간 이유가 됩니다. envoy 쪽에는 fact 215(critical, "if you
   proxy HTTP/3 to HTTP/1 backends") 등 11건이 여기 있습니다.
4. **`other_facts` — 나머지 전부, 한 줄씩.** medium 이하 항목이 팩트당 한
   항목으로 나타납니다. 실제 응답의 coredns는 5건 전부 medium 이하라 이
   목록에만 나타납니다.
5. **병합 규칙.** 같은 인용 문장을 공유하는 팩트들은 한 항목으로 병합되고
   id가 함께 나열됩니다(조건이 서로 다르면 `applies_if_any`) — 실제 응답의
   envoy fact 466이 그런 모양으로, CVE id 5개가 한 항목에 묶이고 서로 다른
   조건 3개가 `applies_if_any`에 나열되어 있습니다. 같은
   어드바이저리가 여러 릴리스 브랜치에서 수정된 경우도 한 항목입니다 —
   실제 응답의 envoy fact 211에 실제로 붙어 있는
   `same_issue_also_addressed_in: ["v1.37.5", "v1.38.3"]`이 그것입니다.
   이때 표시되는 심각도는 그 어드바이저리 그룹의 최대 심각도입니다.
6. **`note` — 컴포넌트별 상태 표시.** 실제 응답에 두 종류가 다 있습니다.
   kubernetes·containerd의 "tracked by ratatosk; no facts on record —
   releases so far were routine"은 추적 중인데 조용한 상태로, 추적하지
   않는 상태(`tracked:false` — 이때 팩트가 없다는 것이 안전하다는 뜻은
   아닙니다)와 구분됩니다.
   cilium·coredns의 "older than every release on record … treat it as
   partial, and re-check"는 실행 버전이 기록된 가장 오래된 릴리스보다도
   낮다는 경고입니다 — 서버는 사용자 환경을 볼 수 없으므로 사용자가 알려
   준 버전을 교차 확인할 수 있는 수단은 이 표시뿐이고, 버전을 실제
   리소스에서 다시 읽으라는 신호입니다.
7. **`hint`와 `privacy`.** `hint`는 두 가지를 알려 줍니다. 이 브리핑이
   권고가 아니라 데이터 분류라는 점, 그리고 다음에 쓸 도구(자세히 보려면
   `detail:"full"`, 드릴다운은 `get_release`·`facts_by_entity`)입니다.
   `privacy`는 이 호출에서 버전이 어디까지 갔는지를 명시합니다.

## check_stack

지금 실행 중인 컴포넌트 버전 목록을 받아, 각 컴포넌트의 업그레이드 경로,
즉 실행 중인 버전보다 새로운 릴리스에 담긴 팩트를 돌려줍니다. "업그레이드
전에 봐야 할 게 있나?"라는 질문의 첫 도구이고, 여러 프로젝트를 한 번의
호출에 담을 수 있어서 프로젝트마다 따로 호출하는 것보다 스택 단위 질문에
맞습니다. 버전 비교는 서버 프로세스 안에서 일어나므로 설치형으로 운영하면
실행 중인 버전이 인프라를 떠나지 않습니다. 특정 릴리스 하나를 깊이 보려면
`get_release`, 특정 CVE·플래그 하나를 추적하려면 `facts_by_entity`로
넘어가세요.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `components` | 예 | array | — | 점검할 실행 중 스택. 항목 형식은 아래 |
| `components[].project` | 예 | string | — | 프로젝트 슬러그(예: `envoy`). 정본은 `list_projects`의 `slug` |
| `components[].version` | 예 | string | — | 지금 실행 중인 버전(예: `v1.36.8`) |
| `components[].target_version` | 아니오 | string | *(없음 = 최신까지 전부)* | 업그레이드 목적지. 실행 버전보다 큰 값이어야 하며, `실행 버전 < 버전 ≤ target`인 팩트만 돌아옵니다. 실행 버전보다 낮거나 같은 값은 구간이 비어 무시되고 `note`로 안내됩니다 |
| `components[].version_source` | 아니오 | string | *(없음)* | 버전을 어디서 읽었는지(예: `daemonset/cilium image tag`, 사용자 진술). 응답에 그대로 되돌아오므로 나중에 그 값을 감사할 수 있습니다. 서버는 사용자 환경을 볼 수 없어 버전을 검증하지 못합니다 — "브리핑 읽는 법"에서 설명한 `note` 표시가 유일한 교차 확인입니다 |
| `detail` | 아니오 | string | `brief` | `brief`: 요약 + critical/high + 나머지 한 줄씩. `full`: 모든 팩트 원문 그대로 — 컴포넌트당 50건 상한이 있고 넘치면 `relevant_facts_omitted`로 표시되므로 `severity_min`이나 `target_version`으로 좁히세요 |
| `severity_min` | 아니오 | string | *(없음 = 전부)* | 이 심각도 이상만: `info`·`low`·`medium`·`high`·`critical` |

### 예시 호출

위 시나리오의 스택을 호스팅 엔드포인트로 그대로 보낸 호출입니다. 아래
JSON은 도구 인자(`arguments`)만 담은 것입니다 — 이를 감싸는 JSON-RPC 전송
형식은 [설치와 사용법](install.ko.md)에 있습니다:

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

### 실제 응답 (일부 생략)

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
        …1건 생략…
      ],
      "check_config": [
        …1건 생략…
        {
          "fact_id": 614, "version": "v1.20.0", "fact_type": "api_version_changed",
          "severity": "high", "mandatory": true,
          "applies_if": "uses the cilium.io/v2alpha1 CiliumNodeConfig CRD",
          "applies_if_target": { "kind": "crd", "name": "cilium.io/v2alpha1 CiliumNodeConfig" },
          "removed_in": "v1.20.0", "deprecated_in": "v1.16",
          "quote": "Remove deprecated `v2alpha1` `CiliumNodeConfig` API that was promoted to `v2` in cilium 1.16."
        }
      ],
      "other_facts": [ …20건 생략… ]
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
        …1건 생략…
        {
          "fact_id": 211, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true, "fixed_in": "v1.36.9",
          "quote": "Embedded NUL in TLS SAN Truncation, Auth Bypass",
          "ids": [ "CVE-2026-47778", "GHSA-f8x4-rw5x-f3r7" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2건 생략…
        {
          "fact_id": 347, "version": "v1.37.5", "fact_type": "security_fix",
          "severity": "high", "mandatory": true, "fixed_in": "v1.37.5",
          "quote": "CVE-2026-47220: REQUESTED_SERVER_NAME crash",
          "ids": [ "CVE-2026-47220", "GHSA-j9wh-4qfm-wf2v" ],
          "same_issue_also_addressed_in": [ "v1.38.3" ]
        }
      ],
      "check_config": [
        …8건 생략…
        {
          "fact_id": 215, "version": "v1.36.9", "fact_type": "security_fix",
          "severity": "critical", "mandatory": true,
          "applies_if": "if you proxy HTTP/3 to HTTP/1 backends",
          "fixed_in": "v1.36.9",
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "ids": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_issue_also_addressed_in": [ "v1.37.5", "v1.38.3" ]
        },
        …2건 생략…
      ],
      "other_facts": [
        …7건 생략…
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
        …6건 생략…
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
        …4건 생략…
      ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation …): action_required = critical/high that applies to every install of this version. check_config = critical/high that applies ONLY IF applies_if holds — …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

한 응답 안에 서로 다른 상태 세 가지가 함께 있는 것을 눈여겨보세요:

- **kubernetes·containerd** — 팩트 0건이지만 `note`에 "추적 중이고
  지금까지의 릴리스는 평이했다"고 적혀 있습니다. 추적하지 않는
  상태(`tracked:false`)와는 다른 상태입니다.
- **cilium·coredns** — 실행 중인 버전이 기록된 가장 오래된 릴리스보다도
  낮아 `note`가 검토 범위가 부분적임을 경고합니다 — 버전이 실제 리소스에서
  읽은 값이 맞는지 재확인하라는 신호입니다.
- **envoy** — 새 팩트 75건이 40건의 이슈로 병합됐습니다. 같은
  어드바이저리가 v1.36.9·v1.37.5·v1.38.3 세 브랜치에서 고쳐졌지만
  `same_issue_also_addressed_in`으로 병합돼 한 항목씩만 남았기 때문입니다.

### 질문 예시

> "우리는 envoy v1.36.8을 쓰는데, v1.37.0으로 올리기 전에 조치할 게 있어?"

> "우리 스택은 Kubernetes 1.31, Cilium 1.16, CoreDNS 1.11인데, 업그레이드
> 전에 봐야 할 게 있나?"

두 질문 모두 에이전트가 `check_stack` 한 번으로 답합니다. 첫 번째 질문은
`target_version`이 채워진 형태로, 두 번째 질문은 컴포넌트 세 개를 한
호출에 담은 형태로 답합니다. 클러스터를 직접 읽을 수 있는 에이전트라면 위
시나리오의 문구 2개처럼 버전 확보까지 통째로 맡길 수도 있습니다.

## list_projects

추적 중인 모든 프로젝트의 명단입니다. 인자가 없고 응답이 작으므로,
프로젝트 이름을 슬러그로 짐작하는 대신 먼저 이 도구를 부르는 것이
정석입니다 — 틀린 슬러그는 오류가 아니라 `check_stack`의 `tracked:false`로
나타나기 때문에, 짐작이 조용히 "커버리지 없음"으로 이어집니다. 다른 다섯
도구가 받는 `project` 인자의 정본이 이 응답의 `slug`입니다.

### 파라미터

없습니다. 빈 인자로 호출합니다.

### 예시 호출

```json
{}
```

### 실제 응답 (일부 생략)

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

기본 필드(`slug`·`name`·`tier`·`category`·`analyzed_releases`) 외에 세
가지 표식이 붙는 항목이 있습니다:

- `image_aliases` — 그 프로젝트가 클러스터에서 다른 이름으로 돌아갈 때
  쓰이는 이름입니다. 이 별칭과 일치하는 이미지나 워크로드는 태그에 적힌
  버전의 그 프로젝트로 보면 됩니다.
- `cluster_core:true` — 클러스터 기반 계층(컨트롤 플레인, 데이터스토어,
  DNS, 런타임, CNI/데이터플레인). 클러스터에 존재하는 `cluster_core`
  프로젝트는 모두 `check_stack` 호출에 포함할 후보입니다.
- `visibility` — 그 컴포넌트를 어떻게 관측하는지, 그리고 어떤 경우에는
  읽히지 않는 것이 정상인지 알려 주는 힌트(예: etcd는 k8s API 밖에 있을 수
  있습니다). 읽지 못한 컴포넌트는 버전을 짐작하는 대신 "확인 못 함"으로
  보고하라는 안내입니다.

### 질문 예시

> "우리 스택은 Kubernetes 1.31, Cilium 1.16, CoreDNS 1.11인데, 업그레이드
> 전에 봐야 할 게 있나?"

`check_stack`과 같은 질문입니다 — 위 시나리오에서 본 대로, 에이전트는
슬러그를 확정하기 위해 이 도구를 먼저 부르고 나서 `check_stack`으로
넘어갑니다.

## list_releases

프로젝트 하나의 최신 릴리스 N개를 한 줄 요약(버전, 날짜, 커버리지,
심각도별 팩트 수, 그리고 어드바이저리 그룹 최고 심각도 —
`max_group_severity`, 그룹에 속한 팩트가 없으면 `null`)으로, 최신순으로
돌려줍니다. "X에 최근 무슨 일이 있었나?"라는 질문 전용 도구입니다. 같은
팩트 데이터를 다루는 `list_facts`와는 정렬이 반대입니다. `list_facts`는
로컬 사본 동기화를 위해 분석이 오래된 순으로 흐르는 피드이고, 최신 소식을
물을 때는 이 도구를 쓰세요. 요약에서 눈에 걸리는 릴리스가 있으면
`get_release`로 파고듭니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 예 | string | — | 프로젝트 슬러그(예: `istio`) |
| `limit` | 아니오 | integer | `5` | 최근 릴리스 몇 건을 볼지. 최대 `20` |

### 예시 호출

```json
{ "project": "cilium", "limit": 3 }
```

### 실제 응답 (일부 생략)

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

`v1.18.12`처럼 `facts_total`이 0인데 `coverage`가 `full_reviewed`인 행은
"노트를 끝까지 읽었고 평이한 릴리스였다"는 뜻입니다 — 데이터가 없는 것과
구분됩니다.

### 질문 예시

> "Cilium에 최근 무슨 일이 있었어?"

에이전트가 `list_releases(project: "cilium")`를 부르고, 위 요약을 근거로
v1.20.0에 팩트가 몰려 있다고 답합니다.

## get_release

검토한 릴리스 한 건의 전부입니다. 겉면 정보(커버리지, 종합 평가, 원문
노트 링크)와 그 릴리스의 모든 팩트가 함께 옵니다. `check_stack`이나
`list_releases`에서 눈에 걸린 릴리스 하나를 근거까지 파고들 때, 그리고
팩트의 원문(`source_url`)을 확인해 판단을 검증할 때 쓰는 도구입니다.
`version`을 생략하면 그 프로젝트의 최신 검토 릴리스가 돌아오므로 "X 최신
릴리스 어때?"라는 질문에도 이 도구를 씁니다. 프라이버시 주의 하나:
`check_stack`과 달리 이 도구는 버전을 인자로 받고, 그 버전은 업스트림 요청
경로에 담겨 전달됩니다 — [README의 "실행 중인 컴포넌트 버전의 처리
방식"](../README.ko.md#실행-중인-컴포넌트-버전의-처리-방식)을 보세요.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 예 | string | — | 프로젝트 슬러그(예: `envoy`) |
| `version` | 아니오 | string | *(없음 = 최신 검토 릴리스)* | 발행된 그대로의 릴리스 태그(예: `v1.38.3`). `v` 접두사는 있어도 없어도 됩니다 — 프로젝트마다 표기가 다르기 때문입니다. 없는 태그를 주면 그 프로젝트의 최근 검토 태그 목록이 담긴 오류가 돌아오니, 그중 하나로 다시 부르면 됩니다 |
| `include_raw` | 아니오 | boolean | `false` | 원문 릴리스 노트 본문을 `raw_notes`로 함께 받습니다 — 추출된 팩트 대신 원문으로 직접 판단하고 싶을 때. 검토가 전부를 담지 못한 경우(커버리지 `insufficient`이거나 팩트 0건)에는 자동으로 포함됩니다 |

### 예시 호출

버전을 생략해 최신 검토 릴리스를 받는 호출입니다:

```json
{ "project": "istio" }
```

### 실제 응답 (일부 생략)

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

겉면 정보부터 읽습니다. `coverage: "full_reviewed"`는 노트를 전부 읽었다는
뜻이고, `assessment`는 릴리스 전체에 대한 한 문단 평가입니다. `facts`가
빈 배열인데 커버리지가 `full_reviewed`라면 읽었지만 평이했다는 뜻입니다.
중대한 결정은 `source_url`의 원문으로 검증하세요.

### 질문 예시

> "Istio 최신 릴리스, 검토 결과 어때?"

에이전트가 `get_release(project: "istio")`를 부르고 위 `assessment`를
근거로 요약해 답합니다.

## facts_by_entity

역조회 인덱스입니다. 정확한 식별자 하나(CVE id, CRD, 피처 게이트, 플래그,
메트릭, 설정 필드, 의존성)를 건드린 모든 팩트를 여러 프로젝트와 릴리스에
걸쳐 모아 줍니다. 대소문자는 구분하지 않습니다. 매니페스트나 보안
공지에서 식별자 하나를 손에 쥐고 "이것과 관련해 무슨 일이 있었지?"라고
물을 때 쓰는 도구입니다. 프로젝트에서 출발하는
`get_release`·`list_releases`와 방향이 반대입니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `name` | 예 | string | — | 조회할 정확한 식별자: CVE id, CRD, 피처 게이트, 플래그, 메트릭, 설정 필드, 의존성 |
| `kind` | 아니오 | string | *(없음 = 전체)* | 식별자 종류로 한정: `api`·`crd`·`feature_gate`·`flag`·`metric`·`config_field`·`extension`·`dependency`·`cve`·`advisory`·`subsystem` |

### 예시 호출

```json
{ "name": "CVE-2026-47778" }
```

### 실제 응답 (일부 생략)

8건이 돌아왔습니다 — envoy의 릴리스 브랜치 5개와, 같은 CVE를 자체 릴리스
노트에서 다룬 istio 3건:

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
    …3건 생략…
  ]
}
```

읽는 법 하나: 팩트마다 심각도 신호가 둘입니다. `severity`는 그 릴리스
노트에 적힌 심각도이고, `group_severity`는 같은 어드바이저리
그룹(`advisory_group_key`) 전체의 최대 심각도입니다. 시급성을 판단할 때는
`group_severity`를 보세요 — 위 istio 1.30.2 팩트는 `severity`가 `low`지만
그룹으로는 `high`입니다.

### 질문 예시

> "CVE-2026-47778, 어느 릴리스에서 고쳐졌어? 우리가 쓰는 envoy도 해당돼?"

에이전트가 `facts_by_entity(name: "CVE-2026-47778")`를 부르고, 브랜치별
`fixed_in`(envoy v1.35.13·v1.36.9·v1.37.5·v1.38.3·v1.39.0)을 근거로
답합니다. 보안 확인은 CVE id와 어드바이저리 id(GHSA) 양쪽으로 조회하세요 —
노트가 둘 중 하나만 인용한 팩트는 그 식별자로만 걸립니다.

## list_facts

팩트의 증분 동기화 피드입니다. `fact_id` 오름차순, 곧 분석이 오래된
것부터 흐르는 피드라서 첫 페이지에 오는 것은 최신 데이터가 아닙니다.
응답의 `next_since`를 다음 호출의 `since`로 넘겨 페이지를 이어가며, 로컬
사본을 최신으로 유지하기 위한 도구입니다. "X의 최신 릴리스"류 질문에는 이 도구가 아니라
`list_releases`나 `get_release`를 쓰세요. 프로젝트·유형·심각도 필터로
피드를 좁힐 수 있습니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 아니오 | string | *(없음 = 전체)* | 프로젝트 슬러그 필터(예: `envoy`) |
| `type` | 아니오 | string | *(없음 = 전체)* | 팩트 유형 필터: `security_fix`·`dependency_bump`·`capability_removed`·`capability_deprecated`·`api_version_changed`·`identifier_renamed`·`validation_tightened`·`default_changed`·`behavior_changed` |
| `severity` | 아니오 | string | *(없음 = 전체)* | 심각도 필터: `info`·`low`·`medium`·`high`·`critical` (정확히 그 심각도만) |
| `since` | 아니오 | integer | *(없음 = 처음부터)* | 커서: 이 값보다 큰 `fact_id`만 돌려받습니다. 이전 응답의 `next_since`를 넣으세요 |
| `limit` | 아니오 | integer | `50` | 페이지 크기. 최대 `200` |

### 예시 호출

```json
{ "type": "security_fix", "severity": "critical", "limit": 2 }
```

### 실제 응답 (일부 생략)

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

다음 페이지는 `{ "type": "security_fix", "severity": "critical", "since": 215 }`
입니다. `next_since`가 `null`로 돌아올 때까지 반복하세요 — `null`이면 더
받을 게 없다는 뜻입니다(`since=null`은 보내지 마세요 — `400`입니다).

### 질문 예시

> "기록된 critical 보안 수정을 훑어줘."

에이전트가 `list_facts(type: "security_fix", severity: "critical")`를
부르고, 위 피드를 페이지 단위로 읽어 정리합니다.

## 프라이버시와 레이트리밋

한 줄 요지: `check_stack`의 버전 비교는 서버 프로세스 안에서 일어나므로,
설치형으로 운영하면 실행 중인 버전이 인프라를 떠나지 않습니다. 경계 전체와
로그 취급은 [README의 "실행 중인 컴포넌트 버전의 처리
방식"](../README.ko.md#실행-중인-컴포넌트-버전의-처리-방식)을 보세요.
업스트림 공개 API의 한도는 IP당 분당 60요청이고, 호스팅 엔드포인트는 이
버킷을 다른 이용자와 나눠 씁니다. 프로젝트별 폴링 대신 `check_stack`
하나로 묶고, 사용량이 많다면 설치형으로 전환하세요.
