package score

import (
	"github.com/justindotpub/adit-code/internal/lang"
)

// DetectCycles finds import cycles using DFS.
func DetectCycles(analyses map[string]*lang.FileAnalysis) []ImportCycle {
	// Build adjacency list: file → set of files it imports from
	graph := make(map[string]map[string]bool)
	for path, fa := range analyses {
		deps := make(map[string]bool)
		for _, imp := range fa.Imports {
			if !isLocalImport(imp.SourceModule) {
				continue
			}
			// Try to resolve module to a file path in analyses
			for otherPath := range analyses {
				if otherPath == path {
					continue
				}
				if moduleMatchesFile(imp.SourceModule, otherPath) {
					deps[otherPath] = true
				}
			}
		}
		if len(deps) > 0 {
			graph[path] = deps
		}
	}

	// DFS cycle detection
	var cycles []ImportCycle
	visited := make(map[string]int) // 0=unvisited, 1=in-progress, 2=done
	var stack []string

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = 1
		stack = append(stack, node)

		for neighbor := range graph[node] {
			switch visited[neighbor] {
			case 0:
				dfs(neighbor)
			case 1:
				// Found a cycle — extract it from stack
				cycleStart := -1
				for i, s := range stack {
					if s == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cyclePath := make([]string, len(stack)-cycleStart)
					copy(cyclePath, stack[cycleStart:])
					cyclePath = append(cyclePath, neighbor) // close the cycle

					cycles = append(cycles, ImportCycle{
						Files:          cyclePath,
						Length:         len(cyclePath) - 1,
						Recommendation: cycleRecommendation(cyclePath),
					})
				}
			}
		}

		stack = stack[:len(stack)-1]
		visited[node] = 2
	}

	for node := range graph {
		if visited[node] == 0 {
			dfs(node)
		}
	}

	return cycles
}

func cycleRecommendation(files []string) string {
	if len(files) <= 3 { // 2-file cycle: A→B→A
		return "extract shared interface or merge — these files are too coupled to be separate"
	}
	return "break the cycle by extracting shared definitions into a separate module"
}

// moduleMatchesFile checks if an import module path could refer to a given file.
// This is a heuristic — it handles relative imports (.foo, ..foo) and bare names.
func moduleMatchesFile(module, filePath string) bool {
	// Strip extension from file path for comparison
	base := filePath
	for _, ext := range []string{".py", ".ts", ".tsx", ".js", ".jsx"} {
		if len(base) > len(ext) && base[len(base)-len(ext):] == ext {
			base = base[:len(base)-len(ext)]
			break
		}
	}

	// Strip leading dots from relative import
	stripped := module
	for len(stripped) > 0 && stripped[0] == '.' {
		stripped = stripped[1:]
	}

	if stripped == "" {
		return false
	}

	// Check if the stripped module name matches the end of the file path
	// e.g., ".constants" matches "testdata/python/constants.py"
	// e.g., "./utils" matches "testdata/typescript/utils.ts"
	if len(base) >= len(stripped) {
		tail := base[len(base)-len(stripped):]
		if tail == stripped {
			// Verify it's at a path boundary
			if len(base) == len(stripped) || base[len(base)-len(stripped)-1] == '/' {
				return true
			}
		}
	}

	return false
}
