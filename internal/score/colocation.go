package score

import (
	"strings"

	"github.com/justindotpub/adit-code/internal/lang"
)

// importKey uniquely identifies an imported name from a module.
type importKey struct {
	Module string
	Name   string
}

// importConsumerMap tracks which files import each (module, name) pair.
type importConsumerMap map[importKey]map[string]bool

// buildImportConsumerMap builds a map of (module, name) → set of consumer files.
// Only includes imports that look like project-local imports.
func buildImportConsumerMap(analyses map[string]*lang.FileAnalysis) importConsumerMap {
	m := make(importConsumerMap)
	for path, fa := range analyses {
		for _, imp := range fa.Imports {
			if !isLocalImport(imp.SourceModule) {
				continue
			}
			key := importKey{Module: imp.SourceModule, Name: imp.Name}
			if m[key] == nil {
				m[key] = make(map[string]bool)
			}
			m[key][path] = true
		}
	}
	return m
}

// isLocalImport returns true if the module path looks like a project-local import.
// Heuristic: relative imports (starting with .) are always local.
// Absolute imports that look like well-known stdlib/third-party are excluded.
func isLocalImport(module string) bool {
	if strings.HasPrefix(module, ".") {
		return true
	}
	// For TypeScript: paths starting with ./ or ../ are local
	if strings.HasPrefix(module, "./") || strings.HasPrefix(module, "../") {
		return true
	}
	// For now, treat bare module names as potentially local
	// (the pipeline will filter based on whether the module maps to a project file)
	return true
}

// ComputeContextReads computes the context reads metric for a file.
func ComputeContextReads(fa *lang.FileAnalysis, consumers importConsumerMap, definitionFiles map[string]bool) ContextReads {
	// Count unique local modules this file imports from
	localModules := make(map[string]bool)
	var relocatable []RelocatableImport

	for _, imp := range fa.Imports {
		if !isLocalImport(imp.SourceModule) {
			continue
		}

		localModules[imp.SourceModule] = true

		// Check if this is a single-consumer import
		key := importKey{Module: imp.SourceModule, Name: imp.Name}
		consumerSet := consumers[key]
		if len(consumerSet) == 1 {
			// Only this file imports this name — it should be co-located here
			relocatable = append(relocatable, RelocatableImport{
				Name:     imp.Name,
				Kind:     imp.Kind,
				From:     imp.SourceModule,
				FromLine: imp.Line,
				To:       fa.Path,
				Reason:   "single consumer",
			})
		}
	}

	return ContextReads{
		Total:       len(localModules),
		Unnecessary: len(relocatable),
		Relocatable: relocatable,
	}
}
