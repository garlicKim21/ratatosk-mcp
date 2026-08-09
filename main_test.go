package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/garlicKim21/ratatosk-mcp/internal/version"
)

// briefReport must sort by version, fold the same issue_key across release
// branches into its earliest fix, and split critical/high from the rest.
func ch(id, ver, kind, family, bucket, matter string, adv ...Advisory) Change {
	return Change{ChangeID: id, Version: ver, Kind: kind, Family: family,
		Bucket: bucket, MatterKey: matter, Advisories: adv}
}

func keysOf(cs []Change) [][]int {
	keys := make([][]int, len(cs))
	for i, c := range cs {
		keys[i] = version.NormalizeVersion(c.Version)
	}
	return keys
}

// 같은 사안이 여러 갈래에 들어오면 **가장 이른 수정**으로 접힌다 — 운영자는
// 한 번 올리고, 그 사안을 닫는 가장 가까운 릴리스가 실행 가능한 답이다.
func TestBriefReportCollapsesMatterToEarliestFix(t *testing.T) {
	crit := Advisory{ID: "CVE-1", Severity: "critical"}
	cs := []Change{
		ch("c285", "v1.38.3", "defect_corrected", "security", "action", "m/sec-1", crit),
		ch("c211", "v1.36.9", "defect_corrected", "security", "action", "m/sec-1", crit),
		ch("c352", "v1.37.5", "defect_corrected", "security", "action", "m/sec-1", crit),
		ch("c219", "v1.36.9", "removed", "breaking", "check", "m/cap-1"),
		ch("c300", "v1.37.0", "default_changed", "breaking", "plan", "m/def-1"),
	}
	sum, action, check, other := briefReport(cs, keysOf(cs))

	if sum.NewChanges != 5 || sum.DistinctMatters != 3 {
		t.Fatalf("summary: got new=%d distinct=%d, want 5/3", sum.NewChanges, sum.DistinctMatters)
	}
	if len(action) != 1 || action[0].ChangeID != "c211" {
		t.Fatalf("action_required: want the earliest fix (c211), got %+v", action)
	}
	if len(action[0].AlsoIn) != 2 {
		t.Errorf("collapsed entry must list the other branches: %+v", action[0].AlsoIn)
	}
	if len(check) != 1 || check[0].ChangeID != "c219" {
		t.Errorf("check bucket routes to check_config: %+v", check)
	}
	if len(other) != 1 || other[0].ChangeID != "c300" {
		t.Errorf("plan bucket routes to the tail: %+v", other)
	}
}

// **버킷이 갈래를 정한다.** 예전엔 심각도만 보고 갈라서, ClusterMesh 사용자에게만
// 해당하는 high 항목이 모두에게 "지금 올리라"로 읽혔다(2026-07-28 실측). 이제는
// 서버가 웹·메일과 같은 규칙으로 계산한 bucket을 그대로 따른다.
func TestBriefReportSplitsByBucketNotSeverity(t *testing.T) {
	crit := Advisory{ID: "CVE-2", Severity: "critical"}
	cs := []Change{
		ch("c1", "v1.36.9", "defect_corrected", "security", "check", "m/a", crit),
		ch("c2", "v1.37.5", "value_changed", "breaking", "action", "m/b"),
	}
	_, action, check, _ := briefReport(cs, keysOf(cs))
	if len(check) != 1 || check[0].ChangeID != "c1" {
		t.Fatalf("critical but conditional must stay in check_config: %+v", check)
	}
	if len(action) != 1 || action[0].ChangeID != "c2" {
		t.Fatalf("unconditional must be action_required regardless of severity: %+v", action)
	}
}

