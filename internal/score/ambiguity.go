package score

import (
	"strings"

	"github.com/justindotpub/adit-code/internal/lang"
)

// pythonDunders are names dictated by Python convention, excluded from ambiguity scoring.
var pythonDunders = map[string]bool{
	"__init__": true, "__repr__": true, "__str__": true,
	"__eq__": true, "__ne__": true, "__lt__": true, "__le__": true,
	"__gt__": true, "__ge__": true, "__hash__": true, "__bool__": true,
	"__len__": true, "__getitem__": true, "__setitem__": true,
	"__delitem__": true, "__iter__": true, "__next__": true,
	"__contains__": true, "__add__": true, "__sub__": true,
	"__mul__": true, "__enter__": true, "__exit__": true,
	"__call__": true, "__new__": true, "__del__": true,
	"__get__": true, "__set__": true, "__delete__": true,
	"setUp": true, "tearDown": true, "setUpClass": true, "tearDownClass": true,
	"setup": true, "teardown": true,
}

// tsConventional are TypeScript/JS names excluded from ambiguity scoring.
var tsConventional = map[string]bool{
	"constructor": true, "render": true,
	"ngOnInit": true, "ngOnDestroy": true,
	"componentDidMount": true, "componentWillUnmount": true,
	"componentDidUpdate": true,
}

// isExcludedName returns true if the name should be excluded from ambiguity scoring.
func isExcludedName(name string) bool {
	if pythonDunders[name] || tsConventional[name] {
		return true
	}
	// Single-underscore private names shorter than 4 chars
	if strings.HasPrefix(name, "_") && !strings.HasPrefix(name, "__") && len(name) < 4 {
		return true
	}
	return false
}

// nameIndex maps short name → list of (file, definition) pairs across the repo.
type nameIndex map[string][]nameEntry

type nameEntry struct {
	File string
	Def  lang.Definition
}

// buildNameIndex builds a cross-file index of all definition names.
func buildNameIndex(analyses map[string]*lang.FileAnalysis) nameIndex {
	idx := make(nameIndex)
	for path, fa := range analyses {
		for _, def := range fa.Definitions {
			if isExcludedName(def.Name) {
				continue
			}
			idx[def.Name] = append(idx[def.Name], nameEntry{File: path, Def: def})
		}
	}
	return idx
}

// ComputeAmbiguity computes grep ambiguity for a single file given the global name index.
func ComputeAmbiguity(fa *lang.FileAnalysis, idx nameIndex) AmbiguityResult {
	total := 0
	unique := 0

	for _, def := range fa.Definitions {
		if isExcludedName(def.Name) {
			continue
		}
		total++

		entries := idx[def.Name]
		// Count unique files (not just entries, since a name can appear
		// multiple times in the same file as different class methods)
		files := make(map[string]bool)
		for _, e := range entries {
			files[e.File] = true
		}
		if len(files) <= 1 {
			unique++
		}
	}

	return AmbiguityResult{
		UniqueNames: unique,
		TotalNames:  total,
	}
}

// CollectAmbiguousNames returns all names defined in 2+ files.
func CollectAmbiguousNames(idx nameIndex, threshold int) []AmbiguousName {
	var result []AmbiguousName

	for name, entries := range idx {
		// Count unique files
		fileSet := make(map[string]bool)
		for _, e := range entries {
			fileSet[e.File] = true
		}
		if len(fileSet) < threshold {
			continue
		}

		var sites []DefinitionSite
		for _, e := range entries {
			sites = append(sites, DefinitionSite{
				File:      e.File,
				Line:      e.Def.Line,
				Qualified: e.Def.QualifiedName,
			})
		}

		result = append(result, AmbiguousName{
			Name:  name,
			Count: len(fileSet),
			Sites: sites,
		})
	}

	return result
}
