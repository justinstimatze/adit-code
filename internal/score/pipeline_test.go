package score_test

import (
	"testing"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	"github.com/justinstimatze/adit-code/internal/score"
)

func newTestPipeline() *score.Pipeline {
	cfg := config.Default()
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
	}
	return score.NewPipeline(frontends, cfg)
}

// --- Size Grade ---

func TestSizeGrades(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		grade string
	}{
		{"tiny (50 lines) = A", "../../testdata/benchmark/size/tiny.py", "A"},
		{"small (400 lines) = A", "../../testdata/benchmark/size/small.py", "A"},
		{"medium (800 lines) = B", "../../testdata/benchmark/size/medium.py", "B"},
		{"large (1600 lines) = C", "../../testdata/benchmark/size/large.py", "C"},
		{"xlarge (3200 lines) = D", "../../testdata/benchmark/size/xlarge.py", "D"},
		{"huge (5500 lines) = F", "../../testdata/benchmark/size/huge.py", "F"},
	}

	pipeline := newTestPipeline()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pipeline.ScoreRepo([]string{tt.file})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Files) != 1 {
				t.Fatalf("expected 1 file, got %d", len(result.Files))
			}
			got := result.Files[0].SizeGrade
			if got != tt.grade {
				t.Errorf("expected grade %s, got %s (lines: %d)", tt.grade, got, result.Files[0].Lines)
			}
		})
	}
}

// --- Co-location / Context Reads ---

func TestColocation(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/colocation"})
	if err != nil {
		t.Fatal(err)
	}

	fileMap := make(map[string]score.FileScore)
	for _, f := range result.Files {
		fileMap[f.Path] = f
	}

	t.Run("self_contained has 0 context reads", func(t *testing.T) {
		f := findFile(t, fileMap, "self_contained.py")
		if f.ContextReads.Total != 0 {
			t.Errorf("expected 0 context reads, got %d", f.ContextReads.Total)
		}
		if f.ContextReads.Unnecessary != 0 {
			t.Errorf("expected 0 unnecessary reads, got %d", f.ContextReads.Unnecessary)
		}
	})

	t.Run("consumer_a imports from 1 module", func(t *testing.T) {
		f := findFile(t, fileMap, "consumer_a.py")
		if f.ContextReads.Total != 1 {
			t.Errorf("expected 1 context read (shared_constants), got %d", f.ContextReads.Total)
		}
	})

	t.Run("SECRET_SAUCE is relocatable to consumer_a", func(t *testing.T) {
		found := false
		for _, r := range result.Summary.Relocatable {
			if r.Name == "SECRET_SAUCE" {
				found = true
				if !containsSubstring(r.To, "consumer_a.py") {
					t.Errorf("SECRET_SAUCE should relocate to consumer_a.py, got %s", r.To)
				}
			}
		}
		if !found {
			t.Error("SECRET_SAUCE should be flagged as relocatable (single consumer)")
		}
	})

	t.Run("LONELY_FLAG is relocatable to consumer_b", func(t *testing.T) {
		found := false
		for _, r := range result.Summary.Relocatable {
			if r.Name == "LONELY_FLAG" {
				found = true
				if !containsSubstring(r.To, "consumer_b.py") {
					t.Errorf("LONELY_FLAG should relocate to consumer_b.py, got %s", r.To)
				}
			}
		}
		if !found {
			t.Error("LONELY_FLAG should be flagged as relocatable (single consumer)")
		}
	})

	t.Run("MAX_RETRIES is NOT relocatable (used by both consumers)", func(t *testing.T) {
		for _, r := range result.Summary.Relocatable {
			if r.Name == "MAX_RETRIES" {
				t.Error("MAX_RETRIES should NOT be relocatable — it has 2 consumers")
			}
		}
	})

	t.Run("TIMEOUT is NOT relocatable (used by both consumers)", func(t *testing.T) {
		for _, r := range result.Summary.Relocatable {
			if r.Name == "TIMEOUT" {
				t.Error("TIMEOUT should NOT be relocatable — it has 2 consumers")
			}
		}
	})
}

// --- Grep Ambiguity ---

func TestAmbiguity(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/ambiguity"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("process is ambiguous (3 files)", func(t *testing.T) {
		found := false
		for _, a := range result.Summary.AmbiguousNames {
			if a.Name == "process" {
				found = true
				if a.Count != 3 {
					t.Errorf("expected process in 3 files, got %d", a.Count)
				}
			}
		}
		if !found {
			t.Error("process should be flagged as ambiguous (defined in 3 files)")
		}
	})

	t.Run("validate is ambiguous (2 files)", func(t *testing.T) {
		found := false
		for _, a := range result.Summary.AmbiguousNames {
			if a.Name == "validate" {
				found = true
				if a.Count != 2 {
					t.Errorf("expected validate in 2 files, got %d", a.Count)
				}
			}
		}
		if !found {
			t.Error("validate should be flagged as ambiguous (defined in 2 files)")
		}
	})

	t.Run("unique_to_a is NOT ambiguous", func(t *testing.T) {
		for _, a := range result.Summary.AmbiguousNames {
			if a.Name == "unique_to_a" {
				t.Error("unique_to_a should NOT be ambiguous — defined in 1 file")
			}
		}
	})

	t.Run("totally_unique_name is NOT ambiguous", func(t *testing.T) {
		for _, a := range result.Summary.AmbiguousNames {
			if a.Name == "totally_unique_name" {
				t.Error("totally_unique_name should NOT be ambiguous")
			}
		}
	})

	t.Run("per-file grep noise is correct", func(t *testing.T) {
		fileMap := make(map[string]score.FileScore)
		for _, f := range result.Files {
			fileMap[f.Path] = f
		}

		// module_a: process (in 3 files = +2 noise), validate (in 2 files = +1 noise) = 3 grep noise
		a := findFile(t, fileMap, "module_a.py")
		if a.Ambiguity.GrepNoise != 3 {
			t.Errorf("module_a: expected grep noise 3 (process:2 + validate:1), got %d", a.Ambiguity.GrepNoise)
		}

		// module_c: process (in 3 files = +2 noise), totally_unique_name (unique = 0) = 2 grep noise
		c := findFile(t, fileMap, "module_c.py")
		if c.Ambiguity.GrepNoise != 2 {
			t.Errorf("module_c: expected grep noise 2 (process:2), got %d", c.Ambiguity.GrepNoise)
		}
	})
}

