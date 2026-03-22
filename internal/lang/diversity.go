package lang

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// ComputeNodeDiversity counts the number of distinct AST node types in the tree.
// Files with more distinct types have more varied structure (complex control flow,
// mixed patterns). Files with fewer types are repetitive (catalogs, registries).
// Validated against SWE-bench: partial r=+0.165 controlling for file size,
// median +0.474 per-repo, positive on 29/29 repos.
func ComputeNodeDiversity(root *tree_sitter.Node) int {
	types := make(map[string]bool)
	walkTypes(root, types)
	return len(types)
}

func walkTypes(node *tree_sitter.Node, types map[string]bool) {
	types[node.Kind()] = true
	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkTypes(node.NamedChild(i), types)
	}
}
