# Contributing

Issues and pull requests are welcome — bug reports, tool UX feedback from
real agent usage, and documentation fixes especially.

## Build & test

No local Go toolchain needed; everything runs through Docker:

```bash
docker run --rm -v $PWD:/src -w /src -e GOFLAGS=-buildvcs=false golang:1.26 \
  sh -c "go build ./... && go test ./..."
```

The Helm chart lints with:

```bash
docker run --rm -v $PWD/charts:/charts alpine/helm:3 lint /charts/ratatosk-mcp
```

## Pull requests

- Keep the read-only, no-credential design: tools must never require secrets,
  and nothing from the user's environment may be sent upstream beyond project
  slugs (see the check_stack privacy contract in the README).
- If you change tool descriptions or add a tool, remember they are read by
  agent LLMs at runtime — descriptions are behavioral instructions, not just
  docs.
- Releases are automated: a version tag builds the multi-arch image, creates
  the GitHub Release, and publishes to the official MCP registry. Maintainers
  handle tagging; PRs should not bump versions.

## Conduct

Be kind. Assume good intent.
