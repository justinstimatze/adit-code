# Adit — AI-Navigability Audit Tool

**Status**: Pre-project. Concept validated through production use in The Stope
(a text-based investigation game, ~30K lines Python, exclusively AI-edited
across 22+ sessions). This document captures everything needed to start building.

## Origin

Adit grew out of structural refactoring work on The Stope (`/home/justin/Documents/lamina/poc/dense/stope/`). The game engine was a 12,232-line single file (`engine.py`). After building a mutation audit tool, running radon/wily complexity analysis, and splitting the file into 4 mixin modules, we realized the principles and tooling were generalizable.

The name: an **adit** is a horizontal tunnel driven into a hillside to provide access to a mine. The tool provides AI agents access to codebases by auditing and enforcing structural properties that make code navigable.

## The Problem

No existing tool audits or enforces code structure specifically for AI coding tool manipulation. The ecosystem has:

- **Context packaging** (Repomix, aider repo map, gitingest) — packing repos for AI consumption. Well-served.
- **Instruction files** (AGENTS.md, CLAUDE.md, .cursorrules) — telling AI how to behave. Maturing under Linux Foundation stewardship.
- **Code health metrics** (radon, CodeScene CodeHealth) — measuring complexity for humans.

What's missing: a tool that tells you "this file is too large for AI to hold in context," "these constants should be co-located with the methods that use them," "this method is called from 3 different files and changes to it require reading all 3."

The CodeScene FORGE/ICSE 2026 paper ("Code for Machines, Not Just Humans") found that CodeHealth predicts AI refactoring success — unhealthy code has 30%+ higher AI defect risk. But they only tested single-file refactoring. Multi-file navigation and editing — where AI coding tools spend most of their time — is where AI-specific structural properties diverge from human readability.

## Core Principles (validated in production)

These come from 22 sessions of exclusively AI-edited development on a ~30K line Python codebase. The human reviews diffs and approves commits but never hand-edits files.

### 1. File size is the primary structural constraint

AI works by reading chunks (~100 lines) and grepping. Empirical sweet spots:

| File size | AI experience |
|-----------|--------------|
| Under 500 lines | Full file readable, total comprehension. Ideal. |
| 500-1500 lines | Key sections + grep. Good. |
| 1500-3000 lines | Multiple reads, grep-dependent. Workable. |
| 3000+ lines | Fragment-based, miss interactions between distant methods. |
| 10K+ lines | Cannot hold file structure in working context. Error-prone. |

These numbers are for ~200K token context windows (Claude Opus). Smaller context windows shift everything down.

### 2. Constants co-locate with the code that uses them

When AI greps for a handler method, the data it needs should be in the same file — not in a separate constants file that requires another read. Co-location > organization.

**Anti-pattern**: `constants.py` with 50 constants used across 10 files.
**Pattern**: Each module contains the constants its methods reference.

### 3. Unique, grep-friendly names

A method called `_handle_share_validation` is unambiguous in grep results across 30 files. A method called `_validate` requires reading context to disambiguate.

### 4. Change locality > abstraction purity

When modifying feature X, all of X's code should ideally be in one file. Three similar lines of code in the same file is better than a shared helper in a different file that requires reading two files to understand the change.

This directly contradicts DRY orthodoxy but is empirically right for AI editing. DRY optimizes for human maintenance of code that changes slowly. AI editing changes code fast and needs to see everything relevant in one read.

### 5. Comments explain WHY, not WHAT

AI reads code directly. Comments explaining obvious code are noise that consumes context window. Comments should explain design decisions, non-obvious constraints, and things that aren't visible in the code.

### 6. Human readability is a non-goal

Except where it overlaps with AI navigability (clear naming, consistent patterns, small files). Formatting, conventional file organization, elaborate docstrings — these are for humans who will never edit this code.

## What Adit Should Do

### Tier 1: Score (read-only analysis)

`adit score .` — per-file AI-navigability score based on:

- **File size** — graduated penalty above sweet spot thresholds
- **Method uniqueness** — how many methods share substrings in their names (grep ambiguity)
- **Cross-file dependency count** — how many other files must be read to understand changes to this file
- **Constant co-location ratio** — what fraction of referenced constants are in the same file vs imported
- **Call chain depth across files** — how many files deep is the deepest call chain from this file

Output: per-file score (A-F or numeric), aggregate repo score, worst offenders list.

### Tier 2: Audit (actionable recommendations)

`adit audit engine.py` — specific recommendations:

- "Split this file: methods X, Y, Z form a natural group (shared constants, mutual calls, no cross-group calls)"
- "Co-locate _TRUST_HINTS with _handle_share — it's the only consumer"
- "Rename _validate to _validate_share — 4 other files have a _validate method"
- "This method has 15 mutation paths — consider extracting pure computation"

### Tier 3: Enforce (pre-commit checks)

`adit enforce` — configurable constraints:

- `max_file_lines: 3000`
- `max_method_cc: 50` (cyclomatic complexity)
- `min_colocation_ratio: 0.8` (80% of referenced constants in same file)
- `max_cross_file_deps: 5`

Runs as pre-commit hook. Informational by default (warns), blocking if configured.

### Tier 4: Mutation audit (state analysis)

`adit mutations engine.py --summary` — AST-based reader/writer classification:

- Direct writes: `self.x = y`
- Chained writes: `self.world.x = y` (indirect write through self.world)
- Known-mutating collection calls: `self.ctx.output.append(...)`
- Classification: pure, read-only, write-only, read-write
- Per-attribute mutation frequency

