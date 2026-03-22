package output

import (
	"testing"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/score"
)

func TestFilterByMinGrade(t *testing.T) {
	files := []score.FileScore{
		{Path: "a.py", SizeGrade: "A", Lines: 100},
		{Path: "b.py", SizeGrade: "B", Lines: 800},
		{Path: "c.py", SizeGrade: "C", Lines: 2000},
		{Path: "d.py", SizeGrade: "D", Lines: 4000},
		{Path: "f.py", SizeGrade: "F", Lines: 6000},
	}

	tests := []struct {
		grade string
		want  int
	}{
		{"A", 5},
		{"B", 4},
		{"C", 3},
		{"D", 2},
		{"F", 1},
		{"X", 5}, // invalid grade returns all
	}

	for _, tt := range tests {
		filtered := FilterByMinGrade(files, tt.grade)
		if len(filtered) != tt.want {
			t.Errorf("FilterByMinGrade(%q): expected %d files, got %d", tt.grade, tt.want, len(filtered))
		}
	}
}

func TestSortFiles(t *testing.T) {
	files := []score.FileScore{
		{Path: "c.py", Lines: 100, BlastRadius: score.BlastRadius{ImportedByCount: 5}},
		{Path: "a.py", Lines: 300, BlastRadius: score.BlastRadius{ImportedByCount: 1}},
		{Path: "b.py", Lines: 200, BlastRadius: score.BlastRadius{ImportedByCount: 10}},
	}

	SortFiles(files, "name")
	if files[0].Path != "a.py" {
		t.Errorf("sort by name: expected a.py first, got %s", files[0].Path)
	}

	SortFiles(files, "size")
	if files[0].Path != "a.py" {
		t.Errorf("sort by size: expected a.py (300) first, got %s (%d)", files[0].Path, files[0].Lines)
	}

	SortFiles(files, "blast")
	if files[0].Path != "b.py" {
		t.Errorf("sort by blast: expected b.py (10) first, got %s", files[0].Path)
	}
}

func TestHasIssues(t *testing.T) {
	clean := &score.RepoScore{Summary: score.RepoSummary{}}
	if HasIssues(clean) {
		t.Error("expected no issues for clean repo")
	}

	withCycles := &score.RepoScore{
		Summary: score.RepoSummary{
			Cycles: []score.ImportCycle{{Files: []string{"a", "b"}, Length: 2}},
		},
	}
	if !HasIssues(withCycles) {
		t.Error("expected issues when cycles present")
	}

	withAmbig := &score.RepoScore{
		Summary: score.RepoSummary{
			AmbiguousNames: []score.AmbiguousName{{Name: "foo", Count: 3}},
		},
	}
	if !HasIssues(withAmbig) {
		t.Error("expected issues when ambiguous names present")
	}
}

func TestCheckFileThresholds(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.MaxFileLines = 500
	cfg.Thresholds.MaxBlastRadius = 5

	f := &score.FileScore{
		Lines:        1000,
		ContextReads: score.ContextReads{Total: 3},
		BlastRadius:  score.BlastRadius{ImportedByCount: 10},
	}

	violations := CheckFileThresholds("big.py", f, cfg)
	if len(violations) != 2 {
		t.Errorf("expected 2 violations (lines + blast), got %d: %v", len(violations), violations)
	}
}

func TestCheckFileThresholds_NoViolations(t *testing.T) {
	cfg := config.Default()
	f := &score.FileScore{
		Lines:        100,
		ContextReads: score.ContextReads{Total: 2},
		BlastRadius:  score.BlastRadius{ImportedByCount: 1},
	}
	violations := CheckFileThresholds("small.py", f, cfg)
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckThresholds_IncludesCycles(t *testing.T) {
	cfg := config.Default()
	result := &score.RepoScore{
		Files: []score.FileScore{{Path: "a.py", Lines: 100}},
		Summary: score.RepoSummary{
			Cycles: []score.ImportCycle{
				{Files: []string{"a.py", "b.py"}, Length: 2},
			},
		},
	}
	violations := CheckThresholds(result, cfg)
	found := false
	for _, v := range violations {
		if len(v) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected cycle violation")
	}
}
