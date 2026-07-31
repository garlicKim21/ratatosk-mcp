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
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/garlicKim21/ratatosk-mcp/internal/version"
)

// buildVersion is stamped by the release workflow via
// -ldflags "-X main.buildVersion=<tag>"; local builds report "dev".
var buildVersion = "dev"

var api *apiClient

func main() {
	setupLogging()
	initAudit()
	base := os.Getenv("RATATOSK_URL")
	if base == "" {
		base = "https://ratatosk.io"
	}
	api = newAPIClient(base)

	server := mcp.NewServer(&mcp.Implementation{Name: "ratatosk", Version: buildVersion}, &mcp.ServerOptions{
		Instructions: "Data source: the public ratatosk.io agent API — release facts extracted by AI from official " +
			"release notes; verify critical decisions against the source URL in get_release (terms: https://ratatosk.io/terms). " +
			"Upstream is rate-limited to 60 requests/minute per caller (a hosted endpoint shares one caller budget); " +
			"prefer check_stack for stack-wide questions " +
			"over polling list_facts per project.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_projects",
		Description: "Every project ratatosk tracks: slug (the canonical id all other tools take), name, " +
			"tier (graduated|incubating), category, analyzed_releases; image_aliases where a project runs " +
			"under other names in clusters (an image or workload matching an alias belongs to that project " +
			"at the version its tag says), and cluster_core:true on the cluster substrate (control plane, " +
			"datastore, DNS, runtime, CNI/dataplane) — every cluster_core project present in a cluster " +
			"belongs in its check_stack call. Some cluster_core entries carry a visibility hint (how the " +
			"component is observed and where it can legitimately be unreadable — e.g. etcd may live outside " +
			"the k8s API): an unreadable one is reported as unchecked, never guessed. Small response, no arguments — " +
			"call this FIRST when you are unsure of a slug instead of guessing (a wrong slug shows up " +
			"as tracked:false in check_stack).",
	}, listProjectsTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_facts",
		Description: "Incremental SYNC feed of release facts (typed, entity-level changes: security fixes, " +
			"removals, deprecations, renames, defaults) for CNCF/cloud-native projects. " +
			"Ordered by fact_id ascending — OLDEST analyzed first, so a single page is NOT the newest data; " +
			"page through with since=<returned next_since> until next_since comes back null. " +
			"Built for keeping a local copy up to date. For 'what is the latest release of X' or " +
			"'recent releases of X', use get_release (omit version for the newest) instead. " +
			"Optionally filter by project/type/severity. " +
			"Facts citing an upstream security advisory carry advisory_group_key (the official notice id, " +
			"e.g. GHSA-… on GitHub; facts citing only CVE ids get a cve:… key) and group_severity — the " +
			"maximum severity across all releases sharing that key — the group-maximum reading, " +
			"with per-release severity as the per-release evidence.",
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
			"version is accepted with or without the leading 'v' (projects disagree on the spelling); " +
			"a wrong tag returns an error listing the project's recent reviewed tags — retry with one of those. " +
			"Set include_raw for the original release note body (raw_notes); when the review is not the full story " +
			"(coverage insufficient, or zero facts) raw_notes is included automatically.",
	}, getReleaseTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_releases",
		Description: "The newest N reviewed releases of one project, as light summaries (version, date, " +
			"coverage, fact counts by severity, max advisory-group severity). THE tool for " +
			"'recent releases of X' / 'what changed in X lately' — newest first, unlike the " +
			"list_facts sync feed which returns oldest-analyzed first. facts_total=0 with " +
			"coverage=full_reviewed means the release was read and is routine (auditable silence). " +
			"Drill into a row with get_release(project, version) for the full facts.",
	}, listReleasesTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_stack",
		Description: "Check the user's running component versions against known facts. Versions are compared " +
			"INSIDE THIS SERVER PROCESS — only project slugs are sent upstream, and this tool never calls the " +
			"server-side /v1/upgrade endpoint. Run the server yourself and running versions never leave your " +
			"infrastructure; on the hosted endpoint they transit server memory only and are not logged. " +
			"Returns, per component, the facts from releases NEWER than the running version (the upgrade path). " +
			"Default is a briefing: summary counts, then critical/high facts split by whether the caller still has to " +
			"check something — action_required applies to everyone, check_config applies only if its applies_if holds " +
			"against the running configuration (resolve it before recommending; an unmet condition is not a reason to " +
			"upgrade, it is a precondition for later — fixed_in is the minimum version to be on before enabling that " +
			"feature). One line each for the rest, " +
			"and the same advisory fixed on several release branches collapsed into one entry " +
			"(shown at the advisory's group-maximum severity). " +
			"Pass version_source per component (where you read the version — e.g. a daemonset image tag, or that the user "+
			"stated it): it is echoed back as an audit trail. This server cannot see your environment, so it cannot "+
			"verify a version or its source; a running version older than every release on record is flagged in note, "+
			"which is the only cross-check available here. "+
			"Use detail:\"full\" for every fact verbatim (capped at 50 per component with relevant_facts_omitted — narrow with severity_min or target_version), " +
			"target_version to limit to one upgrade hop, " +
			"severity_min to filter. Components with zero facts carry tracked:true|false — tracked:false means " +
			"the project is NOT covered by ratatosk, so the absence of facts is no-coverage, not safety. " +
			"In brief mode, facts sharing one quoted sentence are merged with their ids listed together. " +
			"Drill down with get_release or facts_by_entity.",
	}, checkStackTool)

	// Audit stream (P3): observes tools/call when MCP_AUDIT is set; the
	// identity function otherwise. See audit.go and docs/logging-design.md.
	server.AddReceivingMiddleware(auditMiddleware)

	// Two transports, one binary: stdio for local agents (Claude Code, MCP
	// inspector), streamable HTTP for the in-cluster Service the Helm chart
	// deploys — agents in the cluster connect to http://<svc>:8080/mcp.
	if addr := os.Getenv("MCP_HTTP_ADDR"); addr != "" {
		// MCP_HTTP_STATELESS=1 serves every POST on a temporary session: no
		// Mcp-Session-Id round-trip, GET/DELETE answer 405 — and, per go-sdk
		// v1.7.0, it is the only HTTP mode that speaks the 2026-07-28 revision
		// (stdio speaks it regardless). Default stays stateful so existing
		// in-cluster clients see no behavior change; hosted endpoints and
		// horizontally-scaled installs are what the flag is for.
		stateless := os.Getenv("MCP_HTTP_STATELESS") == "1"
		handler := mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{Stateless: stateless})
		mux := http.NewServeMux()
		mux.Handle("/mcp", handler)
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		mode := "stateful"
		if stateless {
			mode = "stateless"
		}
		slog.Info("listening", "transport", "http", "addr", addr+"/mcp", "mode", mode, "upstream", base, "version", buildVersion)
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("http server exited", "kind", errKind(err))
			os.Exit(1)
		}
		return
	}
	slog.Info("listening", "transport", "stdio", "upstream", base, "version", buildVersion)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		slog.Error("stdio session ended with error", "kind", errKind(err))
		os.Exit(1)
	}
	slog.Info("stdio session ended")
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
// list_projects
// ---------------------------------------------------------------------------

