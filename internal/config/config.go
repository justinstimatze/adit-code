package config

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// Config holds all adit configuration.
type Config struct {
	Thresholds ThresholdConfig            `toml:"thresholds"`
	Scan       ScanConfig                 `toml:"scan"`
	Context    ContextConfig              `toml:"context"`
	PerPath    map[string]ThresholdConfig `toml:"per-path"`
}

// ThresholdConfig holds enforcement thresholds.
// Zero values mean "not enforced" (except MaxCycleLength where 0 = no cycles allowed).
type ThresholdConfig struct {
	MaxFileLines        int `toml:"max_file_lines"`
	MaxContextReads     int `toml:"max_context_reads"`
	MaxUnnecessaryReads int `toml:"max_unnecessary_reads"`
	MaxAmbiguityDefs    int `toml:"max_ambiguity_defs"`
	MaxBlastRadius      int `toml:"max_blast_radius"`
	MaxCycleLength      int `toml:"max_cycle_length"`
	MaxNestingDepth     int `toml:"max_nesting_depth"` // 0 = not enforced
	MaxParams           int `toml:"max_params"`        // 0 = not enforced
}

// ScanConfig controls what to scan.
type ScanConfig struct {
	Exclude []string `toml:"exclude"`
}

// ContextConfig holds informational context window settings.
type ContextConfig struct {
	WindowTokens   int     `toml:"window_tokens"`
	UsableFraction float64 `toml:"usable_fraction"`
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
			MaxFileLines:        3000,
			MaxContextReads:     10,
			MaxUnnecessaryReads: 3,
			MaxAmbiguityDefs:    3,
			MaxBlastRadius:      20,
			MaxCycleLength:      0,
		},
		Scan: ScanConfig{
			Exclude: []string{
				"*_test.go", "test_*.py", "*_test.py",
				"*.test.ts", "*.spec.ts",
				"**/__pycache__/**", "**/node_modules/**",
				"**/vendor/**", "**/.venv/**",
				"**/testdata/**", "**/fixtures/**",
			},
		},
		Context: ContextConfig{
			WindowTokens:   1000000,
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
				if pf.Tool.Adit.Thresholds.MaxFileLines > 0 || len(pf.Tool.Adit.Scan.Exclude) > 0 {
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

// ThresholdsForPath returns the thresholds that apply to a given file path.
// If a per-path override matches, its non-zero values override the base thresholds.
// Patterns are checked in sorted order for deterministic behavior.
func (c Config) ThresholdsForPath(filePath string) ThresholdConfig {
	base := c.Thresholds
	if len(c.PerPath) == 0 {
		return base
	}

	// Sort patterns for deterministic matching when multiple patterns match
	patterns := make([]string, 0, len(c.PerPath))
	for pattern := range c.PerPath {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)

	for _, pattern := range patterns {
		override := c.PerPath[pattern]
		matched, _ := filepath.Match(pattern, filepath.Base(filePath))
		if !matched {
			matched, _ = filepath.Match(pattern, filePath)
		}
		if matched {
			return mergeThresholds(base, override)
		}
	}
	return base
}

func mergeThresholds(base, override ThresholdConfig) ThresholdConfig {
	if override.MaxFileLines > 0 {
		base.MaxFileLines = override.MaxFileLines
	}
	if override.MaxContextReads > 0 {
		base.MaxContextReads = override.MaxContextReads
	}
	if override.MaxUnnecessaryReads > 0 {
		base.MaxUnnecessaryReads = override.MaxUnnecessaryReads
	}
	if override.MaxAmbiguityDefs > 0 {
		base.MaxAmbiguityDefs = override.MaxAmbiguityDefs
	}
	if override.MaxBlastRadius > 0 {
		base.MaxBlastRadius = override.MaxBlastRadius
	}
	// MaxCycleLength: 0 means "no cycles allowed", so we can't use 0 as "not set"
	// Only override if the override has a positive value (allowing cycles)
	if override.MaxCycleLength > 0 {
		base.MaxCycleLength = override.MaxCycleLength
	}
	if override.MaxNestingDepth > 0 {
		base.MaxNestingDepth = override.MaxNestingDepth
	}
	if override.MaxParams > 0 {
		base.MaxParams = override.MaxParams
	}
	return base
}

// LoadFile loads configuration from a specific file path.
func LoadFile(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
