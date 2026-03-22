package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/justindotpub/adit-code/internal/config"
	"github.com/justindotpub/adit-code/internal/lang"
	mcpserver "github.com/justindotpub/adit-code/internal/mcp"
	"github.com/justindotpub/adit-code/internal/score"
	"github.com/urfave/cli/v2"
)

const version = "0.1.0"

func main() {
	app := &cli.App{
		Name:    "adit",
		Usage:   "AI-navigability code structure analysis",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "path to config file (adit.toml)",
			},
			&cli.StringFlag{
				Name:  "color",
				Usage: "color output: auto|always|never",
				Value: "auto",
			},
			&cli.StringSliceFlag{
				Name:  "exclude",
				Usage: "glob pattern to exclude (repeatable)",
			},
		},
		Commands: []*cli.Command{
			scoreCmd(),
			enforceCmd(),
			mcpCmd(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "adit: %v\n", err)
		os.Exit(2)
	}
}

func loadConfig(c *cli.Context) config.Config {
	if path := c.String("config"); path != "" {
		cfg, err := config.LoadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "adit: config error: %v\n", err)
			os.Exit(2)
		}
		return cfg
	}
	cfg, _ := config.Load(".")
	return cfg
}

func buildPipeline(c *cli.Context) (*score.Pipeline, config.Config) {
	cfg := loadConfig(c)
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
	}
	return score.NewPipeline(frontends, cfg), cfg
}

func getPaths(c *cli.Context) []string {
	paths := c.Args().Slice()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return paths
}

func scoreCmd() *cli.Command {
	return &cli.Command{
		Name:      "score",
		Usage:     "Analyze code structure and report metrics",
		ArgsUsage: "[PATHS...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "pretty",
				Usage: "human-readable table output (default is JSON)",
			},
			&cli.StringFlag{
				Name:  "diff",
				Usage: "compare against git ref (e.g. HEAD~1)",
			},
			&cli.StringFlag{
				Name:  "min-grade",
				Usage: "only show files at this size grade or worse (A-F)",
			},
			&cli.StringFlag{
				Name:  "sort",
				Usage: "sort by: blast|reads|ambig|size|name",
				Value: "name",
			},
			&cli.BoolFlag{
				Name:  "quiet",
				Usage: "metrics only, no recommendations",
			},
			&cli.BoolFlag{
				Name:  "silent",
				Usage: "no output, exit code only",
			},
		},
		Action: runScore,
	}
}

func runScore(c *cli.Context) error {
	pipeline, _ := buildPipeline(c)
	paths := getPaths(c)

	result, err := pipeline.ScoreRepo(paths)
	if err != nil {
		return fmt.Errorf("score failed: %w", err)
	}

	// Apply min-grade filter
	if minGrade := c.String("min-grade"); minGrade != "" {
		result.Files = filterByMinGrade(result.Files, minGrade)
	}

	// Apply sort
	sortFiles(result.Files, c.String("sort"))

	if c.Bool("silent") {
		if hasIssues(result) {
			os.Exit(1)
		}
		return nil
	}

	if c.Bool("pretty") {
		printPretty(result, c.Bool("quiet"))
	} else {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	return nil
}

func enforceCmd() *cli.Command {
	return &cli.Command{
		Name:      "enforce",
		Usage:     "Check thresholds and exit non-zero on violations",
		ArgsUsage: "[PATHS...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "diff",
				Usage: "only enforce on files changed since git ref",
			},
			&cli.BoolFlag{
				Name:  "quiet",
				Usage: "compact output",
			},
			&cli.BoolFlag{
				Name:  "silent",
				Usage: "no output, exit code only",
			},
		},
		Action: func(c *cli.Context) error {
			pipeline, cfg := buildPipeline(c)
			paths := getPaths(c)

			result, err := pipeline.ScoreRepo(paths)
			if err != nil {
				return fmt.Errorf("enforce failed: %w", err)
			}

			violations := checkThresholds(result, cfg)
			if len(violations) == 0 {
				return nil
			}

			if !c.Bool("silent") {
				for _, v := range violations {
					fmt.Fprintln(os.Stderr, v)
				}
			}
			os.Exit(1)
			return nil
		},
	}
}

func mcpCmd() *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "Start MCP server (stdio) for AI tool integration",
		Action: func(c *cli.Context) error {
			return mcpserver.Run(c.Context)
		},
	}
}

// --- Output helpers ---