// 등급은 인용된 권고의 현재값에서 온다. 갈래마다 권고 구성이 다르면 가장 높은 쪽.
func TestBriefReportTakesHighestAdvisorySeverity(t *testing.T) {
	cs := []Change{
		ch("c10", "v1.36.9", "defect_corrected", "security", "action", "m/g1",
			Advisory{ID: "CVE-a", Severity: "high"}),
		ch("c11", "v1.37.5", "defect_corrected", "security", "action", "m/g1",
			Advisory{ID: "CVE-a", Severity: "high"}, Advisory{ID: "CVE-b", Severity: "critical"}),
	}
	_, action, _, _ := briefReport(cs, keysOf(cs))
	if len(action) != 1 {
		t.Fatalf("one matter → one entry, got %d", len(action))
	}
	if action[0].Severity != "critical" {
		t.Errorf("collapsed entry must show the strongest advisory: got %q", action[0].Severity)
	}
	if action[0].ChangeID != "c10" {
		t.Errorf("but it stays anchored to the earliest fix: got %q", action[0].ChangeID)
	}
}

func TestCoverageNote(t *testing.T) {
	oldest, oldestTag := version.NormalizeVersion("v1.17.18"), "v1.17.18"

	below := coverageNote("1.16.0", version.NormalizeVersion("1.16.0"), oldest, oldestTag)
	if below == "" {
		t.Fatal("a version below every release on record must be flagged")
	}
	for _, want := range []string{"1.16.0", "v1.17.18", "read off a live resource"} {
		if !strings.Contains(below, want) {
			t.Errorf("note must name %q: %s", want, below)
		}
	}

	// Inside the window, and exactly at the earliest, are both normal.
	if n := coverageNote("1.19.5", version.NormalizeVersion("1.19.5"), oldest, oldestTag); n != "" {
		t.Errorf("a version inside the reviewed window must not be flagged: %s", n)
	}
	if n := coverageNote("1.17.18", version.NormalizeVersion("1.17.18"), oldest, oldestTag); n != "" {
		t.Errorf("the earliest release itself must not be flagged: %s", n)
	}
	// No facts at all: the tracked/not-tracked probe already speaks, so stay quiet.
	if n := coverageNote("1.0.0", version.NormalizeVersion("1.0.0"), nil, ""); n != "" {
		t.Errorf("with nothing on record there is nothing to compare: %s", n)
	}
}

// Acceptance criteria from the hub's 60-run campaign: the 8 observed
// confessed sources all flag, a representative sample of the 197 concrete
// sources never does. The vocabulary list deliberately omits "stated" (a
// user-stated version is legitimate in the no-cluster-access deployment) and
// "standard" — every observed confession is caught without them.
func TestConfessionNote(t *testing.T) {
	confessions := []string{
		"user stated/inferred from environment",
		"cilium-operator image tag inferred from typical deployment versioning",
		"inferred from cluster age/k8s version mapping",
		"cilium-operator image tag (assumed from discovery)",
		"coredns deployment image tag (assumed from discovery)",
		"guessed from k8s 1.36 stack (need verification)",
		"inferred from standard k8s deployments",
		"inferred from image in pod/coredns",
	}
	for _, s := range confessions {
		if confessionNote(s) == "" {
			t.Errorf("confessed source must be flagged: %q", s)
		}
	}
	concrete := []string{
		"", // absent is not a confession
		"daemonset/cilium image tag",
		"node v1.36.2",
		"node runtime 2.2.5",
		"kube-apiserver pod image (kube-system)",
		"deployment/coredns image tag",
		"user-provided version",
		"read from cilium-config ConfigMap",
	}
	for _, s := range concrete {
		if n := confessionNote(s); n != "" {
			t.Errorf("concrete source must not be flagged: %q -> %s", s, n)
		}
	}
}

// The condition phrase is read by a model and shown to people; our storage
// enum must not appear in it. Observed live: "configures config_field
// per-upstream read timeout".
func TestConditionPhrase(t *testing.T) {
	cases := []struct{ verb, kind, name, want string }{
		{"configures", "config_field", "per-upstream read timeout", "configures the per-upstream read timeout setting"},
		{"uses", "subsystem", "DoQ", "uses DoQ"},
		{"enables", "feature_gate", "InPlacePodVerticalScaling", "enables the InPlacePodVerticalScaling feature gate"},
		{"depends_on", "dependency", "Go 1.26", "depends on Go 1.26"},
		{"uses", "crd", "HTTPRoute", "uses the HTTPRoute CRD"},
		{"configures", "flag", "--listen-client-http-urls", "configures the --listen-client-http-urls flag"},
		{"uses", "somethingNew", "widget", "uses widget"}, // unknown kind falls through to the name
	}
	for _, c := range cases {
		if got := conditionPhrase(c.verb, c.kind, c.name); got != c.want {
			t.Errorf("conditionPhrase(%q,%q,%q) = %q, want %q", c.verb, c.kind, c.name, got, c.want)
		}
	}
}


