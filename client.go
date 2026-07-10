package main

// Thin client for the ratatosk agent API v1 (docs/rfc/agent-api-build.md §3.2).
// Read-only; the base URL is the only configuration. Types mirror the /v1
// envelope loosely — unknown fields are ignored so the server can evolve.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	Quote     string          `json:"quote"`
	RefIDs    []string        `json:"ids"`
	Condition string          `json:"condition"` // applies_if as one phrase; "" = unconditional
	Raw       json.RawMessage `json:"-"`
}

func (f *Fact) UnmarshalJSON(b []byte) error {
	type alias struct {
		FactID    int    `json:"fact_id"`
		Project   string `json:"project"`
		Version   string `json:"version"`
		FactType  string `json:"fact_type"`
		Severity  string `json:"severity"`
		Mandatory bool   `json:"mandatory"`
		IssueKey  string `json:"issue_key"`
		AppliesIf struct {
			Status     string `json:"status"`
			Verb       string `json:"verb"`
			TargetKind string `json:"target_kind"`
			TargetName string `json:"target_name"`
			Fallback   string `json:"fallback"`
		} `json:"applies_if"`
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
	f.IssueKey = a.IssueKey
	f.Quote, f.RefIDs = a.References.Quote, a.References.IDs
	switch a.AppliesIf.Status {
	case "structured":
		f.Condition = strings.TrimSpace(strings.Join([]string{a.AppliesIf.Verb, a.AppliesIf.TargetKind, a.AppliesIf.TargetName}, " "))
	case "degraded":
		f.Condition = a.AppliesIf.Fallback
	}
	f.Raw = append(f.Raw[:0], b...)
	return nil
}

// MarshalJSON re-emits the verbatim envelope the API returned.
func (f Fact) MarshalJSON() ([]byte, error) { return f.Raw, nil }

type factsPage struct {
	Facts     []Fact `json:"facts"`
	NextSince *int   `json:"next_since"`
}

func (c *apiClient) get(path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ratatosk-mcp/0.2")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 200))
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
func (c *apiClient) listFacts(project, factType, severity string, since, limit int) (*factsPage, error) {
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
	if err := c.get("/v1/facts", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// allProjectFacts pages through every fact for a project. Only the project
// slug is sent to the server — never the caller's running version.
func (c *apiClient) allProjectFacts(project string) ([]Fact, error) {
	var all []Fact
	since := 0
	for {
		page, err := c.listFacts(project, "", "", since, 200)
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

func (c *apiClient) factsByEntity(name, kind string) ([]Fact, error) {
	q := url.Values{}
	q.Set("name", name)
	if kind != "" {
		q.Set("kind", kind)
	}
	var out struct {
		Facts []Fact `json:"facts"`
	}
	if err := c.get("/v1/facts/by-entity", q, &out); err != nil {
		return nil, err
	}
	return out.Facts, nil
}

func (c *apiClient) getRelease(project, versionTag string, includeRaw bool) (json.RawMessage, error) {
	var raw json.RawMessage
	path := "/v1/releases/" + url.PathEscape(project) + "/" + url.PathEscape(versionTag)
	var q url.Values
	if includeRaw {
		q = url.Values{"include": {"raw"}}
	}
	if err := c.get(path, q, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
