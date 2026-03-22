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

// grepCallDetail captures what the AI actually grepped for.
type grepCallDetail struct {
	Pattern string
	Path    string // directory or file targeted
}

func runInvestigation(repoPath, sessionsPath string) {
	// Score the repo
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

	// Parse sessions for detailed tool call data
	fmt.Fprintf(os.Stderr, "Parsing sessions for detailed grep analysis...\n")
	toolCalls := parseAllSessions(sessionsPath)
	grepDetails := parseGrepDetails(sessionsPath)
	fmt.Fprintf(os.Stderr, "Found %d grep patterns\n", len(grepDetails))

	// Build normalized tool call map
	normalizedTC := make(map[string]fileToolCalls)
	for tcPath, tcVal := range toolCalls {
		abs, _ := filepath.Abs(tcPath)
		existing := normalizedTC[abs]
		existing.Reads += tcVal.Reads
		existing.Greps += tcVal.Greps
		normalizedTC[abs] = existing
	}

	// Build adit score map by basename for cross-referencing
	type enrichedFile struct {
		score.FileScore
		SessionReads int
		SessionGreps int
	}
	var files []enrichedFile
	for _, f := range result.Files {
		absPath, _ := filepath.Abs(f.Path)
		tc := normalizedTC[absPath]
		files = append(files, enrichedFile{f, tc.Reads, tc.Greps})
	}

	fmt.Printf("# Metric Investigation Report\n\n")
	fmt.Printf("Repo: %s (%d files scored)\n\n", repoPath, len(files))

	// ===== AMBIGUITY INVESTIGATION =====
	fmt.Printf("## 1. Why Ambiguity Doesn't Correlate\n\n")

	// Bucket files by ambiguity level
	type bucket struct {
		label string
		files []enrichedFile
	}
	ambigBuckets := []bucket{
		{"noise=0", nil},
		{"noise 1-5", nil},
		{"noise >5", nil},
	}
	for _, f := range files {
		switch {
		case f.Ambiguity.GrepNoise == 0:
			ambigBuckets[0].files = append(ambigBuckets[0].files, f)
		case f.Ambiguity.GrepNoise <= 5:
			ambigBuckets[1].files = append(ambigBuckets[1].files, f)
		default:
			ambigBuckets[2].files = append(ambigBuckets[2].files, f)
		}
	}

	fmt.Printf("| Ambiguity Level | Files | Avg Greps | Avg Reads | Avg Lines | Avg CtxReads |\n")
	fmt.Printf("|-----------------|-------|-----------|-----------|-----------|-------------|\n")
	for _, b := range ambigBuckets {
		if len(b.files) == 0 {
			continue
		}
		var greps, reads, lines, ctxReads float64
		for _, f := range b.files {
			greps += float64(f.SessionGreps)
			reads += float64(f.SessionReads)
			lines += float64(f.Lines)
			ctxReads += float64(f.ContextReads.Total)
		}
		n := float64(len(b.files))
		fmt.Printf("| %-15s | %5d | %9.1f | %9.1f | %9.0f | %11.1f |\n",
			b.label, len(b.files), greps/n, reads/n, lines/n, ctxReads/n)
	}

	fmt.Printf("\n**Hypothesis**: Ambiguity measures a property of definitions (how unique are the names\n")
	fmt.Printf("THIS file defines), but tool calls are driven by how often the AI needs to READ this file.\n")
	fmt.Printf("A file with perfectly unique names but imported by 50 others still gets tons of reads.\n")
	fmt.Printf("Ambiguity is a per-search cost (more grep noise), not a per-file cost.\n\n")

	// Check: does ambiguity correlate AFTER controlling for file size?
	fmt.Printf("### Ambiguity vs greps, controlling for file size\n\n")
	fmt.Printf("| Size Grade | 0%% Ambiguity Avg Greps | >0%% Ambiguity Avg Greps | Difference |\n")
	fmt.Printf("|------------|----------------------|------------------------|------------|\n")
	for _, grade := range []string{"A", "B", "C"} {
		var zeroGreps, nonzeroGreps []float64
		for _, f := range files {
			if f.SizeGrade != grade || (f.SessionGreps == 0 && f.SessionReads == 0) {
				continue
			}
			if f.Ambiguity.GrepNoise == 0 {
				zeroGreps = append(zeroGreps, float64(f.SessionGreps))
			} else {
				nonzeroGreps = append(nonzeroGreps, float64(f.SessionGreps))
			}
		}
		if len(zeroGreps) > 0 && len(nonzeroGreps) > 0 {
			z := avgF(zeroGreps)
			nz := avgF(nonzeroGreps)
			fmt.Printf("| %-10s | %5.1f (n=%d)          | %5.1f (n=%d)            | %+.1f       |\n",
				grade, z, len(zeroGreps), nz, len(nonzeroGreps), nz-z)
		}
	}

	// ===== GREP PATTERN ANALYSIS =====
	fmt.Printf("\n## 2. What Does the AI Actually Grep For?\n\n")

	// Analyze grep patterns — are they searching for names we track?
	patternCounts := make(map[string]int)
	for _, g := range grepDetails {
		patternCounts[g.Pattern]++
	}

	type patternEntry struct {
		pattern string
		count   int
	}
	var patterns []patternEntry
	for p, c := range patternCounts {
		patterns = append(patterns, patternEntry{p, c})
	}
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].count > patterns[j].count
	})

	fmt.Printf("Top 20 most-used grep patterns:\n\n")
	fmt.Printf("| Pattern | Count |\n")
	fmt.Printf("|---------|-------|\n")
	limit := min(len(patterns), 20)
	for _, p := range patterns[:limit] {
		display := p.pattern
		if len(display) > 60 {
			display = display[:57] + "..."
		}
		fmt.Printf("| `%-60s` | %5d |\n", display, p.count)
	}

	// Check: how many grep patterns match ambiguous names?
	ambiguousNames := make(map[string]bool)
	for _, a := range result.Summary.AmbiguousNames {
		ambiguousNames[a.Name] = true
	}
	matchesAmbiguous := 0
	for _, g := range grepDetails {
		if ambiguousNames[g.Pattern] {
			matchesAmbiguous++
		}
	}
	fmt.Printf("\nGrep patterns that exactly match an ambiguous name: %d / %d (%.1f%%)\n",
		matchesAmbiguous, len(grepDetails), float64(matchesAmbiguous)/float64(len(grepDetails))*100)

	// ===== CONTEXT READS INVESTIGATION =====
	fmt.Printf("\n## 3. Context Reads: Could We Improve It?\n\n")

	// Current metric: count of unique local modules imported
	// Hypothesis: weighting by imported module SIZE would correlate better
	fmt.Printf("### Weighted context reads (by imported module size)\n\n")

	// Build module size map
	moduleSizes := make(map[string]int)
	for _, f := range result.Files {
		base := filepath.Base(f.Path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		moduleSizes[name] = f.Lines
		moduleSizes[base] = f.Lines
	}

	type weightedRow struct {
		path          string
		contextReads  int
		weightedReads float64
		sessionTotal  int
	}
	var wRows []weightedRow
	for _, f := range files {
		if f.SessionReads == 0 && f.SessionGreps == 0 {
			continue
		}
		// Compute weighted context reads
		weighted := 0.0
		seen := make(map[string]bool)
		for _, imp := range getFileImports(repoPath, f.Path, frontends) {
			mod := imp.SourceModule
			// Strip relative dots
			for strings.HasPrefix(mod, ".") {
				mod = mod[1:]
			}
			if mod == "" || seen[mod] {
				continue
			}
			seen[mod] = true
			size := moduleSizes[mod]
			if size == 0 {
				size = 100 // default for unknown modules
			}
			weighted += math.Log2(float64(size) + 1) // log-weighted by size
		}
		wRows = append(wRows, weightedRow{
			path:          filepath.Base(f.Path),
			contextReads:  f.ContextReads.Total,
			weightedReads: weighted,
			sessionTotal:  f.SessionReads + f.SessionGreps,
		})
	}

	if len(wRows) > 3 {
		// Compute correlations
		st := make([]float64, len(wRows))
		cr := make([]float64, len(wRows))
		wr := make([]float64, len(wRows))
		for i, r := range wRows {
			st[i] = float64(r.sessionTotal)
			cr[i] = float64(r.contextReads)
			wr[i] = r.weightedReads
		}
		fmt.Printf("| Variant | Pearson r vs tool calls |\n")
		fmt.Printf("|---------|------------------------|\n")
		fmt.Printf("| Context Reads (current, count)       | %+.3f |\n", pearson(st, cr))
		fmt.Printf("| Context Reads (log-weighted by size)  | %+.3f |\n", pearson(st, wr))
	}

	// ===== BLAST RADIUS INVESTIGATION =====
	fmt.Printf("\n## 4. Blast Radius: Why It's Inconsistent\n\n")

	// Hypothesis: blast radius correlates with reads (this file is read for context)
	// but NOT with edits to this file
	var blastFiles []enrichedFile
	for _, f := range files {
		if f.SessionReads+f.SessionGreps > 0 {
			blastFiles = append(blastFiles, f)
		}
	}
	sort.Slice(blastFiles, func(i, j int) bool {
		return blastFiles[i].BlastRadius.ImportedByCount > blastFiles[j].BlastRadius.ImportedByCount
	})

	fmt.Printf("Top 10 highest blast radius files:\n\n")
	fmt.Printf("| File | Blast | Reads | Greps | Lines | CtxReads |\n")
	fmt.Printf("|------|-------|-------|-------|-------|----------|\n")
	limit = min(len(blastFiles), 10)
	for _, f := range blastFiles[:limit] {
		fmt.Printf("| %-28s | %5d | %5d | %5d | %5d | %8d |\n",
			filepath.Base(f.Path), f.BlastRadius.ImportedByCount,
			f.SessionReads, f.SessionGreps, f.Lines, f.ContextReads.Total)
	}

	fmt.Printf("\n**Observation**: High-blast files (types.ts, task.ts) tend to be stable definitions\n")
	fmt.Printf("that get READ often for context but are not the files being EDITED. Blast radius\n")
	fmt.Printf("predicts \"how often this file is read by the AI\" rather than \"how hard this file is to edit.\"\n")

	// ===== NEW METRICS TO CONSIDER =====
	fmt.Printf("\n## 5. Potential New Metrics\n\n")

	// Metric idea: "edit density" = edits per read (how often reads turn into edits)
	// Metric idea: "grep noise" = sum of (definition_count - 1) for each name defined in file
	// Metric idea: "import weight" = sum of imported module sizes

	// Compute grep noise (sum of extra definitions for each name)
	nameIdx := make(map[string]int) // name -> count of files defining it
	for _, f := range result.Files {
		seen := make(map[string]bool)
		for _, a := range result.Summary.AmbiguousNames {
			for _, site := range a.Sites {
				abs1, _ := filepath.Abs(site.File)
				abs2, _ := filepath.Abs(f.Path)
				if abs1 == abs2 && !seen[a.Name] {
					seen[a.Name] = true
					nameIdx[f.Path+"::"+a.Name] = a.Count
				}
			}
		}
	}

	// For each file, compute grep noise = sum of (count-1) for each ambiguous name it defines
	type noiseRow struct {
		path         string
		grepNoise    int // from cross-file ambiguous name analysis
		aditNoise    int // from adit's grep_noise metric
		sessionGreps int
		sessionTotal int
	}
	var noiseRows []noiseRow
	for _, f := range files {
		if f.SessionReads+f.SessionGreps == 0 {
			continue
		}
		noise := 0
		for _, a := range result.Summary.AmbiguousNames {
			for _, site := range a.Sites {
				abs1, _ := filepath.Abs(site.File)
				abs2, _ := filepath.Abs(f.Path)
				if abs1 == abs2 {
					noise += a.Count - 1 // extra definitions beyond this one
					break
				}
			}
		}
		noiseRows = append(noiseRows, noiseRow{
			path:         filepath.Base(f.Path),
			grepNoise:    noise,
			aditNoise:    f.Ambiguity.GrepNoise,
			sessionGreps: f.SessionGreps,
			sessionTotal: f.SessionReads + f.SessionGreps,
		})
	}

	if len(noiseRows) > 3 {
		st := make([]float64, len(noiseRows))
		sg := make([]float64, len(noiseRows))
		gn := make([]float64, len(noiseRows))
		an := make([]float64, len(noiseRows))
		for i, r := range noiseRows {
			st[i] = float64(r.sessionTotal)
			sg[i] = float64(r.sessionGreps)
			gn[i] = float64(r.grepNoise)
			an[i] = float64(r.aditNoise)
		}
		fmt.Printf("### Ambiguity reformulations\n\n")
		fmt.Printf("| Metric | vs Total Calls | vs Grep Calls |\n")
		fmt.Printf("|--------|---------------|---------------|\n")
		fmt.Printf("| Grep Noise (adit metric)        | %+.3f         | %+.3f         |\n", pearson(st, an), pearson(sg, an))
		fmt.Printf("| Grep Noise (cross-file calc)    | %+.3f         | %+.3f         |\n", pearson(st, gn), pearson(sg, gn))
	}
}

func getFileImports(repoPath, filePath string, frontends []lang.Frontend) []lang.Import {
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
				return fa.Imports
			}
		}
	}
	return nil
}

func avgF(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func parseGrepDetails(sessionsPath string) []grepCallDetail {
	var details []grepCallDetail
	filepath.Walk(sessionsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

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
				if item.Type == "tool_use" && item.Name == "Grep" {
					var inp grepInput
					if json.Unmarshal(item.Input, &inp) == nil && inp.Pattern != "" {
						details = append(details, grepCallDetail{
							Pattern: inp.Pattern,
							Path:    inp.Path,
						})
					}
				}
			}
		}
		return nil
	})
	return details
}
