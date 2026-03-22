package lang

// Definition is a named symbol defined in a file.
type Definition struct {
	Name          string // Short name: "_validate"
	QualifiedName string // Class-qualified: "MyClass._validate"
	Kind          string // "function" | "method" | "class" | "constant" | "type"
	Line          int
	EndLine       int // last line of the definition (0 if unknown)
	ParamCount    int // number of parameters (excluding self/cls/receiver)
}

// Import is a symbol imported from another module.
type Import struct {
	Name         string // Imported symbol name
	SourceModule string // Module path (relative or absolute)
	Kind         string // "constant" | "function" | "type" | "unknown"
	Line         int
}

// FileAnalysis is the result of parsing a single file.
type FileAnalysis struct {
	Path            string
	Lines           int
	MaxNestingDepth int // deepest control flow / scope nesting
	NodeDiversity   int // count of distinct AST node types
	MaxBranching    int // largest branching factor (switch/match cases)
	Definitions     []Definition
	Imports         []Import
}

// Frontend parses source files for a specific language.
type Frontend interface {
	// Extensions returns file extensions this frontend handles (e.g. [".py"]).
	Extensions() []string

	// Analyze parses a source file and extracts definitions and imports.
	Analyze(path string, src []byte) (*FileAnalysis, error)
}
