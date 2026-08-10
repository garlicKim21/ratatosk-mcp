#!/usr/bin/env bash
# Install rehearsal: walk the documented install paths against a real cluster and
# assert what a user would actually get.
#
# v0.4.0 shipped a chart pinned to the previous image and an agent allowlist
# missing the release's headline tool. Nothing failed — the build passed, helm
# rendered, the server ran — because no check ever installed the thing and looked.
# This is that check. It needs a throwaway cluster (kind) with kubectl and helm
# on PATH; CI runs it on every push and again after the release image is built.
#
#   ./scripts/chart-smoke.sh [version]     # default: the chart's appVersion
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CHART=charts/ratatosk-mcp
IMAGE_REPO=ghcr.io/garlickim21/ratatosk-mcp
VERSION="${1:-$(sed -n 's/^appVersion: *"\{0,1\}\([^"]*\)"\{0,1\}$/\1/p' $CHART/Chart.yaml)}"
NS=kagent
PORT=18099

# main.go is the source of truth for what tools exist.
# The spacing after "Name:" is gofmt's business, not ours: it aligns struct
# fields, so a single-space pattern silently matched nothing the first time a
# field longer than "Name" joined the literal. With set -e that exits 1 before
# printing anything, which reads as an install failure rather than a broken
# regex (v0.7.1, 2026-08-10). Match any run of whitespace, and refuse to
# continue on an empty list instead of rehearsing against nothing.
TOOLS="$(grep -oE '^[[:space:]]*Name:[[:space:]]+"[a-z_]+"' main.go | grep -oE '"[a-z_]+"' | tr -d '"' | sort)"
[ -n "$TOOLS" ] || { echo "  ✗ no tool names found in main.go — the extraction pattern is stale, not the build" >&2; exit 1; }

pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*" >&2; exit 1; }
step() { echo; echo "── $*"; }

cleanup() { [ -n "${PF_PID:-}" ] && kill "$PF_PID" 2>/dev/null || true; }
trap cleanup EXIT

echo "rehearsing ratatosk-mcp $VERSION (tools: $(echo "$TOOLS" | tr '\n' ' '))"

# This script wipes namespace kagent between phases, which would destroy a real
# kagent install. Refuse anything that is not a throwaway kind cluster unless the
# caller says otherwise out loud.
CONTEXT="$(kubectl config current-context 2>/dev/null || echo none)"
case "$CONTEXT" in
  kind-*) ;;
  *) [ "${RATATOSK_SMOKE_ALLOW_ANY_CLUSTER:-}" = "1" ] ||
       fail "context '$CONTEXT' is not a kind cluster; this script deletes namespace $NS. Set RATATOSK_SMOKE_ALLOW_ANY_CLUSTER=1 to override." ;;
esac

# Both phases install the same names — one via helm, one via kubectl — so each run
# starts from an empty namespace. Without this a second run fails on ownership
# metadata, and that noise hides whatever the run was meant to catch.
step "fresh namespace ($CONTEXT)"
kubectl delete ns "$NS" --ignore-not-found --wait --timeout=180s >/dev/null
pass "namespace $NS reset"

step "kagent CRDs (the chart registers RemoteMCPServer + Agent)"
helm pull oci://ghcr.io/kagent-dev/kagent/helm/kagent-crds --destination /tmp >/dev/null 2>&1
helm upgrade --install kagent-crds /tmp/kagent-crds-*.tgz -n "$NS" --create-namespace >/dev/null
kubectl get crd agents.kagent.dev remotemcpservers.kagent.dev >/dev/null
pass "CRDs installed"

# The docs claim the chart runs clean under PSS restricted — enforce it so the
# claim is tested, not trusted.
kubectl label ns "$NS" pod-security.kubernetes.io/enforce=restricted --overwrite >/dev/null

assert_deployed() {
  local how="$1"
  local img
  img="$(kubectl -n $NS get deploy ratatosk-mcp -o jsonpath='{.spec.template.spec.containers[0].image}')"
  [ "$img" = "$IMAGE_REPO:$VERSION" ] || fail "$how deployed $img, expected $IMAGE_REPO:$VERSION"
  pass "$how deploys $img"
  kubectl -n $NS rollout status deploy/ratatosk-mcp --timeout=180s >/dev/null || fail "$how rollout failed"
  pass "$how pod is running (PSS restricted namespace)"

  local listed
  listed="$(kubectl -n $NS get agent ratatosk-agent -o jsonpath='{.spec.declarative.tools[0].mcpServer.toolNames[*]}' | tr ' ' '\n' | sort)"
  [ "$listed" = "$TOOLS" ] || fail "$how agent binds [$(echo "$listed" | tr '\n' ' ')], expected [$(echo "$TOOLS" | tr '\n' ' ')]"
  pass "$how agent binds every registered tool"
}

