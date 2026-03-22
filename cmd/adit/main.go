package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	mcpserver "github.com/justinstimatze/adit-code/internal/mcp"
	"github.com/justinstimatze/adit-code/internal/output"
	"github.com/justinstimatze/adit-code/internal/score"
	aditversion "github.com/justinstimatze/adit-code/internal/version"
	"github.com/urfave/cli/v2"
)

// version is imported from internal/version
var version = aditversion.Version

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

func buildPipeline(c *cli.Context) (*score.Pipeline, config.Config, error) {
	paths := getPaths(c)
	cfg, err := loadConfigForPaths(c, paths)
	if err != nil {
		return nil, cfg, err
	}
	// Append CLI --exclude patterns to config
	if extras := c.StringSlice("exclude"); len(extras) > 0 {
		cfg.Scan.Exclude = append(cfg.Scan.Exclude, extras...)
	}
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
		lang.NewGoFrontend(),
	}
	return score.NewPipeline(frontends, cfg), cfg, nil
}

func getPaths(c *cli.Context) []string {
	paths := c.Args().Slice()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return paths
}

func loadConfigForPaths(c *cli.Context, paths []string) (config.Config, error) {
	if path := c.String("config"); path != "" {
		cfg, err := config.LoadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("config error: %w", err)
		}
		return cfg, nil
	}
	// Search from the first target path, not CWD
	searchDir := "."
	if len(paths) > 0 {
		searchDir = paths[0]
	}
	cfg, _ := config.Load(searchDir)
	return cfg, nil
}

func scoreCmd() *cli.Command {
	return &cli.Command{
		Name:      "score",
		Usage:     "Analyze code structure and report metrics",
		ArgsUsage: "[PATHS...]",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "exclude",
				Usage: "glob pattern to exclude (repeatable)",
			},
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
			&cli.BoolFlag{
				Name:  "sarif",
				Usage: "SARIF v2.1.0 output for CI integration",
			},
		},
		Action: runScore,
	}
}

func runScore(c *cli.Context) error {
	pipeline, cfg, err := buildPipeline(c)
	if err != nil {
		return err
	}
	paths := getPaths(c)

	if ref := c.String("diff"); ref != "" {
		return runScoreDiff(c, pipeline, paths, ref)
	}

	result, err := pipeline.ScoreRepo(paths)
	if err != nil {
		return fmt.Errorf("score failed: %w", err)
	}

	if minGrade := c.String("min-grade"); minGrade != "" {
		result.Files = output.FilterByMinGrade(result.Files, minGrade)
	}
	output.SortFiles(result.Files, c.String("sort"))

	if c.Bool("silent") {
		if output.HasIssues(result) {
			return cli.Exit("", 1)
		}
		return nil
	}

	if c.Bool("sarif") {
		return output.WriteSARIF(os.Stdout, result, cfg, version)
	}

	if c.Bool("pretty") {
		color := output.UseColor(c.String("color"))
		output.PrintPrettyColor(result, version, c.Bool("quiet"), color, cfg)
	} else {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	return nil
}

func runScoreDiff(c *cli.Context, pipeline *score.Pipeline, paths []string, ref string) error {
	result, err := pipeline.ScoreRepoDiff(paths, ref)
	if err != nil {
		return fmt.Errorf("diff failed: %w", err)
	}

	if c.Bool("silent") {
		if result.Regressions > 0 {
			return cli.Exit("", 1)
		}
		return nil
	}

	if c.Bool("pretty") {
		output.PrintDiffPretty(result, version)
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
			&cli.StringSliceFlag{
				Name:  "exclude",
				Usage: "glob pattern to exclude (repeatable)",
			},
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
			&cli.BoolFlag{
				Name:  "sarif",
				Usage: "SARIF v2.1.0 output for CI integration",
			},
		},
		Action: func(c *cli.Context) error {
			pipeline, cfg, err := buildPipeline(c)
			if err != nil {
				return err
			}
			paths := getPaths(c)

			if ref := c.String("diff"); ref != "" {
				return runEnforceDiff(c, pipeline, cfg, paths, ref)
			}
			return runEnforce(c, pipeline, cfg, paths)
		},
	}
}

func runEnforceDiff(c *cli.Context, pipeline *score.Pipeline, cfg config.Config, paths []string, ref string) error {
	diffResult, err := pipeline.ScoreRepoDiff(paths, ref)
	if err != nil {
		return fmt.Errorf("enforce diff failed: %w", err)
	}

	var violations []string
	for _, fd := range diffResult.Files {
		for _, r := range fd.Regressions {
			violations = append(violations, fmt.Sprintf("%s: %s %d→%d (+%d)", fd.Path, r.Metric, r.Before, r.After, r.Delta))
		}
		if fd.After != nil {
			violations = append(violations, output.CheckFileThresholds(fd.Path, fd.After, cfg)...)
		}
	}

	if c.Bool("sarif") {
		if err := output.WriteDiffSARIF(os.Stdout, diffResult, cfg, version); err != nil {
			return err
		}
		if len(violations) > 0 {
			return cli.Exit("", 1)
		}
		return nil
	}

	return reportViolations(c, violations)
}

func runEnforce(c *cli.Context, pipeline *score.Pipeline, cfg config.Config, paths []string) error {
	result, err := pipeline.ScoreRepo(paths)
	if err != nil {
		return fmt.Errorf("enforce failed: %w", err)
	}

	violations := output.CheckThresholds(result, cfg)

	if c.Bool("sarif") {
		if err := output.WriteSARIF(os.Stdout, result, cfg, version); err != nil {
			return err
		}
		if len(violations) > 0 {
			return cli.Exit("", 1)
		}
		return nil
	}

	return reportViolations(c, violations)
}

func reportViolations(c *cli.Context, violations []string) error {
	if len(violations) == 0 {
		return nil
	}
	if !c.Bool("silent") {
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, v)
		}
	}
	return cli.Exit("", 1)
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
