# Influences & Citations

All dependencies and inspirations are MIT or BSD-2 licensed.
adit-code is MIT licensed. It does not reimplement any proprietary metric.

## Academic

- Markus Borg et al., ["Code for Machines, Not Just Humans,"](https://arxiv.org/abs/2601.02200)
  FORGE/ICSE 2026. Found CodeHealth predicts AI refactoring success (30%+
  higher defect risk in unhealthy code). Adit builds on this finding by
  measuring structural properties they didn't test: cross-file navigation
  cost, co-location, grep ambiguity. Adit does NOT reimplement CodeHealth
  (which is proprietary to [CodeScene](https://codescene.com/)).

- Wong et al., ["Static and dynamic distance metrics for feature-based code
  analysis,"](https://doi.org/10.1016/j.jss.2004.02.029) Journal of Systems
  and Software, 2005. Feature dispersion metrics (distance, disparity,
  concentration, dedication). Closest academic precedent for co-location
  scoring, but operates at feature level, not definition-usage level.

- Wong, Abe, De Benedictis, Halim, Peruma, ["Identifier Name Similarities:
  An Exploratory Study,"](https://arxiv.org/abs/2507.18081) ESEM 2025.
  7-category taxonomy of similar identifier names across Java projects.
  Closest academic precedent for grep noise scoring, but classification
  study, not tool.

- Szalay et al., ["Measuring Mangled Name Ambiguity in Large C/C++ Projects,"](https://ceur-ws.org/Vol-1938/)
  CEUR-WS Vol-1938. Measured symbol name ambiguity at linker level. Adjacent
  concept applied to compiled symbols rather than source identifiers.

- Thomas McCabe, "A Complexity Measure," IEEE TSE 1976. Cyclomatic
  complexity — foundational metric referenced in file size budget context.

- Maurice Halstead, *Elements of Software Science*, 1977. Halstead's
  vocabulary metric (η = distinct operators + operands) is the closest
  precedent for adit's node diversity metric. Adit counts distinct AST
  node types rather than lexical tokens, but the intuition is the same:
  varied vocabulary = structural complexity independent of size.

- ["Structural Code Understanding with AST Entropy,"](https://arxiv.org/abs/2508.14288)
  2025. Computes entropy over depth-bounded AST subtrees. Related to
  node diversity but uses subtree frequencies rather than node type counts.

- Butler et al., ["Exploring the Influence of Identifier Names on Code
  Quality,"](https://ieeexplore.ieee.org/abstract/document/5714430) IEEE
  CSMR 2010. Statistically significant associations between flawed identifier
  names and code defects. Foundational work on naming quality, though focused
  on individual name quality, not cross-codebase uniqueness.

- ["Rethinking Code Complexity Through the Lens of Large Language Models,"](https://arxiv.org/abs/2602.07882)
  Feb 2026. Proposes LM-CC, a complexity metric based on LLM entropy.
  Found that after controlling for code length, classical complexity metrics
  show no consistent correlation with LLM performance. Supports file size
  as the primary driver over complexity.

- ["Tokenomics: Quantifying Where Tokens Are Used in Agentic Software
  Engineering,"](https://arxiv.org/abs/2601.14470) Jan 2026. First empirical
  analysis of token distribution in AI coding agents. Code review consumes
  59.4% of tokens; input tokens are 53.9% of total.

- ["LocAgent: Graph-Guided LLM Agents for Code Localization,"](https://arxiv.org/abs/2503.09089)
  ACL 2025. Dependency graph-based localization achieved 92.7% file-level
  accuracy and reduced costs ~86%. Validates that code structure affects
  navigation cost.

- ["LoCoBench-Agent,"](https://arxiv.org/abs/2511.13998) Salesforce, Nov
  2025. Found negative correlation between thorough exploration and
  efficiency. Agents that traverse more files spend more tokens.

- Bishara & Hittner, ["Testing the Significance of a Correlation with
  Nonnormal Data,"](https://pubmed.ncbi.nlm.nih.gov/22563845/) 2012.
  Methodological reference for using Spearman rank correlation alongside
  Pearson on non-normal data (tool call counts are right-skewed count data).

## Principles

- Kent C. Dodds, ["Colocation"](https://kentcdodds.com/blog/colocation).
  Articulated co-location as a design principle for React. Adit quantifies
  it as a computable metric applicable to any language.

- Carson Gross, ["Locality of Behaviour"](https://htmx.org/essays/locality-of-behaviour/).
  Formalized LoB: "The behaviour of a unit of code should be as obvious as
  possible by looking only at that unit of code." Adit measures compliance.

- Kent Beck, *Tidy First?* (O'Reilly, 2023). Cohesion economics framework —
  keeping elements that change together close together, analyzed through
  discounted cash flows and optionality.

- Robert C. Martin, *Clean Code* Ch. 2, "Use Searchable Names." The
  qualitative advice that adit's grep noise metric quantifies.

- Jamie Wong, ["The Grep Test"](https://jamie-wong.com/2013/07/12/grep-test/)
  (2013). Described the principle that identifiers should be greppable.

## Tools & People

All MIT or BSD-2 licensed. No code copied.

- [tree-sitter](https://tree-sitter.github.io/tree-sitter/) (MIT) — parsing infrastructure
- [tree-sitter-python](https://github.com/tree-sitter/tree-sitter-python) (MIT) — Python grammar
- [tree-sitter-typescript](https://github.com/tree-sitter/tree-sitter-typescript) (MIT) — TypeScript grammar
- [tree-sitter-go](https://github.com/tree-sitter/tree-sitter-go) (MIT) — Go grammar
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) (MIT) — official MCP Go SDK
- [BurntSushi/toml](https://github.com/BurntSushi/toml) (MIT) — TOML config parsing
- [urfave/cli](https://github.com/urfave/cli) (MIT) — CLI framework
- [Manuel Odendahl / go-go-golems](https://github.com/go-go-golems) (MIT) —
  glazed (structured CLI output), go-go-mcp (Go MCP implementation).
  Architectural influence: composable structured data output, MCP-first design.

### Complementary tools (not replaced)

- [ruff](https://github.com/astral-sh/ruff) (MIT) — Python linter. Orthogonal: style vs structure.
- [import-linter](https://github.com/seddonym/import-linter) + [grimp](https://github.com/seddonym/grimp) (BSD-2) — architectural boundary enforcement. Complementary: adit finds what to move, import-linter enforces where.
- [ast-grep](https://github.com/ast-grep/ast-grep) (MIT) — structural search/lint/rewrite. Independent.
- [Repomix](https://github.com/yamadashy/repomix) (MIT) — context packing. Sequential: restructure with adit, then pack.
- [pre-commit](https://github.com/pre-commit/pre-commit) (MIT) — hook framework. Integration target for adit enforce.
- [radon](https://github.com/rubik/radon) (MIT) — cyclomatic complexity and maintainability index for Python.
- [CodeScene CodeHealth](https://codescene.com/) (proprietary) — cited as prior art only. Adit measures different properties and does not reimplement CodeHealth's scoring methodology.

## Prior Art

Searched 16+ tree-sitter-based tools including
[ast-grep](https://github.com/ast-grep/ast-grep),
[Semgrep](https://github.com/semgrep/semgrep),
[Probe](https://github.com/probelabs/probe),
[difftastic](https://github.com/Wilfred/difftastic),
[Aider RepoMap](https://aider.chat/docs/repomap.html),
[Sourcegraph SCIP](https://github.com/sourcegraph/scip),
and [rust-code-analysis](https://github.com/nicuveo/rust-code-analysis).
None measure co-location, grep noise, or AI-context-aware file size budgets.

## Validation

Metrics validated against SWE-bench agent trajectories (1,840 files across 49
repos, [nebius/SWE-rebench-openhands-trajectories](https://huggingface.co/datasets/nebius/SWE-rebench-openhands-trajectories)).
File size is the dominant predictor; structural metrics provide diagnostic
value. See [README.md](README.md#validated-against-real-agent-behavior) for
correlation data.
