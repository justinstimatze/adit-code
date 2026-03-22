package score

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/diff"
	"github.com/justinstimatze/adit-code/internal/lang"
	aditversion "github.com/justinstimatze/adit-code/internal/version"
)

// Pipeline orchestrates the two-pass analysis.
type Pipeline struct {
	frontends []lang.Frontend
	cfg       config.Config
}

// NewPipeline creates a pipeline with the given frontends and config.
func NewPipeline(frontends []lang.Frontend, cfg config.Config) *Pipeline {
	return &Pipeline{frontends: frontends, cfg: cfg}
}

// repoContext holds cross-file indexes computed during scoring,
// reusable by ScoreRepoDiff to avoid re-parsing.
type repoContext struct {
	analyses  map[string]*lang.FileAnalysis
	nameIndex nameIndex
	localCtx  localImportContext
	consumers importConsumerMap
	ncIdx     nameConsumerIndex
}

// ScoreRepo scores all files under the given paths.
func (p *Pipeline) ScoreRepo(paths []string) (*RepoScore, error) {
	result, _, err := p.scoreRepoInternal(paths)
	return result, err
}

func (p *Pipeline) scoreRepoInternal(paths []string) (*RepoScore, *repoContext, error) {
	// Collect all source files
	files, err := p.collectFiles(paths)
	if err != nil {
		return nil, nil, err
	}

	// Pass 1: Parse all files in parallel, detect generated code
	type parseResult struct {
		path    string
		fa      *lang.FileAnalysis
		src     []byte
		headers []string
	}

	numWorkers := runtime.NumCPU()
	jobs := make(chan string, len(files))
	resultsCh := make(chan parseResult, len(files))

	// Start worker pool — each worker gets its own frontend instances
	// (tree-sitter parsers are not thread-safe)
	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			// Each worker creates its own frontends
			workerFrontends := p.createFrontends()
			for path := range jobs {
				src, err := os.ReadFile(path) //nolint:gosec
				if err != nil {
					continue
				}
				fa, err := analyzeBytesWithFrontends(path, src, workerFrontends)
				if err != nil {
					continue
				}
				headers := strings.SplitN(string(src), "\n", 6)
				if len(headers) > 5 {
					headers = headers[:5]
				}
				resultsCh <- parseResult{path, fa, src, headers}
			}
		})
	}

	// Send all files to workers
	for _, path := range files {
		jobs <- path
	}
	close(jobs)

	// Wait for workers to finish, then close results
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	analyses := make(map[string]*lang.FileAnalysis)
	fileSources := make(map[string][]byte)
	fileHeaders := make(map[string][]string)
	for r := range resultsCh {
		analyses[r.path] = r.fa
		fileSources[r.path] = r.src
		fileHeaders[r.path] = r.headers
	}

	// Filter out generated files
	projectFiles := make(map[string]bool)
	for path := range analyses {
		projectFiles[path] = true
	}
	genDetector := newGeneratedDetector(projectFiles)
	for path := range analyses {
		if genDetector.IsGenerated(path, fileHeaders[path]) {
			delete(analyses, path)
			delete(projectFiles, path)
		}
	}

	// Build cross-file indexes
	nIdx := buildNameIndex(analyses)
	localCtx := buildLocalImportContext(projectFiles)
	consumers := buildImportConsumerMap(analyses, localCtx)
	ncIdx := buildNameConsumerIndex(consumers)
	impGraph := buildImportGraph(analyses, localCtx)

	// Pass 2: Score each file
	var fileScores []FileScore
	for _, path := range files {
		fa, ok := analyses[path]
		if !ok {
			continue
		}

		// Use cached source for comment analysis
		src := fileSources[path]

		fs := FileScore{
			Path:            path,
			Lines:           fa.Lines,
			SizeGrade:       SizeGrade(fa.Lines),
			MaxNestingDepth: fa.MaxNestingDepth,
			NodeDiversity:   fa.NodeDiversity,
			MaxParams:       ComputeMaxParams(fa),
			Functions:       ComputeFunctionStats(fa),
			ContextReads:    ComputeContextReads(fa, consumers, localCtx),
			Ambiguity:       ComputeAmbiguity(fa, nIdx),
			Comments:        ComputeCommentStats(fa, src),
			Graph:           ComputeGraphMetrics(path, impGraph),
			BlastRadius:     ComputeBlastRadius(path, analyses, ncIdx),
		}
		fileScores = append(fileScores, fs)
	}

	// Build summary
	ambiguousNames := CollectAmbiguousNames(nIdx, 2)
	sort.Slice(ambiguousNames, func(i, j int) bool {
		return ambiguousNames[i].Count > ambiguousNames[j].Count
	})

	cycles := DetectCycles(analyses, localCtx)

	var allRelocatable []RelocatableImport
	var highBlast []FileScore
	for _, fs := range fileScores {
		allRelocatable = append(allRelocatable, fs.ContextReads.Relocatable...)
		if fs.BlastRadius.ImportedByCount >= p.cfg.Thresholds.MaxBlastRadius {
			highBlast = append(highBlast, fs)
		}
	}

	ctx := &repoContext{
		analyses:  analyses,
		nameIndex: nIdx,
		localCtx:  localCtx,
		consumers: consumers,
		ncIdx:     ncIdx,
	}

	return &RepoScore{
		Version:      aditversion.Version,
		Schema:       1,
		FilesScanned: len(fileScores),
		Files:        fileScores,
		Summary: RepoSummary{
			Relocatable:    allRelocatable,
			AmbiguousNames: ambiguousNames,
			Cycles:         cycles,
			HighBlast:      highBlast,
		},
	}, ctx, nil
}