type listProjectsArgs struct{}

func listProjectsTool(ctx context.Context, req *mcp.CallToolRequest, args listProjectsArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "list_projects", req)
	raw, err := api.listProjects(ctx)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
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
	ctx = requestContext(ctx, "list_facts", req)
	page, err := api.listFacts(ctx, args.Project, args.Type, args.Severity, args.Since, args.Limit)
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
	ctx = requestContext(ctx, "facts_by_entity", req)
	if args.Name == "" {
		return errResult(fmt.Errorf("name is required")), nil, nil
	}
	facts, err := api.factsByEntity(ctx, args.Name, args.Kind)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(map[string]any{"facts": facts})
	return res, nil, err
}

// ---------------------------------------------------------------------------
// get_release
// ---------------------------------------------------------------------------

type listReleasesArgs struct {
	Project string `json:"project" jsonschema:"project slug, e.g. istio"`
	Limit   int    `json:"limit,omitempty" jsonschema:"how many recent releases, default 5, max 20"`
}

func listReleasesTool(ctx context.Context, req *mcp.CallToolRequest, args listReleasesArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "list_releases", req)
	if args.Project == "" {
		return errResult(fmt.Errorf("project is required")), nil, nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	raw, err := api.listReleases(ctx, args.Project, limit)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

type getReleaseArgs struct {
	Project    string `json:"project" jsonschema:"project slug, e.g. envoy"`
	Version    string `json:"version,omitempty" jsonschema:"release tag exactly as published, e.g. v1.38.3; omit for the latest reviewed release"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"also return the original release note body as raw_notes — judge from the source instead of the extracted facts"`
}

func getReleaseTool(ctx context.Context, req *mcp.CallToolRequest, args getReleaseArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "get_release", req)
	if args.Project == "" {
		return errResult(fmt.Errorf("project is required")), nil, nil
	}
	raw, err := api.getRelease(ctx, args.Project, args.Version, args.IncludeRaw)
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
	TargetVersion string `json:"target_version,omitempty" jsonschema:"optional upgrade destination, strictly above the running version: only facts with running < version <= target are returned; a target at or below running would make that range empty and is ignored with a note"`
	// VersionSource is an audit trail, never a check: this server cannot see the
	// caller's environment and therefore cannot tell a real citation from an
	// invented one. What it buys is a machine-readable claim to compare against
	// later, and a slot the caller has to fill by actually looking something up.
	VersionSource string `json:"version_source,omitempty" jsonschema:"where the running version was read, e.g. daemonset/cilium image tag or a user-provided value; echoed back so the claim can be audited"`
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
	// Target names the thing the condition is about, when the server stored it
	// structurally — an agent greps a live config for this instead of reading
	// the phrase. Absent when applies_if is only a sentence.
	Target *conditionTarget `json:"applies_if_target,omitempty"`
	// FixedIn is the minimum version that closes this issue: the answer to
	// "before I turn that feature on, what do I need?" — which is the useful
	// form of a fact whose condition the stack does not meet today.
	FixedIn      string   `json:"fixed_in,omitempty"`
	RemovedIn    string   `json:"removed_in,omitempty"`
	DeprecatedIn string   `json:"deprecated_in,omitempty"`
	Quote     string   `json:"quote,omitempty"`
	IDs       []string `json:"ids,omitempty"`
	AlsoIn    []string `json:"same_issue_also_addressed_in,omitempty"`
}

