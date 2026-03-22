# Changelog

All notable changes to adit-code will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] - 2026-03-21

### Added
- Metrics: file size grade, context reads, unnecessary reads, grep noise, blast radius, import cycles, max nesting, max params, node diversity
- Three language frontends: Python, TypeScript, Go (via tree-sitter)
- CLI with `score` (JSON default + `--pretty`), `enforce`, and `mcp` subcommands
- `--diff REF` mode for regression detection against git refs
- MCP server with 8 tools for AI coding agent integration
- Per-path threshold overrides in `adit.toml`
- Config auto-discovery from target path
- Go module resolution via `go.mod`
- Generated file auto-detection (header comments, .gitattributes, filename patterns)
- SWE-bench validation tooling against agent trajectory datasets
- Benchmarked against 33 open source projects across Python, TypeScript, and Go

### Validated against SWE-bench agent trajectories (N=1,840 files, 49 repos)
- **Lines**: median Spearman +0.474 (49/49 positive)
- **Max Nesting**: +0.344 (48/49 positive)
- **Max Params**: +0.311 (46/49 positive)
- **Grep Noise**: +0.241 (38/49 positive)
- **Blast Radius**: +0.165 (40/49 positive)
- **Unnecessary Reads**: +0.135 (39/49 positive)
