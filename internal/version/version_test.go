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
		{"vault-1.2.3", nil},
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
