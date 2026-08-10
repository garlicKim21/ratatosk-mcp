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
// return changes verbatim, so only the fields the tools themselves inspect
// (identity, and what check_stack's brief mode summarizes) get dedicated types.
// Change is one normalized change from /v1 (RFC 20). It replaces the old Fact:
// the server now carries three axes the client used to infer — family (what
// kind of thing), bucket (how to act NOW), and a machine-evaluable applies_if.
//
// check_stack got simpler because of it: "applies to everyone" used to be
// guessed from an empty condition string; now bucket says it outright, and it
// is the SAME rule the website and the weekly email use.
type Change struct {
	ChangeID  string `json:"change_id"`
	MatterKey string `json:"matter_key"`
	Project   string `json:"project"`
	Version   string `json:"version"`
	Family    string `json:"family"`
	Bucket    string `json:"bucket"`
	Kind      string `json:"kind"`
	Quote     string `json:"quote"`
	// Condition is applies_if rendered as one phrase for humans; "" when the
	// change applies unconditionally.
	Condition string `json:"condition"`
	// CondTargets are the identifiers to look for in a live configuration —
	// the structured half of applies_if. Empty when the server could not
	// structure it (Evaluable false), in which case Condition is the sentence.
	CondTargets []string `json:"condition_targets"`
	Evaluable   bool     `json:"condition_evaluable"`
	// Advisories cited by this change, with the ledger's CURRENT severity.
	Advisories []Advisory `json:"advisories"`
	// Window is the deprecation timing when the server knows it.
	Window map[string]string `json:"window"`
	Seq    int               `json:"seq"`
	Raw    json.RawMessage   `json:"-"`
}

type Advisory struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
}

func (c *Change) UnmarshalJSON(b []byte) error {
	type alias struct {
		ChangeID  string `json:"change_id"`
		MatterKey string `json:"matter_key"`
		Project   string `json:"project"`
		Version   string `json:"version"`
		Family    string `json:"family"`
		Bucket    string `json:"bucket"`
		Kind      string `json:"kind"`
		Quote     string `json:"quote"`
		Seq       int    `json:"seq"`
		AppliesIf struct {
			Evaluable bool   `json:"evaluable"`
			Mode      string `json:"mode"`
			Raw       string `json:"raw"`
			Clauses   []struct {
				Kind     string `json:"kind"`
				Name     string `json:"name"`
				Verb     string `json:"verb"`
				Polarity string `json:"polarity"`
			} `json:"clauses"`
		} `json:"applies_if"`
		Advisories []Advisory        `json:"advisories"`
		Window     map[string]string `json:"window"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	c.ChangeID, c.MatterKey = a.ChangeID, a.MatterKey
	c.Project, c.Version = a.Project, a.Version
	c.Family, c.Bucket, c.Kind, c.Quote = a.Family, a.Bucket, a.Kind, a.Quote
	c.Advisories, c.Window, c.Seq = a.Advisories, a.Window, a.Seq
	c.Evaluable = a.AppliesIf.Evaluable
	for _, cl := range a.AppliesIf.Clauses {
		c.CondTargets = append(c.CondTargets, cl.Name)
		c.Condition = joinPhrase(c.Condition, conditionPhrase(cl.Verb, cl.Kind, cl.Name), a.AppliesIf.Mode)
	}
	if c.Condition == "" && a.AppliesIf.Mode != "universal" {
		c.Condition = a.AppliesIf.Raw // 구조화 못 한 조건 — 문장이라도 준다
	}
	c.Raw = append(c.Raw[:0], b...)
	return nil
}

// joinPhrase strings clauses together in the mode the server recorded, so an
// agent reads "A or B" / "A and B" rather than a bare list.
func joinPhrase(acc, next, mode string) string {
	if next == "" {
		return acc
	}
	if acc == "" {
		return next
	}
	sep := " and "
	if mode == "any_of" {
		sep = " or "
	}
	return acc + sep + next
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

func (c Change) MarshalJSON() ([]byte, error) { return c.Raw, nil }

// EffSeverity is what check_stack ranks by: the highest advisory severity the
// change cites. Without an advisory, security-family changes read as "high"
// and everything else falls back to the bucket's urgency — the old per-change
// severity column no longer exists.
func (c Change) EffSeverity() string {
	best := ""
	for _, a := range c.Advisories {
		if severityRank[strings.ToLower(a.Severity)] > severityRank[best] {
			best = strings.ToLower(a.Severity)
		}
	}
	if best != "" {
		return best
	}
	if c.Family == "security" {
		return "high"
	}
	switch c.Bucket {
	case "action":
		return "medium"
	case "check":
		return "low"
	}
	return "info"
}

type changesPage struct {
	Changes   []Change `json:"changes"`
	NextSince *int     `json:"next_since"`
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

// listChanges fetches one page of /v1/changes.
func (c *apiClient) listChanges(ctx context.Context, project, family, bucket string, since, limit int) (*changesPage, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if family != "" {
		q.Set("family", family)
	}
	if bucket != "" {
		q.Set("bucket", bucket)
	}
	if since > 0 {
		q.Set("since", fmt.Sprint(since))
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	var page changesPage
	if err := c.get(ctx, "/v1/changes", "/v1/changes", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// allProjectChanges pages through every change for a project. Only the project
// slug is sent to the server — never the caller's running version.
func (c *apiClient) allProjectChanges(ctx context.Context, project string) ([]Change, error) {
	var all []Change
	since := 0
	for {
		page, err := c.listChanges(ctx, project, "", "", since, 200)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Changes...)
		if page.NextSince == nil {
			return all, nil
		}
		since = *page.NextSince
	}
}

func (c *apiClient) changesByEntity(ctx context.Context, name, kind string) ([]Change, error) {
	q := url.Values{}
	q.Set("name", name)
	if kind != "" {
		q.Set("kind", kind)
	}
	var out struct {
		Changes []Change `json:"changes"`
	}
	if err := c.get(ctx, "/v1/changes/by-entity", "/v1/changes/by-entity", q, &out); err != nil {
		return nil, err
	}
	return out.Changes, nil
}

// getMatter fetches one matter's history across releases. New in RFC 20: the
// same fix lands on several branches carrying different advisory sets, and
// only the full list shows that.
func (c *apiClient) getMatter(ctx context.Context, key string, includeAll bool) (json.RawMessage, error) {
	var raw json.RawMessage
	q := url.Values{}
	if includeAll {
		q.Set("include", "all")
	}
	if err := c.get(ctx, "/v1/matters/{key}", "/v1/matters/"+url.PathEscape(key), q, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// getRelease fetches one reviewed release; an empty versionTag asks the server
// for the project's latest reviewed release.
// listReleases fetches the newest N reviewed releases of a project as light
// summaries — the recency path (v0.4.0): list_changes is an oldest-first sync
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
// changes from an untracked project is never mistaken for audited silence.
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
