package lang

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// nestingNodeKinds are AST node types that increase nesting depth.
// Covers Python, TypeScript/JS, and Go control flow and scope constructs.
var nestingNodeKinds = map[string]bool{
	// Control flow (shared across languages)
	"if_statement":    true,
	"for_statement":   true,
	"while_statement": true,
	"try_statement":   true,
	"with_statement":  true,

	// Python-specific
	"match_statement": true,
	"case_clause":     true,
	"elif_clause":     true,
	"except_clause":   true,

	// TypeScript/JS-specific
	"for_in_statement": true,
	"do_statement":     true,
	"switch_statement": true,
	"switch_case":      true,
	"catch_clause":     true,

	// Go-specific
	"select_statement":      true,
	"type_switch_statement": true,
	"communication_case":    true,
	"expression_case":       true,
	"default_case":          true,

	// Scope-introducing (all languages)
	"function_definition":  true,
	"method_definition":    true,
	"class_definition":     true,
	"function_declaration": true,
	"method_declaration":   true,
	"class_declaration":    true,
	"arrow_function":       true,
	"func_literal":         true,
}

// ComputeMaxNestingDepth walks the AST to find the maximum nesting depth.
func ComputeMaxNestingDepth(root *tree_sitter.Node) int {
	maxDepth := 0
	walkNesting(root, 0, &maxDepth)
	return maxDepth
}

func walkNesting(node *tree_sitter.Node, depth int, maxDepth *int) {
	if nestingNodeKinds[node.Kind()] {
		depth++
		if depth > *maxDepth {
			*maxDepth = depth
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkNesting(node.NamedChild(i), depth, maxDepth)
	}
}
