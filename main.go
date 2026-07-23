// ratatosk-mcp — in-cluster MCP server PoC (docs/rfc/agent-api-build.md Phase 4).
//
// Exposes the ratatosk agent API v1 as read-only MCP tools over stdio, plus
// check_stack: the RFC's privacy loop ("notify, don't tell") — the caller's
// running versions are compared LOCALLY against fact version keys; only
// project slugs ever reach the server.
//
// Config: RATATOSK_URL (default https://ratatosk.io).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/garlicKim21/ratatosk-mcp/internal/version"
)

var api *apiClient

func main() {
	base := os.Getenv("RATATOSK_URL")
	if base == "" {
		base = "https://ratatosk.io"
	}
	api = newAPIClient(base)

	server := mcp.NewServer(&mcp.Implementation{Name: "ratatosk", Version: "0.3.1"}, &mcp.ServerOptions{
		Instructions: "Data source: the public ratatosk.io agent API — release facts extracted by AI from official " +
			"release notes; verify critical decisions against the source URL in get_release (terms: https://ratatosk.io/terms). " +
			"Upstream is rate-limited to 60 requests/minute per IP; prefer check_stack for stack-wide questions " +
			"over polling list_facts per project.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_facts",
		Description: "Incremental feed of release facts (typed, entity-level changes: security fixes, " +
			"removals, deprecations, renames, defaults) for CNCF/cloud-native projects. " +
			"Ordered by fact_id ascending; poll with since=<last fact_id>. " +
			"Call this to survey recent actionable changes, optionally filtered by project/type/severity.",
	}, listFactsTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "facts_by_entity",
		Description: "Reverse index: every fact touching one exact identifier — a CVE id, CRD, feature gate, " +
			"flag, metric, config field, or dependency. Case-insensitive. " +
			"Call this when you have a specific identifier (e.g. from a manifest or advisory) and want to know what changed around it.",
	}, factsByEntityTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_release",
		Description: "One reviewed release: envelope (coverage, assessment, source URL) plus all its facts. " +
			"facts=[] with coverage=full_reviewed means the release was read and is routine — auditable silence. " +
			"Omit version for the latest reviewed release of the project. " +
			"Set include_raw for the original release note body (raw_notes); when the review is not the full story " +
			"(coverage insufficient, or zero facts) raw_notes is included automatically.",
	}, getReleaseTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_stack",
		Description: "Check the user's running component versions against known facts. Versions are compared " +
			"LOCALLY — only project slugs are sent to the ratatosk server, running versions never leave this process. " +
			"Returns, per component, the facts from releases NEWER than the running version (the upgrade path). " +
			"Default is a briefing: summary counts, critical/high facts in action_required, one line each for the rest, " +
			"and the same advisory fixed on several release branches collapsed into one entry. " +
			"Use detail:\"full\" for every fact verbatim, target_version to limit to one upgrade hop, " +
			"severity_min to filter. Components with zero facts carry tracked:true|false — tracked:false means " +
			"the project is NOT covered by ratatosk, so the absence of facts is no-coverage, not safety. " +
			"In brief mode, facts sharing one quoted sentence are merged with their ids listed together. " +
			"Drill down with get_release or facts_by_entity.",
	}, checkStackTool)

	// Two transports, one binary: stdio for local agents (Claude Code, MCP
	// inspector), streamable HTTP for the in-cluster Service the Helm chart
	// deploys — agents in the cluster connect to http://<svc>:8080/mcp.
	if addr := os.Getenv("MCP_HTTP_ADDR"); addr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
		mux := http.NewServeMux()
		mux.Handle("/mcp", handler)
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		log.Printf("ratatosk-mcp: streamable HTTP on %s/mcp (upstream %s)", addr, base)
		log.Fatal(http.ListenAndServe(addr, mux))
	}
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// jsonResult marshals compactly: tool output is read by agents, and the
// indentation whitespace alone was ~40% of a large check_stack response.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

// ---------------------------------------------------------------------------
// list_facts
// ---------------------------------------------------------------------------

