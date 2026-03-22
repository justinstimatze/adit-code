package score

import (
	"path/filepath"
	"strings"

	"github.com/justinstimatze/adit-code/internal/lang"
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
	if name == "(anonymous)" {
		return true
	}
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
// Only indexes names within the same language to avoid false cross-language
// collisions (e.g., UserService in both services.py and services.ts).
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

// langGroup returns a language group key for a file path.
func langGroup(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".py":
		return "python"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".go":
		return "go"
	default:
		return ext
	}
}

// ComputeAmbiguity computes grep noise for a single file given the global name index.
// Grep noise combines two sources:
//   - Definition noise: ambiguous names this file DEFINES (other files also define them)
//   - Reference noise: ambiguous names this file IMPORTS (it will need to disambiguate)
//
// Combined noise correlates at r=+0.521 vs r=+0.430 for definition noise alone
// (validated on Undercity, 8,817 sessions).
func ComputeAmbiguity(fa *lang.FileAnalysis, idx nameIndex) AmbiguityResult {
	grepNoise := 0

	myLang := langGroup(fa.Path)

	// Definition noise: for each name this file defines that is also defined
	// in other files of the same language, add (fileCount - 1).
	for _, def := range fa.Definitions {
		if isExcludedName(def.Name) {
			continue
		}
		entries := idx[def.Name]
		files := make(map[string]bool)
		for _, e := range entries {
			if langGroup(e.File) == myLang {
				files[e.File] = true
			}
		}
		if len(files) > 1 {
			grepNoise += len(files) - 1
		}
	}

	// Reference noise: for each name this file imports that is ambiguous
	// within the same language, add (fileCount - 1).
	seen := make(map[string]bool)
	for _, imp := range fa.Imports {
		if isExcludedName(imp.Name) || seen[imp.Name] {
			continue
		}
		seen[imp.Name] = true
		entries := idx[imp.Name]
		files := make(map[string]bool)
		for _, e := range entries {
			if langGroup(e.File) == myLang {
				files[e.File] = true
			}
		}
		if len(files) > 1 {
			grepNoise += len(files) - 1
		}
	}

	return AmbiguityResult{
		GrepNoise: grepNoise,
	}
}

// CollectAmbiguousNames returns all names defined in 2+ files of the same language.
func CollectAmbiguousNames(idx nameIndex, threshold int) []AmbiguousName {
	var result []AmbiguousName

	for name, entries := range idx {
		// Group by language
		byLang := make(map[string][]nameEntry)
		for _, e := range entries {
			lg := langGroup(e.File)
			byLang[lg] = append(byLang[lg], e)
		}

		for _, langEntries := range byLang {
			fileSet := make(map[string]bool)
			for _, e := range langEntries {
				fileSet[e.File] = true
			}
			if len(fileSet) < threshold {
				continue
			}

			var sites []DefinitionSite
			for _, e := range langEntries {
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
	}

	return result
}
