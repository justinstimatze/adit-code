package score

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/adit-code/internal/lang"
)

// importKey uniquely identifies an imported name from a module.
type importKey struct {
	Module string
	Name   string
}

// importConsumerMap tracks which files import each (module, name) pair.
type importConsumerMap map[importKey]map[string]bool

// localImportContext holds precomputed data for determining if an import is project-local.
type localImportContext struct {
	projectFiles map[string]bool
	goModulePath string          // e.g. "github.com/user/project" from go.mod, empty if not Go
	dirSuffixes  map[string]bool // set of all directory path suffixes for O(1) module matching
}

// buildLocalImportContext creates the context for local import detection.
func buildLocalImportContext(projectFiles map[string]bool) localImportContext {
	ctx := localImportContext{
		projectFiles: projectFiles,
		dirSuffixes:  buildDirSuffixIndex(projectFiles),
	}

	for filePath := range projectFiles {
		dir := filepath.Dir(filePath)
		if !filepath.IsAbs(dir) {
			dir, _ = filepath.Abs(dir)
		}
		for dir != "/" && dir != "." {
			goModPath := filepath.Join(dir, "go.mod")
			if mod := readGoModulePath(goModPath); mod != "" {
				ctx.goModulePath = mod
				return ctx
			}
			dir = filepath.Dir(dir)
		}
		break
	}

	return ctx
}

// readGoModulePath reads the module path from a go.mod file.
func readGoModulePath(path string) string {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// buildDirSuffixIndex creates a set of all directory path suffixes for O(1) module matching.
func buildDirSuffixIndex(projectFiles map[string]bool) map[string]bool {
	idx := make(map[string]bool)
	for filePath := range projectFiles {
		base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		idx[base] = true

		dir := filepath.Dir(filePath)
		parts := strings.Split(filepath.ToSlash(dir), "/")
		for n := 1; n <= len(parts); n++ {
			suffix := strings.Join(parts[len(parts)-n:], "/")
			if suffix != "" && suffix != "." && suffix != "/" {
				idx[suffix] = true
			}
		}
	}
	return idx
}

// buildImportConsumerMap builds a map of (module, name) → set of consumer files.
func buildImportConsumerMap(analyses map[string]*lang.FileAnalysis, ctx localImportContext) importConsumerMap {
	m := make(importConsumerMap)
	for path, fa := range analyses {
		for _, imp := range fa.Imports {
			if !isLocalImport(imp.SourceModule, ctx) {
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

// isLocalImport returns true if the module path refers to a project-local file.
func isLocalImport(module string, ctx localImportContext) bool {
	if strings.HasPrefix(module, ".") {
		return true
	}
	if ctx.goModulePath != "" {
		return strings.HasPrefix(module, ctx.goModulePath+"/") || module == ctx.goModulePath
	}
	return ctx.dirSuffixes[module]
}

// isReExportFile returns true if this file exists to re-export names as a public API surface.
func isReExportFile(path string) bool {
	base := filepath.Base(path)
	return base == "__init__.py" ||
		base == "index.ts" || base == "index.tsx" ||
		base == "index.js" || base == "index.jsx"
}

// ComputeContextReads computes the context reads metric for a file.
func ComputeContextReads(fa *lang.FileAnalysis, consumers importConsumerMap, ctx localImportContext) ContextReads {
	localModules := make(map[string]bool)
	var relocatable []RelocatableImport

	isReExport := isReExportFile(fa.Path)

	for _, imp := range fa.Imports {
		if !isLocalImport(imp.SourceModule, ctx) {
			continue
		}
		localModules[imp.SourceModule] = true

		if isReExport {
			continue
		}

		key := importKey{Module: imp.SourceModule, Name: imp.Name}
		consumerSet := consumers[key]
		if len(consumerSet) == 1 {
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
