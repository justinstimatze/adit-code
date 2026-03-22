package score

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justindotpub/adit-code/internal/config"
	"github.com/justindotpub/adit-code/internal/lang"
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

// ScoreRepo scores all files under the given paths.
func (p *Pipeline) ScoreRepo(paths []string) (*RepoScore, error) {
	// Collect all source files
	files, err := p.collectFiles(paths)
	if err != nil {
		return nil, err
	}

	// Pass 1: Parse all files
	analyses := make(map[string]*lang.FileAnalysis)
	for _, path := range files {
		fa, err := p.analyzeFile(path)
		if err != nil {
			continue // Skip unparseable files
		}
		analyses[path] = fa
	}

	// Build cross-file indexes
	nIdx := buildNameIndex(analyses)
	consumers := buildImportConsumerMap(analyses)
	definitionFiles := make(map[string]bool)
	for path := range analyses {
		definitionFiles[path] = true
	}

	// Pass 2: Score each file
	var fileScores []FileScore
	for _, path := range files {
		fa, ok := analyses[path]
		if !ok {
			continue
		}

		fs := FileScore{
			Path:         path,
			Lines:        fa.Lines,
			SizeGrade:    SizeGrade(fa.Lines),
			ContextReads: ComputeContextReads(fa, consumers, definitionFiles),
			Ambiguity:    ComputeAmbiguity(fa, nIdx),
			BlastRadius:  ComputeBlastRadius(path, analyses, consumers),
		}
		fileScores = append(fileScores, fs)
	}

	// Build summary
	ambiguousNames := CollectAmbiguousNames(nIdx, 2)
	sort.Slice(ambiguousNames, func(i, j int) bool {
		return ambiguousNames[i].Count > ambiguousNames[j].Count
	})

	cycles := DetectCycles(analyses)

	var allRelocatable []RelocatableImport
	var highBlast []FileScore
	for _, fs := range fileScores {
		allRelocatable = append(allRelocatable, fs.ContextReads.Relocatable...)
		if fs.BlastRadius.ImportedByCount >= p.cfg.Thresholds.MaxBlastRadius {
			highBlast = append(highBlast, fs)
		}
	}

	return &RepoScore{
		Version:      "0.1.0",
		Schema:       1,
		FilesScanned: len(fileScores),
		Files:        fileScores,
		Summary: RepoSummary{
			Relocatable:    allRelocatable,
			AmbiguousNames: ambiguousNames,
			Cycles:         cycles,
			HighBlast:      highBlast,
		},
	}, nil
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

		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
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

func (p *Pipeline) analyzeFile(path string) (*lang.FileAnalysis, error) {
	ext := filepath.Ext(path)
	for _, fe := range p.frontends {
		for _, feExt := range fe.Extensions() {
			if ext == feExt {
				src, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				return fe.Analyze(path, src)
			}
		}
	}
	return nil, os.ErrNotExist
}
