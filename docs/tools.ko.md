# 도구 레퍼런스

[English](tools.en.md) · [한국어](tools.ko.md) · [日本語](tools.ja.md)

이 페이지는 ratatosk-mcp가 제공하는 도구
7개([`check_stack`](#check_stack), [`list_projects`](#list_projects),
[`list_releases`](#list_releases), [`get_release`](#get_release),
[`changes_by_entity`](#changes_by_entity), [`get_matter`](#get_matter),
[`list_changes`](#list_changes))의 상세 레퍼런스입니다. 도구마다 어떤
상황에서 쓰는지, 파라미터는 무엇이 있는지, 실제 호출과 응답은 어떤
모습인지, 그리고 에이전트에게 던질 질문 예시까지 정리했습니다. 서버를
연결하는 방법(호스팅 엔드포인트, Docker, Helm, kagent)은
[설치와 사용법](install.ko.md)을 보세요.

> **기준 시점 2026-08-10**: 이 문서의 모든 호출과 응답은 이 저장소의
> 서버를 직접 빌드해 공개 API `https://ratatosk.io`에 붙여 받은 것을
> 그대로 옮긴 것입니다. `check_stack` 예시에 쓴 스택(컴포넌트 5종과
> `version_source` 문구)은 실제 클러스터에서 가져온 구성입니다(익명 처리).
> 릴리스 데이터는 매시간 쌓이므로 지금 호출하면 숫자와 목록이 달라질 수
> 있지만, 응답의 형태는 같습니다. 긴 응답은 분량을 줄이려고 일부를 `…`로
> 생략했습니다.

## 변경 모델 한 장 요약

여기 있는 도구는 전부 같은 단위를 돌려줍니다. **변경(change)** 하나는 한
릴리스에서 일어난 일 하나입니다. CVE가 딸린 수정, 없어진 플래그, 이름이
바뀐 설정 필드, 뒤집힌 기본값 — 그리고 그것을 읽어낸 릴리스 노트의 원문
문장이 늘 함께 붙습니다.

모든 변경은 세 축으로 설명되고, 세 축은 서로 독립입니다.

| 축 | 값 | 답하는 질문 |
|---|---|---|
| `family` | `security` · `breaking` · `deprecated` | 이건 어떤 종류의 일인가? |
| `bucket` | `action` · `check` · `plan` | *지금* 무엇을 해야 하는가? |
| `applies_if` | 조건. 가능한 경우 구조화됨 | 애초에 내가 챙길 일인가? |

**심각도보다 `bucket`을 먼저 보세요.** `action` 항목은 그 버전대를 지나는
모든 설치에 해당합니다. `check` 항목은 `applies_if`가 참인 곳에만
해당하므로, 할 일이 아니라 실제 설정을 보고 판정해야 할 질문입니다. `plan`
항목은 앞으로의 릴리스를 향한 예고입니다. 웹사이트도, 주간 메일도,
`check_stack`도 모두 이 같은 필드로 갈라내기 때문에 세 표면이 우연이 아니라
구조적으로 일치합니다.

나머지 네 필드가 대부분의 무게를 집니다.

- **`matter_key`** — 릴리스를 가로지르는 사안의 정체성. 같은 권고를 다섯
  갈래에서 고쳤다면 `matter_key`는 하나, 변경은 다섯 건입니다.
  [`get_matter`](#get_matter)가 이걸 펼칩니다.
- **`applies_if.clauses`** — 조건을 구조화할 수 있었을 때, 절마다
  `kind`(`api`, `crd`, `feature_gate`, `flag`, `config_field`, `extension`,
  `subsystem`, `dependency` 등)와 `name`, `verb`, `polarity`가 붙고
  `mode`(`all_of`, `any_of`, `universal`)로 묶입니다. 문장을 해석하지 말고
  이 이름들을 실제 설정에서 찾아보세요.
- **`advisories`** — 인용된 CVE·GHSA id와 그 **현재** 심각도. 릴리스 노트가
  당시 주장한 심각도와 다를 수 있습니다.
- **`quote`** — 근거가 된 원문 문장. 판단은 언제든 `source_url`에 대고
  확인할 수 있어야 하니까요.

릴리스의 `changes`가 `[]`라면 노트를 읽었고 운영자가 챙길 것이 없었다는
뜻입니다. 빈칸이 아니라 **감사 가능한 침묵**입니다. `notes_total`은 하나씩
드러내지 않기로 한 일상 기록(봇 의존성 범프 같은 것)의 수입니다.

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
get_release        (cilium, v1.20.0)               ← action_required 파고들기
k8s_get_resources  (customresourcedefinition)
k8s_get_resources  (ciliumnodeconfig, all_namespaces)
k8s_get_resources  (configmap, all_namespaces)
k8s_get_resource_yaml (kube-system/cilium-config)  ← applies_if 판정
```

이름에 대해 한마디: `k8s_*`는 kagent-tools(별개의 MCP 서버)가 제공하는
클러스터 읽기 도구입니다. 이 순서에서 ratatosk 도구는 `list_projects`,
`check_stack`, `get_release` 셋입니다.

이 경로는 세 단계로 나뉩니다.

**1단계 — `list_projects`로 명단을 확정한다.** 클러스터에서 본 이름을 다른
도구들이 받는 정식 슬러그로 옮기는 단계이고, 이름이 다른 경우는
`image_aliases`가 풀어 줍니다. 클러스터의 `cilium-envoy` 데몬셋은 `envoy`
프로젝트입니다.

**2단계 — 스택 전체를 `check_stack` 한 번에.** 다섯 번 나눠 묻는 대신
컴포넌트 다섯을 한 번에 보내고, 각각에 그 버전을 어디서 읽었는지를
`version_source`로 붙입니다.

**3단계 — 판정하고, 파고든다.** 브리핑은 모두에게 해당하는 것과 설정에
달린 것으로 갈라지고, 에이전트는 양쪽을 다 처리했습니다. `action_required`
항목의 릴리스는 `get_release`로 들여다보고, 브리핑만으로 답할 수 없는
`applies_if`는 `cilium-config`를 읽어 판정했습니다.

중요한 건 답의 모양입니다. 에이전트는 "업데이트가 있습니다"라고 하지
않았습니다. 무조건 해당하는 항목이 무엇이고, 실제 설정에 대고 확인한 것이
무엇이며, 확인할 수 없었던 것이 무엇인지를 나눠서 보고했습니다.

## check_stack 브리핑 읽는 법

기본 `check_stack` 응답(`detail:"brief"`)은 컴포넌트마다 같은 구조입니다.
아래 [check_stack 절](#check_stack)의 실측 응답을 옆에 두고 읽으세요.

1. **`changes_scanned`와 `summary` — 숫자.** `changes_scanned`는 그
   프로젝트에 기록된 변경 총수, `summary.new_changes`는 그중 실행 중인
   버전보다 위에 있는 것의 수입니다. `distinct_matters`는 같은
   `matter_key`의 반복을 합친 뒤 남는 수입니다. 실측 응답의 containerd가
   그 예로, 35건을 훑어 11건이 새것이고 사안으로는 9건입니다.
   `by_severity`·`by_family`·`by_bucket`은 모두 원본 변경이 아니라 합친 뒤의
   사안을 셉니다.
2. **세 목록은 오직 `bucket`으로 갈립니다** — `action`은
   `action_required`로, `check`는 `check_config`로, `plan`은
   `other_changes`로 갑니다. 심각도로 가르는 것이 **아닙니다**. 실측
   응답에서 containerd의 `check_config`에는 `low`가 다섯 개 있고, coredns의
   `action_required`에는 `medium`이 하나 있습니다. 심각도는 목록 **안에서**
   급함을 매기고, 목록을 정하는 건 버킷입니다.
3. **`action_required` — 설정과 무관하게 챙길 것.** containerd의 첫 항목이
   권고 id 10개를 안고 있는 보안 롤업이고, 그중 최악이 `critical`이라
   `critical`입니다. 여기 있는 항목은 대개 `applies_if`가 아예 없습니다.
   몇몇은 조건이 붙어 있는데, 그 프로젝트에선 사실상 보편적인 조건이기
   때문입니다(예: "runs http2"가 붙은 envoy 항목).
4. **`check_config` — 권고하기 전에 조건부터 판정할 것.** 여기 있는 항목은
   전부 `applies_if`를 갖고 있습니다. 실제 설정에 대고 확인하기 전까지 이건
   할 일이 아니고, 조건이 맞지 않는다는 사실은 업그레이드 이유가 되지
   않습니다. 대신 앞으로 읽으세요. 그 항목의 `version`은 `applies_if`가
   말하는 것을 **켜기 전에** 도달해 있어야 할 최소 버전입니다.
   `applies_if_targets`는 실제 설정에서 찾아볼 이름을 나열합니다 —
   containerd의 `["CreateContainer", "sandbox"]`, cilium의 `["ipBlock"]`처럼요.
   `applies_if`가 문장뿐일 때는 이 필드가 없습니다.
5. **`other_changes` — `plan` 버킷, 한 줄씩.** 앞으로의 릴리스를 향한 지원
   중단 예고입니다. containerd의 두 항목은 둘 다 `window.deprecated_in`을
   갖고 있고, 그게 시계가 돌기 시작한 시점입니다.
6. **여기서의 `severity`는 ratatosk가 아니라 이 서버가 계산합니다.** 항목의
   `advisories` 중 가장 높은 심각도이고, 권고가 없으면 `security` 가족은
   `high`, 그 외에는 버킷이 정합니다 — `action`은 `medium`, `check`는 `low`,
   `plan`은 `info`. cilium의 `[security]` 태그 붙은 의존성 범프가
   `advisories` 없이 `high`로 보이는 이유가 이것입니다. 권고 자체로
   판단하려면 [`get_release`](#get_release)나
   [`changes_by_entity`](#changes_by_entity)를 쓰세요. 그쪽은 권고를 각자의
   심각도와 함께 돌려줍니다.
7. **병합 규칙 둘, 실측 응답에 다 보입니다.** 갈래를 가로질러 `matter_key`가
   같은 변경들은 가장 이른 수정으로 접히고 나머지는
   `same_matter_also_addressed_in`에 실립니다. 운영자는 한 번 올리므로 그
   사안을 닫는 가장 가까운 릴리스가 실행 가능한 답이기 때문입니다. 이와
   별개로, **한 릴리스 안에서** 인용문이 같은 변경들은 id를 함께 묶어 한
   항목으로 합쳐지고, 조건이 서로 다르면 `applies_if` 대신
   `applies_if_any`가 붙습니다. 합쳐진 항목은 구성원 중 가장 높은 심각도와
   가장 급한 버킷을 따릅니다.
8. **`note` — 컴포넌트별 상태 표식.** 두 종류가 나옵니다. `tracked:false`와
   함께 붙는 "NOT tracked by ratatosk — zero changes means no coverage here,
   not safety", 그리고 "running version … is older than every release on
   record". 뒤엣것은 버전 주장에 대해 여기서 가능한 유일한 교차 확인입니다.
   이 서버는 당신의 환경을 볼 수 없으므로, 기록 전체보다 오래된 버전은 살아
   있는 리소스에서 버전을 다시 읽어 보라는 신호입니다. 두 표식이 다 없고
   `new_changes: 0`이면 조용한 경우입니다. 추적 중이고, 훑었고, 실행 중인
   버전 위에 아무것도 없다는 뜻이죠. 실측 응답의 kubernetes가 그렇습니다 —
   45건을 훑고 새것 0건.
9. **`hint`와 `privacy`.** `hint`는 이 브리핑이 권고가 아니라 데이터 분류라는
   점을 다시 말하고 다음에 쓸 도구를 가리킵니다. `privacy` 줄은 이번 호출에서
   당신의 버전이 어디까지 갔는지를 기록합니다.

## check_stack

실행 중인 컴포넌트 버전 목록을 받아, 컴포넌트마다 업그레이드 경로 위의
변경 — 실행 중인 버전보다 새 릴리스들 — 을 돌려줍니다. "올리기 전에 챙길
게 있나?"가 질문이라면 가장 먼저 잡을 도구입니다. 한 번의 호출이 여러
프로젝트를 덮으므로, 스택 전체를 묻는 질문에는 다른 도구를 프로젝트마다
부르는 것보다 이쪽이 낫습니다.

버전 비교는 **이 서버 프로세스 안에서** 일어납니다. 상류로 나가는 것은
프로젝트 슬러그뿐이고, 이 도구는 서버 쪽 `/v1/upgrade` 엔드포인트를 절대
부르지 않습니다. 서버를 직접 돌리면 실행 중인 버전은 당신의 인프라를 벗어나지
않고, 호스팅 엔드포인트에서는 서버 메모리만 거치며 기록되지 않습니다.

한 릴리스를 깊이 보려면 `get_release`로, CVE나 플래그 하나를 좇으려면
`changes_by_entity`로, 한 사안을 고친 모든 갈래를 보려면 `get_matter`로
넘어가세요.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `components` | 예 | array | — | 확인할 실행 스택. 항목 모양은 아래 |
| `components[].project` | 예 | string | — | 프로젝트 슬러그(예: `envoy`). 정본은 `list_projects`의 `slug` |
| `components[].version` | 예 | string | — | 지금 실행 중인 버전(예: `v1.36.8`) |
| `components[].target_version` | 아니오 | string | *(없음 = 최신까지 전부)* | 업그레이드 목적지. 실행 버전보다 엄격히 위여야 하며 `running < version <= target`인 변경만 돌아옵니다. 실행 버전 이하를 주면 구간이 비므로 무시되고 `note`가 붙습니다 |
| `components[].version_source` | 아니오 | string | *(없음)* | 그 버전을 어디서 읽었는지(예: `daemonset/cilium image tag`, 또는 사용자가 말해 줬다는 사실). 나중에 감사할 수 있도록 그대로 되돌려줍니다. 서버는 당신의 환경을 볼 수 없어 버전을 검증하지 못합니다 — 위에서 설명한 `note` 표식이 여기서 가능한 유일한 교차 확인입니다 |
| `detail` | 아니오 | string | `brief` | `brief`: `summary`와 버킷별 세 목록(병합·한 줄씩). `full`: 해당하는 변경 전문을 `relevant_changes`에(병합 없음, 요약 없음) — 컴포넌트당 50건에서 자르고 잘린 수는 `relevant_changes_omitted`에 |
| `severity_min` | 아니오 | string | *(없음 = 전부)* | 이 심각도 이상만: `info`, `low`, `medium`, `high`, `critical` |

`brief`에서 `other_changes` 꼬리는 컴포넌트당 100건에서 자르고 잘린 수는
`other_changes_omitted`에 실립니다. 조용히 버리는 일은 없습니다.

### 예시 호출

위 시나리오에 나온 스택입니다. 아래 JSON은 도구의 `arguments` 객체만이고,
이게 실려 가는 JSON-RPC 봉투는 [설치와 사용법](install.ko.md)에서 다룹니다.

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

### 실측 응답 (일부 생략)

```json
{
  "components": [
    {
      "project": "kubernetes",
      "running_version": "v1.36.1",
      "version_source": "kubectl version (server)",
      "changes_scanned": 45,
      "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
    },
    {
      "project": "containerd",
      "running_version": "2.2.4",
      "version_source": "node status containerRuntimeVersion",
      "changes_scanned": 35,
      "summary": {
        "new_changes": 11, "distinct_matters": 9,
        "by_severity": { "critical": 1, "high": 1, "low": 5, "info": 2 },
        "by_family": { "breaking": 5, "deprecated": 2, "security": 2 },
        "by_bucket": { "action": 2, "check": 5, "plan": 2 }
      },
      "action_required": [
        {
          "change_id": "containerd:v2.2.5:a74c2b48",
          "matter_key": "containerd/advisory:cve-2026-47262",
          "version": "v2.2.5",
          "kind": "defect_corrected",
          "family": "security",
          "bucket": "action",
          "severity": "critical",
          "quote": "CVE-2026-50195",
          "advisories": [
            "CVE-2026-47262", "CVE-2026-50195", "CVE-2026-53488", "CVE-2026-53489",
            "CVE-2026-53492", "GHSA-33vj-92qq-66hc", "GHSA-cvxm-645q-p574",
            "GHSA-jpcc-p29g-p8mq", "GHSA-rgh6-rfwx-v388", "GHSA-xhf5-7wjv-pqxp"
          ],
          "same_matter_also_addressed_in": [ "v2.3.2" ]
        },
        { "change_id": "containerd:v2.3.1:040369eb", "version": "v2.3.1", "severity": "high",
          "family": "security", "bucket": "action", "quote": "* [**CVE-2026-46680**]",
          "advisories": [ "CVE-2026-46680", "GHSA-fqw6-gf59-qr4w" ] }
      ],
      "check_config": [
        {
          "change_id": "containerd:v2.2.6:b8305908",
          "matter_key": "containerd/api/createcontainer#constraint_changed",
          "version": "v2.2.6",
          "kind": "constraint_changed",
          "family": "breaking",
          "bucket": "check",
          "severity": "low",
          "applies_if": "uses the CreateContainer API and runs sandbox",
          "applies_if_targets": [ "CreateContainer", "sandbox" ],
          "quote": "* cri: reject CreateContainer when sandbox is not running (#13669)",
          "same_matter_also_addressed_in": [ "v2.3.3" ]
        },
        { "change_id": "containerd:v2.3.1:4dff4eb6", "version": "v2.3.1", "severity": "low",
          "kind": "removed", "family": "breaking", "bucket": "check",
          "applies_if": "runs user namespace", "applies_if_targets": [ "user namespace" ],
          "window": { "removed_in": "v2.3.1" },
          "quote": "* Disable overlayfs \"rebase\" capability when running in user namespace" },
        …3건 생략…
      ],
      "other_changes": [
        { "change_id": "containerd:v2.3.0:a4f90153",
          "matter_key": "containerd/api/shim.command#deprecated",
          "version": "v2.3.0", "kind": "deprecated", "family": "deprecated",
          "bucket": "plan", "severity": "info",
          "applies_if": "uses the shim.Command API",
          "applies_if_targets": [ "shim.Command" ],
          "window": { "deprecated_in": "2.3" },
          "quote": "* Deprecate shim.Command" },
        …1건 생략…
      ]
    },
    {
      "project": "cilium",
      "running_version": "v1.19.4",
      "version_source": "daemonset/cilium image tag",
      "changes_scanned": 122,
      "summary": {
        "new_changes": 51, "distinct_matters": 48,
        "by_severity": { "high": 7, "medium": 3, "low": 32, "info": 6 },
        "by_family": { "breaking": 35, "security": 7, "deprecated": 6 },
        "by_bucket": { "action": 8, "check": 34, "plan": 6 }
      },
      "action_required": [
        { "change_id": "cilium:v1.20.0:20adda37",
          "matter_key": "cilium/config_field/cni configuration version#default_changed",
          "version": "v1.20.0", "kind": "default_changed", "family": "breaking",
          "bucket": "action", "severity": "medium",
          "window": { "introduced_in": "v1.20.0" },
          "quote": "the default CNI configuration version moves from 0.3.1 to 1.0.0." },
        { "change_id": "cilium:v1.20.0:4bb6c02d", "version": "v1.20.0", "severity": "high",
          "family": "security", "bucket": "action",
          "quote": "fix(deps): update module google.golang.org/grpc to v1.82.1 [security] (v1.20)" },
        …6건 생략…
      ],
      "check_config": [
        { "change_id": "cilium:v1.19.5:41a4ef5b",
          "matter_key": "cilium/config_field/l2podannouncements.interface#renamed",
          "version": "v1.19.5", "kind": "renamed", "family": "breaking",
          "bucket": "check", "severity": "low",
          "applies_if": "enables L2 pod announcements",
          "applies_if_targets": [ "L2 pod announcements" ],
          "window": { "removed_in": "v1.19.5" },
          "quote": "Remove defunct `l2podAnnouncements.interface` Helm value that rendered a configmap key the agent no longer recognises, causing crash-loops when L2 pod announcements were enabled. Users must use `l2podAnnouncements.interfacePattern` instead." },
        { "change_id": "cilium:v1.19.5:9caac3a9",
          "matter_key": "cilium/api/ipblock#defect_corrected",
          "version": "v1.19.5", "kind": "defect_corrected", "family": "security",
          "bucket": "check", "severity": "high",
          "applies_if": "configures the ipBlock API",
          "applies_if_targets": [ "ipBlock" ],
          "quote": "Fix wildcard namespace bypass for selectorless ipBlock rules" },
        …32건 생략…
      ],
      "other_changes": [ …6건… ]
    },
    {
      "project": "envoy",
      "running_version": "v1.36.7",
      "version_source": "daemonset/cilium-envoy image tag",
      "changes_scanned": 124,
      "summary": {
        "new_changes": 90, "distinct_matters": 45,
        "by_severity": { "high": 12, "medium": 15, "low": 15, "info": 3 },
        "by_family": { "security": 26, "breaking": 16, "deprecated": 3 },
        "by_bucket": { "action": 4, "check": 38, "plan": 3 }
      },
      "action_required": [
        { "change_id": "envoy:v1.36.9:d69a1e5f",
          "matter_key": "envoy/advisory:cve-2026-47261",
          "version": "v1.36.9", "family": "security", "bucket": "action", "severity": "high",
          "quote": "wasm: bumped `com_github_wasmtime` to resolve CVE-2026-47261.",
          "advisories": [ "CVE-2026-47261" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …3건 생략…
      ],
      "check_config": [
        { "change_id": "envoy:v1.36.9:7ccbb3ff",
          "matter_key": "envoy/advisory:cve-2026-48743",
          "version": "v1.36.9", "family": "security", "bucket": "check", "severity": "high",
          "applies_if": "uses HTTP/3 and uses HTTP/1 and uses headers-only request and configures the Content-Length setting",
          "applies_if_targets": [ "HTTP/3", "HTTP/1", "headers-only request", "Content-Length" ],
          "quote": "HTTP/3 to HTTP/1 request smuggling via headers-only request with nonzero Content-Length",
          "advisories": [ "CVE-2026-48743", "GHSA-8phg-2h2q-jgxf" ],
          "same_matter_also_addressed_in": [ "v1.37.5", "v1.38.3" ] },
        …37건 생략…
      ],
      "other_changes": [ …3건… ]
    },
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "summary": {
        "new_changes": 8, "distinct_matters": 8,
        "by_severity": { "critical": 1, "high": 3, "medium": 1, "low": 3 },
        "by_family": { "breaking": 4, "security": 4 },
        "by_bucket": { "action": 3, "check": 5 }
      },
      "action_required": [ …3건… ],
      "check_config": [ …5건… ]
    }
  ],
  "hint": "briefing (a data classification, not a recommendation — changes are provided without warranty and the decision stays with the operator): …",
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

이 응답 하나에 네 가지 상태가 함께 있습니다.

- **kubernetes** — 기록된 변경은 45건인데 v1.36.1보다 위에 있는 것은
  없습니다. 추적 중이고, 훑었고, 조용합니다. 경고할 게 없으니 `note`도
  없습니다.
- **containerd** — 아담한 경우입니다. 새 변경 11건이 사안 9건이 되고 버킷별로
  2 / 5 / 2로 갈립니다. `action_required` 첫 항목이 롤업 하나에서 나온 권고
  id 10개를 안고 있습니다.
- **cilium과 envoy** — 붐비는 경우이자, 버킷으로 가르는 이유입니다. envoy는
  새 변경이 90건인데 `action_required`에 떨어지는 건 4건이고, 38건은 할
  일이 아니라 설정에 대한 질문입니다.
- **coredns** — 나머지 컴포넌트가 조용한 스택에 `critical` 하나가 앉아
  있습니다. 스택 전체를 한 번에 묻는 이유가 바로 이런 것입니다.

추적하지 않는 슬러그는 또 다르게 보입니다. ratatosk가 다루지 않는
`nginx-ingress`를 물으면:

```json
{
  "project": "nginx-ingress",
  "running_version": "v1.14.0",
  "version_source": "user-stated",
  "tracked": false,
  "note": "NOT tracked by ratatosk — zero changes means no coverage here, not safety",
  "changes_scanned": 0,
  "summary": { "new_changes": 0, "distinct_matters": 0, "by_severity": {}, "by_family": {}, "by_bucket": {} }
}
```

슬러그를 추측하면 안 되는 이유가 이것입니다. 결과가 "발견 없음"으로
돌아오는데, 그게 안전처럼 읽히니까요. 확실하지 않으면 `list_projects`를
부르세요.

기록 전체보다 낮은 버전을 실행 중이면 커버리지 표식이 붙습니다:

```json
{
  "project": "coredns",
  "running_version": "v1.9.0",
  "note": "running version v1.9.0 is older than every release on record (earliest on record: v1.14.0) — this covers the reviewed window only, so treat it as partial, and re-check that the running version was read off a live resource",
  "changes_scanned": 11,
  "summary": { "new_changes": 11, "distinct_matters": 11, … }
}
```

### `detail:"full"`

`full`은 더 긴 응답이 아니라 **다른** 응답입니다. 컴포넌트에서 `summary`와
버킷별 세 목록이 사라지고 `relevant_changes`가 들어옵니다. 해당하는 변경을
서버 원형 그대로 — `applies_if`는 구조화된 객체로, `advisories`는 각자의
심각도와 함께, `subjects`와 `seq`까지 — 병합 없이 돌려주므로 같은
`matter_key`의 반복도 모두 나타납니다.

```json
{ "detail": "full", "severity_min": "high",
  "components": [ { "project": "coredns", "version": "v1.14.1", "version_source": "deployment/coredns image tag" } ] }
```

```json
{
  "components": [
    {
      "project": "coredns",
      "running_version": "v1.14.1",
      "version_source": "deployment/coredns image tag",
      "changes_scanned": 11,
      "relevant_changes": [
        {
          "change_id": "coredns:v1.14.2:83ed5f33",
          "matter_key": "coredns/advisory:cve-2026-25679",
          "project": "coredns", "version": "v1.14.2", "version_rank": [ 1, 14, 2 ],
          "released_at": "2026-03-06T06:34:58.000Z",
          "family": "security", "actionability": "act", "bucket": "action",
          "kind": "value_changed",
          "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
          "advisories": [
            { "id": "CVE-2026-25679", "severity": "high" },
            { "id": "CVE-2026-27137", "severity": "high" },
            { "id": "CVE-2026-27138", "severity": "medium" },
            { "id": "CVE-2026-27139", "severity": "low" },
            { "id": "CVE-2026-27142", "severity": "medium" }
          ],
          "subjects": [ { "kind": "dependency", "name": "go 1.26.1", "name_full": "Go 1.26.1", "role": "changed" } ],
          "window": { "introduced_in": "v1.14.2" },
          "quote": "In addition, the release updates the build to Go 1.26.1, which include security fixes addressing CVE-2026-27137, CVE-2026-27138, CVE-2026-27139, CVE-2026-25679, and CVE-2026-27142.",
          "disclosure": "described",
          "source_url": "https://github.com/coredns/coredns/releases/tag/v1.14.2",
          "release_url": "https://ratatosk.io/en/releases/coredns/v1.14.2",
          "seq": 3431
        },
        …3건 생략…
      ]
    }
  ],
  "privacy": "versions were compared locally; only project slugs were sent to the server"
}
```

같은 변경인데도 두 모드의 심각도가 다르다는 점을 보세요. `brief`는
`severity` 하나(그룹의 최댓값인 `high`)를 말하고, `full`은 권고 다섯 개를
각자의 심각도와 함께 돌려줍니다. `full`은 `severity_min`이나
`target_version`으로 좁혀 쓰세요. 바쁜 프로젝트에 필터 없이 걸면 응답이
큽니다.

### 질문 예시

> "우리 envoy가 v1.36.8인데, v1.37.0으로 올리기 전에 챙길 게 있나요?"

> "우리 스택은 Kubernetes 1.31, Cilium 1.16, CoreDNS 1.11입니다. 업그레이드
> 전에 봐야 할 게 있을까요?"

에이전트는 둘 다 `check_stack` 한 번으로 답합니다. 앞엣것은
`target_version`을 채워서, 뒤엣것은 컴포넌트 셋을 한 호출에 담아서요. 위
시나리오처럼 클러스터를 직접 읽는 에이전트라면 버전을 모으는 일까지 맡길 수
있습니다.

## list_projects

추적 중인 프로젝트 전체 명단입니다. 인자가 없고 응답도 작으니, 프로젝트
이름에서 슬러그를 추측하기 전에 이걸 부르세요. 틀린 슬러그는 오류가 아니라
`check_stack`에서 `tracked:false`로 돌아옵니다. 즉 추측 하나가 조용히
"커버리지 없음"으로 바뀝니다. 여기의 `slug`가 다른 모든 도구가 받는
`project` 인자의 정본입니다.

### 파라미터

없습니다. 빈 인자로 부르세요.

### 예시 호출

```json
{}
```

### 실측 응답 (일부 생략)

```json
{
  "projects": [
    { "slug": "argo", "name": "Argo", "tier": "graduated", "category": "cicd", "analyzed_releases": 32 },
    …
    { "slug": "cilium", "name": "Cilium", "tier": "graduated", "category": "networking",
      "analyzed_releases": 21, "cluster_core": true },
    { "slug": "containerd", "name": "containerd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 22, "cluster_core": true,
      "visibility": "observed via node status (nodeInfo.containerRuntimeVersion), never via pods — a workload listing cannot show the runtime" },
    { "slug": "coredns", "name": "CoreDNS", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 7, "cluster_core": true },
    { "slug": "envoy", "name": "Envoy", "tier": "graduated", "category": "networking",
      "analyzed_releases": 23, "image_aliases": [ "cilium-envoy" ], "cluster_core": true },
    { "slug": "etcd", "name": "etcd", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 21, "cluster_core": true,
      "visibility": "may live outside the k8s API, be hidden by a managed control plane, or be replaced entirely — a missing pod is not a missing component; when unreadable, report it under Could not check instead of guessing a version" },
    { "slug": "kubernetes", "name": "Kubernetes", "tier": "graduated", "category": "kubernetes-core",
      "analyzed_releases": 26, "cluster_core": true,
      "image_aliases": [ "kube-apiserver", "kube-controller-manager", "kube-scheduler", "kubelet", "kube-proxy" ] },
    …
  ],
  "count": 76
}
```

기본 필드(`slug`, `name`, `tier`, `category`, `analyzed_releases`) 외에
일부 항목에는 표식 셋이 붙습니다.

- `image_aliases` — 그 프로젝트가 클러스터에서 쓰는 다른 이름. 별칭에
  들어맞는 이미지나 워크로드는 그 프로젝트이고, 버전은 태그가 말하는
  것입니다.
- `cluster_core:true` — 클러스터의 바닥(컨트롤 플레인, 데이터스토어, DNS,
  런타임, CNI·데이터플레인). 클러스터에 존재하는 `cluster_core` 프로젝트는
  전부 `check_stack` 호출의 후보입니다.
- `visibility` — 그 컴포넌트를 어떻게 관측하는지, 그리고 어디서는 못 읽는
  것이 정상인지에 대한 힌트. containerd는 파드 목록이 아니라 노드 상태에서
  읽으라고 하고, etcd는 아예 k8s API 밖에 있을 수 있다고 경고합니다. 읽을 수
  없는 컴포넌트는 추측하지 말고 "확인 못 함"으로 보고해야 합니다.

### 질문 예시

> "우리 스택은 Kubernetes 1.31, Cilium 1.16, CoreDNS 1.11입니다. 업그레이드
> 전에 봐야 할 게 있을까요?"

`check_stack`과 같은 질문입니다. 위 시나리오에서 봤듯 에이전트는 이 도구로
슬러그를 먼저 확정하고 `check_stack`으로 넘어갑니다.

## list_releases

한 프로젝트의 최신 릴리스 N건을 가벼운 요약으로, 최신순으로 돌려줍니다.
"요즘 X에 무슨 일 있었나?"에 답하는 도구입니다. `list_changes`와
대조해 보세요. 그쪽은 같은 데이터를 반대 순서로 — 분석이 오래된 것부터 —
흘려보내는 동기화 피드이고, 로컬 사본을 최신으로 유지하는 용도입니다.
"최근 소식" 질문은 이쪽입니다. 눈에 띄는 행이 있으면 `get_release`로
파고드세요.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 예 | string | — | 프로젝트 슬러그(예: `istio`) |
| `limit` | 아니오 | integer | `5` | 최근 릴리스 몇 건. 최대 `20` |

### 예시 호출

```json
{ "project": "cilium", "limit": 5 }
```

### 실측 응답 (일부 생략)

```json
{
  "project": "cilium",
  "count": 5,
  "releases": [
    {
      "version": "v1.20.0", "version_rank": [ 1, 20, 0 ],
      "released_at": "2026-07-29T15:00:29.000Z", "reviewed_at": "2026-08-09T04:33:14.949Z",
      "changes_total": 45,
      "by_bucket": { "action": 9, "check": 30, "plan": 6 },
      "by_family": { "breaking": 32, "security": 7, "deprecated": 6 },
      "max_severity": null,
      "notes_total": 478,
      "api_url": "https://ratatosk.io/v1/releases/cilium/v1.20.0",
      "release_url": "https://ratatosk.io/en/releases/cilium/v1.20.0"
    },
    {
      "version": "v1.19.6", "released_at": "2026-07-16T22:52:21.000Z",
      "changes_total": 1, "by_bucket": { "check": 1 }, "by_family": { "breaking": 1 },
      "max_severity": null, "notes_total": 46, …
    },
    {
      "version": "v1.18.12", "released_at": "2026-07-16T22:47:50.000Z",
      "changes_total": 0, "by_bucket": {}, "by_family": {},
      "max_severity": null, "notes_total": 20, …
    },
    { "version": "v1.17.18", "changes_total": 0, "notes_total": 10, … },
    { "version": "v1.19.5", "changes_total": 3, "by_bucket": { "check": 3 },
      "by_family": { "breaking": 2, "security": 1 }, "notes_total": 48, … }
  ],
  "hint": "summaries only — fetch /v1/releases/{project}/{version} for a release's full changes"
}
```

읽을 때 셋만 기억하세요.

- **`changes_total: 0`** — v1.18.12와 v1.17.18이 그렇습니다 — 은 노트를 끝까지
  읽었고 그 릴리스가 일상적이었다는 뜻입니다. 데이터가 없는 것과는 다른,
  감사 가능한 침묵입니다. `notes_total`이 노트가 비어 있지 않았음을
  보여줍니다. 각각 일상 기록 20줄과 10줄이 있었습니다.
- **`max_severity`는 그 릴리스의 최고 *권고* 심각도**이고, 어떤 변경도 권고를
  인용하지 않으면 `null`입니다. v1.20.0은 `security` 가족 변경이 7건인데도
  `null`인데, CVE가 붙지 않은 의존성 범프이기 때문입니다. 여기서 `null`은
  "심각한 게 없다"가 아니라 "권고 id가 없다"는 뜻입니다.
- **큰 릴리스에서는 `notes_total`이 `changes_total`을 압도합니다** —
  v1.20.0에서 45 대 478. 그 차이가 하나씩 드러내지 않기로 한 일상
  기록입니다.

### 질문 예시

> "요즘 Cilium에 무슨 일 있었어요?"

에이전트는 `list_releases(project: "cilium")`를 부르고 요약만으로
답합니다. 변경이 v1.20.0에 몰려 있다는 것과, 조용한 릴리스 둘은 건너뛴 게
아니라 읽은 결과라는 것까지요.

## get_release

한 릴리스 전체입니다. 봉투(요약, 원문 URL, 릴리스 URL, 집계)와 그 릴리스의
모든 변경이 함께 옵니다. `check_stack`이나 `list_releases`가 드러낸 릴리스를
파고들 때, 그리고 `source_url`로 원문에 대고 판단을 검증할 때 쓰는
도구입니다. `version`을 생략하면 그 프로젝트의 최신 릴리스가 오므로 "X 최신
릴리스는 어떤가요?"도 여기로 옵니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 예 | string | — | 프로젝트 슬러그(예: `envoy`) |
| `version` | 아니오 | string | *(없음 = 최신 릴리스)* | 공개된 그대로의 릴리스 태그(예: `v1.38.3`). 앞의 `v`는 있어도 없어도 됩니다 — 프로젝트마다 표기가 다르니까요 |
| `include_raw` | 아니오 | boolean | `false` | 릴리스 노트 원문을 `raw_notes`로 함께 받기(상한 있음. 잘리면 `raw_notes_truncated: true`) |

틀린 태그는 그 프로젝트의 최근 태그를 알려주는 `404`라서, 호출한 쪽이 한 번의
재시도로 스스로 고칠 수 있습니다:

```
/v1/releases/cilium/v9.9.9: HTTP 404: {"error":"no reviewed release 'v9.9.9' for
project 'cilium'. Recent reviewed versions: v1.20.0, v1.19.6, v1.18.12, v1.17.18,
v1.19.5 — retry with one of these exact tags."}
```

### 예시 호출

```json
{ "project": "envoy", "version": "v1.38.3" }
```

### 실측 응답 (일부 생략)

```json
{
  "project": "envoy",
  "version": "v1.38.3",
  "version_rank": [ 1, 38, 3 ],
  "released_at": "2026-06-23T23:28:25.000Z",
  "reviewed_at": "2026-08-09T04:33:14.780Z",
  "summary": "Envoy v1.38.3 is a maintenance release with multiple disclosed security fixes and a security-related dependency update. It also disables a broken extension and changes the default for TLS certificate compression, so the release concerns deployments using those features.",
  "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
  "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
  "by_bucket": { "action": 1, "check": 17 },
  "by_family": { "security": 16, "breaking": 2 },
  "max_severity": "high",
  "notes_total": 0,
  "changes": [
    {
      "change_id": "envoy:v1.38.3:54ea382c",
      "matter_key": "envoy/advisory:cve-2026-47261",
      "project": "envoy", "version": "v1.38.3", "version_rank": [ 1, 38, 3 ],
      "released_at": "2026-06-23T23:28:25.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [ { "id": "CVE-2026-47261", "severity": "high" } ],
      "subjects": [
        { "kind": "dependency", "name": "com_github_wasmtime", "name_full": "com_github_wasmtime", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-47261", "name_full": "CVE-2026-47261", "role": "changed" }
      ],
      "window": null,
      "transition": null,
      "remedy": null,
      "symptom": [],
      "quote": "wasm: bumped ``com_github_wasmtime`` to resolve CVE-2026-47261.",
      "disclosure": "described",
      "source_url": "https://github.com/envoyproxy/envoy/releases/tag/v1.38.3",
      "release_url": "https://ratatosk.io/en/releases/envoy/v1.38.3",
      "seq": 13911
    },
    …17건 생략…
  ]
}
```

변경 18건에 `by_bucket`이 1 / 17인 것이 보안 패치 릴리스의 전형적인
모양입니다. 모두가 챙겨야 할 것 하나, 어떤 확장을 쓰느냐에 달린 것 열일곱.

### 감사 가능한 침묵은 근거를 스스로 들고 옵니다

운영자가 챙길 변경이 없는 릴리스라면 `get_release`는 묻지 않아도
`raw_notes`를 함께 보냅니다. 침묵을 믿으라고 하는 대신 원문에서 직접
판단할 수 있게 하려는 것입니다.

```json
{
  "project": "cilium",
  "version": "v1.18.12",
  "summary": "Cilium v1.18.12 adds Gateway access-log configuration and BYOCNI loopback support. It also fixes policy, startup, Gateway validation, IPAM, and metric-label defects, while updating shipped images and dependencies; no security advisories or security-specific flaws are disclosed.",
  "by_bucket": {}, "by_family": {}, "max_severity": null,
  "notes_total": 20,
  "changes": [],
  "raw_notes": "# 1.18.12\n\nSummary of Changes\n------------------\n\n**Minor Changes:**\n* gateway-api: add support for configuring Gateway access logs …",
  "raw_notes_truncated": false
}
```

### 질문 예시

> "Istio 최신 릴리스는 어때요? 뭐가 나왔죠?"

에이전트는 `version` 없이 `get_release(project: "istio")`를 부르고
`summary`와 버킷 집계로 답합니다.

## changes_by_entity

역인덱스입니다. 식별자 하나 — CVE id, CRD, 피처 게이트, 플래그, 메트릭, 설정
필드, 의존성 — 를 건드리는 모든 변경을 프로젝트와 릴리스를 가로질러
모아 옵니다. 대소문자는 구분하지 않습니다. 매니페스트나 권고에서 식별자
하나를 손에 쥐고 그 주변에 무슨 일이 있었는지 알고 싶을 때 쓰세요.
프로젝트에서 출발하는 `get_release`·`list_releases`와는 반대 방향입니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `name` | 예 | string | — | 찾을 정확한 식별자: CVE id, CRD, 피처 게이트, 플래그, 메트릭, 설정 필드, 확장, 서브시스템, 의존성 |
| `kind` | 아니오 | string | *(없음 = 전부)* | 식별자 종류 하나로 한정: `api`, `crd`, `feature_gate`, `flag`, `metric`, `config_field`, `extension`, `dependency`, `cve`, `advisory`, `subsystem` |

인덱스는 변경이 지닌 `subjects`로 만들어집니다. 매칭은 저장된 `name`에
대한 정확한 일치(대소문자 무시)이고 부분 문자열 검색이 아니며, 최대
200건까지 돌아옵니다.

`{ "changes": [] }`는 그 식별자를 이름으로 삼은 변경이 기록에 없다는
뜻이지, 그 주변에 아무 일도 없었다는 뜻이 아닙니다. 결론 내리기 전에
범위를 넓혀 보세요. `kind`부터 빼세요 — 대상에 저장된 종류와 정확히
같아야 하는데, 한 식별자가 늘 짐작한 종류로 분류돼 있지는 않습니다.
그다음 그 변경이 색인됐을 법한 다른 식별자로 시도하세요. GHSA id로만
인용된 권고는 CVE id로 닿지 않고, 그 반대도 마찬가지입니다.

### 예시 호출

```json
{ "name": "CVE-2026-41178" }
```

### 실측 응답

```json
{
  "changes": [
    {
      "change_id": "buildpacks:v0.40.9:83576398",
      "matter_key": "buildpacks/advisory:cve-2026-41178",
      "project": "buildpacks", "version": "v0.40.9", "version_rank": [ 0, 40, 9 ],
      "released_at": "2026-08-09T17:12:10.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [
        { "id": "CVE-2026-41178", "severity": "medium" },
        { "id": "GO-2026-5158", "severity": "medium" }
      ],
      "subjects": [
        { "kind": "dependency", "name": "go.opentelemetry.io/otel", "name_full": "go.opentelemetry.io/otel", "role": "changed" },
        { "kind": "cve", "name": "cve-2026-41178", "name_full": "CVE-2026-41178", "role": "changed" },
        { "kind": "advisory", "name": "go-2026-5158", "name_full": "GO-2026-5158", "role": "changed" }
      ],
      "window": { "introduced_in": "v0.40.9" },
      "quote": "`go.opentelemetry.io/otel` | v1.43.0 → v1.44.0 | GO-2026-5158 / CVE-2026-41178 — baggage header not length-capped",
      "disclosure": "described",
      "source_url": "https://github.com/buildpacks/pack/releases/tag/v0.40.9",
      "release_url": "https://ratatosk.io/en/releases/buildpacks/v0.40.9",
      "seq": 18932
    }
  ]
}
```

이 변경 하나에 색인된 대상이 셋 — 의존성, CVE, Go 권고 — 이라서 같은 항목에
`go.opentelemetry.io/otel`로도, `CVE-2026-41178`로도, `GO-2026-5158`로도
닿습니다. `window.introduced_in`은 수정이 v0.40.9에 들어왔다고 말하고, 그게
도달해 있어야 할 버전입니다.

### 질문 예시

> "CVE-2026-41178, 어느 릴리스에서 고쳐졌고 우리가 쓰는 것에 해당하나요?"

에이전트는 `changes_by_entity(name: "CVE-2026-41178")`를 부르고 프로젝트별
`window.introduced_in`을 읽어 실행 중인 버전과 비교합니다. 보안 확인이라면
CVE id와 권고 id를 둘 다 물어보세요. 노트가 둘 중 하나만 인용한 변경은 그
하나로만 색인됩니다.

## get_matter

한 사안이 나타난 모든 릴리스를 오래된 순으로 돌려줍니다. `matter_key`는
변경에서 그대로 복사해 넣으세요. 대소문자를 구분하고 `/`와 `:`가 들어
있습니다. "*내* 갈래에서는 어느 버전이 이걸 고치나", "이미 처리한 건가"에
답하는 도구입니다.

가장 최신 것만이 아니라 모든 등장을 주는 이유는 이렇습니다. 메인테이너가
한 문제를 지원 중인 다섯 갈래에서 고치면 변경이 다섯 건 생기는데, 갈래마다
실린 권고 묶음이 늘 같지는 않습니다. 최신 것만 듣고 판단하면 오래된 갈래에
있는 사람은 자기 커버리지를 잘못 결론 내립니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `matter_key` | 예 | string | — | 변경에서 그대로 복사한 `matter_key` |
| `include_all` | 아니오 | boolean | `false` | 일상 기록(대부분 봇 의존성 범프)까지 포함 |

### 예시 호출

```json
{ "matter_key": "containerd/advisory:cve-2026-47262" }
```

### 실측 응답 (일부 생략)

```json
{
  "matter_key": "containerd/advisory:cve-2026-47262",
  "project": "containerd",
  "family": "security",
  "occurrences": [
    {
      "change_id": "containerd:v2.2.5:a74c2b48",
      "version": "v2.2.5", "version_rank": [ 2, 2, 5 ],
      "released_at": "2026-06-18T23:11:33.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "defect_corrected",
      "advisories": [
        { "id": "CVE-2026-47262", "severity": "medium" },
        { "id": "CVE-2026-50195", "severity": "medium" },
        { "id": "CVE-2026-53488", "severity": "critical" },
        { "id": "CVE-2026-53489", "severity": "high" },
        { "id": "CVE-2026-53492", "severity": "high" },
        { "id": "GHSA-33vj-92qq-66hc", "severity": "high" },
        …4건 더…
      ],
      "source_url": "https://github.com/containerd/containerd/releases/tag/v2.2.5", …
    },
    { "version": "v2.3.2",  "advisories": [ …id 10개… ], … },
    { "version": "v2.0.10", "advisories": [ "CVE-2026-47262", "CVE-2026-53488", "GHSA-jpcc-p29g-p8mq", "GHSA-xhf5-7wjv-pqxp" ], … },
    { "version": "v1.7.33", "advisories": [ …id 4개… ], … },
    { "version": "v2.1.9",  "advisories": [ "CVE-2026-47262", "GHSA-jpcc-p29g-p8mq" ], … }
  ],
  "includes_notes": false
}
```

이 도구가 존재하는 이유가 바로 이 경우입니다. containerd 롤업 하나가 다섯
갈래에 내려앉았는데, 갈래마다 실린 권고 id가 **10개, 10개, 4개, 4개,
2개**입니다. v2.1.x에 있는 사람이 v2.2.5 항목만 읽으면 자기 갈래가 받지도
않은 수정 여덟 건을 받았다고 여기게 됩니다. `occurrences`는 오래된
순이므로, `version`이 지금 쓰는 것보다 바로 위인 항목이 올라갈 목적지입니다.

`includes_notes`는 일상 기록을 포함했는지를 되돌려주므로, 꼬리가 비어 있는
것인지 뺀 것인지 구분할 수 있습니다.

### 질문 예시

> "우리는 containerd 2.1.8인데, 그 containerd CVE 롤업이 우리한테는
> 고쳐졌나요? 어느 릴리스에서요?"

에이전트는 그 사안을 드러낸 도구 — `check_stack`이든
`changes_by_entity`든 — 에서 `matter_key`를 가져와 `get_matter`를 부르고,
최신 항목이 아니라 2.1 갈래의 항목으로 답합니다.

## list_changes

증분 동기화 피드입니다. `seq` 오름차순 — 분석이 오래된 것부터 — 으로
흐르므로 첫 페이지는 최신 데이터가 **아닙니다**. 로컬 사본을 최신으로
유지하기 위해 있는 도구입니다. "X 최신 릴리스는?"이나 "X 최근 릴리스"에는
`list_releases`나 `get_release`를 쓰세요.

일상 기록은 빠집니다. 이 피드에는 운영자가 챙길 변경만 흐릅니다.

### 파라미터

| 파라미터 | 필수 | 타입 | 기본값 | 설명 |
|---|---|---|---|---|
| `project` | 아니오 | string | *(없음 = 전부)* | 프로젝트 슬러그 필터(예: `envoy`) |
| `family` | 아니오 | string | *(없음 = 전부)* | `security`, `breaking`, `deprecated` |
| `bucket` | 아니오 | string | *(없음 = 전부)* | `action`, `check`, `plan` |
| `since` | 아니오 | integer | *(없음 = 처음부터)* | 커서. `seq`가 이 값보다 큰 변경만. 직전 응답의 `next_since`를 넣으세요 |
| `limit` | 아니오 | integer | `50` | 페이지 크기. 최대 `200` |

### 예시 호출

```json
{ "family": "security", "bucket": "action", "limit": 3 }
```

### 실측 응답 (일부 생략)

```json
{
  "changes": [
    {
      "change_id": "argo:v3.5.0:c140f331",
      "matter_key": "argo/dependency/formidable#value_changed",
      "project": "argo", "version": "v3.5.0", "version_rank": [ 3, 5, 0 ],
      "released_at": "2026-08-04T08:35:57.000Z",
      "family": "security", "actionability": "act", "bucket": "action",
      "kind": "value_changed",
      "applies_if": { "evaluable": false, "mode": "universal", "clauses": [], "raw": null },
      "advisories": [],
      "subjects": [ { "kind": "dependency", "name": "formidable", "name_full": "formidable", "role": "changed" } ],
      "window": null, "transition": null, "remedy": null, "symptom": [],
      "quote": "chore(deps): update dependency formidable to v2.1.3 [security]",
      "disclosure": "undisclosed",
      "source_url": "https://github.com/argoproj/argo-cd/releases/tag/v3.5.0",
      "release_url": "https://ratatosk.io/en/releases/argo/v3.5.0",
      "seq": 1415
    },
    { "change_id": "backstage:v1.50.0:22fe2851", "project": "backstage", "version": "v1.50.0",
      "released_at": "2026-04-14T17:49:58.000Z", "family": "security", "bucket": "action",
      "disclosure": "described", "seq": 1464, … },
    { "change_id": "backstage:v1.50.0:fc3cb011", "project": "backstage", "version": "v1.50.0", "seq": 1465, … }
  ],
  "next_since": 1465
}
```

다음 페이지는
`{ "family": "security", "bucket": "action", "limit": 3, "since": 1465 }`
입니다. `next_since`가 `null`로 돌아올 때까지 반복하세요. `null`은 다
따라잡았다는 뜻입니다. `since: null`은 보내지 마세요 — `400`입니다.
페이지가 짧게 오면 그것으로 순회가 끝납니다. `next_since`는 페이지가 꽉
찼을 때만 값이 있습니다.

`seq`는 분석 커서이지 시간축이 아니라는 점을 유의하세요. 위 첫 페이지에는
8월의 argo 릴리스와 4월의 backstage 릴리스가 함께 있는데, 분석된 순서가
그랬기 때문입니다. 시간순이 필요하면 `released_at`으로 정렬하세요.

argo 항목의 `disclosure: "undisclosed"`는 노트가 그 범프를 보안 관련이라고
표시했을 뿐 권고를 지목하지는 않았다는 뜻입니다. `security` / `action`
변경인데 `advisories`가 비어 있는 이유가 이것입니다.

### 질문 예시

> "ratatosk 데이터의 우리 로컬 사본을 최신으로 유지해 줘."

에이전트는 마지막으로 본 `next_since`를 저장해 두고 거기서 이어받아,
`next_since`가 `null`이 될 때까지 페이지를 걸어갑니다.

## 프라이버시와 호출 한도

요약하면, `check_stack`의 버전 비교는 서버 프로세스 안에서 일어나므로 직접
호스팅하면 실행 중인 버전은 당신의 인프라를 벗어나지 않습니다. 경계와 로그
처리의 전모는
[README의 "실행 중인 컴포넌트 버전의 처리 방식"](../README.ko.md#실행-중인-컴포넌트-버전의-처리-방식)에
있습니다.

상류 공개 API는 호출자당 분당 60회로 제한되고, 호스팅 엔드포인트는 그 한
몫을 이용자들이 나눠 씁니다. 프로젝트마다 폴링하는 대신 스택 질문을
`check_stack` 한 번에 묶고, 많이 쓸 거라면 직접 호스팅으로 옮기세요.

이 데이터는 공식 릴리스 노트에서 AI가 추출한 것입니다. 중요한 결정은 모든
변경이 지닌 `source_url`에 대고 확인하세요([이용약관](https://ratatosk.io/terms)).