This exists as a working prototype in The Stope (`mutation_audit.py`, ~530 lines).

## Existing Assets (in The Stope)

These can be extracted and generalized:

### mutation_audit.py (~530 lines, working)

AST walker that classifies methods as readers vs writers of game state. Detects 3 mutation patterns. Has `--summary`, `--writers-only`, `--attr <name>` modes. Known limitation: method calls on state objects where the method name isn't in `_KNOWN_MUTATING_METHODS` are not flagged (too noisy — getters outnumber mutators).

**Location**: `/home/justin/Documents/lamina/poc/dense/stope/mutation_audit.py`

### complexity-baseline.md (reference document)

Combined radon CC/MI report with manual E/A/C classification (Essential complexity, Accidental complexity, Coupling risk). Shows how to layer complexity metrics with mutation analysis for multiplayer readiness assessment.

**Location**: `/home/justin/Documents/lamina/poc/dense/stope/reference/complexity-baseline.md`

### CLAUDE.md AI-First Codebase section

The principles document. Production-tested across 22 sessions. This is the seed for adit's documentation and opinionated defaults.

**Location**: `/home/justin/Documents/lamina/poc/dense/stope/CLAUDE.md` (top section)

### The engine.py split (case study)

12,232-line monolith split into 4 files via Python mixins. All tests passed. Demonstrates:
- AST-based automated extraction (script removed methods by name, rebuilt file)
- Constant co-location (each mixin carries its own data)
- mypy suppression for mixin classes (`# mypy: disable-error-code="attr-defined,has-type"`)
- Re-export pattern for backwards compatibility (`from engine_social import X` in engine.py)

## Landscape Research (June 2026)

### What exists

| Tool/Paper | What it does | Gap vs Adit |
|-----------|-------------|-------------|
| **Repomix** (22K stars) | Packs repos into XML for AI consumption | Navigation, not restructuring |
| **Aider repo map** | Tree-sitter symbol extraction + graph ranking | Finds code, doesn't reorganize it |
| **AGENTS.md** (Linux Foundation) | Cross-tool instruction standard | Advisory text, no enforcement |
| **CodeScene CodeHealth** (FORGE/ICSE 2026) | Code health predicts AI refactoring success | Commercial, not AI-specific, single-file only |
| **Nx monorepo** | Exposes project graph to agents | Navigation, not structural recommendations |
| **Radon/Wily** | Cyclomatic complexity + maintainability index | Human-oriented metrics, no AI-specific dimension |

### Key academic finding

"Code for Machines, Not Just Humans" (FORGE 2026, arXiv:2601.02200): CodeHealth is the strongest predictor of AI refactoring success. AI defect risk 30%+ higher in unhealthy code. CodeHealth was 3-10x more discriminative than other metrics. **Their conclusion: human-friendly code is AI-friendly code.** But they only tested refactoring, not navigation or multi-file editing.

Our production experience partially contradicts this: a 12K-line file with perfect CodeHealth is WORSE for AI editing than a messier 2K file because the AI can't hold the structure in context. The CodeScene finding is true for single-file refactoring quality but misses the dominant cost in real AI-assisted development: finding and navigating to the right code across files.

### Unfilled niches (what Adit would be first to do)

- File size budgets tied to context window constraints
- Co-location scoring (are constants near their consumers?)
- Structural constraint enforcement (pre-commit, not advisory)
- AI-navigability scoring (distinct from human readability)
- Mutation provenance (reader/writer classification for concurrency)

## Design Constraints

- **Zero runtime dependencies**. `pip install adit` should not pull in anything.
  AST analysis uses stdlib `ast`. No click, no rich, no pydantic. ANSI codes
  for color output, plain text otherwise.
- **Single-file core**. The scoring/audit logic should fit in one file that can
  be vendored if someone doesn't want the dependency. Think `black` before it
  grew — one file you can copy.
- **Callable from other projects**. Stope (the parent project) should be able to
  `pip install adit` and run `adit score .` from its pre-commit hook or Makefile.
  Also importable: `from adit import score_file, analyze_mutations`.
- **No project-specific knowledge**. The tool knows about Python structure
  (AST, files, imports). It does NOT know about game engines, web frameworks,
  or any domain. Configuration handles project-specific thresholds.

## Technical Decisions (not yet made)

- **Language**: Python (matches the ecosystem, can analyze Python ASTs natively). Rust for performance later if needed.
- **AST parsing**: `ast` module for Python. Tree-sitter for multi-language support later.
- **Configuration**: pyproject.toml `[tool.adit]` section, or standalone `.adit.toml`.
- **Output format**: Human-readable terminal output + JSON for CI integration.
- **Scope**: Python-first, then TypeScript/JavaScript (the other major AI-edited language).
- **Distribution**: PyPI package. Single entry point: `adit` CLI. Also importable as library.

## What a v0.1 Looks Like

1. `pip install adit`
2. `adit score .` — prints per-file scores with color-coded grades
3. `adit score --json .` — machine-readable output
4. `adit mutations engine.py --summary` — mutation audit (ported from stope)
5. Configuration in `pyproject.toml`:
   ```toml
   [tool.adit]
   max_file_lines = 3000
   max_method_cc = 50
   ```

The blog post / HN launch should accompany v0.1 with the principles document. The principles are the hook; the tool is the proof.

## Contact / Context

- **Author**: Justin (human) + Claude (AI, sole code author)
- **Parent project**: The Stope (`/home/justin/Documents/lamina/poc/dense/stope/`)
- **This file written**: 2026-03-21, session 22
