package output

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/score"
)

// SARIF output following SARIF v2.1.0 schema for CI integration.
// https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool              sarifTool               `json:"tool"`
	Results           []sarifResult           `json:"results"`
	OriginalURIBaseID map[string]sarifURIBase `json:"originalUriBaseIds,omitempty"`
}

type sarifURIBase struct {
	URI string `json:"uri"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	HelpURI          string          `json:"helpUri,omitempty"`
	DefaultConfig    sarifRuleConfig `json:"defaultConfiguration"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

var sarifRuleRegression = sarifRule{
	ID:               "adit/regression",
	Name:             "MetricRegression",
	ShortDescription: sarifMessage{Text: "Structural metric got worse compared to previous version"},
	DefaultConfig:    sarifRuleConfig{Level: "warning"},
}

var sarifRules = []sarifRule{
	{
		ID:               "adit/file-size",
		Name:             "FileTooLarge",
		ShortDescription: sarifMessage{Text: "File exceeds line count threshold"},
		DefaultConfig:    sarifRuleConfig{Level: "warning"},
	},
	{
		ID:               "adit/context-reads",
		Name:             "TooManyContextReads",
		ShortDescription: sarifMessage{Text: "File requires too many local imports to understand"},
		DefaultConfig:    sarifRuleConfig{Level: "warning"},
	},
	{
		ID:               "adit/unnecessary-reads",
		Name:             "UnnecessaryReads",
		ShortDescription: sarifMessage{Text: "File has single-consumer imports that should be co-located"},
		DefaultConfig:    sarifRuleConfig{Level: "warning"},
	},
	{
		ID:               "adit/grep-noise",
		Name:             "GrepNoise",
		ShortDescription: sarifMessage{Text: "File defines or imports names that collide with other files"},
		DefaultConfig:    sarifRuleConfig{Level: "warning"},
	},
	{
		ID:               "adit/blast-radius",
		Name:             "BlastRadius",
		ShortDescription: sarifMessage{Text: "File is imported by too many other files"},
		DefaultConfig:    sarifRuleConfig{Level: "warning"},
	},
	{
		ID:               "adit/import-cycle",
		Name:             "ImportCycle",
		ShortDescription: sarifMessage{Text: "Circular import dependency detected"},
		DefaultConfig:    sarifRuleConfig{Level: "error"},
	},
	{
		ID:               "adit/ambiguous-name",
		Name:             "AmbiguousName",
		ShortDescription: sarifMessage{Text: "Name defined in multiple files — causes grep noise"},
		DefaultConfig:    sarifRuleConfig{Level: "note"},
	},
	{
		ID:               "adit/relocatable",
		Name:             "RelocatableImport",
		ShortDescription: sarifMessage{Text: "Single-consumer import should be co-located"},
		DefaultConfig:    sarifRuleConfig{Level: "note"},
	},
}

func makeLocation(path string) sarifLocation {
	return makeLocationLine(path, 1)
}

func makeLocationLine(path string, line int) sarifLocation {
	return sarifLocation{
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{
				URI:       filepath.ToSlash(path),
				URIBaseID: "%SRCROOT%",
			},
			Region: &sarifRegion{StartLine: line},
		},
	}
}

// WriteSARIF writes the repo score as a SARIF v2.1.0 JSON document.
func WriteSARIF(w io.Writer, result *score.RepoScore, cfg config.Config, version string) error {
	var results []sarifResult

	for _, f := range result.Files {
		t := cfg.ThresholdsForPath(f.Path)
		loc := makeLocation(f.Path)

		if t.MaxFileLines > 0 && f.Lines > t.MaxFileLines {
			results = append(results, sarifResult{
				RuleID:    "adit/file-size",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("%d lines (max %d)", f.Lines, t.MaxFileLines)},
				Locations: []sarifLocation{loc},
			})
		}
		if t.MaxContextReads > 0 && f.ContextReads.Total > t.MaxContextReads {
			results = append(results, sarifResult{
				RuleID:    "adit/context-reads",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("%d context reads (max %d)", f.ContextReads.Total, t.MaxContextReads)},
				Locations: []sarifLocation{loc},
			})
		}
		if t.MaxUnnecessaryReads > 0 && f.ContextReads.Unnecessary > t.MaxUnnecessaryReads {
			results = append(results, sarifResult{
				RuleID:    "adit/unnecessary-reads",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("%d unnecessary reads (max %d)", f.ContextReads.Unnecessary, t.MaxUnnecessaryReads)},
				Locations: []sarifLocation{loc},
			})
		}
		if t.MaxAmbiguityDefs > 0 && f.Ambiguity.GrepNoise > t.MaxAmbiguityDefs {
			results = append(results, sarifResult{
				RuleID:    "adit/grep-noise",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("grep noise %d (max %d)", f.Ambiguity.GrepNoise, t.MaxAmbiguityDefs)},
				Locations: []sarifLocation{loc},
			})
		}
		if t.MaxBlastRadius > 0 && f.BlastRadius.ImportedByCount > t.MaxBlastRadius {
			results = append(results, sarifResult{
				RuleID:    "adit/blast-radius",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("blast radius %d (max %d)", f.BlastRadius.ImportedByCount, t.MaxBlastRadius)},
				Locations: []sarifLocation{loc},
			})
		}
	}

	for _, cy := range result.Summary.Cycles {
		if cfg.Thresholds.MaxCycleLength == 0 {
			results = append(results, sarifResult{
				RuleID:    "adit/import-cycle",
				Level:     "error",
				Message:   sarifMessage{Text: fmt.Sprintf("import cycle: %s (%d files)", cy.Recommendation, cy.Length)},
				Locations: []sarifLocation{makeLocation(cy.Files[0])},
			})
		}
	}

	// Per-site annotations for ambiguous names (line-level)
	for _, a := range result.Summary.AmbiguousNames {
		for _, site := range a.Sites {
			results = append(results, sarifResult{
				RuleID:    "adit/ambiguous-name",
				Level:     "note",
				Message:   sarifMessage{Text: fmt.Sprintf("%s defined in %d files — grep will return false positives", a.Name, a.Count)},
				Locations: []sarifLocation{makeLocationLine(site.File, site.Line)},
			})
		}
	}

	// Per-import annotations for relocatable imports (line-level)
	for _, r := range result.Summary.Relocatable {
		results = append(results, sarifResult{
			RuleID:    "adit/relocatable",
			Level:     "note",
			Message:   sarifMessage{Text: fmt.Sprintf("%s (%s) only used in %s — co-locate it", r.Name, r.Kind, r.To)},
			Locations: []sarifLocation{makeLocationLine(r.From, r.FromLine)},
		})
	}

	if results == nil {
		results = []sarifResult{}
	}

	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "adit",
					Version:        version,
					InformationURI: "https://github.com/justinstimatze/adit-code",
					Rules:          sarifRules,
				},
			},
			Results: results,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

