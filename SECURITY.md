# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately via GitHub:
**[Report a vulnerability](https://github.com/garlicKim21/ratatosk-mcp/security/advisories/new)** —
do not open a public issue for security problems.

You can expect an initial response within a few days. Fixes ship as a patch
release; every release is built and published automatically from a tag.

## Scope

This repository contains the MCP server only. It is a read-only client of the
public ratatosk.io API: no credentials, no cluster permissions, and the
`check_stack` tool compares versions locally so running versions never leave
the process. Reports about the ratatosk.io service itself are also welcome
through the same channel.

## Supported versions

Only the latest release is supported. Upgrade before reporting if possible.
