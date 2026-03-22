package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all adit configuration.
type Config struct {
	Thresholds ThresholdConfig `toml:"thresholds"`
	Scan       ScanConfig      `toml:"scan"`
	Context    ContextConfig   `toml:"context"`
}

// ThresholdConfig holds enforcement thresholds.
type ThresholdConfig struct {
	MaxFileLines       int `toml:"max_file_lines"`
	MaxContextReads    int `toml:"max_context_reads"`
	MaxUnnecessaryReads int `toml:"max_unnecessary_reads"`
	MaxAmbiguityDefs   int `toml:"max_ambiguity_defs"`
	MaxBlastRadius     int `toml:"max_blast_radius"`
	MaxCycleLength     int `toml:"max_cycle_length"`
}

// ScanConfig controls what to scan.
type ScanConfig struct {
	Languages []string `toml:"languages"`
	Exclude   []string `toml:"exclude"`
}

// ContextConfig holds informational context window settings.
type ContextConfig struct {
	WindowTokens    int     `toml:"window_tokens"`
	UsableFraction  float64 `toml:"usable_fraction"`
}

// pyprojectFile wraps the [tool.adit] section of pyproject.toml.
type pyprojectFile struct {
	Tool struct {
		Adit Config `toml:"adit"`
	} `toml:"tool"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Thresholds: ThresholdConfig{
			MaxFileLines:       3000,
			MaxContextReads:    10,
			MaxUnnecessaryReads: 3,
			MaxAmbiguityDefs:   3,
			MaxBlastRadius:     20,
			MaxCycleLength:     0,
		},
		Scan: ScanConfig{
			Languages: []string{"python", "typescript"},
			Exclude: []string{
				"*_test.go", "test_*.py", "*_test.py",
				"*.test.ts", "*.spec.ts",
				"**/__pycache__/**", "**/node_modules/**",
				"**/vendor/**", "**/.venv/**",
				"**/testdata/**", "**/fixtures/**",
			},
		},
		Context: ContextConfig{
			WindowTokens:   200000,
			UsableFraction: 0.75,
		},
	}
}

// Load finds and loads configuration, walking up from startDir.
// Checks adit.toml first, then pyproject.toml [tool.adit].
// Falls back to defaults for any missing values.
func Load(startDir string) (Config, error) {
	cfg := Default()

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return cfg, err
	}

	for {
		// Try adit.toml first
		aditPath := filepath.Join(dir, "adit.toml")
		if _, err := os.Stat(aditPath); err == nil {
			if _, err := toml.DecodeFile(aditPath, &cfg); err != nil {
				return cfg, err
			}
			return cfg, nil
		}

		// Try pyproject.toml [tool.adit]
		pyprojectPath := filepath.Join(dir, "pyproject.toml")
		if _, err := os.Stat(pyprojectPath); err == nil {
			var pf pyprojectFile
			if _, err := toml.DecodeFile(pyprojectPath, &pf); err == nil {
				// Only use if [tool.adit] was present (check for non-zero values)
				if pf.Tool.Adit.Thresholds.MaxFileLines > 0 || len(pf.Tool.Adit.Scan.Languages) > 0 {
					cfg = pf.Tool.Adit
					return cfg, nil
				}
			}
		}

		// Walk up
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return cfg, nil
}

// LoadFile loads configuration from a specific file path.
func LoadFile(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