type conditionTarget struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// conditional reports whether this entry carries a condition an agent still
// has to resolve against the running configuration.
func (b briefFact) conditional() bool { return b.Condition != "" || len(b.ConditionsAny) > 0 }

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
	VersionSource  string        `json:"version_source,omitempty"`
	TargetVersion  string        `json:"target_version,omitempty"`
	Tracked        *bool         `json:"tracked,omitempty"` // set only on zero-fact components
	Note           string        `json:"note,omitempty"`
	FactsScanned   int           `json:"facts_scanned"`
	Summary        *briefSummary `json:"summary,omitempty"`
	ActionRequired []briefFact   `json:"action_required,omitempty"`
	CheckConfig    []briefFact   `json:"check_config,omitempty"`
	OtherFacts     []briefFact   `json:"other_facts,omitempty"`
	OtherOmitted   int           `json:"other_facts_omitted,omitempty"`
	RelevantFacts  []Fact        `json:"relevant_facts,omitempty"` // detail:"full" only
	RelevantOmitted int          `json:"relevant_facts_omitted,omitempty"`
}

// otherFactsCap bounds the one-liner tail of a brief report; whatever is cut
// is counted in other_facts_omitted — never dropped silently.
const otherFactsCap = 100

// relevantFactsCap bounds a detail:"full" component — a long upgrade path can
// otherwise be tens of thousands of tokens (observed: envoy v1.36.0→latest,
// 75 facts ≈ 73 KB). The cut is counted in relevant_facts_omitted — never
// silent; agents narrow with severity_min/target_version or drill down.
const relevantFactsCap = 50

