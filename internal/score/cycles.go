package score

import (
	"github.com/justinstimatze/adit-code/internal/lang"
)

// DetectCycles finds import cycles using DFS on the import graph.
func DetectCycles(analyses map[string]*lang.FileAnalysis, localCtx localImportContext) []ImportCycle {
	graph := buildImportGraph(analyses, localCtx)

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
				cycleStart := -1
				for i, s := range stack {
					if s == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cyclePath := make([]string, 0, len(stack)-cycleStart+1)
					cyclePath = append(cyclePath, stack[cycleStart:]...)
					cyclePath = append(cyclePath, neighbor)

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
	if len(files) <= 3 {
		return "extract shared interface or merge — these files are too coupled to be separate"
	}
	return "break the cycle by extracting shared definitions into a separate module"
}
