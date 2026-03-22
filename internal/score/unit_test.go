package score

import (
	"testing"

	"github.com/justinstimatze/adit-code/internal/lang"
)

// --- CommentStats ---

func TestComputeCommentStats_PythonComments(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.py", Lines: 10}
	src := []byte("# comment\nimport os\n\n# another comment\ndef foo():\n    pass\n")
	cs := ComputeCommentStats(fa, src)
	if cs.CommentLines != 2 {
		t.Errorf("expected 2 comment lines, got %d", cs.CommentLines)
	}
}

func TestComputeCommentStats_GoBlockComment(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.go", Lines: 6}
	src := []byte("package main\n\n/* this is\n   a multi-line\n   comment */\nfunc main() {}\n")
	cs := ComputeCommentStats(fa, src)
	if cs.CommentLines != 3 {
		t.Errorf("expected 3 comment lines for block comment, got %d", cs.CommentLines)
	}
}

func TestComputeCommentStats_PythonDocstring(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.py", Lines: 5}
	// Line 1: `"""This is` opens docstring (comment)
	// Line 2: `a docstring."""` closes docstring (comment)
	src := []byte("def foo():\n    \"\"\"This is\n    a docstring.\"\"\"\n    pass\n")
	cs := ComputeCommentStats(fa, src)
	if cs.CommentLines != 2 {
		t.Errorf("expected 2 docstring lines, got %d", cs.CommentLines)
	}
}

func TestComputeCommentStats_SingleLineDocstring(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.py", Lines: 3}
	src := []byte("def foo():\n    \"\"\"Short docstring.\"\"\"\n    pass\n")
	cs := ComputeCommentStats(fa, src)
	if cs.CommentLines != 1 {
		t.Errorf("expected 1 docstring line, got %d", cs.CommentLines)
	}
}

func TestComputeCommentStats_EmptyFile(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.py"}
	cs := ComputeCommentStats(fa, []byte(""))
	if cs.CommentLines != 0 || cs.CodeLines != 0 {
		t.Errorf("empty file should have 0 comment and code lines, got %d/%d", cs.CommentLines, cs.CodeLines)
	}
}

func TestComputeCommentStats_Density(t *testing.T) {
	fa := &lang.FileAnalysis{Path: "test.py", Lines: 4}
	// 5 lines total (trailing newline = empty line), 2 comments, 2 code, 1 blank
	src := []byte("# comment\ncode\n# comment\ncode\n")
	cs := ComputeCommentStats(fa, src)
	if cs.CommentLines != 2 {
		t.Errorf("expected 2 comment lines, got %d", cs.CommentLines)
	}
	// density = 2/5 = 0.4
	if cs.Density < 0.39 || cs.Density > 0.41 {
		t.Errorf("expected ~0.4 density, got %f", cs.Density)
	}
}

// --- FunctionStats ---

func TestComputeFunctionStats_Basic(t *testing.T) {
	fa := &lang.FileAnalysis{
		Definitions: []lang.Definition{
			{Name: "foo", Kind: "function", Line: 1, EndLine: 10},
			{Name: "bar", Kind: "function", Line: 12, EndLine: 15},
			{Name: "MyClass", Kind: "class", Line: 20, EndLine: 50},
		},
	}
	fs := ComputeFunctionStats(fa)
	if fs.Count != 2 {
		t.Errorf("expected 2 functions, got %d", fs.Count)
	}
	if fs.MaxLength != 10 {
		t.Errorf("expected max length 10, got %d", fs.MaxLength)
	}
	if fs.AvgLength != 7 {
		t.Errorf("expected avg length 7, got %d", fs.AvgLength)
	}
}

func TestComputeFunctionStats_IncludesMethodsAndLambdas(t *testing.T) {
	fa := &lang.FileAnalysis{
		Definitions: []lang.Definition{
			{Name: "m", Kind: "method", Line: 1, EndLine: 5},
			{Name: "", Kind: "lambda", Line: 10, EndLine: 12},
			{Name: "", Kind: "arrow_function", Line: 15, EndLine: 20},
			{Name: "", Kind: "closure", Line: 25, EndLine: 30},
		},
	}
	fs := ComputeFunctionStats(fa)
	if fs.Count != 4 {
		t.Errorf("expected 4 function-like defs, got %d", fs.Count)
	}
}

