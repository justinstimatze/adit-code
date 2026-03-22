package lang

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// branchKinds maps parent node types to the child node types that count as branches.
var branchKinds = map[string]map[string]bool{
	"if_statement":          {"elif_clause": true, "else_clause": true},
	"match_statement":       {"case_clause": true},
	"switch_statement":      {"switch_case": true, "switch_default": true, "expression_case": true, "default_case": true},
	"select_statement":      {"communication_case": true, "default_case": true},
	"type_switch_statement": {"type_case": true, "default_case": true},
}

// ComputeMaxBranching finds the maximum branching factor of any decision
// point in the file. A 40-case switch/match has branching factor 40.
func ComputeMaxBranching(root *tree_sitter.Node) int {
	maxBranch := 0
	walkBranching(root, &maxBranch)
	return maxBranch
}

func walkBranching(node *tree_sitter.Node, maxBranch *int) {
	childKinds, isBranchNode := branchKinds[node.Kind()]
	if isBranchNode {
		branches := countBranches(node, childKinds)
		if node.Kind() == "if_statement" {
			branches++ // the if clause itself is a branch
		}
		if branches > *maxBranch {
			*maxBranch = branches
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkBranching(node.NamedChild(i), maxBranch)
	}
}

func countBranches(node *tree_sitter.Node, kinds map[string]bool) int {
	count := 0
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if kinds[child.Kind()] {
			count++
		}
		// Some languages nest cases inside a body node
		if child.Kind() == "switch_body" || child.Kind() == "block" {
			for j := uint(0); j < child.NamedChildCount(); j++ {
				if kinds[child.NamedChild(j).Kind()] {
					count++
				}
			}
		}
	}
	return count
}
