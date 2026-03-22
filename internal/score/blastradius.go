package score

import (
	"sort"

	"github.com/justinstimatze/adit-code/internal/lang"
)

// nameConsumerIndex maps a name to all its consumer map entries.
// This avoids O(n) scan of the full consumer map per definition.
type nameConsumerIndex map[string][]importKeyConsumers

type importKeyConsumers struct {
	key       importKey
	consumers map[string]bool
}

// buildNameConsumerIndex creates a lookup from name to consumer entries.
func buildNameConsumerIndex(consumers importConsumerMap) nameConsumerIndex {
	idx := make(nameConsumerIndex)
	for key, consumerSet := range consumers {
		idx[key.Name] = append(idx[key.Name], importKeyConsumers{key, consumerSet})
	}
	return idx
}

// ComputeBlastRadius computes how many files import from this file.
func ComputeBlastRadius(path string, analyses map[string]*lang.FileAnalysis, ncIdx nameConsumerIndex) BlastRadius {
	importedBy := make(map[string]bool)
	nameConsumers := make(map[string]int)

	fa := analyses[path]
	if fa == nil {
		return BlastRadius{}
	}

	for _, def := range fa.Definitions {
		consumersForName := make(map[string]bool)
		entries := ncIdx[def.Name]
		for _, entry := range entries {
			for consumer := range entry.consumers {
				if consumer != path {
					importedBy[consumer] = true
					consumersForName[consumer] = true
				}
			}
		}
		nameConsumers[def.Name] = len(consumersForName)
	}

	var importedByList []string
	for f := range importedBy {
		importedByList = append(importedByList, f)
	}
	sort.Strings(importedByList)

	var mostExported []ExportedName
	for name, count := range nameConsumers {
		mostExported = append(mostExported, ExportedName{Name: name, Consumers: count})
	}
	sort.Slice(mostExported, func(i, j int) bool {
		return mostExported[i].Consumers > mostExported[j].Consumers
	})
	if len(mostExported) > 5 {
		mostExported = mostExported[:5]
	}

	return BlastRadius{
		ImportedByCount: len(importedBy),
		ImportedBy:      importedByList,
		MostExported:    mostExported,
	}
}
