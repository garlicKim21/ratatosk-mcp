package main

import (
	"strings"
	"testing"

	"github.com/garlicKim21/ratatosk-mcp/internal/version"
)

// briefReport must sort by version, fold the same issue_key across release
// branches into its earliest fix, and split critical/high from the rest.
func TestBriefReport(t *testing.T) {
	facts := []Fact{
		// same advisory fixed on three branches — arrives out of order
		{FactID: 285, Version: "v1.38.3", FactType: "security_fix", Severity: "critical", Mandatory: true, IssueKey: "sec-1"},
		{FactID: 211, Version: "v1.36.9", FactType: "security_fix", Severity: "critical", Mandatory: true, IssueKey: "sec-1"},
		{FactID: 352, Version: "v1.37.5", FactType: "security_fix", Severity: "critical", Mandatory: true, IssueKey: "sec-1"},
		// distinct medium fact → other_facts
		{FactID: 219, Version: "v1.36.9", FactType: "capability_removed", Severity: "medium", Mandatory: true, IssueKey: "cap-1"},
		// no issue_key → never folded
		{FactID: 300, Version: "v1.37.0", FactType: "default_changed", Severity: "low"},
	}
	keys := make([][]int, len(facts))
	for i, f := range facts {
		keys[i] = version.NormalizeVersion(f.Version)
	}

	sum, action, check, other := briefReport(facts, keys)

	if len(check) != 0 {
		t.Errorf("no fact carries a condition, so check_config must be empty: %+v", check)
	}
	if sum.NewFacts != 5 || sum.DistinctIssues != 3 {
		t.Fatalf("summary: got new=%d distinct=%d, want 5/3", sum.NewFacts, sum.DistinctIssues)
	}
	if sum.Mandatory != 2 {
		t.Errorf("mandatory: got %d, want 2 (folded copies counted once)", sum.Mandatory)
	}
	if len(action) != 1 || action[0].FactID != 211 {
		t.Fatalf("action_required: want exactly the earliest fix (fact 211), got %+v", action)
	}
	if got := action[0].AlsoIn; len(got) != 2 || got[0] != "v1.37.5" || got[1] != "v1.38.3" {
		t.Errorf("same_issue_also_addressed_in: got %v, want [v1.37.5 v1.38.3]", got)
	}
	if len(other) != 2 || other[0].FactID != 219 || other[1].FactID != 300 {
		t.Errorf("other_facts: got %+v, want facts 219 then 300 in version order", other)
	}
	if sum.BySeverity["critical"] != 1 || sum.ByType["security_fix"] != 1 {
		t.Errorf("counts must be over distinct issues: %v %v", sum.BySeverity, sum.ByType)
	}
}

// The same advisory extracted from different branches' notes gets different
// issue_keys and can get different severity judgments (each note is analyzed
// independently). briefReport must fold on advisory_group_key and show the
// strongest judgment in the group — observed live: envoy CVE-2026-47692,
// high on v1.36.9 but critical on v1.37.5.
func TestBriefReportAdvisoryGroupSeverity(t *testing.T) {
	facts := []Fact{
		{FactID: 210, Version: "v1.36.9", FactType: "security_fix", Severity: "high", IssueKey: "sec-a", GroupKey: "adv:g1"},
		{FactID: 351, Version: "v1.37.5", FactType: "security_fix", Severity: "critical", Mandatory: true, IssueKey: "sec-b", GroupKey: "adv:g1"},
		// group judged medium first, high later → must be promoted into action_required
		{FactID: 400, Version: "v1.36.9", FactType: "security_fix", Severity: "medium", IssueKey: "sec-c", GroupKey: "adv:g2"},
		{FactID: 401, Version: "v1.38.3", FactType: "security_fix", Severity: "high", IssueKey: "sec-d", GroupKey: "adv:g2"},
	}
	keys := make([][]int, len(facts))
	for i, f := range facts {
		keys[i] = version.NormalizeVersion(f.Version)
	}

	sum, action, check, other := briefReport(facts, keys)

	if sum.DistinctIssues != 2 || len(other) != 0 || len(check) != 0 {
		t.Fatalf("want 2 distinct unconditional issues, all in action_required; got distinct=%d action=%d check=%d other=%d",
			sum.DistinctIssues, len(action), len(check), len(other))
	}
	if action[0].FactID != 210 || action[0].Severity != "critical" || !action[0].Mandatory {
		t.Errorf("group adv:g1: want earliest fix (210) shown at group-max severity critical and mandatory, got %+v", action[0])
	}
	if got := action[0].AlsoIn; len(got) != 1 || got[0] != "v1.37.5" {
		t.Errorf("group adv:g1 also_in: got %v, want [v1.37.5]", got)
	}
	if action[1].FactID != 400 || action[1].Severity != "high" {
		t.Errorf("group adv:g2: want fact 400 promoted to high, got %+v", action[1])
	}
	if sum.BySeverity["critical"] != 1 || sum.BySeverity["high"] != 1 || sum.BySeverity["medium"] != 0 {
		t.Errorf("by_severity must count collapsed entries at group-max severity: %v", sum.BySeverity)
	}
	if sum.Mandatory != 1 {
		t.Errorf("mandatory: got %d, want 1 (any branch mandatory marks the issue)", sum.Mandatory)
	}
}