// --- Blast Radius ---

func TestBlastRadius(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/blastradius"})
	if err != nil {
		t.Fatal(err)
	}

	fileMap := make(map[string]score.FileScore)
	for _, f := range result.Files {
		fileMap[f.Path] = f
	}

	t.Run("hub has blast radius of 4 (imported by 4 spokes)", func(t *testing.T) {
		hub := findFile(t, fileMap, "hub.py")
		if hub.BlastRadius.ImportedByCount != 4 {
			t.Errorf("expected hub blast radius 4, got %d", hub.BlastRadius.ImportedByCount)
			for _, f := range hub.BlastRadius.ImportedBy {
				t.Logf("  imported by: %s", f)
			}
		}
	})

	t.Run("shared_helper is most exported (3 consumers)", func(t *testing.T) {
		hub := findFile(t, fileMap, "hub.py")
		found := false
		for _, e := range hub.BlastRadius.MostExported {
			if e.Name == "shared_helper" {
				found = true
				if e.Consumers != 3 {
					t.Errorf("expected shared_helper consumers=3, got %d", e.Consumers)
				}
			}
		}
		if !found {
			t.Error("shared_helper should be in most_exported")
			for _, e := range hub.BlastRadius.MostExported {
				t.Logf("  exported: %s (%d consumers)", e.Name, e.Consumers)
			}
		}
	})

	t.Run("spokes have blast radius 0", func(t *testing.T) {
		for _, name := range []string{"spoke_1.py", "spoke_2.py", "spoke_3.py", "spoke_4.py"} {
			spoke := findFile(t, fileMap, name)
			if spoke.BlastRadius.ImportedByCount != 0 {
				t.Errorf("%s: expected blast radius 0, got %d", name, spoke.BlastRadius.ImportedByCount)
			}
		}
	})

	t.Run("rarely_used has 1 consumer", func(t *testing.T) {
		hub := findFile(t, fileMap, "hub.py")
		for _, e := range hub.BlastRadius.MostExported {
			if e.Name == "rarely_used" {
				if e.Consumers != 1 {
					t.Errorf("expected rarely_used consumers=1, got %d", e.Consumers)
				}
				return
			}
		}
		// It might not be in MostExported if the list is truncated, that's OK
	})
}

// --- Import Cycles ---

func TestImportCycles(t *testing.T) {
	pipeline := newTestPipeline()
	result, err := pipeline.ScoreRepo([]string{"../../testdata/benchmark/cycles"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("detects alpha-beta cycle", func(t *testing.T) {
		if len(result.Summary.Cycles) == 0 {
			t.Fatal("expected at least 1 import cycle, got 0")
		}

		found := false
		for _, cy := range result.Summary.Cycles {
			hasAlpha := false
			hasBeta := false
			for _, f := range cy.Files {
				if containsSubstring(f, "alpha.py") {
					hasAlpha = true
				}
				if containsSubstring(f, "beta.py") {
					hasBeta = true
				}
			}
			if hasAlpha && hasBeta {
				found = true
				if cy.Length != 2 {
					t.Errorf("expected cycle length 2, got %d", cy.Length)
				}
			}
		}
		if !found {
			t.Error("expected cycle involving alpha.py and beta.py")
			for _, cy := range result.Summary.Cycles {
				t.Logf("  cycle: %v", cy.Files)
			}
		}
	})

	t.Run("standalone is not in any cycle", func(t *testing.T) {
		for _, cy := range result.Summary.Cycles {
			for _, f := range cy.Files {
				if containsSubstring(f, "standalone.py") {
					t.Error("standalone.py should not be in any cycle")
				}
			}
		}
	})
}

// --- Helpers ---

func findFile(t *testing.T, fileMap map[string]score.FileScore, suffix string) score.FileScore {
	t.Helper()
	for path, f := range fileMap {
		if containsSubstring(path, suffix) {
			return f
		}
	}
	t.Fatalf("file matching %q not found in results. Available: %v", suffix, keys(fileMap))
	return score.FileScore{}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > len(sub) && s[len(s)-len(sub):] == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func keys(m map[string]score.FileScore) []string {
	k := make([]string, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	return k
}
