package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Thresholds.MaxFileLines != 3000 {
		t.Errorf("expected MaxFileLines=3000, got %d", cfg.Thresholds.MaxFileLines)
	}
	if cfg.Thresholds.MaxCycleLength != 0 {
		t.Errorf("expected MaxCycleLength=0, got %d", cfg.Thresholds.MaxCycleLength)
	}
	if cfg.Context.WindowTokens != 1000000 {
		t.Errorf("expected WindowTokens=1000000, got %d", cfg.Context.WindowTokens)
	}
	if len(cfg.Scan.Exclude) == 0 {
		t.Error("expected non-empty default exclude patterns")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adit.toml")
	err := os.WriteFile(path, []byte(`
[thresholds]
max_file_lines = 5000
max_blast_radius = 30

[scan]
exclude = ["*.test.ts"]
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Thresholds.MaxFileLines != 5000 {
		t.Errorf("expected MaxFileLines=5000, got %d", cfg.Thresholds.MaxFileLines)
	}
	if cfg.Thresholds.MaxBlastRadius != 30 {
		t.Errorf("expected MaxBlastRadius=30, got %d", cfg.Thresholds.MaxBlastRadius)
	}
}

func TestLoad_FindsAditToml(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "src", "pkg")
	if err := os.MkdirAll(sub, 0750); err != nil {
		t.Fatal(err)
	}
	err := os.WriteFile(filepath.Join(dir, "adit.toml"), []byte(`
[thresholds]
max_file_lines = 9999
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Thresholds.MaxFileLines != 9999 {
		t.Errorf("expected config found by walking up, got MaxFileLines=%d", cfg.Thresholds.MaxFileLines)
	}
}

func TestLoad_FallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Thresholds.MaxFileLines != 3000 {
		t.Errorf("expected defaults, got MaxFileLines=%d", cfg.Thresholds.MaxFileLines)
	}
}

func TestLoad_PyprojectToml(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[tool.adit.thresholds]
max_file_lines = 4000

[tool.adit.scan]
exclude = ["*.test.py"]
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Thresholds.MaxFileLines != 4000 {
		t.Errorf("expected pyproject config, got MaxFileLines=%d", cfg.Thresholds.MaxFileLines)
	}
}

func TestThresholdsForPath_NoOverrides(t *testing.T) {
	cfg := Default()
	th := cfg.ThresholdsForPath("src/main.py")
	if th.MaxFileLines != 3000 {
		t.Errorf("expected base threshold, got %d", th.MaxFileLines)
	}
}

func TestThresholdsForPath_MatchesPattern(t *testing.T) {
	cfg := Default()
	cfg.PerPath = map[string]ThresholdConfig{
		"test_*.py": {MaxFileLines: 8000, MaxContextReads: 50},
	}
	th := cfg.ThresholdsForPath("test_handlers.py")
	if th.MaxFileLines != 8000 {
		t.Errorf("expected overridden MaxFileLines=8000, got %d", th.MaxFileLines)
	}
	if th.MaxContextReads != 50 {
		t.Errorf("expected overridden MaxContextReads=50, got %d", th.MaxContextReads)
	}
	// Non-overridden fields should keep base values
	if th.MaxBlastRadius != 20 {
		t.Errorf("expected base MaxBlastRadius=20, got %d", th.MaxBlastRadius)
	}
}

func TestThresholdsForPath_NoMatch(t *testing.T) {
	cfg := Default()
	cfg.PerPath = map[string]ThresholdConfig{
		"test_*.py": {MaxFileLines: 8000},
	}
	th := cfg.ThresholdsForPath("main.py")
	if th.MaxFileLines != 3000 {
		t.Errorf("expected base threshold for non-matching file, got %d", th.MaxFileLines)
	}
}

func TestThresholdsForPath_DeterministicOrder(t *testing.T) {
	cfg := Default()
	cfg.PerPath = map[string]ThresholdConfig{
		"*.py":      {MaxFileLines: 1000},
		"test_*.py": {MaxFileLines: 8000},
	}
	// "*.py" sorts before "test_*.py", so *.py matches first
	th := cfg.ThresholdsForPath("test_foo.py")
	if th.MaxFileLines != 1000 {
		t.Errorf("expected first matching pattern (*.py → 1000), got %d", th.MaxFileLines)
	}
}

func TestMergeThresholds(t *testing.T) {
	base := ThresholdConfig{
		MaxFileLines:        3000,
		MaxContextReads:     10,
		MaxUnnecessaryReads: 3,
		MaxAmbiguityDefs:    3,
		MaxBlastRadius:      20,
		MaxCycleLength:      0,
	}
	override := ThresholdConfig{
		MaxFileLines:    5000,
		MaxContextReads: 25,
	}
	result := mergeThresholds(base, override)
	if result.MaxFileLines != 5000 {
		t.Errorf("expected overridden MaxFileLines=5000, got %d", result.MaxFileLines)
	}
	if result.MaxContextReads != 25 {
		t.Errorf("expected overridden MaxContextReads=25, got %d", result.MaxContextReads)
	}
	if result.MaxUnnecessaryReads != 3 {
		t.Errorf("expected base MaxUnnecessaryReads=3, got %d", result.MaxUnnecessaryReads)
	}
}