func TestComputeFunctionStats_NoSpanInfo(t *testing.T) {
	fa := &lang.FileAnalysis{
		Definitions: []lang.Definition{
			{Name: "foo", Kind: "function", Line: 5, EndLine: 0},
		},
	}
	fs := ComputeFunctionStats(fa)
	if fs.Count != 0 {
		t.Errorf("expected 0 (no span info), got count %d", fs.Count)
	}
}

func TestComputeFunctionStats_Empty(t *testing.T) {
	fa := &lang.FileAnalysis{}
	fs := ComputeFunctionStats(fa)
	if fs.Count != 0 || fs.MaxLength != 0 || fs.AvgLength != 0 {
		t.Errorf("expected all zeros, got %+v", fs)
	}
}

// --- MaxParams ---

func TestComputeMaxParams(t *testing.T) {
	fa := &lang.FileAnalysis{
		Definitions: []lang.Definition{
			{Name: "foo", Kind: "function", ParamCount: 3},
			{Name: "bar", Kind: "function", ParamCount: 7},
			{Name: "baz", Kind: "function", ParamCount: 2},
		},
	}
	if got := ComputeMaxParams(fa); got != 7 {
		t.Errorf("expected max params 7, got %d", got)
	}
}

func TestComputeMaxParams_Zero(t *testing.T) {
	fa := &lang.FileAnalysis{}
	if got := ComputeMaxParams(fa); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// --- GraphMetrics ---

func TestComputeGraphMetrics_Linear(t *testing.T) {
	// a -> b -> c
	graph := importGraph{
		"a": {"b": true},
		"b": {"c": true},
	}
	m := ComputeGraphMetrics("a", graph)
	if m.TransitiveDepth != 2 {
		t.Errorf("expected depth 2, got %d", m.TransitiveDepth)
	}
	if m.TransitiveCount != 2 {
		t.Errorf("expected count 2, got %d", m.TransitiveCount)
	}
}

func TestComputeGraphMetrics_Diamond(t *testing.T) {
	// a -> b, a -> c, b -> d, c -> d
	graph := importGraph{
		"a": {"b": true, "c": true},
		"b": {"d": true},
		"c": {"d": true},
	}
	m := ComputeGraphMetrics("a", graph)
	if m.TransitiveDepth != 2 {
		t.Errorf("expected depth 2, got %d", m.TransitiveDepth)
	}
	if m.TransitiveCount != 3 {
		t.Errorf("expected count 3 (b,c,d), got %d", m.TransitiveCount)
	}
}

func TestComputeGraphMetrics_Isolated(t *testing.T) {
	graph := importGraph{}
	m := ComputeGraphMetrics("a", graph)
	if m.TransitiveDepth != 0 || m.TransitiveCount != 0 {
		t.Errorf("expected 0/0, got %d/%d", m.TransitiveDepth, m.TransitiveCount)
	}
}

func TestComputeGraphMetrics_Cycle(t *testing.T) {
	// a -> b -> a (cycle should not infinite loop)
	graph := importGraph{
		"a": {"b": true},
		"b": {"a": true},
	}
	m := ComputeGraphMetrics("a", graph)
	if m.TransitiveCount != 1 {
		t.Errorf("expected count 1 (b only), got %d", m.TransitiveCount)
	}
}

// --- SizeGrade ---

func TestSizeGrade(t *testing.T) {
	tests := []struct {
		lines int
		grade string
	}{
		{0, "A"}, {100, "A"}, {499, "A"},
		{500, "B"}, {1000, "B"}, {1499, "B"},
		{1500, "C"}, {2999, "C"},
		{3000, "D"}, {4999, "D"},
		{5000, "F"}, {10000, "F"},
	}
	for _, tt := range tests {
		if got := SizeGrade(tt.lines); got != tt.grade {
			t.Errorf("SizeGrade(%d) = %s, want %s", tt.lines, got, tt.grade)
		}
	}
}

// --- Generated File Detection ---

func TestGeneratedDetector_FilenamePatterns(t *testing.T) {
	d := &generatedDetector{
		patterns:    make(map[string]bool),
		checkedDirs: make(map[string]bool),
	}

	tests := []struct {
		path string
		want bool
	}{
		{"foo_generated.go", true},
		{"foo.pb.go", true},
		{"foo_pb2.py", true},
		{"foo.min.js", true},
		{"foo.min.css", true},
		{"foo.bundle.js", true},
		{"foo.gen.go", true},
		{"migrations/001_init.py", true},
		{"generated/types.ts", true},
		{"src/handlers.py", false},
		{"main.go", false},
	}
	for _, tt := range tests {
		got := d.IsGenerated(tt.path, nil)
		if got != tt.want {
			t.Errorf("IsGenerated(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestGeneratedDetector_HeaderMarkers(t *testing.T) {
	d := &generatedDetector{
		patterns:    make(map[string]bool),
		checkedDirs: make(map[string]bool),
	}

	tests := []struct {
		lines []string
		want  bool
	}{
		{[]string{"// Code generated by protoc-gen-go. DO NOT EDIT."}, true},
		{[]string{"# Auto-generated by script.py"}, true},
		{[]string{"// @generated"}, true},
		{[]string{"// This file is generated by the build system"}, true},
		{[]string{"// Machine generated, do not edit"}, true},
		{[]string{"// Normal source file"}, false},
		{[]string{"package main"}, false},
	}
	for _, tt := range tests {
		got := d.IsGenerated("normal.go", tt.lines)
		if got != tt.want {
			t.Errorf("IsGenerated with header %q = %v, want %v", tt.lines[0], got, tt.want)
		}
	}
}

func TestGeneratedDetector_GitattributesPatterns(t *testing.T) {
	d := &generatedDetector{
		patterns:    map[string]bool{"*.generated.ts": true, "vendor/*": true},
		checkedDirs: make(map[string]bool),
	}

	if !d.IsGenerated("foo.generated.ts", nil) {
		t.Error("expected .gitattributes pattern to match")
	}
	if d.IsGenerated("foo.ts", nil) {
		t.Error("expected non-matching file to pass")
	}
}

// --- Regressions ---

func TestComputeRegressions_Detects(t *testing.T) {
	before := &FileScore{
		Lines:        100,
		ContextReads: ContextReads{Total: 5, Unnecessary: 1},
		Ambiguity:    AmbiguityResult{GrepNoise: 2},
		BlastRadius:  BlastRadius{ImportedByCount: 3},
	}
	after := &FileScore{
		Lines:        150,
		ContextReads: ContextReads{Total: 8, Unnecessary: 3},
		Ambiguity:    AmbiguityResult{GrepNoise: 5},
		BlastRadius:  BlastRadius{ImportedByCount: 6},
	}
	regs := computeRegressions(before, after)
	if len(regs) != 5 {
		t.Errorf("expected 5 regressions, got %d", len(regs))
		for _, r := range regs {
			t.Logf("  %s: %d→%d (+%d)", r.Metric, r.Before, r.After, r.Delta)
		}
	}
}

func TestComputeRegressions_NoRegression(t *testing.T) {
	score := &FileScore{
		Lines:        100,
		ContextReads: ContextReads{Total: 5},
		Ambiguity:    AmbiguityResult{GrepNoise: 2},
		BlastRadius:  BlastRadius{ImportedByCount: 3},
	}
	regs := computeRegressions(score, score)
	if len(regs) != 0 {
		t.Errorf("expected 0 regressions for identical scores, got %d", len(regs))
	}
}

func TestComputeRegressions_Improvement(t *testing.T) {
	before := &FileScore{Lines: 200, ContextReads: ContextReads{Total: 10}}
	after := &FileScore{Lines: 100, ContextReads: ContextReads{Total: 5}}
	regs := computeRegressions(before, after)
	if len(regs) != 0 {
		t.Errorf("improvements should not be regressions, got %d", len(regs))
	}
}
