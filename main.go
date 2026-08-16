// ratatosk-mcp — in-cluster MCP server PoC (docs/rfc/agent-api-build.md Phase 4).
//
// Exposes the ratatosk agent API v1 as read-only MCP tools over stdio, plus
// check_stack: the RFC's privacy loop ("notify, don't tell") — the caller's
// running versions are compared LOCALLY against change version keys; only
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
	"strconv"
	"strings"
	"time"

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
		Instructions: "Data source: the public ratatosk.io agent API — release changes extracted by AI from official " +
			"release notes; verify critical decisions against the source URL in get_release (terms: https://ratatosk.io/terms). " +
			"Every change carries three axes: family (security|breaking|deprecated — what kind of thing it is), " +
			"bucket (action|check|plan — how to act NOW), and applies_if (a condition you evaluate against the running " +
			"setup, structured into targets when the server could). Read bucket before severity: an 'action' entry " +
			"applies to every install, a 'check' entry only to setups its applies_if matches. " +
			"Rate limits: the upstream API allows 1200 requests/minute per IP, and the hosted endpoint additionally " +
			"allows each caller 60 tool calls/minute. One tool call is not one request — check_stack costs one " +
			"upstream request per component — so prefer a single check_stack for stack-wide questions over polling " +
			"list_changes per project.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Annotations: &mcp.ToolAnnotations{Title: "List tracked projects", ReadOnlyHint: true},
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
		Name:        "list_changes",
		Annotations: &mcp.ToolAnnotations{Title: "Sync release changes", ReadOnlyHint: true},
		Description: "Incremental SYNC feed of release changes for CNCF/cloud-native projects. " +
			"Ordered by seq ascending — OLDEST analyzed first, so a single page is NOT the newest data; " +
			"page through with since=<returned next_since> until next_since comes back null. " +
			"Built for keeping a local copy up to date. For 'what is the latest release of X' or " +
			"'recent releases of X', use list_releases or get_release (omit version for the newest) instead. " +
			"Filter by project, by family (security|breaking|deprecated — what kind of thing it is) and by " +
			"bucket (action|check|plan — how to act now). The routine record (bot dependency bumps and the like) " +
			"is excluded by default. Two fields matter most: applies_if tells you whether an entry is yours to act " +
			"on — when its targets are present, look them up in the running configuration instead of parsing the " +
			"sentence; matter_key is the identity of the underlying matter across releases, which get_matter expands.",
	}, listChangesTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_matter",
		Annotations: &mcp.ToolAnnotations{Title: "One matter across releases", ReadOnlyHint: true},
		Description: "Every release in which one matter appeared, oldest first. Take matter_key verbatim from a " +
			"change (case-sensitive, contains '/' and ':'). Use it to answer 'which version fixes this for MY " +
			"branch' and 'have I already handled this'. Why every occurrence and not just the newest: the same " +
			"containerd security roll-up landed on five branches carrying 2, 4 and 10 advisories respectively — " +
			"told only the newest, someone on the 2-advisory branch would assume they were fully covered. " +
			"Set include_all for the routine record too (mostly bot dependency bumps).",
	}, getMatterTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "changes_by_entity",
		Annotations: &mcp.ToolAnnotations{Title: "Changes touching one identifier", ReadOnlyHint: true},
		Description: "Reverse index: every change touching one exact identifier — a CVE id, CRD, feature gate, " +
			"flag, metric, config field, or dependency. Case-insensitive. " +
			"Call this when you have a specific identifier (e.g. from a manifest or advisory) and want to know what changed around it.",
	}, changesByEntityTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_release",
		Annotations: &mcp.ToolAnnotations{Title: "One reviewed release", ReadOnlyHint: true},
		Description: "One reviewed release: envelope (summary, source URL, release URL) plus all its changes. " +
			"changes=[] means the release was read and nothing operator-facing was recorded — auditable " +
			"silence, not a gap. Each change carries family (security|breaking|deprecated), actionability, " +
			"bucket (act now / check first / plan ahead), a machine-evaluable applies_if, cited advisories " +
			"with CURRENT severity, and the verbatim quote it came from. by_bucket/by_family/max_severity " +
			"summarize the same set; notes_total counts routine entries not shown individually. " +
			"Omit version for the latest reviewed release of the project. " +
			"version is accepted with or without the leading 'v' (projects disagree on the spelling); " +
			"a wrong tag returns an error listing the project's recent reviewed tags — retry with one of those. " +
			"Set include_raw for the original release note body (raw_notes).",
	}, getReleaseTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_releases",
		Annotations: &mcp.ToolAnnotations{Title: "Recent releases of a project", ReadOnlyHint: true},
		Description: "The newest N reviewed releases of one project, as light summaries (version, " +
			"released/reviewed dates, changes_total, counts by bucket and by family, max advisory " +
			"severity, notes_total). THE tool for 'recent releases of X' / 'what changed in X lately' — " +
			"newest first, unlike the list_changes sync feed which walks oldest-first by seq. " +
			"changes_total=0 means the release was read and is routine (auditable silence). " +
			"Drill into a row with get_release(project, version) for the full changes.",
	}, listReleasesTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_stack",
		Annotations: &mcp.ToolAnnotations{Title: "Check a running stack", ReadOnlyHint: true},
		Description: "Check the user's running component versions against known changes. Versions are compared " +
			"INSIDE THIS SERVER PROCESS — only project slugs are sent upstream, and this tool never calls the " +
			"server-side /v1/upgrade endpoint. Run the server yourself and running versions never leave your " +
			"infrastructure; on the hosted endpoint they transit server memory only and are not logged. " +
			"Returns, per component, the changes from releases NEWER than the running version (the upgrade path). " +
			"Default is a briefing: summary (new_changes, distinct_matters, by_severity, by_family, by_bucket), " +
			"then the items split by what the caller must do — action_required applies to everyone, check_config " +
			"applies only if its applies_if holds against the running configuration (resolve it before " +
			"recommending; an unmet condition is not a reason to upgrade, it is a precondition for later — " +
			"the entry's version is the minimum to be on before enabling that feature). " +
			"The split comes from the server's bucket field, the SAME rule the website and the weekly email use. " +
			"Repeat appearances of one matter_key (the same issue fixed on several release branches) collapse " +
			"into one entry, and same_matter_also_addressed_in names every other release on record that carried " +
			"it. Branch-aware: a matter already fixed at or below the running version ON THE RUNNING BRANCH is " +
			"excluded (the install has it), counted in note — so a backport visible on a newer branch is not " +
			"reported as outstanding work. Line-aware: a repository can publish separate products or channels " +
			"(containerd api/, Flatcar lts vs stable, openfeature flagd vs core) and there is NO version order " +
			"between lines, so only the line your version belongs to is compared — pass the tag as published, " +
			"prefix included (\"flagd/v0.16.1\", \"lts-4081.3.9\"), or the wrong line is compared. " +
			"Pass version_source per component (where you read the version — e.g. a daemonset image tag, or that the user " +
			"stated it): it is echoed back as an audit trail. This server cannot see your environment, so it cannot " +
			"verify a version or its source; a running version older than every release on record is flagged in note, " +
			"which is the only cross-check available here. " +
			"Use detail:\"full\" for every change verbatim in relevant_changes (capped at 50 per component with " +
			"relevant_changes_omitted — narrow with severity_min or target_version), " +
			"target_version to limit to one upgrade hop, " +
			"severity_min to filter. Components with zero changes carry tracked:true|false — tracked:false means " +
			"the project is NOT covered by ratatosk, so the absence of changes is no-coverage, not safety. " +
			"Drill down with get_release or changes_by_entity.",
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
		// Per-caller limiting sits in front of the MCP handler, not upstream:
		// this process is the one that can tell hosted callers apart, and the
		// upstream API deliberately cannot (see ratelimit.go). Off by default,
		// which is what a self-hosted single-tenant server wants.
		rl := rateLimitFromEnv(os.Getenv("MCP_RATE_LIMIT_PER_MIN"))
		mux := http.NewServeMux()
		mux.Handle("/mcp", rateLimitMiddleware(rl, handler))
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		mode := "stateful"
		if stateless {
			mode = "stateless"
		}
		// The limit is a configured number, not a caller attribute — safe to log.
		rateField := "off"
		if rl != nil {
			rateField = strconv.Itoa(rl.limit) + "/min per caller"
		}
		slog.Info("listening", "transport", "http", "addr", addr+"/mcp", "mode", mode, "upstream", base, "version", buildVersion, "rate_limit", rateField)
		// Explicit timeouts: this is an unauthenticated public endpoint, and the
		// zero-value http.Server leaves every timeout unset (unbounded), so a
		// client that dribbles a request header one byte at a time holds a
		// goroutine/FD forever (slowloris) — and does it below the rate limiter,
		// which only counts requests that finish their headers. WriteTimeout is
		// generous because a large check_stack legitimately streams a while.
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      120 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		if err := srv.ListenAndServe(); err != nil {
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
// list_changes
// ---------------------------------------------------------------------------

type listChangesArgs struct {
	Project string `json:"project,omitempty" jsonschema:"project slug filter, e.g. envoy, istio, cilium"`
	Family  string `json:"family,omitempty" jsonschema:"security|breaking|deprecated — what kind of thing it is"`
	Bucket  string `json:"bucket,omitempty" jsonschema:"action|check|plan — how to act now. action applies to everyone; check only if applies_if matches your setup; plan is announced for later"`
	Since   int    `json:"since,omitempty" jsonschema:"cursor: return changes with seq greater than this"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

func listChangesTool(ctx context.Context, req *mcp.CallToolRequest, args listChangesArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "list_changes", req)
	page, err := api.listChanges(ctx, args.Project, args.Family, args.Bucket, args.Since, args.Limit)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(page)
	return res, nil, err
}

// ---------------------------------------------------------------------------
// changes_by_entity
// ---------------------------------------------------------------------------

type changesByEntityArgs struct {
	Name string `json:"name" jsonschema:"exact identifier to look up: CVE id, CRD, feature gate, flag, metric, config field, dependency"`
	Kind string `json:"kind,omitempty" jsonschema:"optional: api|crd|feature_gate|flag|metric|config_field|extension|dependency|cve|advisory|subsystem"`
}

func changesByEntityTool(ctx context.Context, req *mcp.CallToolRequest, args changesByEntityArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "changes_by_entity", req)
	if args.Name == "" {
		return errResult(fmt.Errorf("name is required")), nil, nil
	}
	changes, err := api.changesByEntity(ctx, args.Name, args.Kind)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(map[string]any{"changes": changes})
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
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"also return the original release note body as raw_notes — judge from the source instead of the extracted changes"`
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
	Version       string `json:"version" jsonschema:"the version currently running, exactly as the project publishes it — keep any release-line prefix (flagd/v0.16.1, lts-4081.3.9, api/v1.11.1), since only that line is compared. e.g. v1.36.8"`
	TargetVersion string `json:"target_version,omitempty" jsonschema:"optional upgrade destination, strictly above the running version: only changes with running < version <= target are returned; a target at or below running would make that range empty and is ignored with a note"`
	// VersionSource is an audit trail, never a check: this server cannot see the
	// caller's environment and therefore cannot tell a real citation from an
	// invented one. What it buys is a machine-readable claim to compare against
	// later, and a slot the caller has to fill by actually looking something up.
	VersionSource string `json:"version_source,omitempty" jsonschema:"where the running version was read, e.g. daemonset/cilium image tag or a user-provided value; echoed back so the claim can be audited"`
}

type checkStackArgs struct {
	Components  []stackComponent `json:"components" jsonschema:"the running stack to check"`
	SeverityMin string           `json:"severity_min,omitempty" jsonschema:"only changes at or above this severity: info|low|medium|high|critical"`
	Detail      string           `json:"detail,omitempty" jsonschema:"brief (default): summary + the items to act on, split by bucket; full: every change verbatim"`
}

var severityRank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// briefChange is one upgrade-path change compressed to what an agent decides on.
// Evidence entities and URLs live one get_release call away.
type briefChange struct {
	ChangeID  string `json:"change_id"`
	MatterKey string `json:"matter_key,omitempty"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Family    string `json:"family,omitempty"`
	// Bucket is why this entry is in the list it is in — the server's own
	// rule, the same one the website and the weekly email use.
	Bucket        string   `json:"bucket"`
	Severity      string   `json:"severity"`
	Condition     string   `json:"applies_if,omitempty"`     // empty = applies unconditionally
	ConditionsAny []string `json:"applies_if_any,omitempty"` // merged entry with diverging conditions
	// Targets name the things the condition is about, when the server could
	// structure it — grep a live config for these instead of reading the
	// phrase. Absent when applies_if is only a sentence (evaluable false).
	Targets []string `json:"applies_if_targets,omitempty"`
	// Window carries deprecation timing when the server knows it.
	Window map[string]string `json:"window,omitempty"`
	Quote  string            `json:"quote,omitempty"`
	IDs    []string          `json:"advisories,omitempty"`
	AlsoIn []string          `json:"same_matter_also_addressed_in,omitempty"`
}

// conditional reports whether this entry carries a condition an agent still
// has to resolve against the running configuration.
func (b briefChange) conditional() bool { return b.Condition != "" || len(b.ConditionsAny) > 0 }

type briefSummary struct {
	NewChanges      int            `json:"new_changes"`
	DistinctMatters int            `json:"distinct_matters"`
	BySeverity      map[string]int `json:"by_severity"`
	ByFamily        map[string]int `json:"by_family"`
	ByBucket        map[string]int `json:"by_bucket"`
}

type componentReport struct {
	Project         string        `json:"project"`
	RunningVersion  string        `json:"running_version"`
	VersionSource   string        `json:"version_source,omitempty"`
	TargetVersion   string        `json:"target_version,omitempty"`
	Tracked         *bool         `json:"tracked,omitempty"` // set only on zero-change components
	Note            string        `json:"note,omitempty"`
	ChangesScanned  int           `json:"changes_scanned"`
	Summary         *briefSummary `json:"summary,omitempty"`
	ActionRequired  []briefChange `json:"action_required,omitempty"`
	CheckConfig     []briefChange `json:"check_config,omitempty"`
	OtherChanges    []briefChange `json:"other_changes,omitempty"`
	OtherOmitted    int           `json:"other_changes_omitted,omitempty"`
	RelevantChanges []Change      `json:"relevant_changes,omitempty"` // detail:"full" only
	RelevantOmitted int           `json:"relevant_changes_omitted,omitempty"`
}

// otherChangesCap bounds the one-liner tail of a brief report; whatever is cut
// is counted in other_changes_omitted — never dropped silently.
const otherChangesCap = 100

// maxComponents bounds one check_stack call. Each component fans out to full
// upstream paging plus a probe on a miss, so an unbounded list is a request
// amplifier against our own API. Real stacks are tens of items.
const maxComponents = 100

// relevantChangesCap bounds a detail:"full" component — a long upgrade path can
// otherwise be tens of thousands of tokens (observed: envoy v1.36.0→latest,
// 75 changes ≈ 73 KB). The cut is counted in relevant_changes_omitted — never
// silent; agents narrow with severity_min/target_version or drill down.
const relevantChangesCap = 50

// coverageNote flags a running version that sits below every release we hold a
// change for. It is the one cross-check this server can make on a claim about an
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
		"running version %s is older than every release on record (earliest on record: %s) — "+
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
// running-as-target and read the guaranteed zero changes as "no issues". An
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
	// One tool call costs one rate-limit token, but each component fans out to
	// full upstream paging (/v1/changes) plus a tracked-project probe on a
	// zero-result miss. Without a cap, a single call amplifies into thousands
	// of upstream requests against our own API. A real stack is tens of items.
	if len(args.Components) > maxComponents {
		return errResult(fmt.Errorf("too many components: %d (max %d)", len(args.Components), maxComponents)), nil, nil
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
			rep.Note = "running version could not be parsed; showing no changes (use list_changes to inspect manually)"
			reports = append(reports, rep)
			continue
		}
		targetKey, targetNote := resolveTarget(currentKey, comp.TargetVersion)
		if targetNote != "" {
			notes = append(notes, targetNote)
		}
		changes, err := api.allProjectChanges(ctx, comp.Project)
		if err != nil {
			rep.Note = "fetch failed: " + err.Error()
			reports = append(reports, rep)
			continue
		}
		rep.ChangesScanned = len(changes)
		// Zero changes is ambiguous: audited silence (tracked, routine releases)
		// or no coverage at all (unknown slug). Probe and say which — an agent
		// must never read "not tracked" as "safe" (2026-07-23 kagent finding).
		if len(changes) == 0 {
			if tracked, perr := api.projectTracked(ctx, comp.Project); perr != nil {
				notes = append(notes, "tracking probe failed: "+perr.Error())
			} else {
				rep.Tracked = &tracked
				if tracked {
					notes = append(notes, "tracked by ratatosk; no changes on record — releases so far were routine")
				} else {
					notes = append(notes, "NOT tracked by ratatosk — zero changes means no coverage here, not safety")
				}
			}
		}

		// Changes from releases strictly newer than the running version =
		// the upgrade path the operator has not yet absorbed. Minus the ones
		// their own branch already closed — see addressedOnRunningBranch.
		alreadyHave := addressedOnRunningBranch(changes, currentKey)
		allOccurrences := occurrencesByMatter(changes)
		unparseable := 0
		settled := 0
		offLine := 0
		var relevant []Change
		var keys [][]int
		var oldest []int
		var oldestTag string
		for _, f := range changes {
			// A release line is a separate product or channel published from the
			// same repository — containerd's "api/" module, Flatcar's lts beside
			// stable, openfeature's flagd beside core. Two lines have no order
			// between them, so a version from another line is not "newer": it is
			// not comparable at all. Gate before the key is even computed, or the
			// comparison silently succeeds and offers an upgrade that is a
			// different piece of software (observed live: containerd v1.7.28 was
			// offered api/v1.11.0, 2026-08-10).
			if !version.SameLine(comp.Version, f.Version) {
				offLine++
				continue
			}
			changeKey := version.NormalizeVersion(f.Version)
			if changeKey == nil {
				unparseable++
				continue
			}
			if oldest == nil || version.Compare(changeKey, oldest) < 0 {
				oldest, oldestTag = changeKey, f.Version
			}
			if version.Compare(changeKey, currentKey) <= 0 {
				continue
			}
			// The same matter, already fixed at or below the running version on
			// the running branch: the operator has it. Counted, not silently
			// dropped — the note says how many and why.
			if f.MatterKey != "" {
				if _, ok := alreadyHave[f.MatterKey]; ok {
					settled++
					continue
				}
			}
			if targetKey != nil && version.Compare(changeKey, targetKey) > 0 {
				continue
			}
			if severityRank[strings.ToLower(f.EffSeverity())] < minRank {
				continue
			}
			relevant = append(relevant, f)
			keys = append(keys, changeKey)
		}
		if unparseable > 0 {
			notes = append(notes, fmt.Sprintf("%d change(s) skipped (unparseable release tag)", unparseable))
		}
		if offLine > 0 {
			notes = append(notes, fmt.Sprintf(
				"%d change(s) from other release lines of this project were excluded — %q is line %q, and lines (separate products or channels published from one repository) have no version order between them",
				offLine, comp.Version, lineLabel(version.Line(comp.Version))))
		}
		// A version whose line does not exist here is almost always a dropped
		// prefix, not a real gap. Agents read versions off image tags, and an
		// image tag rarely carries the release-line prefix: knative publishes
		// "knative-v1.12.0" while its images say "v1.12.0", and every one of
		// that project's releases would fall on the other side of the line
		// gate. Silence would look like "nothing to do" (hub question,
		// 2026-08-10).
		if lines := linesPresent(changes); len(lines) > 0 && !slices.Contains(lines, version.Line(comp.Version)) {
			notes = append(notes, fmt.Sprintf(
				"no releases on record for line %s — this project publishes %s. If the version came from an image tag, the release-line prefix was probably dropped: pass the tag as the project publishes it (e.g. %s)",
				lineLabel(version.Line(comp.Version)), lineList(lines), exampleTag(changes, lines[0])))
		}
		if settled > 0 {
			notes = append(notes, fmt.Sprintf(
				"%d change(s) on newer branches were excluded: the same matter is already fixed at or below %s on your branch — expand any matter_key with get_matter to see every branch",
				settled, comp.Version))
		}
		if n := coverageNote(comp.Version, currentKey, oldest, oldestTag); n != "" {
			notes = append(notes, n)
		}
		if n := confessionNote(comp.VersionSource); n != "" {
			notes = append(notes, n)
		}
		rep.Note = strings.Join(notes, "; ")

		if full {
			rep.RelevantChanges = relevant
			if extra := len(rep.RelevantChanges) - relevantChangesCap; extra > 0 {
				rep.RelevantChanges = rep.RelevantChanges[:relevantChangesCap]
				rep.RelevantOmitted = extra
			}
			if rep.RelevantChanges == nil {
				rep.RelevantChanges = []Change{}
			}
			reports = append(reports, rep)
			continue
		}
		rep.Summary, rep.ActionRequired, rep.CheckConfig, rep.OtherChanges = briefReport(relevant, keys, allOccurrences)
		if extra := len(rep.OtherChanges) - otherChangesCap; extra > 0 {
			rep.OtherChanges = rep.OtherChanges[:otherChangesCap]
			rep.OtherOmitted = extra
		}
		reports = append(reports, rep)
	}

	out := map[string]any{
		"components": reports,
		"privacy":    "versions were compared locally; only project slugs were sent to the server",
	}
	if !full {
		out["hint"] = "briefing (a data classification, not a recommendation — changes are provided without warranty and " +
			"the decision stays with the operator). The three lists split on the server's bucket field, NOT on severity: " +
			"action_required = bucket action, which applies to every install of this version. " +
			"check_config = bucket check, which applies ONLY IF applies_if holds — read the running configuration and " +
			"decide before you recommend anything; an unmet condition is not an upgrade reason. Report an unmet one " +
			"forward instead: this entry's version is the minimum to be on BEFORE enabling what applies_if describes. " +
			"other_changes = bucket plan, announced for a future release, one line each. " +
			"applies_if_targets names the things to look for when the server stored the condition structurally. " +
			"severity here is derived by this server (the highest of the entry's advisories, else a default from " +
			"family and bucket), so rank urgency with it but never decide who is affected with it. " +
			"Changes on the same matter_key are merged (ids listed together, " +
			"applies_if_any when conditions differ); one matter fixed on several release branches is one " +
			"entry, and same_matter_also_addressed_in names every other release on record that carried it — " +
			"expand it with get_matter(matter_key) to see which branch carries which advisories. " +
			"A matter already fixed at or below the running version on the running branch is excluded and " +
			"counted in note: the install already has it. Changes from other release lines of the same " +
			"project (separate products or channels — no version order between them) are excluded and " +
			"counted in note too. " +
			`Call again with detail:"full" for every change verbatim, or drill down: get_release(project, version) ` +
			"for one release with evidence and source URL, changes_by_entity(name) for one CVE/flag/CRD."
	}
	res, err := jsonResult(out)
	return res, nil, err
}

// briefReport compresses the upgrade path: sorted by version, the same matter
// on several branches collapsed into its earliest fix (an operator upgrades
// once — the nearest release that closes it is the actionable one), then split
// three ways.
//
// The split now comes from the server's `bucket`, not from a local guess.
// Severity alone used to decide what surfaced as action_required, so a high
// finding that only bites ClusterMesh users read as "upgrade now" to every
// reader (observed live, 2026-07-28); the client then patched that with a
// "has a condition?" heuristic. The server computes the same distinction for
// the website and the weekly email, so all three surfaces now agree by
// construction rather than by coincidence.
func briefReport(relevant []Change, keys [][]int, allOccurrences map[string][]string) (*briefSummary, []briefChange, []briefChange, []briefChange) {
	idx := make([]int, len(relevant))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		if c := version.Compare(keys[idx[a]], keys[idx[b]]); c != 0 {
			return c < 0
		}
		return relevant[idx[a]].ChangeID < relevant[idx[b]].ChangeID
	})

	sum := &briefSummary{
		NewChanges: len(relevant),
		BySeverity: map[string]int{},
		ByFamily:   map[string]int{},
		ByBucket:   map[string]int{},
	}
	byMatter := map[string]*briefChange{}
	var ordered []*briefChange
	for _, i := range idx {
		f := relevant[i]
		if kept, ok := byMatter[f.MatterKey]; ok && f.MatterKey != "" {
			if f.Version != kept.Version && !slices.Contains(kept.AlsoIn, f.Version) {
				kept.AlsoIn = append(kept.AlsoIn, f.Version)
			}
			if severityRank[strings.ToLower(f.EffSeverity())] > severityRank[strings.ToLower(kept.Severity)] {
				kept.Severity = f.EffSeverity()
			}
			// 같은 사안이라도 갈래마다 급이 다를 수 있다 — 더 급한 쪽으로 올린다.
			if bucketRank(f.Bucket) < bucketRank(kept.Bucket) {
				kept.Bucket = f.Bucket
			}
			continue
		}
		ids := make([]string, 0, len(f.Advisories))
		for _, a := range f.Advisories {
			ids = append(ids, a.ID)
		}
		bc := &briefChange{
			ChangeID:  f.ChangeID,
			MatterKey: f.MatterKey,
			Version:   f.Version,
			Kind:      f.Kind,
			Family:    f.Family,
			Bucket:    f.Bucket,
			Severity:  f.EffSeverity(),
			Condition: f.Condition,
			Targets:   f.CondTargets,
			Window:    f.Window,
			Quote:     f.Quote,
			IDs:       ids,
		}
		if f.MatterKey != "" {
			byMatter[f.MatterKey] = bc
		}
		ordered = append(ordered, bc)
		sum.ByFamily[f.Family]++
	}
	sum.DistinctMatters = len(ordered)
	for _, bc := range ordered {
		sum.BySeverity[bc.Severity]++
		sum.ByBucket[bc.Bucket]++
	}

	// same_matter_also_addressed_in now names every release on record that
	// carried this matter, not only the ones inside the returned window. The
	// window is bounded by the running version, so the branch that actually
	// matters to a given operator was the one most likely to be missing (hub
	// report, 2026-08-10).
	for _, bc := range ordered {
		if bc.MatterKey == "" {
			continue
		}
		bc.AlsoIn = nil
		for _, v := range allOccurrences[bc.MatterKey] {
			if v != bc.Version {
				bc.AlsoIn = append(bc.AlsoIn, v)
			}
		}
	}

	action := []briefChange{}
	check := []briefChange{}
	other := []briefChange{}
	for _, bc := range mergeSharedQuotes(ordered) {
		switch bc.Bucket {
		case "action":
			action = append(action, *bc)
		case "check":
			check = append(check, *bc)
		default:
			other = append(other, *bc)
		}
	}
	return sum, action, check, other
}

// sameBranch reports whether two version keys sit on the same release branch,
// which for every project on record means sharing major and minor. A key too
// short to name a branch answers false — the conservative direction, since the
// only thing this decides is whether to hide something.
func sameBranch(a, b []int) bool {
	return len(a) >= 2 && len(b) >= 2 && a[0] == b[0] && a[1] == b[1]
}

// addressedOnRunningBranch collects the matters this install already carries.
//
// The upgrade path is "releases newer than the running version", which is the
// right window for finding work — and the wrong one for deciding whether work
// is still outstanding. Maintainers backport one fix onto every supported
// branch, so the same matter_key lands on v2.2.4 and v2.3.1 on the same day.
// An operator running 2.2.5 already has it; showing them the v2.3.1 occurrence
// as action_required tells them to go fix something that is fixed (observed on
// a live cluster, 2026-08-10, containerd CVE-2026-46680 and CVE-2026-47262).
//
// Branch equality is what makes this safe. "Any occurrence at or below the
// running version" would be wrong in the other direction: a fix backported to
// v2.1.9 says nothing about v2.2.4, which was cut from a different branch and
// may never have received it. Only an occurrence on the running version's own
// branch, at or below it, proves the running build contains the fix.
func addressedOnRunningBranch(changes []Change, currentKey []int) map[string]string {
	out := map[string]string{}
	for _, c := range changes {
		if c.MatterKey == "" {
			continue
		}
		key := version.NormalizeVersion(c.Version)
		if key == nil || !sameBranch(key, currentKey) || version.Compare(key, currentKey) > 0 {
			continue
		}
		// Keep the earliest such occurrence: it is the one that answers
		// "since when did we have this".
		if prev, ok := out[c.MatterKey]; ok {
			if prevKey := version.NormalizeVersion(prev); prevKey != nil && version.Compare(prevKey, key) <= 0 {
				continue
			}
		}
		out[c.MatterKey] = c.Version
	}
	return out
}

// occurrencesByMatter indexes every version a matter appeared in, so a brief
// entry can name the sibling branches that carry the same fix instead of only
// the ones that happen to fall inside the returned window.
func occurrencesByMatter(changes []Change) map[string][]string {
	out := map[string][]string{}
	for _, c := range changes {
		if c.MatterKey == "" || slices.Contains(out[c.MatterKey], c.Version) {
			continue
		}
		out[c.MatterKey] = append(out[c.MatterKey], c.Version)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool {
			a, b := version.NormalizeVersion(out[k][i]), version.NormalizeVersion(out[k][j])
			if a == nil || b == nil {
				return out[k][i] < out[k][j]
			}
			return version.Compare(a, b) < 0
		})
	}
	return out
}

// linesPresent lists the release lines this project actually publishes, in a
// stable order so the note reads the same twice.
func linesPresent(changes []Change) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range changes {
		l := version.Line(c.Version)
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// lineList renders the available lines for a human: "lines \"core\", \"flagd\"".
func lineList(lines []string) string {
	quoted := make([]string, len(lines))
	for i, l := range lines {
		quoted[i] = strconv.Quote(lineLabel(l))
	}
	if len(quoted) == 1 {
		return "only line " + quoted[0]
	}
	return "lines " + strings.Join(quoted, ", ")
}

// exampleTag returns a real tag from the given line, so the note shows the
// shape the caller should have sent rather than describing it.
func exampleTag(changes []Change, line string) string {
	for _, c := range changes {
		if version.Line(c.Version) == line {
			return c.Version
		}
	}
	return ""
}

// lineLabel names a release line for a human-facing note; the main line has no
// prefix and needs a word rather than an empty string.
func lineLabel(line string) string {
	if line == "" {
		return "main"
	}
	return line
}

// bucketRank orders buckets by urgency — action beats check beats plan.
func bucketRank(b string) int {
	switch b {
	case "action":
		return 0
	case "check":
		return 1
	case "plan":
		return 2
	}
	return 3
}

func mergeSharedQuotes(ordered []*briefChange) []*briefChange {
	byQuote := map[string]*briefChange{}
	out := make([]*briefChange, 0, len(ordered))
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
		if bucketRank(bf.Bucket) < bucketRank(kept.Bucket) {
			kept.Bucket = bf.Bucket // 병합해도 급한 쪽을 따른다
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

// ---------------------------------------------------------------------------
// get_matter
// ---------------------------------------------------------------------------

type getMatterArgs struct {
	MatterKey  string `json:"matter_key" jsonschema:"the matter_key taken verbatim from a change (case-sensitive; contains '/' and ':')"`
	IncludeAll bool   `json:"include_all,omitempty" jsonschema:"also include the routine record (mostly bot dependency bumps); off by default"`
}

func getMatterTool(ctx context.Context, req *mcp.CallToolRequest, args getMatterArgs) (*mcp.CallToolResult, any, error) {
	ctx = requestContext(ctx, "get_matter", req)
	if args.MatterKey == "" {
		return errResult(fmt.Errorf("matter_key is required — copy it from a change object")), nil, nil
	}
	raw, err := api.getMatter(ctx, args.MatterKey, args.IncludeAll)
	if err != nil {
		return errResult(err), nil, nil
	}
	res, err := jsonResult(raw)
	return res, nil, err
}
