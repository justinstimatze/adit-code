# adit-code — Claude Code Instructions

## Build & Test

```bash
go build ./cmd/adit        # Build binary
go test ./... -count=1     # Run all tests
./adit score --pretty .    # Self-test on own source
```

Go 1.25+ required (for modelcontextprotocol/go-sdk).

## Architecture

Layer model: **Library -> MCP Server -> CLI**.

```
internal/score/     Language-agnostic scoring engine. Receives parsed data, computes metrics.
internal/lang/      Tree-sitter frontends (Python, TypeScript, Go, Rust). Each implements Frontend interface.
internal/config/    Config loading from adit.toml or pyproject.toml [tool.adit]. Per-path overrides.
internal/mcp/       MCP server (stdio, 9 tools). Thin wrapper around the scoring pipeline.
internal/output/    Pretty printing and threshold checking.
internal/diff/      Git operations for --diff mode (changed files, file-at-ref).
cmd/adit/main.go    CLI entry point. score, enforce, mcp subcommands.
cmd/adit-validate/  Research tool for correlating metrics with git history and session transcripts.
dist/pypi/          PyPI package metadata (not yet published).
dist/npm/           npm package metadata (not yet published).
```

## Key Design Decisions

- **JSON-default output.** CI and AI tools are the primary consumers. `--pretty` is opt-in for humans.
- **No composite score.** Five independent metrics, five independent thresholds. A file can be grade A on size but terrible on co-location — that tells you exactly what to fix.
- **All scoring is language-agnostic.** The `internal/score/` package never imports tree-sitter. It receives `[]lang.FileAnalysis` structs. Language-specific parsing lives in `internal/lang/`.
- **Two-pass pipeline.** Pass 1: parse all files, collect imports and definitions. Pass 2: score each file using cross-file maps (consumer counts, name index).
- **Import kind heuristic.** Python: UPPER_CASE = constant, CamelCase = type, lowercase = function. TypeScript: `import type` syntax detected. Not perfect, good enough for prioritization.
- **Go module resolution.** Reads `go.mod` to determine the module path, then checks if imports start with it. Correctly distinguishes project-local from stdlib/third-party imports.
- **Config auto-discovery.** Walks up from the target path (not CWD) to find `adit.toml` or `pyproject.toml [tool.adit]`.

## Adding a New Language Frontend

1. Create `internal/lang/newlang.go` implementing the `Frontend` interface
2. The interface has two methods: `Extensions() []string` and `Analyze(path string, src []byte) (*FileAnalysis, error)`
3. Use tree-sitter to parse. Extract imports and definitions into the shared data structures.
4. Register the frontend in `cmd/adit/main.go` `buildPipeline()` and `internal/mcp/server.go` `newPipeline()`
5. Add test fixtures in `testdata/newlang/` and tests in `internal/lang/newlang_test.go`

## Adding a New Metric

1. Create `internal/score/newmetric.go` with the scoring function
2. Add result fields to `FileScore` and `RepoSummary` in `internal/score/types.go`
3. Call from `internal/score/pipeline.go` `ScoreRepo()`
4. Wire into JSON output (automatic via struct tags) and `--pretty` output in `internal/output/pretty.go`
5. Add threshold to `internal/config/config.go` and `CheckFileThresholds()` in `internal/output/helpers.go`
6. Add MCP tool in `internal/mcp/server.go` if the metric warrants its own query

## Conventions

- Zero runtime dependencies beyond tree-sitter and MCP SDK. No logging frameworks, no color libraries.
- ANSI colors are raw escape codes, gated by TTY detection and `--color` flag.
- All Go dependencies must be MIT or BSD-2 licensed. Check before adding.
- Test files use fixtures in `testdata/`. Inline synthetic code strings for unit tests.
- The `internal/score/` package must remain importable without tree-sitter (language-agnostic).

## Metric Design Notes

Validated against SWE-bench agent trajectories (1,840 files across 49 repos,
nebius/SWE-rebench-openhands-trajectories). Median per-repo Spearman rank
correlations with agent tool call counts:

- **Lines** (median +0.474, 49/49 positive). The baseline predictor.
- **Max Nesting** (+0.344, 48/49). Depth of control flow nesting.
- **Max Params** (+0.311, 46/49). Largest function parameter count.
- **Grep Noise** (+0.241, 38/49). Ambiguous names causing search noise.
- **Blast Radius** (+0.165, 40/49). Files imported by many others.
- **Unnecessary Reads** (+0.135, 39/49). Single-consumer imports.

File size dominates. The structural metrics tell you *what specifically to fix*
rather than predicting *better* than size alone.

