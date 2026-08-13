package version

// Golden test against testdata/version-golden.json.
//
// The table is a COPY — the worker's copy is the source of truth:
//
//	ratatosk/apps/worker/internal/version/testdata/version-golden.json
//
// Edit it there and refresh this one verbatim (see the JSON's _copy block).
// The same answer key is also sat by apps/web/src/lib/version.ts, so all three
// implementations of these semantics are checked against one exam rather than
// against each other's memory of it.
//
// Unlike the worker's test this one drives the package's own Compare, which is
// the extra function this copy carries: element-wise int[] ordering, the order
// the API's version_key column is queried with.

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type goldenNormalize struct {
	Raw  string `json:"raw"`
	Key  []int  `json:"key"`
	Note string `json:"note"`
}

type goldenSplit struct {
	Raw  string `json:"raw"`
	Line string `json:"line"`
	Rest string `json:"rest"`
	Note string `json:"note"`
}

type goldenCompare struct {
	A        string `json:"a"`
	B        string `json:"b"`
	Verdict  string `json:"verdict"`
	WebOrder string `json:"web_order"`
	Note     string `json:"note"`
}

type goldenTable struct {
	Normalize []goldenNormalize `json:"normalize"`
	SplitLine []goldenSplit     `json:"split_line"`
	Compare   []goldenCompare   `json:"compare"`
	// Cases where today's answer is not the sensible one — asserted anyway so
	// the three copies cannot drift apart, fenced off so nobody mistakes them
	// for endorsed behaviour. See the section's _why in the JSON.
	KnownDivergence struct {
		Normalize []goldenNormalize `json:"normalize"`
		SplitLine []goldenSplit     `json:"split_line"`
	} `json:"known_divergence"`
}

func loadGolden(t *testing.T) goldenTable {
	t.Helper()
	b, err := os.ReadFile("testdata/version-golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var g goldenTable
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(g.Normalize) == 0 || len(g.SplitLine) == 0 || len(g.Compare) == 0 {
		t.Fatal("golden table is missing a section")
	}
	return g
}

func TestGoldenNormalize(t *testing.T) {
	g := loadGolden(t)
	cases := append(append([]goldenNormalize{}, g.Normalize...), g.KnownDivergence.Normalize...)
	for _, c := range cases {
		got := NormalizeVersion(c.Raw)
		if len(got) == 0 && len(c.Key) == 0 {
			continue // nil and [] both mean "no key"
		}
		if !reflect.DeepEqual(got, c.Key) {
			t.Errorf("NormalizeVersion(%q) = %v, want %v (%s)", c.Raw, got, c.Key, c.Note)
		}
	}
}

func TestGoldenSplitLine(t *testing.T) {
	g := loadGolden(t)
	cases := append(append([]goldenSplit{}, g.SplitLine...), g.KnownDivergence.SplitLine...)
	for _, c := range cases {
		line, rest := SplitLine(c.Raw)
		if line != c.Line || rest != c.Rest {
			t.Errorf("SplitLine(%q) = (%q, %q), want (%q, %q) (%s)", c.Raw, line, rest, c.Line, c.Rest, c.Note)
		}
		if got := Line(c.Raw); got != c.Line {
			t.Errorf("Line(%q) = %q, want %q", c.Raw, got, c.Line)
		}
	}
}

func TestGoldenCompare(t *testing.T) {
	g := loadGolden(t)
	for _, c := range g.Compare {
		got := "different_line"
		if SameLine(c.A, c.B) {
			switch Compare(NormalizeVersion(c.A), NormalizeVersion(c.B)) {
			case 1:
				got = "a_newer"
			case -1:
				got = "b_newer"
			default:
				got = "equal"
			}
		}
		if got != c.Verdict {
			t.Errorf("compare(%q, %q) = %s, want %s (%s)", c.A, c.B, got, c.Verdict, c.Note)
		}
		switch c.WebOrder {
		case "a_first", "b_first", "tie":
		default:
			t.Errorf("compare(%q, %q): web_order %q is not one of a_first/b_first/tie", c.A, c.B, c.WebOrder)
		}
		if c.Verdict == "a_newer" && c.WebOrder != "a_first" {
			t.Errorf("compare(%q, %q): a is newer but web_order is %q", c.A, c.B, c.WebOrder)
		}
		if c.Verdict == "b_newer" && c.WebOrder != "b_first" {
			t.Errorf("compare(%q, %q): b is newer but web_order is %q", c.A, c.B, c.WebOrder)
		}
	}
}