// One extraction sentence covering several CVEs arrives as one fact per id
// with an identical quote (observed live: envoy v1.39.0, same quote seven
// times in a brief report). Brief mode merges them: ids together, strongest
// severity, diverging applies_if collected into applies_if_any. Same quote on
// a different version stays separate.
func TestBriefReportMergesSharedQuotes(t *testing.T) {
	q := "Security fixes were added for ext_authz (CVE-1), ext_proc (CVE-2)."
	facts := []Fact{
		{FactID: 466, Version: "v1.39.0", FactType: "security_fix", Severity: "medium", Mandatory: true,
			IssueKey: "s1", Quote: q, Condition: "uses extension ext_authz", RefIDs: []string{"CVE-1"}},
		{FactID: 467, Version: "v1.39.0", FactType: "security_fix", Severity: "high",
			IssueKey: "s2", Quote: q, Condition: "uses extension ext_proc", RefIDs: []string{"CVE-2"}},
		{FactID: 500, Version: "v1.38.9", FactType: "security_fix", Severity: "medium",
			IssueKey: "s3", Quote: q, RefIDs: []string{"CVE-1"}},
	}
	keys := make([][]int, len(facts))
	for i, f := range facts {
		keys[i] = version.NormalizeVersion(f.Version)
	}

	sum, action, check, other := briefReport(facts, keys)

	if sum.DistinctIssues != 3 {
		t.Fatalf("summary stays per issue: got distinct=%d, want 3", sum.DistinctIssues)
	}
	if len(action)+len(check)+len(other) != 2 {
		t.Fatalf("want 2 rendered entries (v1.39.0 merged, v1.38.9 separate), got action=%d check=%d other=%d",
			len(action), len(check), len(other))
	}
	// The merged entry is high but only bites installs using ext_authz or
	// ext_proc, so it belongs in check_config — not in the list a reader takes
	// as "upgrade now".
	if len(action) != 0 {
		t.Errorf("a conditional entry must not reach action_required: %+v", action)
	}
	var merged *briefFact
	for i := range check {
		if check[i].Version == "v1.39.0" {
			merged = &check[i]
		}
	}
	if merged == nil {
		t.Fatalf("merged v1.39.0 entry must sit in check_config at max severity high; check=%+v other=%+v", check, other)
	}
	if len(merged.IDs) != 2 || merged.IDs[0] != "CVE-1" || merged.IDs[1] != "CVE-2" {
		t.Errorf("ids concatenated: got %v", merged.IDs)
	}
	if merged.Severity != "high" || !merged.Mandatory {
		t.Errorf("merged entry: want strongest severity high + mandatory, got %s/%v", merged.Severity, merged.Mandatory)
	}
	if merged.Condition != "" || len(merged.ConditionsAny) != 2 {
		t.Errorf("diverging conditions must move to applies_if_any: cond=%q any=%v", merged.Condition, merged.ConditionsAny)
	}
}

// A high fact whose condition the caller has not resolved is not an action.
// Live case (2026-07-28): cilium v1.19.6 fixes a ClusterMesh blackhole, and an
// agent read severity alone and told a cluster with no ClusterMesh to upgrade
// first thing. fixed_in has to ride along — it is the answer to "before I turn
// ClusterMesh on, what do I need?".
func TestBriefReportKeepsConditionalOutOfActionRequired(t *testing.T) {
	facts := []Fact{
		{FactID: 492, Version: "v1.19.6", FactType: "behavior_changed", Severity: "high", Mandatory: true,
			IssueKey: "cm-1", FixedIn: "v1.19.6", Quote: "Fix ClusterMesh service affinity annotation ...",
			Condition: `uses service.cilium.io/affinity: "none" in ClusterMesh with no local endpoints`},
		{FactID: 600, Version: "v1.19.6", FactType: "security_fix", Severity: "high", Mandatory: true,
			IssueKey: "sec-1", FixedIn: "v1.19.6", Quote: "Fix an unauthenticated crash in the agent."},
	}
	keys := make([][]int, len(facts))
	for i, f := range facts {
		keys[i] = version.NormalizeVersion(f.Version)
	}

	_, action, check, _ := briefReport(facts, keys)

	if len(action) != 1 || action[0].FactID != 600 {
		t.Fatalf("action_required must hold only the unconditional fact 600, got %+v", action)
	}
	if len(check) != 1 || check[0].FactID != 492 {
		t.Fatalf("check_config must hold the conditional fact 492, got %+v", check)
	}
	if check[0].FixedIn != "v1.19.6" || action[0].FixedIn != "v1.19.6" {
		t.Errorf("fixed_in must survive into the brief: check=%q action=%q", check[0].FixedIn, action[0].FixedIn)
	}
}

// The server never sees the caller's cluster, so it cannot verify a running
// version — with one exception: a version below everything on record. That is
// the shape a fabricated version took live (cilium 1.16.0 reported for a 1.19.5
// install), and the shape a genuinely ancient install takes too. Both deserve
// the same warning, and neither may pass silently.
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