func filterByMinGrade(files []score.FileScore, minGrade string) []score.FileScore {
	gradeRank := map[string]int{"A": 1, "B": 2, "C": 3, "D": 4, "F": 5}
	minRank, ok := gradeRank[strings.ToUpper(minGrade)]
	if !ok {
		return files
	}
	var filtered []score.FileScore
	for _, f := range files {
		rank := gradeRank[f.SizeGrade]
		if rank >= minRank {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func sortFiles(files []score.FileScore, sortBy string) {
	switch sortBy {
	case "blast":
		sort.Slice(files, func(i, j int) bool {
			return files[i].BlastRadius.ImportedByCount > files[j].BlastRadius.ImportedByCount
		})
	case "reads":
		sort.Slice(files, func(i, j int) bool {
			return files[i].ContextReads.Total > files[j].ContextReads.Total
		})
	case "ambig":
		sort.Slice(files, func(i, j int) bool {
			ai := files[i].Ambiguity.TotalNames - files[i].Ambiguity.UniqueNames
			aj := files[j].Ambiguity.TotalNames - files[j].Ambiguity.UniqueNames
			return ai > aj
		})
	case "size":
		sort.Slice(files, func(i, j int) bool {
			return files[i].Lines > files[j].Lines
		})
	default: // "name"
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
	}
}

func hasIssues(result *score.RepoScore) bool {
	return len(result.Summary.Relocatable) > 0 ||
		len(result.Summary.AmbiguousNames) > 0 ||
		len(result.Summary.Cycles) > 0
}

func checkThresholds(result *score.RepoScore, cfg config.Config) []string {
	var violations []string
	t := cfg.Thresholds

	for _, f := range result.Files {
		if t.MaxFileLines > 0 && f.Lines > t.MaxFileLines {
			violations = append(violations, fmt.Sprintf("%s: %d lines (max %d)", f.Path, f.Lines, t.MaxFileLines))
		}
		if t.MaxContextReads > 0 && f.ContextReads.Total > t.MaxContextReads {
			violations = append(violations, fmt.Sprintf("%s: %d context reads (max %d)", f.Path, f.ContextReads.Total, t.MaxContextReads))
		}
		if t.MaxUnnecessaryReads > 0 && f.ContextReads.Unnecessary > t.MaxUnnecessaryReads {
			violations = append(violations, fmt.Sprintf("%s: %d unnecessary reads (max %d)", f.Path, f.ContextReads.Unnecessary, t.MaxUnnecessaryReads))
		}
		if t.MaxBlastRadius > 0 && f.BlastRadius.ImportedByCount > t.MaxBlastRadius {
			violations = append(violations, fmt.Sprintf("%s: blast radius %d (max %d)", f.Path, f.BlastRadius.ImportedByCount, t.MaxBlastRadius))
		}
	}

	if t.MaxCycleLength == 0 && len(result.Summary.Cycles) > 0 {
		for _, cy := range result.Summary.Cycles {
			violations = append(violations, fmt.Sprintf("import cycle: %s", strings.Join(cy.Files, " → ")))
		}
	}

	return violations
}

func printPretty(result *score.RepoScore, quiet bool) {
	fmt.Printf("\nadit v%s\n\n", version)

	// Table header
	fmt.Printf("  %-30s %5s  %4s  %5s  %8s  %8s  %5s\n",
		"File", "Lines", "Size", "Reads", "Unneeded", "Uniq", "Blast")
	fmt.Printf("  %s\n", strings.Repeat("─", 78))

	for _, f := range result.Files {
		uniq := fmt.Sprintf("%d/%d", f.Ambiguity.UniqueNames, f.Ambiguity.TotalNames)
		fmt.Printf("  %-30s %5d    %s   %5d  %8d  %8s  %5d\n",
			f.Path, f.Lines, f.SizeGrade, f.ContextReads.Total,
			f.ContextReads.Unnecessary, uniq, f.BlastRadius.ImportedByCount)
	}

	if quiet {
		fmt.Printf("\n  %d files scanned\n\n", result.FilesScanned)
		return
	}

	// Relocatable imports
	if len(result.Summary.Relocatable) > 0 {
		fmt.Printf("\n  Co-locate (single-consumer imports):\n")
		for _, r := range result.Summary.Relocatable {
			fmt.Printf("    %-20s (%s)  %s:%d → %s\n", r.Name, r.Kind, r.From, r.FromLine, r.To)
		}
	}

	// Ambiguous names
	if len(result.Summary.AmbiguousNames) > 0 {
		fmt.Printf("\n  Ambiguous names:\n")
		for _, a := range result.Summary.AmbiguousNames {
			var locs []string
			for _, s := range a.Sites {
				locs = append(locs, fmt.Sprintf("%s:%d", s.File, s.Line))
			}
			fmt.Printf("    %-20s %d defs: %s\n", a.Name, a.Count, strings.Join(locs, " "))
		}
	}

	// High blast radius
	if len(result.Summary.HighBlast) > 0 {
		fmt.Printf("\n  High blast radius:\n")
		for _, f := range result.Summary.HighBlast {
			fmt.Printf("    %s imported by %d files\n", f.Path, f.BlastRadius.ImportedByCount)
		}
	}

	// Import cycles
	if len(result.Summary.Cycles) > 0 {
		fmt.Printf("\n  Import cycles:\n")
		for _, cy := range result.Summary.Cycles {
			fmt.Printf("    %s\n", strings.Join(cy.Files, " → "))
		}
	} else if !quiet {
		fmt.Printf("\n  Import cycles: none\n")
	}

	fmt.Printf("\n  %d files scanned\n\n", result.FilesScanned)
}