type listFactsArgs struct {
	Project  string `json:"project,omitempty" jsonschema:"project slug filter, e.g. envoy, istio, cilium"`
	Type     string `json:"type,omitempty" jsonschema:"fact_type filter: security_fix|dependency_bump|capability_removed|capability_deprecated|api_version_changed|identifier_renamed|validation_tightened|default_changed|behavior_changed"`
	Severity string `json:"severity,omitempty" jsonschema:"info|low|medium|high|critical"`
	Since    int    `json:"since,omitempty" jsonschema:"cursor: return facts with fact_id greater than this"`
	Limit    int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

func listFactsTool(ctx context.Context, req *mcp.CallToolRequest, args listFactsArgs) (*mcp.CallToolResult, any, error) {
	page, err := api.listFacts(args.Project, args.Type, args.Severity, args.Since, args.Limit)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(page)
	return res, nil, err
}

// ---------------------------------------------------------------------------
// facts_by_entity
// ---------------------------------------------------------------------------

type factsByEntityArgs struct {
	Name string `json:"name" jsonschema:"exact identifier to look up: CVE id, CRD, feature gate, flag, metric, config field, dependency"`
	Kind string `json:"kind,omitempty" jsonschema:"optional: api|crd|feature_gate|flag|metric|config_field|extension|dependency|cve|advisory|subsystem"`
}

func factsByEntityTool(ctx context.Context, req *mcp.CallToolRequest, args factsByEntityArgs) (*mcp.CallToolResult, any, error) {
	if args.Name == "" {
		return errResult(fmt.Errorf("name is required")), nil, nil
	}
	facts, err := api.factsByEntity(args.Name, args.Kind)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(map[string]any{"facts": facts})
	return res, nil, err
}

// ---------------------------------------------------------------------------
// get_release
// ---------------------------------------------------------------------------

type getReleaseArgs struct {
	Project    string `json:"project" jsonschema:"project slug, e.g. envoy"`
	Version    string `json:"version,omitempty" jsonschema:"release tag exactly as published, e.g. v1.38.3; omit for the latest reviewed release"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"also return the original release note body as raw_notes — judge from the source instead of the extracted facts"`
}

func getReleaseTool(ctx context.Context, req *mcp.CallToolRequest, args getReleaseArgs) (*mcp.CallToolResult, any, error) {
	if args.Project == "" {
		return errResult(fmt.Errorf("project is required")), nil, nil
	}
	raw, err := api.getRelease(args.Project, args.Version, args.IncludeRaw)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

// ---------------------------------------------------------------------------
// check_stack — the privacy loop
// ---------------------------------------------------------------------------

type stackComponent struct {
	Project       string `json:"project" jsonschema:"project slug, e.g. envoy"`
	Version       string `json:"version" jsonschema:"the version currently running, e.g. v1.36.8"`
	TargetVersion string `json:"target_version,omitempty" jsonschema:"optional upgrade destination: only facts with running < version <= target are returned"`
}

type checkStackArgs struct {
	Components  []stackComponent `json:"components" jsonschema:"the running stack to check"`
	SeverityMin string           `json:"severity_min,omitempty" jsonschema:"only facts at or above this severity: info|low|medium|high|critical"`
	Detail      string           `json:"detail,omitempty" jsonschema:"brief (default): summary + critical/high facts + one-liners for the rest; full: every fact verbatim"`
}

var severityRank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// briefFact is one upgrade-path fact compressed to what an agent decides on.
// Evidence entities and URLs live one get_release call away.
type briefFact struct {
	FactID    int      `json:"fact_id"`
	Version   string   `json:"version"`
	FactType  string   `json:"fact_type"`
	Severity  string   `json:"severity"`
	Mandatory bool     `json:"mandatory,omitempty"`
	Condition string   `json:"applies_if,omitempty"` // empty = applies unconditionally
	ConditionsAny []string `json:"applies_if_any,omitempty"` // merged entry with diverging conditions
	Quote     string   `json:"quote,omitempty"`
	IDs       []string `json:"ids,omitempty"`
	AlsoIn    []string `json:"same_issue_also_addressed_in,omitempty"`
}

type briefSummary struct {
	NewFacts       int            `json:"new_facts"`
	DistinctIssues int            `json:"distinct_issues"`
	Mandatory      int            `json:"mandatory"`
	BySeverity     map[string]int `json:"by_severity"`
	ByType         map[string]int `json:"by_type"`
}

type componentReport struct {
	Project        string        `json:"project"`
	RunningVersion string        `json:"running_version"`
	TargetVersion  string        `json:"target_version,omitempty"`
	Tracked        *bool         `json:"tracked,omitempty"` // set only on zero-fact components
	Note           string        `json:"note,omitempty"`
	FactsScanned   int           `json:"facts_scanned"`
	Summary        *briefSummary `json:"summary,omitempty"`
	ActionRequired []briefFact   `json:"action_required,omitempty"`
	OtherFacts     []briefFact   `json:"other_facts,omitempty"`
	OtherOmitted   int           `json:"other_facts_omitted,omitempty"`
	RelevantFacts  []Fact        `json:"relevant_facts,omitempty"` // detail:"full" only
}

// otherFactsCap bounds the one-liner tail of a brief report; whatever is cut
// is counted in other_facts_omitted — never dropped silently.
const otherFactsCap = 100

func checkStackTool(ctx context.Context, req *mcp.CallToolRequest, args checkStackArgs) (*mcp.CallToolResult, any, error) {
	if len(args.Components) == 0 {
		return errResult(fmt.Errorf("components is required")), nil, nil
	}
	minRank := 0
	if args.SeverityMin != "" {
		r, ok := severityRank[strings.ToLower(args.SeverityMin)]
		if !ok {
			return errResult(fmt.Errorf("severity_min must be one of info|low|medium|high|critical")), nil, nil
		}
		minRank = r
	}
	var full bool
	switch strings.ToLower(args.Detail) {
	case "", "brief":
	case "full":
		full = true
	default:
		return errResult(fmt.Errorf(`detail must be "brief" or "full"`)), nil, nil
	}

	reports := make([]componentReport, 0, len(args.Components))
	for _, comp := range args.Components {
		rep := componentReport{Project: comp.Project, RunningVersion: comp.Version, TargetVersion: comp.TargetVersion}
		var notes []string
		currentKey := version.NormalizeVersion(comp.Version)
		if currentKey == nil {
			rep.Note = "running version could not be parsed; showing no facts (use list_facts to inspect manually)"
			reports = append(reports, rep)
			continue
		}
		var targetKey []int
		if comp.TargetVersion != "" {
			if targetKey = version.NormalizeVersion(comp.TargetVersion); targetKey == nil {
				notes = append(notes, "target_version could not be parsed; ignored")
			}
		}
		facts, err := api.allProjectFacts(comp.Project)
		if err != nil {
			rep.Note = "fetch failed: " + err.Error()
			reports = append(reports, rep)
			continue
		}
		rep.FactsScanned = len(facts)
		// Zero facts is ambiguous: audited silence (tracked, routine releases)
		// or no coverage at all (unknown slug). Probe and say which — an agent
		// must never read "not tracked" as "safe" (2026-07-23 kagent finding).
		if len(facts) == 0 {
			if tracked, perr := api.projectTracked(comp.Project); perr != nil {
				notes = append(notes, "tracking probe failed: "+perr.Error())
			} else {
				rep.Tracked = &tracked
				if tracked {
					notes = append(notes, "tracked by ratatosk; no facts on record — releases so far were routine")
				} else {
					notes = append(notes, "NOT tracked by ratatosk — zero facts means no coverage here, not safety")
				}
			}
		}

		// Facts from releases strictly newer than the running version =
		// the upgrade path the operator has not yet absorbed.
		unparseable := 0
		var relevant []Fact
		var keys [][]int
		for _, f := range facts {
			factKey := version.NormalizeVersion(f.Version)
			if factKey == nil {
				unparseable++
				continue
			}
			if version.Compare(factKey, currentKey) <= 0 {
				continue
			}
			if targetKey != nil && version.Compare(factKey, targetKey) > 0 {
				continue
			}
			if severityRank[strings.ToLower(f.Severity)] < minRank {
				continue
			}
			relevant = append(relevant, f)
			keys = append(keys, factKey)
		}
		if unparseable > 0 {
			notes = append(notes, fmt.Sprintf("%d fact(s) skipped (unparseable release tag)", unparseable))
		}
		rep.Note = strings.Join(notes, "; ")

		if full {
			rep.RelevantFacts = relevant
			if rep.RelevantFacts == nil {
				rep.RelevantFacts = []Fact{}
			}
			reports = append(reports, rep)
			continue
		}
		rep.Summary, rep.ActionRequired, rep.OtherFacts = briefReport(relevant, keys)
		if extra := len(rep.OtherFacts) - otherFactsCap; extra > 0 {
			rep.OtherFacts = rep.OtherFacts[:otherFactsCap]
			rep.OtherOmitted = extra
		}
		reports = append(reports, rep)
	}

	out := map[string]any{
		"components": reports,
		"privacy":    "versions were compared locally; only project slugs were sent to the server",
	}
	if !full {
		out["hint"] = "briefing: critical/high facts are in action_required, the rest one line each in other_facts; " +
			"facts sharing one quoted sentence are merged (ids listed together, applies_if_any when conditions differ); " +
			"the same advisory fixed on several release branches is one entry (same_issue_also_addressed_in). " +
			`Call again with detail:"full" for every fact verbatim, or drill down: get_release(project, version) ` +
			"for one release with evidence and source URL, facts_by_entity(name) for one CVE/flag/CRD."
	}
	res, err := jsonResult(out)
	return res, nil, err
}

// briefReport compresses the upgrade path: sorted by version, the same
// advisory (falling back to issue_key when no advisory is attached) on
// several branches collapsed into its earliest fix (an operator upgrades
// once — the nearest release that fixes the issue is the actionable one),
// then split into action_required (critical/high) and one-liner rest.
// Extraction judges each release note independently, so the same advisory
// can carry different severities across branches — a collapsed entry shows
// the strongest judgment in its group.
func briefReport(relevant []Fact, keys [][]int) (*briefSummary, []briefFact, []briefFact) {
	idx := make([]int, len(relevant))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		if c := version.Compare(keys[idx[a]], keys[idx[b]]); c != 0 {
			return c < 0
		}
		return relevant[idx[a]].FactID < relevant[idx[b]].FactID
	})

	sum := &briefSummary{
		NewFacts:   len(relevant),
		BySeverity: map[string]int{},
		ByType:     map[string]int{},
	}
	byIssue := map[string]*briefFact{}
	var ordered []*briefFact
	for _, i := range idx {
		f := relevant[i]
		key := f.GroupKey
		if key == "" {
			key = f.IssueKey
		}
		if key != "" {
			if kept, ok := byIssue[key]; ok {
				if f.Version != kept.Version && !slices.Contains(kept.AlsoIn, f.Version) {
					kept.AlsoIn = append(kept.AlsoIn, f.Version)
				}
				if severityRank[strings.ToLower(f.Severity)] > severityRank[strings.ToLower(kept.Severity)] {
					kept.Severity = f.Severity
				}
				if f.Mandatory {
					kept.Mandatory = true
				}
				continue
			}
		}
		bf := &briefFact{
			FactID:    f.FactID,
			Version:   f.Version,
			FactType:  f.FactType,
			Severity:  f.Severity,
			Mandatory: f.Mandatory,
			Condition: f.Condition,
			Quote:     f.Quote,
			IDs:       f.RefIDs,
		}
		if key != "" {
			byIssue[key] = bf
		}
		ordered = append(ordered, bf)
		sum.ByType[f.FactType]++
	}
	sum.DistinctIssues = len(ordered)
	// severity/mandatory counts reflect the collapsed entries, whose severity
	// may have been raised by a later branch's judgment
	for _, bf := range ordered {
		sum.BySeverity[bf.Severity]++
		if bf.Mandatory {
			sum.Mandatory++
		}
	}

	action := []briefFact{}
	other := []briefFact{}
	for _, bf := range mergeSharedQuotes(ordered) {
		if r := severityRank[strings.ToLower(bf.Severity)]; r >= severityRank["high"] {
			action = append(action, *bf)
		} else {
			other = append(other, *bf)
		}
	}
	return sum, action, other
}

