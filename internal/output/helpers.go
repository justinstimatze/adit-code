package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/score"
)

// FilterByMinGrade returns only files at the given size grade or worse.
func FilterByMinGrade(files []score.FileScore, minGrade string) []score.FileScore {
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

// SortFiles sorts file scores by the given key.
func SortFiles(files []score.FileScore, sortBy string) {
	switch sortBy {
	case "blast":
		sort.Slice(files, func(i, j int) bool {
			return files[i].BlastRadius.ImportedByCount > files[j].BlastRadius.ImportedByCount
		})
	case "reads":
		sort.Slice(files, func(i, j int) bool {
			return files[i].ContextReads.Total > files[j].ContextReads.Total
		})
	case "ambig", "noise":
		sort.Slice(files, func(i, j int) bool {
			return files[i].Ambiguity.GrepNoise > files[j].Ambiguity.GrepNoise
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

// HasIssues returns true if the repo score has any findings.
func HasIssues(result *score.RepoScore) bool {
	return len(result.Summary.Relocatable) > 0 ||
		len(result.Summary.AmbiguousNames) > 0 ||
		len(result.Summary.Cycles) > 0
}

// CheckFileThresholds checks a single file against thresholds.
func CheckFileThresholds(path string, f *score.FileScore, cfg config.Config) []string {
	var violations []string
	t := cfg.ThresholdsForPath(path)
	if t.MaxFileLines > 0 && f.Lines > t.MaxFileLines {
		violations = append(violations, fmt.Sprintf("%s: %d lines (max %d)", path, f.Lines, t.MaxFileLines))
	}
	if t.MaxContextReads > 0 && f.ContextReads.Total > t.MaxContextReads {
		violations = append(violations, fmt.Sprintf("%s: %d context reads (max %d)", path, f.ContextReads.Total, t.MaxContextReads))
	}
	if t.MaxUnnecessaryReads > 0 && f.ContextReads.Unnecessary > t.MaxUnnecessaryReads {
		violations = append(violations, fmt.Sprintf("%s: %d unnecessary reads (max %d)", path, f.ContextReads.Unnecessary, t.MaxUnnecessaryReads))
	}
	if t.MaxAmbiguityDefs > 0 && f.Ambiguity.GrepNoise > t.MaxAmbiguityDefs {
		violations = append(violations, fmt.Sprintf("%s: grep noise %d (max %d)", path, f.Ambiguity.GrepNoise, t.MaxAmbiguityDefs))
	}
	if t.MaxBlastRadius > 0 && f.BlastRadius.ImportedByCount > t.MaxBlastRadius {
		violations = append(violations, fmt.Sprintf("%s: blast radius %d (max %d)", path, f.BlastRadius.ImportedByCount, t.MaxBlastRadius))
	}
	if t.MaxNestingDepth > 0 && f.MaxNestingDepth > t.MaxNestingDepth {
		violations = append(violations, fmt.Sprintf("%s: nesting depth %d (max %d)", path, f.MaxNestingDepth, t.MaxNestingDepth))
	}
	if t.MaxParams > 0 && f.MaxParams > t.MaxParams {
		violations = append(violations, fmt.Sprintf("%s: max params %d (max %d)", path, f.MaxParams, t.MaxParams))
	}
	return violations
}

// CheckThresholds checks all files against thresholds.
func CheckThresholds(result *score.RepoScore, cfg config.Config) []string {
	var violations []string

	for _, f := range result.Files {
		violations = append(violations, CheckFileThresholds(f.Path, &f, cfg)...)
	}

	t := cfg.Thresholds
	if t.MaxCycleLength == 0 && len(result.Summary.Cycles) > 0 {
		for _, cy := range result.Summary.Cycles {
			violations = append(violations, fmt.Sprintf("import cycle: %s", strings.Join(cy.Files, " → ")))
		}
	}

	return violations
}