// ScoreRepoDiff scores changed files between a git ref and HEAD, reporting regressions.
func (p *Pipeline) ScoreRepoDiff(paths []string, ref string) (*DiffResult, error) {
	// 1. Score all files at HEAD and get cross-file indexes
	headResult, rctx, err := p.scoreRepoInternal(paths)
	if err != nil {
		return nil, err
	}

	// Build lookup by path
	headScores := make(map[string]FileScore)
	for _, f := range headResult.Files {
		headScores[f.Path] = f
	}

	// 2. Find repo root and changed files
	startPath := "."
	if len(paths) > 0 {
		startPath = paths[0]
	}
	repoRoot, err := diff.RepoRoot(startPath)
	if err != nil {
		return nil, err
	}

	changedFiles, err := diff.ChangedFiles(repoRoot, ref)
	if err != nil {
		return nil, err
	}

	// Reuse indexes from HEAD scoring
	nIdx := rctx.nameIndex
	localCtx := rctx.localCtx
	consumers := rctx.consumers
	analyses := rctx.analyses
	ncIdx := rctx.ncIdx

	// 4. For each changed file, compute before/after scores
	var fileDiffs []FileDiff
	totalRegressions := 0

	for _, relPath := range changedFiles {
		absPath := filepath.Join(repoRoot, relPath)

		// Find the HEAD score
		afterScore, hasAfter := headScores[absPath]
		if !hasAfter {
			// Try relative path match
			for path, score := range headScores {
				if strings.HasSuffix(path, relPath) {
					afterScore = score
					hasAfter = true
					break
				}
			}
		}

		// Read content at REF
		refContent, err := diff.FileAtRef(repoRoot, ref, relPath)
		var beforeScore *FileScore

		if err == nil {
			// Parse and score at REF using HEAD's cross-file context
			fa, parseErr := p.analyzeBytes(absPath, refContent)
			if parseErr == nil {
				bs := FileScore{
					Path:            absPath,
					Lines:           fa.Lines,
					SizeGrade:       SizeGrade(fa.Lines),
					MaxNestingDepth: fa.MaxNestingDepth,
					NodeDiversity:   fa.NodeDiversity,
					MaxParams:       ComputeMaxParams(fa),
					Functions:       ComputeFunctionStats(fa),
					ContextReads:    ComputeContextReads(fa, consumers, localCtx),
					Ambiguity:       ComputeAmbiguity(fa, nIdx),
					BlastRadius:     ComputeBlastRadius(absPath, analyses, ncIdx),
				}
				beforeScore = &bs
			}
		}

		// Determine status and compute regressions
		status := "modified"
		if beforeScore == nil && hasAfter {
			status = "added"
		} else if beforeScore != nil && !hasAfter {
			status = "deleted"
		}

		var regressions []Regression
		if beforeScore != nil && hasAfter {
			regressions = computeRegressions(beforeScore, &afterScore)
		}
		totalRegressions += len(regressions)

		fd := FileDiff{
			Path:        relPath,
			Status:      status,
			Before:      beforeScore,
			Regressions: regressions,
		}
		if hasAfter {
			a := afterScore
			fd.After = &a
		}
		fileDiffs = append(fileDiffs, fd)
	}

	return &DiffResult{
		Version:      aditversion.Version,
		Schema:       1,
		Ref:          ref,
		FilesChanged: len(fileDiffs),
		Regressions:  totalRegressions,
		Files:        fileDiffs,
	}, nil
}

