package score

import (
	"path/filepath"
	"strings"

	"github.com/justinstimatze/adit-code/internal/lang"
)

// GraphMetrics describes a file's position in the dependency graph.
type GraphMetrics struct {
	TransitiveDepth int `json:"transitive_depth"` // longest import chain from this file
	TransitiveCount int `json:"transitive_count"` // unique files reachable via imports
}

// importGraph maps file -> set of files it imports from (direct dependencies).
type importGraph map[string]map[string]bool

// moduleFileIndex maps module name suffixes to file paths for O(1) resolution.
type moduleFileIndex map[string][]string

func buildModuleFileIndex(analyses map[string]*lang.FileAnalysis) moduleFileIndex {
	idx := make(moduleFileIndex)
	for path := range analyses {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		idx[base] = append(idx[base], path)

		// Also index by directory name
		dir := filepath.Base(filepath.Dir(path))
		if dir != "." && dir != "/" {
			idx[dir] = append(idx[dir], path)
		}
	}
	return idx
}

// buildImportGraph builds a directed graph of file-level import relationships.
func buildImportGraph(analyses map[string]*lang.FileAnalysis, localCtx localImportContext) importGraph {
	graph := make(importGraph)
	mfIdx := buildModuleFileIndex(analyses)

	for path, fa := range analyses {
		deps := make(map[string]bool)
		for _, imp := range fa.Imports {
			if !isLocalImport(imp.SourceModule, localCtx) {
				continue
			}
			// Resolve module to file paths via index
			modName := imp.SourceModule
			for len(modName) > 0 && modName[0] == '.' {
				modName = modName[1:]
			}
			if modName == "" {
				continue
			}
			for _, candidate := range mfIdx[modName] {
				if candidate != path {
					deps[candidate] = true
				}
			}
		}
		if len(deps) > 0 {
			graph[path] = deps
		}
	}

	return graph
}

// ComputeGraphMetrics computes transitive dependency metrics for a file.
func ComputeGraphMetrics(path string, graph importGraph) GraphMetrics {
	visited := make(map[string]bool)
	maxDepth := 0

	var walk func(node string, depth int)
	walk = func(node string, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		for dep := range graph[node] {
			if !visited[dep] {
				visited[dep] = true
				walk(dep, depth+1)
			}
		}
	}

	visited[path] = true
	walk(path, 0)

	return GraphMetrics{
		TransitiveDepth: maxDepth,
		TransitiveCount: len(visited) - 1,
	}
}
