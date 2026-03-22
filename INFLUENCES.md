# Influences & Citations

All dependencies and inspirations are MIT or BSD-2 licensed.
adit-code is MIT licensed. It does not reimplement any proprietary metric.

## Academic

- Markus Borg et al., "Code for Machines, Not Just Humans," FORGE/ICSE 2026
  (arXiv:2601.02200). Found CodeHealth predicts AI refactoring success (30%+
  higher defect risk in unhealthy code). Adit builds on this finding by
  measuring structural properties they didn't test: cross-file navigation
  cost, co-location, grep ambiguity. Adit does NOT reimplement CodeHealth
  (which is proprietary to CodeScene).

- Wong et al., "Static and dynamic distance metrics for feature-based code
  analysis," Journal of Systems and Software, 2004. Feature dispersion
  metrics (distance, disparity, concentration, dedication). Closest academic
  precedent for co-location scoring, but operates at feature level, not
  definition-usage level.

- Wong, Abe, De Benedictis, Halim, Peruma, "Identifier Name Similarities:
  An Exploratory Study," ESEM 2025 (arXiv:2507.18081). 7-category taxonomy
  of similar identifier names across Java projects. Closest academic
  precedent for grep ambiguity scoring, but classification study, not tool.

- Szalay et al., "Measuring Mangled Name Ambiguity in Large C/C++ Projects,"
  CEUR-WS Vol-1938. Measured symbol name ambiguity at linker level. Adjacent
  concept applied to compiled symbols rather than source identifiers.

- Thomas McCabe, "A Complexity Measure," IEEE TSE 1976. Cyclomatic
  complexity — foundational metric referenced in file size budget context.

- Butler et al., "Exploring the Influence of Identifier Names on Code
  Quality," IEEE CSMR 2010. Statistically significant associations between
  flawed identifier names and code defects. Foundational work on naming
  quality, though focused on individual name quality, not cross-codebase
  uniqueness.

## Principles

- Kent C. Dodds, "Colocation" (kentcdodds.com/blog/colocation). Articulated
  co-location as a design principle for React. Adit quantifies it as a
  computable metric applicable to any language.

- Carson Gross, "Locality of Behaviour" (htmx.org/essays/locality-of-behaviour).
  Formalized LoB: "The behaviour of a unit of code should be as obvious as
  possible by looking only at that unit of code." Adit measures compliance.

- Kent Beck, "Tidy First?" (O'Reilly, 2023). Cohesion economics framework —
  keeping elements that change together close together, analyzed through
  discounted cash flows and optionality.

- Robert C. Martin, "Clean Code" Ch. 2, "Use Searchable Names." The
  qualitative advice that adit's grep ambiguity metric quantifies.

- Jamie Wong, "The Grep Test" (2013 blog post). Described the principle that
  identifiers should be greppable. No tooling or quantitative metric; adit
  is the first tool to measure this.

## Tools & People (all MIT or BSD-2; no code copied)

- tree-sitter (MIT) — parsing infrastructure
- tree-sitter-python (MIT) — Python grammar
- tree-sitter-typescript (MIT) — TypeScript grammar
- modelcontextprotocol/go-sdk (MIT) — official MCP Go SDK
- BurntSushi/toml (MIT) — TOML config parsing
- urfave/cli (MIT) — CLI framework
- Manuel Odendahl / go-go-golems (MIT) — glazed (structured CLI output),
  go-go-mcp (Go MCP implementation). Architectural influence: composable
  structured data output, MCP-first design.

### Complementary tools (not replaced)

- ruff (MIT) — Python linter. Orthogonal: style vs structure.
- import-linter + grimp (BSD-2) — architectural boundary enforcement.
  Complementary: adit finds what to move, import-linter enforces boundaries.
- ast-grep (MIT) — structural search/lint/rewrite. Independent.
- Repomix (MIT) — context packing. Sequential: restructure with adit, then
  pack with Repomix.
- pre-commit (MIT) — hook framework. Integration target for adit enforce.
- radon (MIT) — cyclomatic complexity and maintainability index for Python.
- CodeScene CodeHealth (proprietary) — cited as prior art only. Adit measures
  different properties (co-location, grep ambiguity, context budget) and does
  not reimplement CodeHealth's scoring methodology.

## Prior Art Search (confirmed novel, 2026-03-21)

Searched 16+ tree-sitter-based tools (ast-grep, Semgrep, sqry, Probe,
tree-sitter-stack-graphs, GitHub Semantic, Sourcegraph SCIP, rust-code-analysis,
tree-sitter-analyzer, CodeGraph, Rhizome, difftastic, diffsitter, Aider RepoMap,
tree-sitter-graph, IBM tree-sitter-codeviews). None measure co-location,
grep ambiguity, or AI-context-aware file size budgets.

## Production Experience

22 sessions of AI-only editing on The Stope (~30K lines Python). Human reviews
diffs and approves commits but never hand-edits files. Principles and thresholds
derived from this production use.
