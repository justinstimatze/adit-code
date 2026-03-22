package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	"github.com/justinstimatze/adit-code/internal/score"
)

type sessionMessage struct {
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentItem struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type readInput struct {
	FilePath string `json:"file_path"`
}

// fileToolCalls tracks how many times each file was targeted by Read/Grep.
type fileToolCalls struct {
	Reads int
	Greps int
}

func runToolCallValidation(repoPath, sessionsPath string) {
	// Step 1: Score the repo
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

	// Step 2: Parse session JSONL files for tool calls
	fmt.Fprintf(os.Stderr, "Parsing session transcripts from %s...\n", sessionsPath)
	toolCalls := parseAllSessions(sessionsPath)
	fmt.Fprintf(os.Stderr, "Found tool call data for %d files\n", len(toolCalls))

	// Step 3: Correlate
	type correlationRow struct {
		Path             string `json:"path"`
		Lines            int    `json:"lines"`
		MaxFnLength      int    `json:"max_fn_length"`
		SizeGrade        string `json:"size_grade"`
		ContextReads     int    `json:"context_reads"`
		UnnecessaryReads int    `json:"unnecessary_reads"`
		GrepNoise        int    `json:"grep_noise"`
		BlastRadius      int    `json:"blast_radius"`
		SessionReads     int    `json:"session_reads"`
		SessionGreps     int    `json:"session_greps"`
		TotalToolCalls   int    `json:"total_tool_calls"`
	}

	var rows []correlationRow
	// Build multiple lookup indexes for matching tool calls to scored files.
	// Session transcripts may reference old paths (renamed projects), so we
	// match by: absolute path, relative path suffix, and basename.
	tcByAbs := make(map[string]fileToolCalls)  // full absolute path
	tcByRel := make(map[string]fileToolCalls)  // relative path within project (e.g., "src/foo.ts")
	tcByBase := make(map[string]fileToolCalls) // basename only (e.g., "foo.ts")
	for tcPath, tcVal := range toolCalls {
		abs, err := filepath.Abs(tcPath)
		if err != nil {
			abs = tcPath
		}
		existing := tcByAbs[abs]
		existing.Reads += tcVal.Reads
		existing.Greps += tcVal.Greps
		tcByAbs[abs] = existing

		// Extract relative path: strip common prefixes like /home/user/Documents/project/
		// Use the last 3 path components as a relative key
		parts := strings.Split(filepath.ToSlash(tcPath), "/")
		if len(parts) >= 3 {
			relKey := strings.Join(parts[len(parts)-3:], "/")
			ex := tcByRel[relKey]
			ex.Reads += tcVal.Reads
			ex.Greps += tcVal.Greps
			tcByRel[relKey] = ex
		}
		if len(parts) >= 2 {
			relKey2 := strings.Join(parts[len(parts)-2:], "/")
			ex := tcByRel[relKey2]
			ex.Reads += tcVal.Reads
			ex.Greps += tcVal.Greps
			tcByRel[relKey2] = ex
		}

		base := filepath.Base(tcPath)
		ex := tcByBase[base]
		ex.Reads += tcVal.Reads
		ex.Greps += tcVal.Greps
		tcByBase[base] = ex
	}

	for _, f := range result.Files {
		absPath, _ := filepath.Abs(f.Path)
		base := filepath.Base(f.Path)
		fParts := strings.Split(filepath.ToSlash(f.Path), "/")

		// Try matching in order: absolute path, relative suffix, basename
		tc := tcByAbs[absPath]
		if tc.Reads == 0 && tc.Greps == 0 && len(fParts) >= 3 {
			relKey := strings.Join(fParts[len(fParts)-3:], "/")
			tc = tcByRel[relKey]
		}
		if tc.Reads == 0 && tc.Greps == 0 && len(fParts) >= 2 {
			relKey := strings.Join(fParts[len(fParts)-2:], "/")
			tc = tcByRel[relKey]
		}
		if tc.Reads == 0 && tc.Greps == 0 {
			tc = tcByBase[base]
		}
		if tc.Reads == 0 && tc.Greps == 0 {
			continue
		}

		rows = append(rows, correlationRow{
			Path:             filepath.Base(f.Path),
			Lines:            f.Lines,
			MaxFnLength:      f.Functions.MaxLength,
			SizeGrade:        f.SizeGrade,
			ContextReads:     f.ContextReads.Total,
			UnnecessaryReads: f.ContextReads.Unnecessary,
			GrepNoise:        f.Ambiguity.GrepNoise,
			BlastRadius:      f.BlastRadius.ImportedByCount,
			SessionReads:     tc.Reads,
			SessionGreps:     tc.Greps,
			TotalToolCalls:   tc.Reads + tc.Greps,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TotalToolCalls > rows[j].TotalToolCalls
	})

	// Step 4: Report
	fmt.Printf("# adit-validate: Tool Call Correlation Report\n\n")
	fmt.Printf("Repo: %s\n", repoPath)
	fmt.Printf("Sessions: %s\n", sessionsPath)
	fmt.Printf("Files with both adit scores and tool call data: %d\n\n", len(rows))

	// Extract arrays for correlation
	sessionTotal := make([]float64, len(rows))
	sessionReads := make([]float64, len(rows))
	sessionGreps := make([]float64, len(rows))
	lines := make([]float64, len(rows))
	maxFn := make([]float64, len(rows))
	ctxReads := make([]float64, len(rows))
	unnecessary := make([]float64, len(rows))
	noise := make([]float64, len(rows))
	blast := make([]float64, len(rows))

	for i, r := range rows {
		sessionTotal[i] = float64(r.TotalToolCalls)
		sessionReads[i] = float64(r.SessionReads)
		sessionGreps[i] = float64(r.SessionGreps)
		lines[i] = float64(r.Lines)
		maxFn[i] = float64(r.MaxFnLength)
		ctxReads[i] = float64(r.ContextReads)
		unnecessary[i] = float64(r.UnnecessaryReads)
		noise[i] = float64(r.GrepNoise)
		blast[i] = float64(r.BlastRadius)
	}

	fmt.Printf("## Correlations: adit metrics vs actual AI tool calls\n\n")
	fmt.Printf("N = %d files\n\n", len(rows))
	fmt.Printf("### Total tool calls (Read + Grep) vs adit metrics\n\n")
	fmt.Printf("| Metric | Pearson r | Spearman r | Interpretation |\n")
	fmt.Printf("|--------|-----------|------------|----------------|\n")
	printCorrBoth("Lines", sessionTotal, lines)
	printCorrBoth("Max Function Length", sessionTotal, maxFn)
	printCorrBoth("Context Reads", sessionTotal, ctxReads)
	printCorrBoth("Unnecessary Reads", sessionTotal, unnecessary)
	printCorrBoth("Grep Noise", sessionTotal, noise)
	printCorrBoth("Blast Radius", sessionTotal, blast)

	fmt.Printf("\n### Read tool calls only vs adit metrics\n\n")
	fmt.Printf("| Metric | Pearson r | Interpretation |\n")
	fmt.Printf("|--------|-----------|----------------|\n")
	printCorr("Lines", sessionReads, lines)
	printCorr("Max Function Length", sessionReads, maxFn)
	printCorr("Context Reads", sessionReads, ctxReads)
	printCorr("Unnecessary Reads", sessionReads, unnecessary)
	printCorr("Grep Noise", sessionReads, noise)
	printCorr("Blast Radius", sessionReads, blast)

	if len(sessionGreps) > 0 && sum(sessionGreps) > 0 {
		fmt.Printf("\n### Grep tool calls only vs adit metrics\n\n")
		fmt.Printf("| Metric | Pearson r | Interpretation |\n")
		fmt.Printf("|--------|-----------|----------------|\n")
		printCorr("Lines", sessionGreps, lines)
		printCorr("Context Reads", sessionGreps, ctxReads)
		printCorr("Grep Noise", sessionGreps, noise)
		printCorr("Blast Radius", sessionGreps, blast)
	}

	// Bucket by size grade
	fmt.Printf("\n## Average tool calls by size grade\n\n")
	fmt.Printf("| Grade | Files | Avg Reads | Avg Greps | Avg Total |\n")
	fmt.Printf("|-------|-------|-----------|-----------|-----------|\n")
	for _, grade := range []string{"A", "B", "C", "D", "F"} {
		var reads, greps, total []int
		for _, r := range rows {
			if r.SizeGrade == grade {
				reads = append(reads, r.SessionReads)
				greps = append(greps, r.SessionGreps)
				total = append(total, r.TotalToolCalls)
			}
		}
		if len(reads) == 0 {
			continue
		}
		fmt.Printf("| %s     | %5d | %9.1f | %9.1f | %9.1f |\n",
			grade, len(reads), avg(reads), avg(greps), avg(total))
	}

	// Top 15 most tool-call-heavy files
	fmt.Printf("\n## Top 15 files by total AI tool calls\n\n")
	fmt.Printf("| File | Reads | Greps | Total | Lines | Grade | CtxReads | Noise | Blast |\n")
	fmt.Printf("|------|-------|-------|-------|-------|-------|----------|-------|-------|\n")
	limit := min(len(rows), 15)
	for _, r := range rows[:limit] {
		fmt.Printf("| %-28s | %5d | %5d | %5d | %5d | %s     | %8d | %5d | %5d |\n",
			r.Path, r.SessionReads, r.SessionGreps, r.TotalToolCalls,
			r.Lines, r.SizeGrade, r.ContextReads, r.GrepNoise, r.BlastRadius)
	}
}

func parseAllSessions(sessionsPath string) map[string]fileToolCalls {
	result := make(map[string]fileToolCalls)

	// Find all JSONL files (main + subagent)
	var files []string
	filepath.Walk(sessionsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})

	for _, f := range files {
		parseSessionFile(f, result)
	}

	return result
}

func parseSessionFile(path string, result map[string]fileToolCalls) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	for decoder.More() {
		var msg sessionMessage
		if err := decoder.Decode(&msg); err != nil {
			// Try line-by-line fallback
			break
		}

		var content []contentItem
		if err := json.Unmarshal(msg.Message.Content, &content); err != nil {
			continue
		}

		for _, item := range content {
			if item.Type != "tool_use" {
				continue
			}

			switch item.Name {
			case "Read":
				var inp readInput
				if json.Unmarshal(item.Input, &inp) == nil && inp.FilePath != "" {
					// Store by full path
					tc := result[inp.FilePath]
					tc.Reads++
					result[inp.FilePath] = tc
				}
			case "Grep":
				var inp grepInput
				if json.Unmarshal(item.Input, &inp) == nil && inp.Path != "" {
					// Store by full path
					tc := result[inp.Path]
					tc.Greps++
					result[inp.Path] = tc
				}
			}
		}
	}
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func avg(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0
	for _, x := range xs {
		s += x
	}
	return float64(s) / float64(len(xs))
}

// pearson and printCorr are defined in main.go — use them via package scope
// (they're already in the same package)

// Spearman rank correlation — more robust for non-linear relationships
func spearman(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 3 {
		return 0
	}
	rx := ranks(x)
	ry := ranks(y)
	return pearson(rx, ry)
}

func ranks(data []float64) []float64 {
	type indexedValue struct {
		val float64
		idx int
	}
	iv := make([]indexedValue, len(data))
	for i, v := range data {
		iv[i] = indexedValue{v, i}
	}
	sort.Slice(iv, func(i, j int) bool {
		return iv[i].val < iv[j].val
	})

	r := make([]float64, len(data))
	for rank, item := range iv {
		r[item.idx] = float64(rank + 1)
	}
	return r
}

func printCorrBoth(name string, x, y []float64) {
	p := pearson(x, y)
	s := spearman(x, y)
	interp := "negligible"
	avg := (math.Abs(p) + math.Abs(s)) / 2
	switch {
	case avg > 0.7:
		interp = "strong"
	case avg > 0.4:
		interp = "moderate"
	case avg > 0.2:
		interp = "weak"
	}
	if p > 0 {
		interp += " positive"
	} else if p < 0 {
		interp += " negative"
	}
	fmt.Printf("| %-20s | %+.3f    | %+.3f     | %-20s |\n", name, p, s, interp)
}
