# adit-code

[![CI](https://github.com/justinstimatze/adit-code/actions/workflows/ci.yml/badge.svg)](https://github.com/justinstimatze/adit-code/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/justinstimatze/adit-code?v=1)](https://goreportcard.com/report/github.com/justinstimatze/adit-code)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Structural analysis for AI-edited codebases. Finds the files that cost your
agent the most tool calls and tells you exactly what to fix.

AI coding agents read files, grep for definitions, and trace imports. When a
file has 51 parameters on one function, 8 ambiguous names polluting every grep,
and single-consumer imports scattered across the repo, the agent burns context
re-reading and re-searching. adit finds these structural hot spots and maps
each one to a specific refactoring action.

> *An **adit** is a horizontal tunnel driven into a hillside to provide access
> to a mine. adit-code tunnels into your codebase to find the structural hot
> spots.*

## What It Reports

```
$ adit score --pretty .

adit v0.1.0

  File              Lines  Size   Nest  Reads  Unneeded  Noise  Blast
  ────────────────────────────────────────────────────────────────────
  engine.py          3200    D       8     12        4      8      8
  handlers.py         890    B       5      7        3      2      3
  utils.py            420    A       3      2        0      0     15

  Co-locate (single-consumer imports):
    TRUST_HINTS (const)   constants.py:15 -> handlers.py

  Ambiguous names:
    _validate   3 defs: engine.py:42 handlers.py:88 auth.py:15

  Uncommented (large files, 0 comments — AI agents benefit from comments):
    engine.py (3200 lines)

  Import cycles: none
```

Every finding maps to a refactoring the agent (or you) can execute:

| Metric | What to do |
|--------|------------|
| **Lines** (Grade D/F) | Split this file — agent can't hold it in context |
| **Max Nesting** (>6) | Extract nested logic — agent loses track of control flow |
| **Max Params** (>10) | Use a config object — agent will misorder arguments |
| **Grep Noise** | Rename `_validate` — agent gets 5 false positives per search |
| **Unnecessary Reads** | Move `X` to its only consumer — saves a `Read` tool call |
| **Import Cycles** | Break the A→B→A cycle — agent loops reading both files |
| **Blast Radius** | 30 files import this — agent must verify callers after edits |

## Validated Against Real Agent Behavior

Correlated against [SWE-bench agent trajectories](https://huggingface.co/datasets/nebius/SWE-rebench-openhands-trajectories)
— 1,840 files across 49 repos. File size is the strongest
predictor of agent tool call cost (median Spearman +0.474, positive on all 49
repos). The structural metrics tell you what specifically to fix:

| Metric | Median per-repo | Positive repos |
|--------|----------------|----------------|
| **Lines** | **+0.474** | **49/49** |
| **Max Nesting** | +0.344 | 48/49 |
| **Max Params** | +0.311 | 46/49 |
| **Grep Noise** | +0.241 | 38/49 |
| **Blast Radius** | +0.165 | 40/49 |
| **Unnecessary Reads** | +0.135 | 39/49 |

File size dominates. The structural metrics don't predict *better* — they
tell the agent *what to fix*. `wc -l` says "this file is 3,200 lines."
adit says "split it, refactor the 15-parameter function into a config
object, and extract those 8 levels of nesting into helpers."

## Install

**Binary release** (no Go required):

```bash
# Download from GitHub Releases
curl -sL https://github.com/justinstimatze/adit-code/releases/latest/download/adit-code_linux_amd64.tar.gz | tar xz
sudo mv adit /usr/local/bin/
```

Binaries available for Linux (amd64) and macOS (amd64, arm64).

**Docker:**

```bash
docker run --rm -v "$PWD":/src -w /src ghcr.io/justinstimatze/adit-code score --pretty .
```

**From source** (requires Go 1.25+):

```bash
go install github.com/justinstimatze/adit-code/cmd/adit@latest
```

Single binary. Analyzes Python, TypeScript, and Go with one tool — no need
for separate linters per language. Uses tree-sitter for parsing. No runtime
dependencies.

Designed for codebases where AI agents do most or all of the editing.
Some metrics overlap with traditional linters — adit's value is the
co-location and grep noise metrics that no linter checks, cross-language
consistency, and the `--diff` CI gate that prevents structural regressions.

## Quick Start

```bash
adit score .                          # JSON (default — for CI and AI tools)
adit score --pretty .                 # Human-readable table
adit score --diff HEAD~1 --pretty .   # What changed and what got worse
adit enforce .                        # CI gate — exit 1 if thresholds exceeded
adit enforce --diff HEAD~1 .          # Only enforce on changed files
adit mcp                              # MCP server for Claude Code / Codex
```

## What It Measures

Five metrics. No composite score — each maps to a specific agent cost.

**Unnecessary Reads** — Single-consumer imports that should be co-located.
`TRUST_HINTS` is imported by only `handlers.py` — move it there and save
the agent a `Read` tool call. *No existing linter checks this.*

**Grep Noise** — How many false positives does the agent hit when searching
for names in this file? `_validate` defined in 5 files = 4 extra grep results
the agent must open, read, and discard. *No existing linter checks this.*

**File Size** — How many `Read` calls does this file cost? Agents read in
chunks. Larger files require more reads and the agent loses coherence across
distant methods. Grades A (<500 lines) through F (5000+).

**Blast Radius** — How many files import from this one? High-blast files are
central definitions the agent must re-read for context on every edit.

**Import Cycles** — Circular dependencies that trap the agent in read loops
(A reads B, B reads A, repeat).

## Configuration

Create `adit.toml` in your project root:

```toml
[thresholds]
max_file_lines = 3000
max_context_reads = 10
max_unnecessary_reads = 3
max_ambiguity_defs = 3
max_blast_radius = 20
max_cycle_length = 0           # 0 = no cycles allowed

[scan]
exclude = ["test_*.py", "*.test.ts", "**/__pycache__/**", "**/node_modules/**"]

# Looser thresholds for specific paths
[per-path."test_*.py"]
max_file_lines = 5000
```

Also reads `[tool.adit]` from `pyproject.toml`. Config is auto-discovered by
walking up from the target path.

## AI Agent Integration

### MCP Server (Claude Code, Codex, any MCP-compatible agent)

```json
{
  "mcpServers": {
    "adit": { "command": "adit", "args": ["mcp"] }
  }
}
```

8 tools including `adit_briefing` — call before editing any file to get
cross-file warnings upfront:

```
handler.py: 890 lines (Grade B), nesting 5, 47 AST types
⚠ _validate defined in 5 other files — use qualified search
⚠ 30 files import from here — verify callers after editing
⚠ TRUST_HINTS only used here — co-locate from constants.py
```

This front-loads information the agent would otherwise discover one grep
at a time. Also: `adit_score_repo`, `adit_score_file`, `adit_relocatable`,
`adit_ambiguous`, `adit_blast_radius`, `adit_cycles`, `adit_diff`.

### CLI + JSON (any agent that shells out)

JSON-default output. Any agent that can run commands and parse JSON works:

```bash
adit score .              # full analysis
adit enforce --diff $REF  # PR gate
```

### CLAUDE.md / AGENTS.md

Add to your project's AI instruction file:

```markdown
## Structural Health
This project uses adit for code structure analysis.
- Before committing: run `adit enforce .` — fix violations, don't skip.
- When adit reports relocatable imports: move the definition to the consumer file.
- When adit reports ambiguous names: rename to be more distinctive.
```

### PR Review Bot

adit can comment on PRs with structural regressions. Copy
`.github/workflows/pr-review.yml` to your repo — it runs `adit --diff`
on every PR and posts a comment like:

> **adit structural review**
>
> ⚠️ **2 regression(s):**
> - `handler.py`: max_nesting_depth 5→8 ▲ — consider extracting nested logic
> - `utils.py`: lines 420→680 ▲ — consider splitting

### Pre-commit Hook

```yaml
repos:
  - repo: local
    hooks:
      - id: adit
        name: adit enforce
        entry: adit enforce
        language: system
        types_or: [python, ts, tsx, go]
```

## CLI Reference

```
adit score [PATHS...]                  # JSON (default)
adit score --pretty [PATHS...]         # Human table
adit score --sarif [PATHS...]          # SARIF v2.1.0 (report only, exit 0)
adit score --diff REF [PATHS...]       # Compare against git ref
adit score --min-grade C .             # Only files at grade C or worse
adit score --sort blast .              # Sort: blast|reads|noise|size|name
adit score --exclude "*.gen.go" .      # Exclude files by glob pattern
adit score --quiet .                   # Metrics only, no recommendations
adit score --silent .                  # No output, exit code only

adit enforce [PATHS...]                # Exit 1 if thresholds exceeded
adit enforce --diff REF .              # Only enforce on changed files
adit enforce --sarif .                 # SARIF output + exit code
adit enforce --silent .                # Exit code only

adit mcp                               # MCP server (stdio)
```

Exit codes: **0** = pass, **1** = issues found, **2** = tool error.

## Performance

Parallel parsing with O(1) module resolution. Cross-file analysis uses
indexed lookups rather than O(n²) scans:

| Repo size | Time |
|-----------|------|
| <200 files | <1s |
| ~500 files | ~3s |
| ~1,000 files | ~10s |

## Across Open Source Projects

adit scored against 33 open source projects across Python, TypeScript, and Go.
Generated files (migrations, protobuf, etc.) are auto-detected and excluded.

**Python**

| Project | Files | A | B | C | D | F | Nest | Params |
|---------|-------|---|---|---|---|---|------|--------|
| [Sentry](https://github.com/getsentry/sentry) | 4,226 | 3,932 | 268 | 21 | 4 | 1 | 13 | 29 |
| [PyTorch](https://github.com/pytorch/pytorch) | 2,142 | 1,608 | 375 | 104 | 33 | 22 | 11 | 53 |
| [yt-dlp](https://github.com/yt-dlp/yt-dlp) | 1,125 | 1,056 | 55 | 10 | 3 | 1 | 11 | 15 |
| [CPython](https://github.com/python/cpython) stdlib | 1,110 | 903 | 147 | 45 | 9 | 6 | 11 | 26 |
| [SymPy](https://github.com/sympy/sympy) | 923 | 640 | 189 | 67 | 22 | 5 | 13 | 22 |
| [Salt](https://github.com/saltstack/salt) | 910 | 646 | 198 | 45 | 15 | 6 | 16 | 51 |
| [Django](https://github.com/django/django) | 853 | 774 | 65 | 13 | 1 | - | 11 | 25 |
| [OpenStack Nova](https://github.com/openstack/nova) | 776 | 681 | 64 | 23 | 5 | 3 | 10 | 47 |
| [Ansible](https://github.com/ansible/ansible) | 577 | 498 | 68 | 10 | 1 | - | 12 | 25 |
| [pandas](https://github.com/pandas-dev/pandas) | 452 | 333 | 74 | 26 | 13 | 6 | 10 | 35 |
| [scikit-learn](https://github.com/scikit-learn/scikit-learn) | 383 | 239 | 108 | 29 | 6 | 1 | 9 | 24 |
| [Odoo](https://github.com/odoo/odoo) | 344 | 292 | 39 | 10 | 2 | 1 | 12 | 15 |
| [Scrapy](https://github.com/scrapy/scrapy) | 177 | 165 | 12 | - | - | - | 8 | 15 |
| [Celery](https://github.com/celery/celery) | 161 | 138 | 21 | 2 | - | - | 11 | 30 |
| [matplotlib](https://github.com/matplotlib/matplotlib) | 156 | 87 | 38 | 21 | 8 | 2 | 9 | 24 |
| [FastAPI](https://github.com/tiangolo/fastapi) | 48 | 41 | 4 | 1 | 2 | - | 11 | 38 |
| [Flask](https://github.com/pallets/flask) | 24 | 17 | 6 | 1 | - | - | 7 | 10 |

**TypeScript**

| Project | Files | A | B | C | D | F | Nest | Params |
|---------|-------|---|---|---|---|---|------|--------|
| [TypeScript](https://github.com/microsoft/TypeScript) compiler | 709 | 557 | 96 | 31 | 12 | 13 | 18 | 26 |
| [TypeORM](https://github.com/typeorm/typeorm) | 491 | 441 | 34 | 8 | 6 | 2 | 19 | 4 |
| [Prisma](https://github.com/prisma/prisma) client | 422 | 414 | 7 | - | - | 1 | 8 | 6 |
| [Hono](https://github.com/honojs/hono) | 208 | 192 | 14 | 2 | - | - | 13 | 6 |
| [Deno std](https://github.com/denoland/std) path | 98 | 97 | 1 | - | - | - | 21 | 4 |
| [tRPC](https://github.com/trpc/trpc) server | 82 | 77 | 5 | - | - | - | 8 | 3 |
| [React Router](https://github.com/remix-run/react-router) | 68 | 53 | 10 | 3 | 1 | 1 | 9 | 21 |

**Go**

| Project | Files | A | B | C | D | F | Nest | Params |
|---------|-------|---|---|---|---|---|------|--------|
| [CockroachDB](https://github.com/cockroachdb/cockroach) sql | 2,231 | 1,826 | 325 | 52 | 16 | 12 | 13 | 22 |
| [Kubernetes](https://github.com/kubernetes/kubernetes) pkg | 1,980 | 1,804 | 148 | 24 | 2 | 2 | 9 | 40 |
| [Terraform](https://github.com/hashicorp/terraform) | 1,201 | 1,087 | 105 | 7 | 2 | - | 11 | 8 |
| [Docker](https://github.com/moby/moby) daemon | 944 | 866 | 74 | 4 | - | - | 10 | 9 |
| [GitHub CLI](https://github.com/cli/cli) | 389 | 366 | 22 | 1 | - | - | 8 | 10 |
| [geppetto](https://github.com/go-go-golems/geppetto) | 220 | 206 | 14 | - | - | - | 11 | 8 |
| [glazed](https://github.com/go-go-golems/glazed) | 210 | 197 | 13 | - | - | - | 9 | 6 |
| [TiDB](https://github.com/pingcap/tidb) executor | 193 | 128 | 53 | 9 | 2 | 1 | 11 | 16 |
| [go-go-mcp](https://github.com/go-go-golems/go-go-mcp) | 144 | 137 | 7 | - | - | - | 9 | 5 |

**Key findings:**
- **Salt** has the worst parameter complexity (51 params) and deepest nesting
  (16 levels). PyTorch follows with 53 max params. Nova has 47.
- **TypeScript compiler** and **TypeORM** have the deepest nesting in TS
  (18-19). Deno std reaches 21 despite tiny file sizes.
- **Go projects** have lower max params (5-40) and moderate nesting (8-13).
  CockroachDB is the exception with 12 Grade F files.

Reproduce on your own repos:

```bash
adit score --pretty /path/to/project
```

## License

MIT. All dependencies MIT or BSD-2. See [INFLUENCES.md](INFLUENCES.md) for
full citation record, prior art search, and license audit.
