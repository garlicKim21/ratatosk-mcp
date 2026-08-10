package version

import (
	"reflect"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		// Clean semver (the ~90% case) — with and without a leading v.
		{"v1.24.3", []int{1, 24, 3}},
		{"1.24.3", []int{1, 24, 3}},
		{"v2", []int{2}},
		{"1.11.0", []int{1, 11, 0}},
		// Component prefixes.
		{"knative-v1.12.0", []int{1, 12, 0}},
		{"chart/1.5.0", []int{1, 5, 0}},
		{"ce@2.3.4", []int{2, 3, 4}},
		{"api/v1.2.0", []int{1, 2, 0}},
		// Channel / calver.
		{"edge-26.1.4", []int{26, 1, 4}},
		// Flatcar tags every channel alike; "stable-" parsed and "lts-" did not,
		// which silently excluded the LTS train from every range query.
		{"stable-4593.2.4", []int{4593, 2, 4}},
		{"lts-4081.3.7", []int{4081, 3, 7}},
		{"stable-2.14.0", []int{2, 14, 0}},
		// Four components (e.g. cloud-custodian).
		{"0.9.40.1", []int{0, 9, 40, 1}},
		// Pre-release / build metadata: drop the trailing tag, keep the core.
		{"1.24.3-rc1", []int{1, 24, 3}},
		{"1.2.3+build.7", []int{1, 2, 3}},
		// Whitespace tolerated.
		{"  v3.0.1  ", []int{3, 0, 1}},
		// Unparseable → nil (auto-excluded from ranges).
		{"redirect", nil},
		{"", nil},
		{"   ", nil},
		{"latest", nil},
		// "v" not followed by a digit must NOT be stripped as the semver v.
		// Under the line model this is line "vault" at 1.2.3, not "ault-1.2.3":
		// SplitLine cuts at the separator before the semver "v" rule ever runs,
		// so a name that merely starts with v survives intact.
		{"vault-1.2.3", []int{1, 2, 3}},
		{"velero-1.2.3", []int{1, 2, 3}},
		// Sub-component and channel lines keep only the number; which line it
		// belongs to is SameLine's question, never the key's.
		{"flagd/v0.16.1", []int{0, 16, 1}},
		{"flagd-proxy/v0.9.7", []int{0, 9, 7}},
		{"cesql@v1.0.0", []int{1, 0, 0}},
		{"wash-v0.43.0", []int{0, 43, 0}},
	}
	for _, c := range cases {
		got := NormalizeVersion(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("NormalizeVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Range ordering: within a project the int-array keys must sort the way an
// operator expects (1.9.x < 1.10.x — the classic string-sort trap).
func TestVersionKeyOrdering(t *testing.T) {
	less := func(a, b []int) bool {
		for i := 0; i < len(a) && i < len(b); i++ {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
		}
		return len(a) < len(b)
	}
	if !less(NormalizeVersion("v1.9.0"), NormalizeVersion("v1.10.0")) {
		t.Error("1.9.0 should sort before 1.10.0")
	}
	if !less(NormalizeVersion("v1.24.3"), NormalizeVersion("v1.24.10")) {
		t.Error("1.24.3 should sort before 1.24.10")
	}
}

// Release lines: the tag says which product or channel it belongs to, and two
// lines have no order between them. Getting this wrong is not a near miss —
// containerd v1.7.28 was offered api/v1.11.0 as an upgrade (2026-08-10).
func TestSplitLine(t *testing.T) {
	cases := []struct{ in, line, rest string }{
		// Main line — no prefix.
		{"v1.24.3", "", "v1.24.3"},
		{"1.24.3", "", "1.24.3"},
		{"1.24.3-rc1", "", "1.24.3-rc1"}, // a pre-release suffix is not a line
		{"1.2.3+build.7", "", "1.2.3+build.7"},
		// Sub-components.
		{"api/v1.11.1", "api", "v1.11.1"},
		{"flagd/v0.16.1", "flagd", "v0.16.1"},
		{"core/v0.15.0", "core", "v0.15.0"},
		{"cesql@v1.0.0", "cesql", "v1.0.0"},
		{"wash-v0.43.0", "wash", "v0.43.0"},
		// Longest wins, or flagd-proxy would collapse into flagd.
		{"flagd-proxy/v0.9.7", "flagd-proxy", "v0.9.7"},
		// Channels.
		{"lts-4081.3.9", "lts", "4081.3.9"},
		{"stable-4593.2.4", "stable", "4593.2.4"},
		{"edge-26.1.4", "edge", "26.1.4"},
		// Whole-project conventions: one line for everything, so nothing splits.
		{"knative-v1.12.0", "knative", "v1.12.0"},
		// Not versions at all.
		{"redirect", "", "redirect"},
		{"latest", "", "latest"},
		{"", "", ""},
	}
	for _, c := range cases {
		line, rest := SplitLine(c.in)
		if line != c.line || rest != c.rest {
			t.Errorf("SplitLine(%q) = (%q, %q), want (%q, %q)", c.in, line, rest, c.line, c.rest)
		}
	}
}

func TestSameLine(t *testing.T) {
	same := [][2]string{
		{"v1.7.28", "v2.2.5"},              // both main line
		{"api/v1.11.0", "api/v1.11.1"},     // both the api sub-component
		{"lts-4081.3.7", "lts-4081.3.9"},   // same channel
		{"flagd/v0.15.0", "flagd/v0.16.1"}, // same product
	}
	for _, p := range same {
		if !SameLine(p[0], p[1]) {
			t.Errorf("SameLine(%q, %q) = false, want true", p[0], p[1])
		}
	}
	differ := [][2]string{
		{"v1.7.28", "api/v1.11.0"},          // the live defect this prevents
		{"lts-4081.3.7", "stable-4593.2.4"}, // independent channels
		{"flagd/v0.16.1", "core/v0.15.0"},   // independent products
		{"flagd/v0.16.1", "flagd-proxy/v0.9.7"},
	}
	for _, p := range differ {
		if SameLine(p[0], p[1]) {
			t.Errorf("SameLine(%q, %q) = true, want false", p[0], p[1])
		}
	}
}
