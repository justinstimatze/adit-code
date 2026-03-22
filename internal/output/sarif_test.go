package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/score"
)

func TestWriteSARIF_Basic(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.MaxFileLines = 100

	result := &score.RepoScore{
		Files: []score.FileScore{
			{
				Path:         "big.py",
				Lines:        500,
				SizeGrade:    "B",
				ContextReads: score.ContextReads{Total: 2},
				Ambiguity:    score.AmbiguityResult{GrepNoise: 0},
				BlastRadius:  score.BlastRadius{ImportedByCount: 1},
			},
			{
				Path:         "small.py",
				Lines:        50,
				SizeGrade:    "A",
				ContextReads: score.ContextReads{Total: 1},
				Ambiguity:    score.AmbiguityResult{GrepNoise: 0},
				BlastRadius:  score.BlastRadius{ImportedByCount: 0},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteSARIF(&buf, result, cfg, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}

	var sarif map[string]any
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if sarif["version"] != "2.1.0" {
		t.Errorf("expected SARIF version 2.1.0, got %v", sarif["version"])
	}

	runs := sarif["runs"].([]any)
	run := runs[0].(map[string]any)
	results := run["results"].([]any)

	// Only big.py exceeds threshold (500 > 100)
	if len(results) != 1 {
		t.Errorf("expected 1 SARIF result (big.py lines violation), got %d", len(results))
		for _, r := range results {
			rm := r.(map[string]any)
			t.Logf("  %s: %s", rm["ruleId"], rm["message"].(map[string]any)["text"])
		}
	}

	if len(results) > 0 {
		r := results[0].(map[string]any)
		if r["ruleId"] != "adit/file-size" {
			t.Errorf("expected rule adit/file-size, got %v", r["ruleId"])
		}
		// Check relative path with uriBaseId
		locs := r["locations"].([]any)
		loc := locs[0].(map[string]any)
		phys := loc["physicalLocation"].(map[string]any)
		art := phys["artifactLocation"].(map[string]any)
		if art["uriBaseId"] != "%SRCROOT%" {
			t.Errorf("expected uriBaseId %%SRCROOT%%, got %v", art["uriBaseId"])
		}
		if art["uri"] != "big.py" {
			t.Errorf("expected relative URI big.py, got %v", art["uri"])
		}
		// Check region
		region := phys["region"].(map[string]any)
		if region["startLine"].(float64) != 1 {
			t.Errorf("expected startLine 1, got %v", region["startLine"])
		}
	}
}

func TestWriteSARIF_NoViolations(t *testing.T) {
	cfg := config.Default()
	result := &score.RepoScore{
		Files: []score.FileScore{
			{Path: "ok.py", Lines: 100, SizeGrade: "A"},
		},
	}

	var buf bytes.Buffer
	err := WriteSARIF(&buf, result, cfg, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}

	var sarif map[string]any
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatal(err)
	}

	runs := sarif["runs"].([]any)
	run := runs[0].(map[string]any)
	results := run["results"].([]any)

	if len(results) != 0 {
		t.Errorf("expected 0 SARIF results, got %d", len(results))
	}
}

func TestWriteSARIF_Cycles(t *testing.T) {
	cfg := config.Default() // MaxCycleLength = 0 means no cycles allowed

	result := &score.RepoScore{
		Files: []score.FileScore{},
		Summary: score.RepoSummary{
			Cycles: []score.ImportCycle{
				{Files: []string{"a.py", "b.py"}, Length: 2, Recommendation: "a.py → b.py → a.py"},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteSARIF(&buf, result, cfg, "0.1.0"); err != nil {
		t.Fatal(err)
	}

	var sarif map[string]any
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatal(err)
	}

	runs := sarif["runs"].([]any)
	run := runs[0].(map[string]any)
	results := run["results"].([]any)

	if len(results) != 1 {
		t.Errorf("expected 1 cycle result, got %d", len(results))
	}
	if len(results) > 0 {
		r := results[0].(map[string]any)
		if r["ruleId"] != "adit/import-cycle" {
			t.Errorf("expected adit/import-cycle, got %v", r["ruleId"])
		}
		if r["level"] != "error" {
			t.Errorf("expected error level for cycles, got %v", r["level"])
		}
	}
}
