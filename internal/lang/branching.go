package lang

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// branchKinds maps parent node types to the child node types that count as branches.
// if_statement/if_expression are handled separately by countIfChainLength: some
// grammars (Python) expose an else-if chain as flat elif_clause/else_clause siblings,
// while others (Go, Rust, TypeScript) nest each further link one level down, so a
// flat child-kind count alone would cap a long chain's branching factor at 1 or 2.
var branchKinds = map[string]map[string]bool{
	"match_statement":       {"case_clause": true},
	"switch_statement":      {"switch_case": true, "switch_default": true, "expression_case": true, "default_case": true},
	"select_statement":      {"communication_case": true, "default_case": true},
	"type_switch_statement": {"type_case": true, "default_case": true},
	"match_expression":      {"match_arm": true},
}

// ComputeMaxBranching finds the maximum branching factor of any decision
// point in the file. A 40-case switch/match has branching factor 40.
func ComputeMaxBranching(root *tree_sitter.Node) int {
	maxBranch := 0
	walkBranching(root, &maxBranch)
	return maxBranch
}

func walkBranching(node *tree_sitter.Node, maxBranch *int) {
	kind := node.Kind()
	if kind == "if_statement" || kind == "if_expression" {
		if branches := countIfChainLength(node); branches > *maxBranch {
			*maxBranch = branches
		}
	} else if childKinds, isBranchNode := branchKinds[kind]; isBranchNode {
		if branches := countBranches(node, childKinds); branches > *maxBranch {
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
		if child.Kind() == "switch_body" || child.Kind() == "block" || child.Kind() == "match_block" {
			for j := uint(0); j < child.NamedChildCount(); j++ {
				if kinds[child.NamedChild(j).Kind()] {
					count++
				}
			}
		}
	}
	return count
}

// countElseClauseLength counts the branches contributed by a single else_clause
// wrapper node: it recurses into a nested if (the chain continues) or counts 1
// for a terminal else body.
func countElseClauseLength(clause *tree_sitter.Node) int {
	if clause.NamedChildCount() == 0 {
		return 1
	}
	if inner := clause.NamedChild(0); inner.Kind() == "if_statement" || inner.Kind() == "if_expression" {
		return countIfChainLength(inner)
	}
	return 1
}

// countIfChainLength returns the total branching factor of an if/else-if/else
// chain rooted at node (an if_statement or if_expression), counting the initial
// if plus every subsequent link. It handles three grammar shapes:
//   - flat siblings (Python): elif_clause/else_clause appear as direct children
//     of the outermost if_statement, each already a complete arm.
//   - clause-wrapped nesting (Rust, TypeScript): the next link sits inside a
//     single-child else_clause, which holds either the next if node (chain
//     continues) or a plain body (terminal else).
//   - bare nesting (Go): the next if_statement is itself the last named child,
//     with no wrapper, when there is no separate terminal else in between.
func countIfChainLength(node *tree_sitter.Node) int {
	branches := 1
	n := node.NamedChildCount()
	sawFlatClause := false
	for i := uint(0); i < n; i++ {
		switch child := node.NamedChild(i); child.Kind() {
		case "elif_clause":
			branches++
			sawFlatClause = true
		case "else_clause":
			branches += countElseClauseLength(child)
			sawFlatClause = true
		}
	}
	if sawFlatClause {
		return branches
	}

	// No flat elif_clause/else_clause siblings: check the tail position for a
	// bare nested if (Go's else-if) or a terminal else block with no wrapper.
	if n >= 3 {
		switch tail := node.NamedChild(n - 1); tail.Kind() {
		case "if_statement", "if_expression":
			branches += countIfChainLength(tail)
		default:
			branches++ // terminal else with no condition
		}
	}
	return branches
}