// mergeSharedQuotes folds brief entries that carry the same (version, quote):
// one extraction sentence covering several ids is stored as one fact per id,
// and brief mode must not repeat that sentence per id (observed live: envoy
// v1.39.0 repeated one quote seven times). ids concatenate, severity shows the
// strongest, and diverging applies_if conditions move to applies_if_any.
// Quote-less entries are never merged. Summary counts stay per distinct issue —
// this is presentation-level folding only.
func mergeSharedQuotes(ordered []*briefFact) []*briefFact {
	byQuote := map[string]*briefFact{}
	out := make([]*briefFact, 0, len(ordered))
	for _, bf := range ordered {
		if bf.Quote == "" {
			out = append(out, bf)
			continue
		}
		k := bf.Version + "\x00" + bf.Quote
		kept, ok := byQuote[k]
		if !ok {
			byQuote[k] = bf
			out = append(out, bf)
			continue
		}
		for _, id := range bf.IDs {
			if !slices.Contains(kept.IDs, id) {
				kept.IDs = append(kept.IDs, id)
			}
		}
		for _, v := range bf.AlsoIn {
			if !slices.Contains(kept.AlsoIn, v) {
				kept.AlsoIn = append(kept.AlsoIn, v)
			}
		}
		if severityRank[strings.ToLower(bf.Severity)] > severityRank[strings.ToLower(kept.Severity)] {
			kept.Severity = bf.Severity
		}
		if bf.Mandatory {
			kept.Mandatory = true
		}
		if bf.Condition != kept.Condition || len(kept.ConditionsAny) > 0 {
			if kept.Condition != "" && !slices.Contains(kept.ConditionsAny, kept.Condition) {
				kept.ConditionsAny = append(kept.ConditionsAny, kept.Condition)
			}
			if bf.Condition != "" && !slices.Contains(kept.ConditionsAny, bf.Condition) {
				kept.ConditionsAny = append(kept.ConditionsAny, bf.Condition)
			}
			kept.Condition = ""
		}
	}
	return out
}
