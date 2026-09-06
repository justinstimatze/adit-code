# Changelog

All notable changes to adit-code will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/).

## [0.2.0] - 2026-09-06

### Added
- Fourth language frontend: Rust (via tree-sitter-rust), covering imports,
  functions, structs, enums, traits, impl blocks, modules, and macro_rules!
- FFI boundary metric: counts `extern "C"` crossings per file, surfaced in
  `--pretty` output, JSON, `--diff` regression tracking, and a new
  `adit_ffi_boundary` MCP tool (9 tools total, up from 8)
- `MacroReferencedDefs` warning in `adit_briefing`: flags definitions only
  reachable via macro invocation, invisible to plain identifier search

### Fixed
- Else-if chain undercounting in the max-branching metric: Go, Rust, and
  TypeScript all nest `else if` one level deeper than Python's flat
  `elif_clause` siblings, capping long chains at branching factor 2
  regardless of length
- `cmd/adit-validate` frontend list brought to parity across all four
  languages (three of its four construction sites were also missing Go)

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