To reproduce: `python3 cmd/swe-bench-validate/main.py && python3 cmd/swe-bench-validate/correlate.py`

## Running adit on Itself

```bash
./adit score --pretty cmd/ internal/
```

The Go source should score well. If it doesn't, fix the structure.
Note: `Extensions` and `Analyze` show as ambiguous because they're defined in
all three language frontend files (implementing the same interface). This is
expected and not actionable — interface implementations inherently share names.

## Investigation Results (completed)

1. **Reference-based ambiguity** — IMPLEMENTED. `grep_noise` now includes both
   definition noise (names this file defines that collide) and reference noise
   (ambiguous names this file imports).

2. **Max Function Length** — IMPLEMENTED but weak. Partial correlation +0.156
   after controlling for file size. Mostly a proxy for lines. Kept in JSON
   for catalog-vs-monolith distinction but not a strong predictor.

3. **SWE-bench validation** — COMPLETED. 1,840 files across 49 repos from
   nebius/SWE-rebench-openhands-trajectories. File size is the dominant
   predictor (median Spearman +0.474). Structural metrics add diagnostic
   value — they tell you what to fix, not that they predict better than size.

## Known Blind Spots

- **Catalogs vs algorithms.** The size grade treats all lines as equal, but
  a 10K-line flat catalog of independent entries (builtins.go, _extractors.py)
  is fundamentally different from a 10K-line algorithm with cross-references
  (checker.ts, _axes.py). The catalog is easy to edit at any chunk; the
  algorithm requires understanding distant interactions. A future metric
  could measure **function length distribution** — a 5K-line file with 200
  functions averaging 25 lines is a catalog (each chunk is self-contained).
  A 5K-line file with 10 functions averaging 500 lines is a monolith (the
  AI can't read a single function in one chunk). Max and avg function length
  are cheap to compute from tree-sitter node boundaries. Internal coupling
  (do methods call each other?) is a complementary but more expensive signal.

- **Generated code.** Auto-detected via header comments, .gitattributes, and
  filename patterns, but detection is heuristic. Some generated files may
  slip through (especially generated-but-hand-edited hybrids).

- **Macro-only references (Rust).** `MacroReferenceCount` only sees raw
  identifier tokens inside `macro_rules!` invocations — it has no visibility
  into proc-macro expansion at all, so it helps with declarative-macro
  forwarding patterns and nothing else. A matched identifier is only credited
  when its name is unambiguous among local definitions; two same-named defs
  sharing a match are both skipped rather than double-credited. Surfaced via
  `FileScore.MacroReferencedDefs` and an `adit_briefing` warning.

## Not Yet Published

- PyPI binary wrapper (`pip install adit-code`) — metadata ready in `dist/pypi/`
- npm binary wrapper (`npm install adit-code`) — metadata ready in `dist/npm/`

<!-- defn:begin -->
## Code Navigation and Editing

**The database is authoritative. Files are an I/O projection.** This project
is indexed in defn. For **Go code**, use the `code` MCP tool — **not**
Read, Edit, Write, or Grep. Reserve those built-in tools for non-Go files
(YAML, JSON, Markdown, shell, `go.mod`).

```
code(op: "read", name: "handleEdit")           -- full source by name
code(op: "read", name: "server.go:272")        -- or by file:line
code(op: "impact", name: "Render")             -- blast radius + test coverage
code(op: "edit", name: "Foo", new_body: "...") -- edit, auto-emit + build
code(op: "search", pattern: "%Auth%")          -- name pattern (% wildcard)
code(op: "search", pattern: "authentication")  -- body text search
code(op: "test", name: "Render")               -- run affected tests only
```

All ops: read, search, impact, explain, untested, edit, create, delete, rename, move, test, apply, diff, history, find, sync, query, overview, patch.

### Why defn for Go, not Read/Edit/Grep

- `code(op:"read")` returns a whole definition by name — no line-number guessing, no reading a file to find one function.
- `code(op:"edit")` updates one definition, emits the file, and rebuilds the reference graph in one call. A raw file Edit leaves defn's graph stale until a `sync`.
- `code(op:"rename")` / `move` update every reference and import site across the repo in one call — many fragile Edits otherwise.
- `code(op:"impact")` gives callers + transitive blast radius + test coverage before you touch anything.

If you do edit a `.go` file with a built-in tool, call `code(op:"sync", file:"path")` afterward so the graph stays correct.

**Rule of thumb:** run `impact` before modifying an existing definition; skip it for brand-new ones.
<!-- defn:end -->
