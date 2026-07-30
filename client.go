package main

// Thin client for the ratatosk agent API v1 (docs/rfc/agent-api-build.md §3.2).
// Read-only; the base URL is the only configuration. Types mirror the /v1
// envelope loosely — unknown fields are ignored so the server can evolve.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	http    *http.Client
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Fact keeps the envelope as raw-ish JSON-friendly structs; the MCP tools
// return facts verbatim, so only the fields the tools themselves inspect
// (identity, and what check_stack's brief mode summarizes) get dedicated types.
type Fact struct {
	FactID    int             `json:"fact_id"`
	Project   string          `json:"project"`
	Version   string          `json:"version"`
	FactType  string          `json:"fact_type"`
	Severity  string          `json:"severity"`
	Mandatory bool            `json:"mandatory"`
	IssueKey  string          `json:"issue_key"`
	GroupKey  string          `json:"advisory_group_key"`
	// GroupSeverity is the server's storage-layer maximum across every release
	// sharing the group key (an advisory id, or a CVE id when no advisory is
	// cited) — broader than any locally visible fold (it includes branches
	// outside the current query). Empty on facts with neither id and on older
	// servers.
	GroupSeverity string `json:"group_severity"`
	Quote     string          `json:"quote"`
	RefIDs    []string        `json:"ids"`
	Condition string          `json:"condition"` // applies_if as one phrase; "" = unconditional
	// CondKind/CondTarget survive from a structured applies_if so an agent can
	// look the thing up in a live config instead of parsing the phrase. Empty
	// when the server only had a fallback sentence.
	CondKind   string `json:"condition_kind"`
	CondTarget string `json:"condition_target"`
	// The version that ends the exposure. FixedIn is what turns a condition
	// into a precondition — "before you enable X, be on at least this".
	FixedIn      string          `json:"fixed_in"`
	RemovedIn    string          `json:"removed_in"`
	DeprecatedIn string          `json:"deprecated_in"`
	Raw          json.RawMessage `json:"-"`
}

