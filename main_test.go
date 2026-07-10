package main

import (
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

	sum, action, other := briefReport(facts, keys)

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
