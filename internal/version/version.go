// Package version turns a raw release version string into a monotonic,
// per-project sort key — an int component array — for the agent API's range
// queries (docs/rfc/agent-api.md §5.7). Comparison is only meaningful WITHIN a
// single project (cross-project version compare is nonsense).
//
// COPY of apps/worker/internal/version (source of truth) — bundled here so the
// in-cluster MCP server compares versions client-side and never sends them to
// the server (RFC §5.7 privacy). Keep in sync when the worker's copy changes.
package version

import (
	"strconv"
	"strings"
)

// Compare orders two version keys element-wise (Postgres int[] semantics):
// a shorter key that is a prefix of a longer one sorts first.
func Compare(a, b []int) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// Known component/channel prefixes stripped before the numeric run. These are
// the real shapes seen in raw_releases (knative-v…, edge-…, chart/…, ce@…);
// the leading semver "v" is handled separately (only when it precedes a digit,
// so a name like "vault-…" isn't mangled).
var componentPrefixes = []string{
	// "lts-" joined "stable-" on 2026-08-10: Flatcar tags every channel the same
	// way, and LTS releases were parsing to nil (see the worker's copy).
	"knative-", "api/", "chart/", "ce@", "edge-", "stable-", "lts-", "release-",
}

// NormalizeVersion returns the version's component ints, or nil when the string
// can't be parsed (e.g. "redirect" — a collection artifact). A nil key means the
// row is simply excluded from range queries.
//
// Examples: "v1.24.3"→[1 24 3], "edge-26.1.4"→[26 1 4], "knative-v1.12.0"→[1 12 0],
// "1.24.3-rc1"→[1 24 3] (trailing pre-release dropped), "redirect"→nil.
func NormalizeVersion(raw string) []int {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}

	// Strip known component/channel prefixes (repeatedly — e.g. "knative-v…").
	for changed := true; changed; {
		changed = false
		for _, p := range componentPrefixes {
			if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
				s = s[len(p):]
				changed = true
			}
		}
	}

	// Strip a single leading v/V only when it precedes a digit (semver "v1.2.3").
	if len(s) >= 2 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		s = s[1:]
	}

	// Take the leading dotted-numeric run; stop at the first component without a
	// leading digit, and stop after a component with a trailing non-digit
	// (so "3-rc1" contributes 3 and ends the key).
	var out []int
	for _, part := range strings.Split(s, ".") {
		i := 0
		for i < len(part) && part[i] >= '0' && part[i] <= '9' {
			i++
		}
		if i == 0 {
			break
		}
		n, err := strconv.Atoi(part[:i])
		if err != nil {
			break
		}
		out = append(out, n)
		if i < len(part) {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