// coverageNote flags a running version that sits below every release we hold a
// fact for. It is the one cross-check this server can make on a claim about an
// environment it cannot see, and it catches two different things with the same
// sentence: a genuinely ancient install, where a briefing drawn from the
// reviewed window is not the whole upgrade path, and a version that was never
// read off a live resource (2026-07-28: an agent holding no version at all
// supplied cilium 1.16.0 against an actual 1.19.5 — every answer after that was
// confidently wrong and nothing downstream could tell).
func coverageNote(running string, runningKey, oldest []int, oldestTag string) string {
	if oldest == nil || version.Compare(runningKey, oldest) >= 0 {
		return ""
	}
	return fmt.Sprintf(
		"running version %s is older than every release on record (earliest with facts: %s) — "+
			"this covers the reviewed window only, so treat it as partial, and re-check that the "+
			"running version was read off a live resource",
		running, oldestTag)
}

// confessionRe matches version_source values that describe an inference
// rather than a live read. Sixty controlled runs (2026-07-29 hub campaign)
// gave this a clean separation: all 8 versions whose source contained such
// vocabulary were hallucinations, all 197 citing a concrete read were correct,
// zero crossover. The model does not forge sources — it confesses. "stated"
// and "standard" stay OFF the list: a user-stated version is a legitimate
// source in the no-cluster-access deployment, and every observed confession
// is caught without them.
var confessionRe = regexp.MustCompile(`(?i)infer|assum|guess|estimat|likely|probab|typical|need verification|not verified|unverified`)

// confessionNote turns a self-confessed inferred source into a warning on the
// component's note — the same channel as the low-version warning. Pattern
// matching the caller's own words, never the environment: the server still
// sees no cluster, so this is the second of exactly two cross-checks it can
// make on a version claim.
func confessionNote(src string) string {
	if src == "" || !confessionRe.MatchString(src) {
		return ""
	}
	if len(src) > 80 {
		src = src[:80] + "…"
	}
	return fmt.Sprintf("version_source reads as an inference, not a live read (%q) — treat the "+
		"running version as unverified and re-read it off a live resource; in measurement, every "+
		"self-described inferred version was wrong", src)
}

// resolveTarget parses target_version against the running version. A target
// at or below running asks for the range (running, target], which is empty by
// construction — in measurement, five of twenty agent runs sent exactly
// running-as-target and read the guaranteed zero facts as "no issues". An
// empty-by-construction range is therefore refused loudly (field ignored,
// note says why and teaches the intended use) instead of honored silently.
func resolveTarget(currentKey []int, targetRaw string) ([]int, string) {
	if targetRaw == "" {
		return nil, ""
	}
	targetKey := version.NormalizeVersion(targetRaw)
	if targetKey == nil {
		return nil, "target_version could not be parsed; ignored"
	}
	if version.Compare(targetKey, currentKey) <= 0 {
		return nil, fmt.Sprintf("target_version (%s) is not above the running version, so the checked range "+
			"(running, target] is empty by construction — the field was ignored and the full upgrade path is shown. "+
			"To preview one upgrade hop, keep version = what is running now and set target_version to the destination.", targetRaw)
	}
	return targetKey, ""
}

