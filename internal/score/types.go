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

// AmbiguityResult is the per-file grep ambiguity measurement.
type AmbiguityResult struct {
	UniqueNames int `json:"unique_names"`
	TotalNames  int `json:"total_names"`
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
	Recommendation string  `json:"recommendation"`
}

// FileScore is the complete analysis result for a single file.
type FileScore struct {
	Path         string          `json:"path"`
	Lines        int             `json:"lines"`
	SizeGrade    string          `json:"size_grade"`
	ContextReads ContextReads    `json:"context_reads"`
	Ambiguity    AmbiguityResult `json:"ambiguity"`
	BlastRadius  BlastRadius     `json:"blast_radius"`
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
