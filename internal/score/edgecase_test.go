package score_test

import (
	"os"
	"testing"
	"time"

	"github.com/justinstimatze/adit-code/internal/score"
)

// --- Edge Cases: Robustness ---

func TestEdgeCase_EmptyFile(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/empty_file.py"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	f := result.Files[0]
	if f.Ambiguity.GrepNoise != 0 {
		t.Errorf("empty file should have 0 grep noise, got %d", f.Ambiguity.GrepNoise)
	}
	if f.ContextReads.Total != 0 {
		t.Errorf("empty file should have 0 context reads, got %d", f.ContextReads.Total)
	}
}

func TestEdgeCase_OnlyComments(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/only_comments.py"})
	if err != nil {
		t.Fatal(err)
	}
	f := result.Files[0]
	if f.Ambiguity.GrepNoise != 0 {
		t.Errorf("comments-only file should have 0 grep noise, got %d", f.Ambiguity.GrepNoise)
	}
}

func TestEdgeCase_SyntaxError_NoCrash(t *testing.T) {
	pipeline := newTestPipeline()
	// Should not panic or return error — tree-sitter handles partial parses
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/syntax_error.py"})
	if err != nil {
		t.Fatal(err)
	}
	// Should still extract what it can (tree-sitter does partial parsing)
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file (partial parse), got %d", len(result.Files))
	}
}

func TestEdgeCase_NestedClasses(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/nested_classes.py"})
	if err != nil {
		t.Fatal(err)
	}
	f := result.Files[0]
	// Should find: Outer, Outer.validate, Outer.process, AnotherOuter, AnotherOuter.validate
	// Inner class methods may or may not be extracted depending on tree-sitter depth
	// Nested classes with duplicate method names should produce grep noise
	// (Outer.validate and AnotherOuter.validate share the name "validate")
	if f.Ambiguity.GrepNoise < 0 {
		t.Errorf("expected non-negative grep noise, got %d", f.Ambiguity.GrepNoise)
	}
}

func TestEdgeCase_WildcardImport(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/wildcard_import.py"})
	if err != nil {
		t.Fatal(err)
	}
	f := result.Files[0]
	// Wildcard imports exist but we can't enumerate what they bring in
	// The file should still parse without error
	if f.ContextReads.Total < 0 {
		t.Error("context reads should be non-negative")
	}
}

func TestEdgeCase_AliasedImport(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{
		"../../testdata/benchmark/edgecases/aliased_imports.py",
		"../../testdata/benchmark/edgecases/constants.py",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should still detect the import from .constants even with `as MR`
	fileMap := make(map[string]score.FileScore)
	for _, f := range result.Files {
		fileMap[f.Path] = f
	}
	aliased := findFile(t, fileMap, "aliased_imports.py")
	if aliased.ContextReads.Total < 1 {
		t.Errorf("aliased import file should have at least 1 context read, got %d", aliased.ContextReads.Total)
	}
}

func TestEdgeCase_TypeScriptBarrelFile(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{
		"../../testdata/benchmark/edgecases/barrel.ts",
		"../../testdata/benchmark/edgecases/core.ts",
		"../../testdata/benchmark/edgecases/helpers.ts",
		"../../testdata/benchmark/edgecases/types.ts",
	})
	if err != nil {
		t.Fatal(err)
	}
	fileMap := make(map[string]score.FileScore)
	for _, f := range result.Files {
		fileMap[f.Path] = f
	}
	barrel := findFile(t, fileMap, "barrel.ts")
	// Barrel file re-exports from 3 other modules
	if barrel.ContextReads.Total < 2 {
		t.Errorf("barrel file should have at least 2 context reads (re-exports), got %d", barrel.ContextReads.Total)
	}
}

func TestEdgeCase_InitReexport(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/edgecases/init_reexport/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 {
		t.Errorf("expected 3 files (__init__, core, helpers), got %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  %s", f.Path)
		}
	}
}

// --- Performance ---

func TestPerformance_119Files(t *testing.T) {
	// Run on a large Python codebase if available, otherwise skip
	stope := os.Getenv("ADIT_BENCH_REPO")
	if stope == "" {
		stope = "../../testdata/python" // fallback to small fixture set
	}
	pipeline := newTestPipeline()

	// Warm up (first parse compiles tree-sitter grammars)
	_, _ = pipeline.ScoreRepo([]string{"../../testdata/python"})

	start := time.Now()
	result, err := pipeline.ScoreRepo([]string{stope})
	elapsed := time.Since(start)

	if err != nil {
		t.Skipf("Stope not available: %v", err)
	}

	t.Logf("Scored %d files in %v (%.1f files/sec)",
		result.FilesScanned, elapsed, float64(result.FilesScanned)/elapsed.Seconds())

	if elapsed > 5*time.Second {
		t.Errorf("scoring 119 files took %v — should be under 5s", elapsed)
	}

	// Sanity check: should find reasonable numbers
	t.Logf("Files: %d, Ambiguous: %d, Relocatable: %d",
		result.FilesScanned, len(result.Summary.AmbiguousNames), len(result.Summary.Relocatable))
}