func checkStackTool(ctx context.Context, req *mcp.CallToolRequest, args checkStackArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "check_stack", req)
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
		rep := componentReport{
			Project:        comp.Project,
			RunningVersion: comp.Version,
			VersionSource:  comp.VersionSource,
			TargetVersion:  comp.TargetVersion,
		}
		var notes []string
		currentKey := version.NormalizeVersion(comp.Version)
		if currentKey == nil {
			rep.Note = "running version could not be parsed; showing no facts (use list_facts to inspect manually)"
			reports = append(reports, rep)
			continue
		}
		targetKey, targetNote := resolveTarget(currentKey, comp.TargetVersion)
		if targetNote != "" {
			notes = append(notes, targetNote)
		}
		facts, err := api.allProjectFacts(ctx, comp.Project)
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
			if tracked, perr := api.projectTracked(ctx, comp.Project); perr != nil {
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
		var oldest []int
		var oldestTag string
		for _, f := range facts {
			factKey := version.NormalizeVersion(f.Version)
			if factKey == nil {
				unparseable++
				continue
			}
			if oldest == nil || version.Compare(factKey, oldest) < 0 {
				oldest, oldestTag = factKey, f.Version
			}
			if version.Compare(factKey, currentKey) <= 0 {
				continue
			}
			if targetKey != nil && version.Compare(factKey, targetKey) > 0 {
				continue
			}
			if severityRank[strings.ToLower(f.EffSeverity())] < minRank {
				continue
			}
			relevant = append(relevant, f)
			keys = append(keys, factKey)
		}
		if unparseable > 0 {
			notes = append(notes, fmt.Sprintf("%d fact(s) skipped (unparseable release tag)", unparseable))
		}
		if n := coverageNote(comp.Version, currentKey, oldest, oldestTag); n != "" {
			notes = append(notes, n)
		}
		if n := confessionNote(comp.VersionSource); n != "" {
			notes = append(notes, n)
		}
		rep.Note = strings.Join(notes, "; ")

		if full {
			rep.RelevantFacts = relevant
			if extra := len(rep.RelevantFacts) - relevantFactsCap; extra > 0 {
				rep.RelevantFacts = rep.RelevantFacts[:relevantFactsCap]
				rep.RelevantOmitted = extra
			}
			if rep.RelevantFacts == nil {
				rep.RelevantFacts = []Fact{}
			}
			reports = append(reports, rep)
			continue
		}
		rep.Summary, rep.ActionRequired, rep.CheckConfig, rep.OtherFacts = briefReport(relevant, keys)
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
		out["hint"] = "briefing (a data classification, not a recommendation — facts are provided without warranty and " +
			"the decision stays with the operator): action_required = critical/high that applies to every install of this version. " +
			"check_config = critical/high that applies ONLY IF applies_if holds — read the running configuration and " +
			"decide before you recommend anything; an unmet condition is not an upgrade reason. Report an unmet one " +
			"forward instead: fixed_in is the minimum version to be on BEFORE enabling what applies_if describes. " +
			"applies_if_target names the thing to look for when the server stored it structurally. The rest is one " +
			"line each in other_facts; facts sharing one quoted sentence are merged (ids listed together, " +
			"applies_if_any when conditions differ); the same advisory fixed on several release branches is one " +
			"entry (same_issue_also_addressed_in). " +
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
// then split three ways. Severity alone used to decide what surfaced as
// action_required, so a high fact that only bites ClusterMesh users read as
// "upgrade now" to every reader (observed live, 2026-07-28). A condition the
// caller has not resolved is not an action yet: those go to check_config, and
// action_required keeps only what applies to everyone.
// Extraction judges each release note independently, so the same advisory
// can carry different severities across branches — a collapsed entry shows
// the strongest judgment in its group.
func briefReport(relevant []Fact, keys [][]int) (*briefSummary, []briefFact, []briefFact, []briefFact) {
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
				if severityRank[strings.ToLower(f.EffSeverity())] > severityRank[strings.ToLower(kept.Severity)] {
					kept.Severity = f.Severity
				}
				if f.Mandatory {
					kept.Mandatory = true
				}
				continue
			}
		}
		bf := &briefFact{
			FactID:       f.FactID,
			Version:      f.Version,
			FactType:     f.FactType,
			Severity:     f.EffSeverity(),
			Mandatory:    f.Mandatory,
			Condition:    f.Condition,
			FixedIn:      f.FixedIn,
			RemovedIn:    f.RemovedIn,
			DeprecatedIn: f.DeprecatedIn,
			Quote:        f.Quote,
			IDs:          f.RefIDs,
		}
		if f.CondTarget != "" {
			bf.Target = &conditionTarget{Kind: f.CondKind, Name: f.CondTarget}
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
	check := []briefFact{}
	other := []briefFact{}
	for _, bf := range mergeSharedQuotes(ordered) {
		switch {
		case severityRank[strings.ToLower(bf.Severity)] < severityRank["high"]:
			other = append(other, *bf)
		case bf.conditional():
			check = append(check, *bf)
		default:
			action = append(action, *bf)
		}
	}
	return sum, action, check, other
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
