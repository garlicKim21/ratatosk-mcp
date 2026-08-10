// Package version turns a raw release version string into a monotonic,
// per-project sort key — an int component array — for the agent API's range
// queries (docs/rfc/agent-api.md §5.7). Comparison is only meaningful WITHIN a
// single RELEASE LINE: across projects it is nonsense, and across lines of one
// project (containerd's "api/" beside its main tags, Flatcar's lts beside
// stable) it is nonsense that succeeds — see SplitLine. Callers that compare
// must gate on SameLine first; the key alone cannot tell them apart.
//
// COPY of apps/worker/internal/version (source of truth) — bundled here so the
// in-cluster MCP server compares versions client-side and never sends them to
// the server (RFC §5.7 privacy). Keep in sync when the worker's copy changes.
package version

import (
	"strconv"
	"strings"
)

// Known component/channel prefixes stripped before the numeric run. These are
// the real shapes seen in raw_releases (knative-v…, edge-…, chart/…, ce@…);
// the leading semver "v" is handled separately (only when it precedes a digit,
// so a name like "vault-…" isn't mangled).
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

// SplitLine separates a release-line prefix from the version that follows it.
//
// Several tracked repositories publish more than one independent product or
// channel and say which in the tag: containerd ships "api/v1.11.1" beside
// "v2.2.5", openfeature ships "flagd/v0.16.1" beside "core/v0.15.0", Flatcar
// ships "lts-4081.3.9" beside "stable-4593.2.4". Those are separate version
// lines. Comparing across them is meaningless — and worse than meaningless
// when it succeeds, because the answer looks like an upgrade path
// (containerd v1.7.28 was being offered api/v1.11.0 as newer, 2026-08-10).
//
// The line is data the maintainer already wrote down, so read it instead of
// keeping a hand-maintained prefix list. The old list was the bug: "stable-"
// was on it and "lts-" was not, so Flatcar's LTS train parsed to nil and
// dropped out of every range query.
//
// The rule: the longest prefix ending in "/", "@" or "-" whose remainder still
// looks like a version. Longest wins so "flagd-proxy/v0.9.7" yields
// "flagd-proxy" rather than "flagd". A tag with no such prefix is the main
// line and returns "".
//
// Projects that prefix every tag the same way (knative-v…, edge-…) end up with
// one line for the whole project, so a same-line comparison behaves exactly as
// before — the rule is self-correcting where there is nothing to separate.
func SplitLine(raw string) (line, rest string) {
	s := strings.TrimSpace(raw)
	best := -1
	for i := 0; i < len(s); i++ {
		if c := s[i]; c != '/' && c != '@' && c != '-' {
			continue
		}
		if looksLikeVersion(s[i+1:]) {
			best = i
		}
	}
	if best < 0 {
		return "", s
	}
	return s[:best], s[best+1:]
}

// Line is SplitLine's prefix alone — "" for the main line.
func Line(raw string) string {
	line, _ := SplitLine(raw)
	return line
}

// SameLine reports whether two raw tags belong to the same release line.
// Comparison between different lines is not merely wrong, it is undefined:
// there is no order between "flagd" and "core".
func SameLine(a, b string) bool { return Line(a) == Line(b) }

// looksLikeVersion reports whether s opens with a version number, allowing the
// semver "v". It decides where a line prefix ends.
func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	return s != "" && s[0] >= '0' && s[0] <= '9'
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

	// Drop the release-line prefix, whatever it is (SplitLine). The key orders
	// versions WITHIN one line; which line a tag belongs to is a separate
	// question that SameLine answers, and callers that compare must ask it —
	// two lines have no order between them to encode here.
	_, s = SplitLine(s)

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
