// adit-validate: Correlate adit metrics with git history to validate
// that the metrics predict actual editing cost.
//
// Usage: adit-validate <repo-path>
//
// Outputs aggregate statistics only — no file contents.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	"github.com/justinstimatze/adit-code/internal/score"
)

type fileData struct {
	Path             string `json:"path"`
	Lines            int    `json:"lines"`
	SizeGrade        string `json:"size_grade"`
	ContextReads     int    `json:"context_reads"`
	UnnecessaryReads int    `json:"unnecessary_reads"`
	GrepNoise        int    `json:"grep_noise"`
	BlastRadius      int    `json:"blast_radius"`
	Commits          int    `json:"commits"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: adit-validate <repo-path> [--sessions <sessions-path>]\n")
		fmt.Fprintf(os.Stderr, "  Without --sessions: correlates adit metrics with git commit frequency\n")
		fmt.Fprintf(os.Stderr, "  With --sessions:    correlates adit metrics with actual AI tool call counts\n")
		os.Exit(2)
	}
	repoPath := os.Args[1]

	// Check for --sessions, --investigate, --investigations flags
	sessionsPath := ""
	investigate := false
	investigations := false
	for i, arg := range os.Args {
		if arg == "--sessions" && i+1 < len(os.Args) {
			sessionsPath = os.Args[i+1]
		}
		if arg == "--investigate" {
			investigate = true
		}
		if arg == "--investigations" {
			investigations = true
		}
	}
	if sessionsPath != "" {
		if investigations {
			runInvestigations(repoPath, sessionsPath)
		} else if investigate {
			runInvestigation(repoPath, sessionsPath)
		} else {
			runToolCallValidation(repoPath, sessionsPath)
		}
		return
	}

	// Step 1: Score the repo with adit
	fmt.Fprintf(os.Stderr, "Scoring %s...\n", repoPath)
	cfg := config.Default()
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
	}
	pipeline := score.NewPipeline(frontends, cfg)
	result, err := pipeline.ScoreRepo([]string{repoPath})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Score failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Scored %d files\n", result.FilesScanned)

	// Step 2: Get per-file churn from git history
	fmt.Fprintf(os.Stderr, "Analyzing git history...\n")
	churn, err := getFileChurn(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Git analysis failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Found churn data for %d files\n", len(churn))

	// Step 3: Correlate
	var data []fileData
	for _, f := range result.Files {
		// Normalize path for matching
		relPath := f.Path
		if abs, err := filepath.Abs(f.Path); err == nil {
			if rel, err := filepath.Rel(repoPath, abs); err == nil {
				relPath = rel
			}
		}

		commits := churn[relPath]
		if commits == 0 {
			// Try matching by filename only
			base := filepath.Base(relPath)
			for k, v := range churn {
				if filepath.Base(k) == base {
					commits = v
					break
				}
			}
		}

		data = append(data, fileData{
			Path:             relPath,
			Lines:            f.Lines,
			SizeGrade:        f.SizeGrade,
			ContextReads:     f.ContextReads.Total,
			UnnecessaryReads: f.ContextReads.Unnecessary,
			GrepNoise:        f.Ambiguity.GrepNoise,
			BlastRadius:      f.BlastRadius.ImportedByCount,
			Commits:          commits,
		})
	}

	// Sort by commits descending
	sort.Slice(data, func(i, j int) bool {
		return data[i].Commits > data[j].Commits
	})

	// Step 4: Compute correlations
	fmt.Fprintf(os.Stderr, "\n")

	// Only include files with at least 1 commit for correlation
	var withHistory []fileData
	for _, d := range data {
		if d.Commits > 0 {
			withHistory = append(withHistory, d)
		}
	}

	fmt.Printf("# adit-validate: Predictive Validation Report\n\n")
	fmt.Printf("Repo: %s\n", repoPath)
	fmt.Printf("Files scored: %d\n", len(data))
	fmt.Printf("Files with git history: %d\n\n", len(withHistory))

	// Correlation: commits vs each metric
	fmt.Printf("## Correlations (Pearson r) — commits vs adit metrics\n\n")

	commits := extractFloat(withHistory, func(d fileData) float64 { return float64(d.Commits) })
	lines := extractFloat(withHistory, func(d fileData) float64 { return float64(d.Lines) })
	reads := extractFloat(withHistory, func(d fileData) float64 { return float64(d.ContextReads) })
	unnecessary := extractFloat(withHistory, func(d fileData) float64 { return float64(d.UnnecessaryReads) })
	noise := extractFloat(withHistory, func(d fileData) float64 { return float64(d.GrepNoise) })
	blast := extractFloat(withHistory, func(d fileData) float64 { return float64(d.BlastRadius) })

	fmt.Printf("| Metric | Pearson r | Interpretation |\n")
	fmt.Printf("|--------|-----------|----------------|\n")
	printCorr("Lines", commits, lines)
	printCorr("Context Reads", commits, reads)
	printCorr("Unnecessary Reads", commits, unnecessary)
	printCorr("Grep Noise", commits, noise)
	printCorr("Blast Radius", commits, blast)

	// Bucket analysis: group files by size grade, show avg commits
	fmt.Printf("\n## Average commits by size grade\n\n")
	fmt.Printf("| Grade | Files | Avg Commits | Median Commits |\n")
	fmt.Printf("|-------|-------|-------------|----------------|\n")
	for _, grade := range []string{"A", "B", "C", "D", "F"} {
		var gradeCommits []int
		for _, d := range withHistory {
			if d.SizeGrade == grade {
				gradeCommits = append(gradeCommits, d.Commits)
			}
		}
		if len(gradeCommits) == 0 {
			continue
		}
		sort.Ints(gradeCommits)
		sum := 0
		for _, c := range gradeCommits {
			sum += c
		}
		avg := float64(sum) / float64(len(gradeCommits))
		median := gradeCommits[len(gradeCommits)/2]
		fmt.Printf("| %s     | %5d | %11.1f | %14d |\n", grade, len(gradeCommits), avg, median)
	}

	// Top 10 highest-churn files with their adit metrics
	fmt.Printf("\n## Top 15 highest-churn files\n\n")
	fmt.Printf("| File | Commits | Lines | Grade | Reads | Unneeded | Noise | Blast |\n")
	fmt.Printf("|------|---------|-------|-------|-------|----------|-------|-------|\n")
	limit := min(len(data), 15)
	for _, d := range data[:limit] {
		name := filepath.Base(d.Path)
		fmt.Printf("| %-30s | %4d | %5d | %s     | %5d | %8d | %5d | %5d |\n",
			name, d.Commits, d.Lines, d.SizeGrade, d.ContextReads, d.UnnecessaryReads,
			d.GrepNoise, d.BlastRadius)
	}

	// Output raw data as JSON for further analysis
	fmt.Printf("\n## Raw data\n\n")
	fmt.Printf("```json\n")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(data)
	fmt.Printf("```\n")

	return
}

func getFileChurn(repoPath string) (map[string]int, error) {
	// git log --format="" --name-only to get all files touched per commit
	cmd := exec.Command("git", "-C", repoPath, "log", "--format=", "--name-only")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	churn := make(map[string]int)
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		churn[line]++
	}
	return churn, nil
}

func extractFloat(data []fileData, fn func(fileData) float64) []float64 {
	result := make([]float64, len(data))
	for i, d := range data {
		result[i] = fn(d)
	}
	return result
}

func pearson(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 3 {
		return 0
	}
	n := float64(len(x))
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

func printCorr(name string, commits, metric []float64) {
	r := pearson(commits, metric)
	interp := "negligible"
	switch {
	case math.Abs(r) > 0.7:
		interp = "strong"
	case math.Abs(r) > 0.4:
		interp = "moderate"
	case math.Abs(r) > 0.2:
		interp = "weak"
	}
	if r > 0 {
		interp += " positive"
	} else if r < 0 {
		interp += " negative"
	}
	fmt.Printf("| %-20s | %+.3f    | %-20s |\n", name, r, interp)
}