func (f *Fact) UnmarshalJSON(b []byte) error {
	type alias struct {
		FactID    int    `json:"fact_id"`
		Project   string `json:"project"`
		Version   string `json:"version"`
		FactType  string `json:"fact_type"`
		Severity  string `json:"severity"`
		Mandatory bool   `json:"mandatory"`
		IssueKey      string `json:"issue_key"`
		GroupKey      string `json:"advisory_group_key"`
		GroupSeverity string `json:"group_severity"`
		AppliesIf struct {
			Status     string `json:"status"`
			Verb       string `json:"verb"`
			TargetKind string `json:"target_kind"`
			TargetName string `json:"target_name"`
			Fallback   string `json:"fallback"`
		} `json:"applies_if"`
		Affected struct {
			FixedIn      string `json:"fixed_in"`
			RemovedIn    string `json:"removed_in"`
			DeprecatedIn string `json:"deprecated_in"`
		} `json:"affected"`
		References struct {
			IDs   []string `json:"ids"`
			Quote string   `json:"quote"`
		} `json:"references"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	f.FactID, f.Project, f.Version = a.FactID, a.Project, a.Version
	f.FactType, f.Severity, f.Mandatory = a.FactType, a.Severity, a.Mandatory
	f.IssueKey, f.GroupKey, f.GroupSeverity = a.IssueKey, a.GroupKey, a.GroupSeverity
	f.Quote, f.RefIDs = a.References.Quote, a.References.IDs
	f.FixedIn, f.RemovedIn, f.DeprecatedIn = a.Affected.FixedIn, a.Affected.RemovedIn, a.Affected.DeprecatedIn
	switch a.AppliesIf.Status {
	case "structured":
		f.Condition = conditionPhrase(a.AppliesIf.Verb, a.AppliesIf.TargetKind, a.AppliesIf.TargetName)
		f.CondKind, f.CondTarget = a.AppliesIf.TargetKind, a.AppliesIf.TargetName
	case "degraded":
		f.Condition = a.AppliesIf.Fallback
	}
	f.Raw = append(f.Raw[:0], b...)
	return nil
}

// conditionPhrase renders the stored (verb, kind, name) triple as English.
// The kind is our storage enum, and joining the three raw put it on display:
// "configures config_field per-upstream read timeout" is what a live kagent
// transcript fed its model on 2026-07-28. Unknown kinds fall through to the
// bare name, so a new kind on the server reads plainly instead of leaking.
func conditionPhrase(verb, kind, name string) string {
	verb = strings.ReplaceAll(strings.TrimSpace(verb), "_", " ")
	name = strings.TrimSpace(name)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if name == "" {
		return strings.TrimSpace(verb + " " + strings.ReplaceAll(kind, "_", " "))
	}
	var target string
	switch kind {
	case "flag":
		target = "the " + name + " flag"
	case "feature_gate":
		target = "the " + name + " feature gate"
	case "config_field":
		target = "the " + name + " setting"
	case "crd":
		target = "the " + name + " CRD"
	case "api":
		target = "the " + name + " API"
	case "extension":
		target = "the " + name + " extension"
	case "metric":
		target = "the " + name + " metric"
	default: // subsystem, dependency, and whatever the server adds next
		target = name
	}
	return strings.TrimSpace(verb + " " + target)
}

// MarshalJSON re-emits the verbatim envelope the API returned.
func (f Fact) MarshalJSON() ([]byte, error) { return f.Raw, nil }

// EffSeverity is the severity check_stack reasons with: the server's advisory
// group maximum when present, else the fact's own severity.
func (f Fact) EffSeverity() string {
	if f.GroupSeverity != "" {
		return f.GroupSeverity
	}
	return f.Severity
}

type factsPage struct {
	Facts     []Fact `json:"facts"`
	NextSince *int   `json:"next_since"`
}

// get fetches one /v1 resource. endpoint is the log-safe route PATTERN
// ("/v1/releases/{project}/{version}") — path itself can embed a running
// version, so it appears in errors returned to the agent (which needs the
// detail) but never in a log field (the invariant).
func (c *apiClient) get(ctx context.Context, endpoint, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ratatosk-mcp/"+buildVersion)
	if tp := traceparentFrom(ctx); tp != "" {
		req.Header.Set("traceparent", tp)
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "upstream fetch failed", "upstream", endpoint, "kind", errKind(err))
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		slog.ErrorContext(ctx, "upstream body read failed", "upstream", endpoint, "kind", errKind(err))
		return err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		slog.DebugContext(ctx, "upstream ok", "upstream", endpoint, "status", resp.StatusCode, "ms", time.Since(start).Milliseconds())
	case resp.StatusCode == http.StatusTooManyRequests:
		slog.WarnContext(ctx, "upstream rate limited", "upstream", endpoint, "status", resp.StatusCode)
	case resp.StatusCode >= 500:
		slog.ErrorContext(ctx, "upstream error", "upstream", endpoint, "status", resp.StatusCode)
	default:
		// A client mistake (unknown slug, bad params) is answered with guidance
		// by the tool itself — that answer IS the handling. DEBUG, never ERROR:
		// one confused agent's typo loop must not page an operator.
		slog.DebugContext(ctx, "upstream rejected request", "upstream", endpoint, "status", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		// 400 chars so self-correcting errors survive intact — the /v1 404 now
		// carries the project's recent reviewed tags for the agent to retry with.
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 400))
	}
	return json.Unmarshal(body, out)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// listFacts fetches one page of /v1/facts.
func (c *apiClient) listFacts(ctx context.Context, project, factType, severity string, since, limit int) (*factsPage, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if factType != "" {
		q.Set("type", factType)
	}
	if severity != "" {
		q.Set("severity", severity)
	}
	if since > 0 {
		q.Set("since", fmt.Sprint(since))
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	var page factsPage
	if err := c.get(ctx, "/v1/facts", "/v1/facts", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// allProjectFacts pages through every fact for a project. Only the project
// slug is sent to the server — never the caller's running version.
func (c *apiClient) allProjectFacts(ctx context.Context, project string) ([]Fact, error) {
	var all []Fact
	since := 0
	for {
		page, err := c.listFacts(ctx, project, "", "", since, 200)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Facts...)
		if page.NextSince == nil {
			return all, nil
		}
		since = *page.NextSince
	}
}

func (c *apiClient) factsByEntity(ctx context.Context, name, kind string) ([]Fact, error) {
	q := url.Values{}
	q.Set("name", name)
	if kind != "" {
		q.Set("kind", kind)
	}
	var out struct {
		Facts []Fact `json:"facts"`
	}
	if err := c.get(ctx, "/v1/facts/by-entity", "/v1/facts/by-entity", q, &out); err != nil {
		return nil, err
	}
	return out.Facts, nil
}

// getRelease fetches one reviewed release; an empty versionTag asks the server
// for the project's latest reviewed release.
// listReleases fetches the newest N reviewed releases of a project as light
// summaries — the recency path (v0.4.0): list_facts is an oldest-first sync
// feed, so "recent releases of X" questions must not be answered from it.
func (c *apiClient) listReleases(ctx context.Context, project string, limit int) (json.RawMessage, error) {
	var raw json.RawMessage
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if err := c.get(ctx, "/v1/releases/{project}", "/v1/releases/"+url.PathEscape(project), q, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *apiClient) getRelease(ctx context.Context, project, versionTag string, includeRaw bool) (json.RawMessage, error) {
	var raw json.RawMessage
	path := "/v1/releases/" + url.PathEscape(project)
	endpoint := "/v1/releases/{project}"
	if versionTag != "" {
		path += "/" + url.PathEscape(versionTag)
		endpoint = "/v1/releases/{project}/{version}"
	}
	var q url.Values
	if includeRaw {
		q = url.Values{"include": {"raw"}}
	}
	if err := c.get(ctx, endpoint, path, q, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// projectTracked probes the version-less latest-release route (v0.3.0):
// 200 = tracked project, 404 = unknown slug. check_stack uses it so zero
// facts from an untracked project is never mistaken for audited silence.
func (c *apiClient) projectTracked(ctx context.Context, project string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/releases/"+url.PathEscape(project), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "ratatosk-mcp/"+buildVersion)
	if tp := traceparentFrom(ctx); tp != "" {
		req.Header.Set("traceparent", tp)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "upstream fetch failed", "upstream", "/v1/releases/{project}", "kind", errKind(err))
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		// The probe's answer, not a failure: 404 here MEANS "not tracked".
		return false, nil
	default:
		if resp.StatusCode >= 500 {
			slog.ErrorContext(ctx, "upstream error", "upstream", "/v1/releases/{project}", "status", resp.StatusCode)
		}
		return false, fmt.Errorf("tracking probe: HTTP %d", resp.StatusCode)
	}
}

// listProjects fetches the tracked-project roster — the canonical slug list
// agents resolve against before check_stack/get_release (v0.3.2 slug-discovery
// gap fix: kagent guessed slugs and only learned via tracked:false).
func (c *apiClient) listProjects(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/v1/projects", "/v1/projects", nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