step "path A — helm with kagent.enabled=true (docs: install.*.md)"
helm upgrade --install ratatosk-mcp "$CHART" -n "$NS" --set kagent.enabled=true >/dev/null
assert_deployed "helm"

step "the served endpoint answers"
kubectl -n $NS port-forward svc/ratatosk-mcp $PORT:8080 >/dev/null 2>&1 &
PF_PID=$!
for _ in $(seq 1 20); do curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 && break; sleep 1; done
curl -sf -o /dev/null "http://127.0.0.1:$PORT/healthz" || fail "/healthz did not answer"
pass "/healthz 200"

SESSION="$(curl -s -D- -o /tmp/mcp-init.json -X POST "http://127.0.0.1:$PORT/mcp" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' \
  | grep -i '^mcp-session-id' | tr -d '\r' | cut -d' ' -f2)"
[ -n "$SESSION" ] || fail "no MCP session id"

curl -s -X POST "http://127.0.0.1:$PORT/mcp" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' > /tmp/mcp-tools.json

python3 - "$VERSION" <<'PY'
import json, re, sys
want_version = sys.argv[1]
def payload(path):
    for line in open(path):
        line = line.strip()
        if line.startswith("data: "):
            line = line[6:]
        if line.startswith("{"):
            return json.loads(line)
    raise SystemExit(f"  ✗ no JSON-RPC payload in {path}")
served = payload("/tmp/mcp-init.json")["result"]["serverInfo"]["version"]
if served != want_version:
    raise SystemExit(f"  ✗ served version {served}, expected {want_version}")
print(f"  ✓ serverInfo reports {served}")
names = sorted(t["name"] for t in payload("/tmp/mcp-tools.json")["result"]["tools"])
open("/tmp/mcp-served-tools", "w").write("\n".join(names))
print(f"  ✓ tools/list serves {len(names)}: {' '.join(names)}")
PY
[ "$(cat /tmp/mcp-served-tools)" = "$TOOLS" ] || fail "served tools differ from the ones registered in main.go"
pass "served tools match main.go"

kill "$PF_PID" 2>/dev/null || true; PF_PID=""

step "path B — plain manifests (docs: install.*.md, examples/kagent)"
helm uninstall ratatosk-mcp -n "$NS" >/dev/null
kubectl wait --for=delete pod -l app.kubernetes.io/name=ratatosk-mcp -n "$NS" --timeout=120s >/dev/null 2>&1 || true
for f in ratatosk-deploy ratatosk-remote-mcpserver ratatosk-agent; do
  kubectl apply -f "examples/kagent/$f.yaml" >/dev/null
done
assert_deployed "manifests"

step "path C — local stdio (docs: install.*.md)"
if command -v docker >/dev/null 2>&1; then
  { printf '%s\n' \
      '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' \
      '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
      '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 5; } \
    | docker run -i --rm "$IMAGE_REPO:$VERSION" > /tmp/stdio.out 2>/dev/null
  python3 - "$VERSION" <<'PY'
import json, sys
want = sys.argv[1]
got_version = got_tools = None
for line in open("/tmp/stdio.out"):
    try: msg = json.loads(line)
    except ValueError: continue
    if msg.get("id") == 1: got_version = msg["result"]["serverInfo"]["version"]
    if msg.get("id") == 2: got_tools = sorted(t["name"] for t in msg["result"]["tools"])
if got_version != want:
    raise SystemExit(f"  ✗ stdio served {got_version}, expected {want}")
open("/tmp/stdio-tools", "w").write("\n".join(got_tools or []))
print(f"  ✓ stdio serves {got_version} with {len(got_tools or [])} tools")
PY
  [ "$(cat /tmp/stdio-tools)" = "$TOOLS" ] || fail "stdio tools differ from main.go"
  pass "stdio tools match main.go"
else
  echo "  – docker unavailable, skipped"
fi

echo
echo "all documented install paths serve $VERSION with every registered tool"
