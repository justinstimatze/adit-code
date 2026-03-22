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

// sessionStats captures per-file, per-session tool call patterns.
type sessionStats struct {
	// Per-file, per-session read counts (how many times was file X read in session Y?)
	fileSessionReads map[string]map[string]int // file -> sessionID -> count
	// Per-file, per-session edit counts
	fileSessionEdits map[string]map[string]int // file -> sessionID -> count
	// Per-file, per-session: was this file edited then re-edited?
	fileReEditSessions map[string]int // file -> count of sessions with 2+ edits
	// Total sessions per file
	fileSessions map[string]int // file -> count of distinct sessions touching this file
}

type editEvent struct {
	FilePath  string
	SessionID string
	Timestamp string
}

func runInvestigations(repoPath, sessionsPath string) {
	// Score the repo
	fmt.Fprintf(os.Stderr, "Scoring %s...\n", repoPath)
	cfg := config.Default()
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
		lang.NewGoFrontend(),
	}
	pipeline := score.NewPipeline(frontends, cfg)
	result, err := pipeline.ScoreRepo([]string{repoPath})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Score failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Scored %d files\n", result.FilesScanned)

	// Parse sessions for detailed per-session tool call data
	fmt.Fprintf(os.Stderr, "Parsing session transcripts...\n")
	stats := parseSessionStats(sessionsPath)
	fmt.Fprintf(os.Stderr, "Parsed %d files across sessions\n", len(stats.fileSessions))

	fmt.Printf("# Investigation Report\n\n")
	fmt.Printf("Repo: %s\n", repoPath)
	fmt.Printf("Sessions: %s\n\n", sessionsPath)

	// === Investigation 1: Session-level re-reads ===
	fmt.Printf("## Investigation 1: Session-Level Re-Reads\n\n")
	fmt.Printf("How many times does the AI re-read the same file within a single session?\n")
	fmt.Printf("High re-read counts suggest the AI loses coherence across large files.\n\n")

	type rereadRow struct {
		file         string
		avgReads     float64 // average reads per session
		maxReads     int     // max reads in any single session
		sessions     int     // total sessions touching this file
		lines        int
		sizeGrade    string
		contextReads int
		grepNoise    int
	}

	var rereadRows []rereadRow
	for _, f := range result.Files {
		absPath, _ := filepath.Abs(f.Path)
		sessionReads, ok := stats.fileSessionReads[absPath]
		if !ok {
			continue
		}

		totalReads := 0
		maxReads := 0
		for _, count := range sessionReads {
			totalReads += count
			if count > maxReads {
				maxReads = count
			}
		}
		nSessions := len(sessionReads)
		if nSessions == 0 {
			continue
		}

		rereadRows = append(rereadRows, rereadRow{
			file:         filepath.Base(f.Path),
			avgReads:     float64(totalReads) / float64(nSessions),
			maxReads:     maxReads,
			sessions:     nSessions,
			lines:        f.Lines,
			sizeGrade:    f.SizeGrade,
			contextReads: f.ContextReads.Total,
			grepNoise:    f.Ambiguity.GrepNoise,
		})
	}

	if len(rereadRows) > 3 {
		// Correlate avg re-reads with adit metrics
		avgR := make([]float64, len(rereadRows))
		maxR := make([]float64, len(rereadRows))
		lines := make([]float64, len(rereadRows))
		ctxReads := make([]float64, len(rereadRows))
		noise := make([]float64, len(rereadRows))

		for i, r := range rereadRows {
			avgR[i] = r.avgReads
			maxR[i] = float64(r.maxReads)
			lines[i] = float64(r.lines)
			ctxReads[i] = float64(r.contextReads)
			noise[i] = float64(r.grepNoise)
		}

		fmt.Printf("### Correlations: avg reads-per-session vs adit metrics\n\n")
		fmt.Printf("| Metric | vs Avg Reads/Session | vs Max Reads in Session |\n")
		fmt.Printf("|--------|---------------------|------------------------|\n")
		fmt.Printf("| Lines                | %+.3f               | %+.3f                  |\n", pearson(avgR, lines), pearson(maxR, lines))
		fmt.Printf("| Context Reads        | %+.3f               | %+.3f                  |\n", pearson(avgR, ctxReads), pearson(maxR, ctxReads))
		fmt.Printf("| Grep Noise           | %+.3f               | %+.3f                  |\n", pearson(avgR, noise), pearson(maxR, noise))

		// Bucket by size grade
		fmt.Printf("\n### Avg reads-per-session by size grade\n\n")
		fmt.Printf("| Grade | Files | Avg Reads/Session | Max Reads in Any Session |\n")
		fmt.Printf("|-------|-------|-------------------|-------------------------|\n")
		for _, grade := range []string{"A", "B", "C", "D", "F"} {
			var avgs []float64
			maxMax := 0
			for _, r := range rereadRows {
				if r.sizeGrade == grade {
					avgs = append(avgs, r.avgReads)
					if r.maxReads > maxMax {
						maxMax = r.maxReads
					}
				}
			}
			if len(avgs) == 0 {
				continue
			}
			s := 0.0
			for _, a := range avgs {
				s += a
			}
			fmt.Printf("| %s     | %5d | %17.1f | %23d |\n", grade, len(avgs), s/float64(len(avgs)), maxMax)
		}

		// Top 10 most re-read files
		sort.Slice(rereadRows, func(i, j int) bool {
			return rereadRows[i].avgReads > rereadRows[j].avgReads
		})
		fmt.Printf("\n### Top 15 most re-read files (avg reads per session)\n\n")
		fmt.Printf("| File | Avg Reads | Max Reads | Sessions | Lines | Grade | CtxReads | Noise |\n")
		fmt.Printf("|------|-----------|-----------|----------|-------|-------|----------|-------|\n")
		limit := min(len(rereadRows), 15)
		for _, r := range rereadRows[:limit] {
			fmt.Printf("| %-28s | %9.1f | %9d | %8d | %5d | %s     | %8d | %5d |\n",
				r.file, r.avgReads, r.maxReads, r.sessions, r.lines, r.sizeGrade, r.contextReads, r.grepNoise)
		}
	}

	// === Investigation 2: Edit Success Rate ===
	fmt.Printf("\n## Investigation 2: Edit Success Rate\n\n")
	fmt.Printf("How often does the AI re-edit the same file within a session?\n")
	fmt.Printf("Re-edits suggest the first edit was wrong or incomplete.\n\n")

	type editRow struct {
		file             string
		totalSessions    int
		reEditSessions   int
		reEditRate       float64
		lines            int
		sizeGrade        string
		contextReads     int
		unnecessaryReads int
		grepNoise        int
	}

	var editRows []editRow
	for _, f := range result.Files {
		absPath, _ := filepath.Abs(f.Path)
		sessionEdits, ok := stats.fileSessionEdits[absPath]
		if !ok {
			continue
		}

		totalSessions := len(sessionEdits)
		reEditSessions := 0
		for _, count := range sessionEdits {
			if count >= 2 {
				reEditSessions++
			}
		}
		if totalSessions == 0 {
			continue
		}

		editRows = append(editRows, editRow{
			file:             filepath.Base(f.Path),
			totalSessions:    totalSessions,
			reEditSessions:   reEditSessions,
			reEditRate:       float64(reEditSessions) / float64(totalSessions),
			lines:            f.Lines,
			sizeGrade:        f.SizeGrade,
			contextReads:     f.ContextReads.Total,
			unnecessaryReads: f.ContextReads.Unnecessary,
			grepNoise:        f.Ambiguity.GrepNoise,
		})
	}

	if len(editRows) > 3 {
		// Correlate re-edit rate with adit metrics
		reEditRate := make([]float64, len(editRows))
		lines := make([]float64, len(editRows))
		ctxReads := make([]float64, len(editRows))
		unnecessary := make([]float64, len(editRows))
		noise := make([]float64, len(editRows))

		for i, r := range editRows {
			reEditRate[i] = r.reEditRate
			lines[i] = float64(r.lines)
			ctxReads[i] = float64(r.contextReads)
			unnecessary[i] = float64(r.unnecessaryReads)
			noise[i] = float64(r.grepNoise)
		}

		fmt.Printf("### Correlations: re-edit rate vs adit metrics\n\n")
		fmt.Printf("| Metric | vs Re-Edit Rate |\n")
		fmt.Printf("|--------|----------------|\n")
		printCorr("Lines", reEditRate, lines)
		printCorr("Context Reads", reEditRate, ctxReads)
		printCorr("Unnecessary Reads", reEditRate, unnecessary)
		printCorr("Grep Noise", reEditRate, noise)

		// Bucket by size grade
		fmt.Printf("\n### Re-edit rate by size grade\n\n")
		fmt.Printf("| Grade | Files Edited | Avg Re-Edit Rate | Files with Re-Edits |\n")
		fmt.Printf("|-------|-------------|------------------|--------------------|\n")
		for _, grade := range []string{"A", "B", "C", "D", "F"} {
			var rates []float64
			reEditCount := 0
			for _, r := range editRows {
				if r.sizeGrade == grade {
					rates = append(rates, r.reEditRate)
					if r.reEditSessions > 0 {
						reEditCount++
					}
				}
			}
			if len(rates) == 0 {
				continue
			}
			s := 0.0
			for _, r := range rates {
				s += r
			}
			fmt.Printf("| %s     | %11d | %15.1f%% | %18d |\n", grade, len(rates), s/float64(len(rates))*100, reEditCount)
		}

		// Top re-edited files
		sort.Slice(editRows, func(i, j int) bool {
			return editRows[i].reEditRate > editRows[j].reEditRate
		})
		fmt.Printf("\n### Top 15 files by re-edit rate (edited 2+ times per session)\n\n")
		fmt.Printf("| File | Re-Edit Rate | Re-Edit Sessions | Total Sessions | Lines | Grade |\n")
		fmt.Printf("|------|-------------|-----------------|----------------|-------|-------|\n")
		limit := min(len(editRows), 15)
		for _, r := range editRows[:limit] {
			fmt.Printf("| %-28s | %10.0f%% | %15d | %14d | %5d | %s     |\n",
				r.file, r.reEditRate*100, r.reEditSessions, r.totalSessions, r.lines, r.sizeGrade)
		}
	}

	// === Investigation 3: Reference-Based Ambiguity ===
	fmt.Printf("\n## Investigation 3: Reference-Based Ambiguity\n\n")
	fmt.Printf("Does a file that *references* ambiguous names (not just defines them)\n")
	fmt.Printf("correlate with more tool calls?\n\n")

	// Build a set of all ambiguous names and their definition counts
	ambiguousNameCounts := make(map[string]int)
	for _, a := range result.Summary.AmbiguousNames {
		ambiguousNameCounts[a.Name] = a.Count
	}

	// For each file, count how many of its imports reference ambiguous names
	type refAmbigRow struct {
		file         string
		refNoise     int // sum of (count-1) for each ambiguous name this file imports
		defNoise     int // adit's current grep_noise (definition-based)
		sessionTotal int
		lines        int
	}

	toolCalls := parseAllSessions(sessionsPath)
	normalizedTC := make(map[string]fileToolCalls)
	for tcPath, tcVal := range toolCalls {
		abs, _ := filepath.Abs(tcPath)
		existing := normalizedTC[abs]
		existing.Reads += tcVal.Reads
		existing.Greps += tcVal.Greps
		normalizedTC[abs] = existing
	}

	var refRows []refAmbigRow
	for _, f := range result.Files {
		absPath, _ := filepath.Abs(f.Path)
		tc := normalizedTC[absPath]
		if tc.Reads+tc.Greps == 0 {
			continue
		}

		// Count reference noise: for each import name that is ambiguous, add (count-1)
		refNoise := 0
		fa := getFileAnalysis(repoPath, f.Path, frontends)
		if fa != nil {
			for _, imp := range fa.Imports {
				if count, ok := ambiguousNameCounts[imp.Name]; ok {
					refNoise += count - 1
				}
			}
		}

		refRows = append(refRows, refAmbigRow{
			file:         filepath.Base(f.Path),
			refNoise:     refNoise,
			defNoise:     f.Ambiguity.GrepNoise,
			sessionTotal: tc.Reads + tc.Greps,
			lines:        f.Lines,
		})
	}

	if len(refRows) > 3 {
		st := make([]float64, len(refRows))
		rn := make([]float64, len(refRows))
		dn := make([]float64, len(refRows))

		for i, r := range refRows {
			st[i] = float64(r.sessionTotal)
			rn[i] = float64(r.refNoise)
			dn[i] = float64(r.defNoise)
		}

		fmt.Printf("### Correlations: tool calls vs ambiguity formulations\n\n")
		fmt.Printf("| Metric | Pearson r | Interpretation |\n")
		fmt.Printf("|--------|-----------|----------------|\n")
		printCorr("Grep Noise (defs, current)", st, dn)
		printCorr("Ref Noise (imports)", st, rn)

		// Combined
		combined := make([]float64, len(refRows))
		for i := range refRows {
			combined[i] = float64(refRows[i].refNoise + refRows[i].defNoise)
		}
		printCorr("Combined (def + ref noise)", st, combined)
	}
}