func computeRegressions(before, after *FileScore) []Regression {
	var regressions []Regression
	check := func(metric string, b, a int) {
		if a > b {
			regressions = append(regressions, Regression{
				Metric: metric,
				Before: b,
				After:  a,
				Delta:  a - b,
			})
		}
	}
	check("lines", before.Lines, after.Lines)
	check("context_reads", before.ContextReads.Total, after.ContextReads.Total)
	check("unnecessary_reads", before.ContextReads.Unnecessary, after.ContextReads.Unnecessary)
	check("grep_noise", before.Ambiguity.GrepNoise, after.Ambiguity.GrepNoise)
	check("blast_radius", before.BlastRadius.ImportedByCount, after.BlastRadius.ImportedByCount)
	return regressions
}

// ScoreFile scores a single file (still requires scanning siblings for cross-file metrics).
func (p *Pipeline) ScoreFile(path string) (*FileScore, error) {
	// To compute cross-file metrics, we need the directory context
	dir := filepath.Dir(path)
	repo, err := p.ScoreRepo([]string{dir})
	if err != nil {
		return nil, err
	}

	for _, fs := range repo.Files {
		if fs.Path == path {
			return &fs, nil
		}
	}

	return nil, os.ErrNotExist
}

func (p *Pipeline) collectFiles(paths []string) ([]string, error) {
	extMap := make(map[string]bool)
	for _, fe := range p.frontends {
		for _, ext := range fe.Extensions() {
			extMap[ext] = true
		}
	}

	var files []string
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			ext := filepath.Ext(root)
			if extMap[ext] {
				files = append(files, root)
			}
			continue
		}

		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
			if err != nil {
				return err
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if p.shouldExcludeDir(base) {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if !extMap[ext] {
				return nil
			}
			if p.shouldExcludeFile(path) {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

func (p *Pipeline) shouldExcludeDir(name string) bool {
	excluded := map[string]bool{
		"__pycache__": true, "node_modules": true, "vendor": true,
		".venv": true, ".git": true, ".tox": true, ".mypy_cache": true,
		"testdata": false, "fixtures": false, // only excluded via glob patterns
	}
	return excluded[name]
}

func (p *Pipeline) shouldExcludeFile(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range p.cfg.Scan.Exclude {
		// Simple glob matching
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Also try matching against full path for ** patterns
		if strings.Contains(pattern, "/") {
			if matched, _ := filepath.Match(pattern, path); matched {
				return true
			}
		}
	}
	return false
}

func (p *Pipeline) createFrontends() []lang.Frontend {
	result := make([]lang.Frontend, len(p.frontends))
	for i, fe := range p.frontends {
		// Create new instances of the same types
		switch fe.(type) {
		case *lang.PythonFrontend:
			result[i] = lang.NewPythonFrontend()
		case *lang.TypeScriptFrontend:
			result[i] = lang.NewTypeScriptFrontend()
		case *lang.GoFrontend:
			result[i] = lang.NewGoFrontend()
		default:
			result[i] = fe // fallback: share (unsafe but better than panic)
		}
	}
	return result
}

func analyzeBytesWithFrontends(path string, src []byte, frontends []lang.Frontend) (*lang.FileAnalysis, error) {
	ext := filepath.Ext(path)
	for _, fe := range frontends {
		if slices.Contains(fe.Extensions(), ext) {
			return fe.Analyze(path, src)
		}
	}
	return nil, os.ErrNotExist
}

func (p *Pipeline) analyzeBytes(path string, src []byte) (*lang.FileAnalysis, error) {
	ext := filepath.Ext(path)
	for _, fe := range p.frontends {
		if slices.Contains(fe.Extensions(), ext) {
			return fe.Analyze(path, src)
		}
	}
	return nil, os.ErrNotExist
}
