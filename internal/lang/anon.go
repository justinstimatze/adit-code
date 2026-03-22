package lang

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// anonymousFunctionKinds maps tree-sitter node kinds to our Definition kinds
// for anonymous/inline function expressions across languages.
var anonymousFunctionKinds = map[string]string{
	// Python
	"lambda": "lambda",

	// TypeScript / JavaScript
	"arrow_function":      "arrow_function",
	"function_expression": "closure",

	// Go
	"func_literal": "closure",
}

// collectAnonymousFunctions recursively walks the AST to find anonymous
// function nodes and returns them as Definitions with line spans.
// Only collects functions that span more than minLines lines (to skip
// trivial one-liner lambdas).
func collectAnonymousFunctions(root *tree_sitter.Node, src []byte, minLines int) []Definition {
	var defs []Definition
	walkForAnon(root, src, minLines, &defs)
	return defs
}

func walkForAnon(node *tree_sitter.Node, src []byte, minLines int, defs *[]Definition) {
	kind := node.Kind()

	if defKind, ok := anonymousFunctionKinds[kind]; ok {
		startLine := int(node.StartPosition().Row) + 1
		endLine := int(node.EndPosition().Row) + 1
		length := endLine - startLine + 1

		if length >= minLines {
			*defs = append(*defs, Definition{
				Name:          "(anonymous)",
				QualifiedName: "(anonymous)",
				Kind:          defKind,
				Line:          startLine,
				EndLine:       endLine,
			})
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkForAnon(node.NamedChild(i), src, minLines, defs)
	}
}