// A target at or below the running version defines an empty range — measured
// agent runs sent exactly that and read the guaranteed zero as "no issues".
// The guard must drop the field (so facts return) and say why in the note.
func TestResolveTarget(t *testing.T) {
	running := version.NormalizeVersion("v1.36.8")
	cases := []struct {
		name, target string
		wantKey      bool
		wantNoteSub  string
	}{
		{"absent", "", false, ""},
		{"unparseable", "latest", false, "could not be parsed"},
		{"equal to running", "v1.36.8", false, "empty by construction"},
		{"below running", "v1.36.7", false, "empty by construction"},
		{"spelling variant of running", "1.36.8", false, "empty by construction"},
		{"above running", "v1.36.9", true, ""},
	}
	for _, c := range cases {
		key, note := resolveTarget(running, c.target)
		if (key != nil) != c.wantKey {
			t.Errorf("%s: targetKey presence = %v, want %v", c.name, key != nil, c.wantKey)
		}
		if c.wantNoteSub == "" && note != "" {
			t.Errorf("%s: unexpected note %q", c.name, note)
		}
		if c.wantNoteSub != "" && !strings.Contains(note, c.wantNoteSub) {
			t.Errorf("%s: note %q does not contain %q", c.name, note, c.wantNoteSub)
		}
	}
}

// Acceptance criterion from the 2026-07-30 hub campaign (proposal 6): replaying
// the self-nullifying call — target_version equal to the running version —
// must bring the action_required facts back instead of a silent zero.
func TestCheckStackIgnoresSelfNullifyingTarget(t *testing.T) {
	// Wire-shape fixture (references/affected envelopes), because Fact
	// round-trips through custom (Un)MarshalJSON that preserves the raw form.
	page := `{"changes":[
		{"change_id":"c1","matter_key":"m/sec-1","project":"envoy","version":"v1.36.9","kind":"defect_corrected","family":"security","bucket":"action","quote":"crash","advisories":[{"id":"CVE-x","severity":"high"}]},
		{"change_id":"c2","matter_key":"m/def-1","project":"envoy","version":"v1.36.9","kind":"default_changed","family":"breaking","bucket":"plan"},
		{"change_id":"c3","matter_key":"m/old-1","project":"envoy","version":"v1.36.7","kind":"defect_corrected","family":"security","bucket":"action","advisories":[{"id":"CVE-y","severity":"high"}]}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/changes" {
			t.Errorf("unexpected upstream path %s", r.URL.Path)
		}
		w.Write([]byte(page))
	}))
	defer ts.Close()
	orig := api
	api = newAPIClient(ts.URL)
	defer func() { api = orig }()

	res, _, err := checkStackTool(context.Background(), nil, checkStackArgs{
		Components: []stackComponent{{Project: "envoy", Version: "v1.36.8", TargetVersion: "v1.36.8"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Components []struct {
			Summary struct {
				NewChanges int `json:"new_changes"`
			} `json:"summary"`
			ActionRequired []struct {
				ChangeID string `json:"change_id"`
			} `json:"action_required"`
			Note string `json:"note"`
		} `json:"components"`
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal tool output: %v", err)
	}
	c := out.Components[0]
	if c.Summary.NewChanges != 2 {
		t.Errorf("new_changes = %d, want 2 (changes above running, target ignored)", c.Summary.NewChanges)
	}
	if len(c.ActionRequired) != 1 || c.ActionRequired[0].ChangeID != "c1" {
		t.Errorf("action_required = %+v, want exactly change c1 back", c.ActionRequired)
	}
	if !strings.Contains(c.Note, "empty by construction") {
		t.Errorf("note %q must explain the ignored target", c.Note)
	}
}