func parseSessionStats(sessionsPath string) sessionStats {
	stats := sessionStats{
		fileSessionReads:   make(map[string]map[string]int),
		fileSessionEdits:   make(map[string]map[string]int),
		fileReEditSessions: make(map[string]int),
		fileSessions:       make(map[string]int),
	}

	filepath.Walk(sessionsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		// Determine session ID from path
		sessionID := filepath.Base(filepath.Dir(path))
		if strings.HasSuffix(path, ".jsonl") && !strings.Contains(path, "subagents") {
			sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
		}

		decoder := json.NewDecoder(f)
		for decoder.More() {
			var msg sessionMessage
			if err := decoder.Decode(&msg); err != nil {
				break
			}

			var content []contentItem
			if json.Unmarshal(msg.Message.Content, &content) != nil {
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
						abs, _ := filepath.Abs(inp.FilePath)
						if stats.fileSessionReads[abs] == nil {
							stats.fileSessionReads[abs] = make(map[string]int)
						}
						stats.fileSessionReads[abs][sessionID]++
						stats.fileSessions[abs]++ // rough count
					}
				case "Edit", "Write":
					var inp struct {
						FilePath string `json:"file_path"`
					}
					if json.Unmarshal(item.Input, &inp) == nil && inp.FilePath != "" {
						abs, _ := filepath.Abs(inp.FilePath)
						if stats.fileSessionEdits[abs] == nil {
							stats.fileSessionEdits[abs] = make(map[string]int)
						}
						stats.fileSessionEdits[abs][sessionID]++
					}
				}
			}
		}
		return nil
	})

	// Count re-edit sessions
	for file, sessions := range stats.fileSessionEdits {
		for _, count := range sessions {
			if count >= 2 {
				stats.fileReEditSessions[file]++
			}
		}
	}

	return stats
}

func getFileAnalysis(repoPath, filePath string, frontends []lang.Frontend) *lang.FileAnalysis {
	ext := filepath.Ext(filePath)
	for _, fe := range frontends {
		for _, feExt := range fe.Extensions() {
			if ext == feExt {
				src, err := os.ReadFile(filePath)
				if err != nil {
					return nil
				}
				fa, err := fe.Analyze(filePath, src)
				if err != nil {
					return nil
				}
				return fa
			}
		}
	}
	return nil
}

// Suppress unused import warning
var _ = math.Abs
