package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/score"
	"golang.org/x/term"
)

const (
	ansiRed   = "\033[31m"
	ansiReset = "\033[0m"
)

// UseColor determines whether to use ANSI color output.
func UseColor(colorFlag string) bool {
	switch colorFlag {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
}

// PrintPretty prints a human-readable table of repo scores.
func PrintPretty(result *score.RepoScore, version string, quiet bool, cfg ...config.Config) {
	PrintPrettyColor(result, version, quiet, false, cfg...)
}

// PrintPrettyColor prints a human-readable table with optional ANSI color for threshold violations.
func PrintPrettyColor(result *score.RepoScore, version string, quiet, color bool, cfg ...config.Config) {
	fmt.Printf("\nadit v%s\n\n", version)

	fmt.Printf("  %-30s %5s  %4s  %5s  %5s  %8s  %5s  %5s\n",
		"File", "Lines", "Size", "Nest", "Reads", "Unneeded", "Noise", "Blast")
	fmt.Printf("  %s\n", strings.Repeat("─", 84))

	for _, f := range result.Files {
		var t *config.ThresholdConfig
		if len(cfg) > 0 {
			tc := cfg[0].ThresholdsForPath(f.Path)
			t = &tc
		}
		fmt.Printf("  %-30s %5d  %s  %s  %s  %s  %s  %s\n",
			f.Path, f.Lines,
			fmtGrade(f.SizeGrade, f.Lines, t, color),
			fmtCol(f.MaxNestingDepth, 5, threshold(t, "nest"), color),
			fmtCol(f.ContextReads.Total, 5, threshold(t, "reads"), color),
			fmtCol(f.ContextReads.Unnecessary, 8, threshold(t, "unneeded"), color),
			fmtCol(f.Ambiguity.GrepNoise, 5, threshold(t, "noise"), color),
			fmtCol(f.BlastRadius.ImportedByCount, 5, threshold(t, "blast"), color))
	}

	if quiet {
		fmt.Printf("\n  %d files scanned\n\n", result.FilesScanned)
		return
	}

	if len(result.Summary.Relocatable) > 0 {
		fmt.Printf("\n  Co-locate (single-consumer imports):\n")
		for _, r := range result.Summary.Relocatable {
			fmt.Printf("    %-20s (%s)  %s:%d → %s\n", r.Name, r.Kind, r.From, r.FromLine, r.To)
		}
	}

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

	if len(result.Summary.HighBlast) > 0 {
		fmt.Printf("\n  High blast radius:\n")
		for _, f := range result.Summary.HighBlast {
			fmt.Printf("    %s imported by %d files\n", f.Path, f.BlastRadius.ImportedByCount)
		}
	}

	// Flag large files with no comments
	var uncommented []score.FileScore
	for _, f := range result.Files {
		if f.Lines >= 200 && f.Comments.CommentLines == 0 {
			uncommented = append(uncommented, f)
		}
	}
	if len(uncommented) > 0 {
		fmt.Printf("\n  Uncommented (large files, 0 comments — AI agents benefit from comments):\n")
		for _, f := range uncommented {
			fmt.Printf("    %s (%d lines)\n", f.Path, f.Lines)
		}
	}

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

// PrintDiffPretty prints a human-readable diff report.
func PrintDiffPretty(result *score.DiffResult, version string) {
	fmt.Printf("\nadit v%s — diff vs %s\n\n", version, result.Ref)

	fmt.Printf("  %-25s %12s %10s %8s %8s %5s\n",
		"File", "Lines", "Reads", "Noise", "Blast", "Size")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))

	for _, fd := range result.Files {
		if fd.Status == "added" {
			a := fd.After
			fmt.Printf("  %-25s %12s %10s %8s %8s %5s\n",
				fd.Path,
				fmt.Sprintf("(+%d)", a.Lines),
				fmt.Sprintf("—→%d", a.ContextReads.Total),
				fmt.Sprintf("—→%d", a.Ambiguity.GrepNoise),
				fmt.Sprintf("—→%d", a.BlastRadius.ImportedByCount),
				a.SizeGrade)
			continue
		}
		if fd.Status == "deleted" {
			fmt.Printf("  %-25s %12s\n", fd.Path, "(deleted)")
			continue
		}

		b := fd.Before
		a := fd.After
		if b == nil || a == nil {
			continue
		}

		fmt.Printf("  %-25s %s %s %s %s %5s\n",
			fd.Path,
			diffCol(b.Lines, a.Lines, 12),
			diffCol(b.ContextReads.Total, a.ContextReads.Total, 10),
			diffCol(b.Ambiguity.GrepNoise, a.Ambiguity.GrepNoise, 8),
			diffCol(b.BlastRadius.ImportedByCount, a.BlastRadius.ImportedByCount, 8),
			a.SizeGrade)
	}

	if result.Regressions > 0 {
		fmt.Printf("\n  Regressions:\n")
		for _, fd := range result.Files {
			for _, r := range fd.Regressions {
				fmt.Printf("    %s: %s %d→%d (+%d)\n", fd.Path, r.Metric, r.Before, r.After, r.Delta)
			}
		}
	}

	fmt.Printf("\n  %d files changed, %d regressions\n\n", result.FilesChanged, result.Regressions)
}

func diffCol(before, after, width int) string {
	if before == after {
		s := fmt.Sprintf("%d", after)
		return fmt.Sprintf("%*s", width, s)
	}
	arrow := "▲"
	if after < before {
		arrow = "▼"
	}
	s := fmt.Sprintf("%d→%d %s", before, after, arrow)
	return fmt.Sprintf("%*s", width, s)
}

func warn(s string, color bool) string {
	if color {
		return ansiRed + s + ansiReset
	}
	return s
}

// threshold returns the max value for a metric from config, or 0 if no config.
func threshold(t *config.ThresholdConfig, metric string) int {
	if t == nil {
		return 0
	}
	switch metric {
	case "noise":
		return t.MaxAmbiguityDefs
	case "blast":
		return t.MaxBlastRadius
	case "reads":
		return t.MaxContextReads
	case "unneeded":
		return t.MaxUnnecessaryReads
	case "nest":
		return t.MaxNestingDepth
	default:
		return 0
	}
}

// fmtCol formats a numeric column, adding ! suffix if it exceeds the threshold.
func fmtCol(val, width, max int, color bool) string {
	if max > 0 && val > max {
		return warn(fmt.Sprintf("%*d!", width-1, val), color)
	}
	return fmt.Sprintf("%*d", width, val)
}

// fmtGrade formats the size grade column, warning if lines exceed threshold.
func fmtGrade(grade string, lines int, t *config.ThresholdConfig, color bool) string {
	s := fmt.Sprintf("  %s", grade)
	if t != nil && t.MaxFileLines > 0 && lines > t.MaxFileLines {
		return warn(s, color)
	}
	return s
}