// WriteDiffSARIF writes diff results as SARIF, including regressions and threshold violations.
func WriteDiffSARIF(w io.Writer, diffResult *score.DiffResult, cfg config.Config, version string) error {
	var results []sarifResult

	rules := append([]sarifRule{}, sarifRules...)
	rules = append(rules, sarifRuleRegression)

	for _, fd := range diffResult.Files {
		for _, r := range fd.Regressions {
			results = append(results, sarifResult{
				RuleID:    "adit/regression",
				Level:     "warning",
				Message:   sarifMessage{Text: fmt.Sprintf("%s %d→%d (+%d)", r.Metric, r.Before, r.After, r.Delta)},
				Locations: []sarifLocation{makeLocation(fd.Path)},
			})
		}
		if fd.After != nil {
			t := cfg.ThresholdsForPath(fd.Path)
			loc := makeLocation(fd.Path)
			if t.MaxFileLines > 0 && fd.After.Lines > t.MaxFileLines {
				results = append(results, sarifResult{
					RuleID:    "adit/file-size",
					Level:     "warning",
					Message:   sarifMessage{Text: fmt.Sprintf("%d lines (max %d)", fd.After.Lines, t.MaxFileLines)},
					Locations: []sarifLocation{loc},
				})
			}
			if t.MaxContextReads > 0 && fd.After.ContextReads.Total > t.MaxContextReads {
				results = append(results, sarifResult{
					RuleID:    "adit/context-reads",
					Level:     "warning",
					Message:   sarifMessage{Text: fmt.Sprintf("%d context reads (max %d)", fd.After.ContextReads.Total, t.MaxContextReads)},
					Locations: []sarifLocation{loc},
				})
			}
			if t.MaxUnnecessaryReads > 0 && fd.After.ContextReads.Unnecessary > t.MaxUnnecessaryReads {
				results = append(results, sarifResult{
					RuleID:    "adit/unnecessary-reads",
					Level:     "warning",
					Message:   sarifMessage{Text: fmt.Sprintf("%d unnecessary reads (max %d)", fd.After.ContextReads.Unnecessary, t.MaxUnnecessaryReads)},
					Locations: []sarifLocation{loc},
				})
			}
			if t.MaxAmbiguityDefs > 0 && fd.After.Ambiguity.GrepNoise > t.MaxAmbiguityDefs {
				results = append(results, sarifResult{
					RuleID:    "adit/grep-noise",
					Level:     "warning",
					Message:   sarifMessage{Text: fmt.Sprintf("grep noise %d (max %d)", fd.After.Ambiguity.GrepNoise, t.MaxAmbiguityDefs)},
					Locations: []sarifLocation{loc},
				})
			}
			if t.MaxBlastRadius > 0 && fd.After.BlastRadius.ImportedByCount > t.MaxBlastRadius {
				results = append(results, sarifResult{
					RuleID:    "adit/blast-radius",
					Level:     "warning",
					Message:   sarifMessage{Text: fmt.Sprintf("blast radius %d (max %d)", fd.After.BlastRadius.ImportedByCount, t.MaxBlastRadius)},
					Locations: []sarifLocation{loc},
				})
			}
		}
	}

	if results == nil {
		results = []sarifResult{}
	}

	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "adit",
					Version:        version,
					InformationURI: "https://github.com/justinstimatze/adit-code",
					Rules:          rules,
				},
			},
			Results: results,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}
