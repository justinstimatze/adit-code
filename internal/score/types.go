package score

// RelocatableImport is a single-consumer import that should be co-located
// with its only consumer file.
type RelocatableImport struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`      // "constant" | "function" | "type" | "unknown"
	From     string `json:"from"`      // source file path
	FromLine int    `json:"from_line"` // line number in source file
	To       string `json:"to"`        // destination (consumer) file path
	Reason   string `json:"reason"`    // e.g. "single consumer"
}

// ContextReads measures how many files the AI must read to understand this file.
type ContextReads struct {
	Total       int                 `json:"total"`
	Unnecessary int                 `json:"unnecessary"`
	Relocatable []RelocatableImport `json:"relocatable,omitempty"`
}

// DefinitionSite is one location where a name is defined.
type DefinitionSite struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Qualified string `json:"qualified_name"`
}

// AmbiguousName is a name defined in multiple files.
type AmbiguousName struct {
	Name  string           `json:"name"`
	Count int              `json:"count"`
	Sites []DefinitionSite `json:"sites"`
}

// AmbiguityResult is the per-file grep noise measurement.
// Grep noise = sum of (definition_count - 1) for each name this file defines
// that is also defined in other files. Measures how much search noise this
// file's definitions create across the codebase.
type AmbiguityResult struct {
	GrepNoise int `json:"grep_noise"`
}

// ExportedName tracks how many files consume a particular exported name.
type ExportedName struct {
	Name      string `json:"name"`
	Consumers int    `json:"consumers"`
}

// BlastRadius measures how many files could break if this file is edited.
type BlastRadius struct {
	ImportedByCount int            `json:"imported_by_count"`
	ImportedBy      []string       `json:"imported_by,omitempty"`
	MostExported    []ExportedName `json:"most_exported,omitempty"`
}

// ImportCycle is a circular dependency chain between files.
type ImportCycle struct {
	Files          []string `json:"files"`
	Length         int      `json:"length"`
	Recommendation string   `json:"recommendation"`
}

// FunctionStats describes the distribution of function/method lengths in a file.
type FunctionStats struct {
	Count     int `json:"count"`
	MaxLength int `json:"max_length"` // longest function in lines
	AvgLength int `json:"avg_length"` // average function length in lines
}

// FileScore is the complete analysis result for a single file.
type FileScore struct {
	Path            string          `json:"path"`
	Lines           int             `json:"lines"`
	SizeGrade       string          `json:"size_grade"`
	MaxNestingDepth int             `json:"max_nesting_depth"`
	NodeDiversity   int             `json:"node_diversity"`
	MaxParams       int             `json:"max_params"`
	Functions       FunctionStats   `json:"functions"`
	ContextReads    ContextReads    `json:"context_reads"`
	Ambiguity       AmbiguityResult `json:"ambiguity"`
	Comments        CommentStats    `json:"comments"`
	Graph           GraphMetrics    `json:"graph"`
	BlastRadius     BlastRadius     `json:"blast_radius"`
}

// RepoSummary aggregates cross-file findings.
type RepoSummary struct {
	Relocatable    []RelocatableImport `json:"relocatable,omitempty"`
	AmbiguousNames []AmbiguousName     `json:"ambiguous_names,omitempty"`
	Cycles         []ImportCycle       `json:"cycles,omitempty"`
	HighBlast      []FileScore         `json:"high_blast_radius,omitempty"`
}

// RepoScore is the top-level output of adit score.
type RepoScore struct {
	Version      string      `json:"version"`
	Schema       int         `json:"schema"`
	FilesScanned int         `json:"files_scanned"`
	Files        []FileScore `json:"files"`
	Summary      RepoSummary `json:"summary"`
}

// Regression is a metric that got worse between two versions.
type Regression struct {
	Metric string `json:"metric"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Delta  int    `json:"delta"` // positive = worse
}

// FileDiff compares a file's metrics between a ref and HEAD.
type FileDiff struct {
	Path        string       `json:"path"`
	Status      string       `json:"status"` // "modified" | "added" | "deleted"
	Before      *FileScore   `json:"before,omitempty"`
	After       *FileScore   `json:"after,omitempty"`
	Regressions []Regression `json:"regressions,omitempty"`
}

// DiffResult is the output of adit score --diff.
type DiffResult struct {
	Version      string     `json:"version"`
	Schema       int        `json:"schema"`
	Ref          string     `json:"ref"`
	FilesChanged int        `json:"files_changed"`
	Regressions  int        `json:"regressions_count"`
	Files        []FileDiff `json:"files"`
}
