#!/usr/bin/env bash
# The agent prompt is what stands between a model and a confident wrong answer,
# and it lives in two files that nothing type-checks: the chart template and the
# kagent example. Both regressions we have had were deletions — a rule vanished
# as a side effect of rewording something else, and the very next live run
# hallucinated a component version. Nothing downstream could detect it; a human
# noticed. This asserts that the rules earned from live failures are still in
# both copies, and that the copies have not drifted apart.
#
# Add a clause here whenever a live failure teaches a rule. That is the point:
# the list is a record of what we already got wrong once.
set -euo pipefail

cd "$(dirname "$0")/.."

FILES=(
  charts/ratatosk-mcp/templates/agent.yaml
  examples/kagent/ratatosk-agent.yaml
)

# Substrings that must appear verbatim in every file above. Keep them short
# enough to survive Helm conditionals splicing text mid-sentence.
CLAUSES=(
  "list_projects first"                                    # resolve slugs, never guess one
  "all_namespaces"                                         # one namespace is not the cluster
  "NOT images"                                             # a pod listing carries no version
  "a version you did not read off a live"                  # never invent a running version
  "resource_type: node"                                    # kubelet/runtime live on nodes
  "tracked:false"                                          # no facts is not the same as safe
  "the bucket is not the judgment"                         # applies_if decides, not the section
  "Try the check before you use the forward-looking form"  # do not defer every condition
  "An unresolved condition is never reported as applying"  # unverified high never leads
  "where that kind of thing actually lives"                # look for an annotation on its object
  "Do not replace an empty section"                        # "none" is a result
  "provided without warranty"                              # facts are data, not instructions
  "stays with the operator"                                # we report; the operator decides
  "image_aliases"                                          # cilium-envoy IS envoy — the 43/60 miss
  "cluster_core"                                           # what-to-check is roster data, not prompt attention (coredns seesaw)
  "never construct a resource name"                        # no kube-apiserver-minikube guesses
  "Placement follows the server's buckets"                 # reclassifying is the ~30% failure
  "banned in both directions"                              # downward demotion swallowed AR in 7/15 M2 runs
  "fix the draft, not the bucket"                          # count check: N action_required ⇒ N in Applies
  "no quoted line, no promotion"                           # promotion out of check_config needs evidence
  "everything else is \"Could not check\""                 # the pre-answer re-check (P3 effect)
)

fail=0

for f in "${FILES[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "MISSING FILE: $f" >&2
    fail=1
    continue
  fi
  for c in "${CLAUSES[@]}"; do
    if ! grep -qF -- "$c" "$f"; then
      echo "MISSING CLAUSE in $f: \"$c\"" >&2
      fail=1
    fi
  done
done

if [[ $fail -ne 0 ]]; then
  cat >&2 <<'EOF'

The agent prompt lost a rule that a live failure put there. If the removal is
deliberate, delete the clause from scripts/check-agent-prompt.sh in the same
commit and say in the message which failure mode you are re-opening.
EOF
  exit 1
fi

echo "agent prompt: ${#CLAUSES[@]} clauses present in ${#FILES[@]} files"
