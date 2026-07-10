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

	server := mcp.NewServer(&mcp.Implementation{Name: "ratatosk", Version: "0.1.0"}, nil)

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
			"facts=[] with coverage=full_reviewed means the release was read and is routine — auditable silence.",
	}, getReleaseTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "check_stack",
		Description: "Check the user's running component versions against known facts. Versions are compared " +
			"LOCALLY — only project slugs are sent to the ratatosk server, running versions never leave this process. " +
			"Returns, per component, the facts from releases NEWER than the running version (the upgrade path).",
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

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
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
	Project string `json:"project" jsonschema:"project slug, e.g. envoy"`
	Version string `json:"version" jsonschema:"release tag exactly as published, e.g. v1.38.3"`
}

func getReleaseTool(ctx context.Context, req *mcp.CallToolRequest, args getReleaseArgs) (*mcp.CallToolResult, any, error) {
	if args.Project == "" || args.Version == "" {
		return errResult(fmt.Errorf("project and version are required")), nil, nil
	}
	raw, err := api.getRelease(args.Project, args.Version)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

// ---------------------------------------------------------------------------
// check_stack — the privacy loop
// ---------------------------------------------------------------------------

type stackComponent struct {
	Project string `json:"project" jsonschema:"project slug, e.g. envoy"`
	Version string `json:"version" jsonschema:"the version currently running, e.g. v1.36.8"`
}

type checkStackArgs struct {
	Components []stackComponent `json:"components" jsonschema:"the running stack to check"`
}

type componentReport struct {
	Project        string `json:"project"`
	RunningVersion string `json:"running_version"`
	Note           string `json:"note,omitempty"`
	FactsScanned   int    `json:"facts_scanned"`
	RelevantFacts  []Fact `json:"relevant_facts"`
}

func checkStackTool(ctx context.Context, req *mcp.CallToolRequest, args checkStackArgs) (*mcp.CallToolResult, any, error) {
	if len(args.Components) == 0 {
		return errResult(fmt.Errorf("components is required")), nil, nil
	}
	reports := make([]componentReport, 0, len(args.Components))
	for _, comp := range args.Components {
		rep := componentReport{Project: comp.Project, RunningVersion: comp.Version, RelevantFacts: []Fact{}}
		currentKey := version.NormalizeVersion(comp.Version)
		if currentKey == nil {
			rep.Note = "running version could not be parsed; showing no facts (use list_facts to inspect manually)"
			reports = append(reports, rep)
			continue
		}
		facts, err := api.allProjectFacts(comp.Project)
		if err != nil {
			rep.Note = "fetch failed: " + err.Error()
			reports = append(reports, rep)
			continue
		}
		rep.FactsScanned = len(facts)
		unparseable := 0
		for _, f := range facts {
			factKey := version.NormalizeVersion(f.Version)
			if factKey == nil {
				unparseable++
				continue
			}
			// Facts from releases strictly newer than the running version =
			// the upgrade path the operator has not yet absorbed.
			if version.Compare(factKey, currentKey) > 0 {
				rep.RelevantFacts = append(rep.RelevantFacts, f)
			}
		}
		if unparseable > 0 {
			rep.Note = fmt.Sprintf("%d fact(s) skipped (unparseable release tag)", unparseable)
		}
		reports = append(reports, rep)
	}
	res, err := jsonResult(map[string]any{
		"components": reports,
		"privacy":    "versions were compared locally; only project slugs were sent to the server",
	})
	return res, nil, err
}
