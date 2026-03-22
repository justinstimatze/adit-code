package score

import (
	"sort"

	"github.com/justindotpub/adit-code/internal/lang"
)

// ComputeBlastRadius computes how many files import from this file.
func ComputeBlastRadius(path string, analyses map[string]*lang.FileAnalysis, consumers importConsumerMap) BlastRadius {
	// Find all files that import from this file's module path.
	// We need to figure out what module name(s) correspond to this file.
	// For now, we track which names defined in this file are imported by others.

	importedBy := make(map[string]bool)
	nameConsumers := make(map[string]int)

	fa := analyses[path]
	if fa == nil {
		return BlastRadius{}
	}

	// For each name defined in this file, check if other files import it
	for _, def := range fa.Definitions {
		// Check all consumer map entries for this name
		for key, consumerSet := range consumers {
			if key.Name == def.Name {
				for consumer := range consumerSet {
					if consumer != path {
						importedBy[consumer] = true
						nameConsumers[def.Name]++
					}
				}
			}
		}
	}

	// Build imported-by list
	var importedByList []string
	for f := range importedBy {
		importedByList = append(importedByList, f)
	}
	sort.Strings(importedByList)

	// Build most-exported list
	var mostExported []ExportedName
	for name, count := range nameConsumers {
		mostExported = append(mostExported, ExportedName{Name: name, Consumers: count})
	}
	sort.Slice(mostExported, func(i, j int) bool {
		return mostExported[i].Consumers > mostExported[j].Consumers
	})
	// Keep top 5
	if len(mostExported) > 5 {
		mostExported = mostExported[:5]
	}

	return BlastRadius{
		ImportedByCount: len(importedBy),
		ImportedBy:      importedByList,
		MostExported:    mostExported,
	}
}
