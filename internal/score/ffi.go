package score

import "github.com/justinstimatze/adit-code/internal/lang"

// FFIBoundary measures how much a file's structure bridges to foreign
// (non-source-language) code across an extern "C" boundary -- coupling
// invisible to use/mod import tracking. Currently only Rust frontends
// populate the underlying signals (extern "C" fn definitions and extern
// "C" { ... } declaration blocks); files in other languages always score
// zero.
type FFIBoundary struct {
	ExternCCount int      `json:"extern_c_count"`
	ExternCNames []string `json:"extern_c_names,omitempty"`
}

// ComputeFFIBoundary counts extern "C" boundary crossings in a file: both
// directions the Rust frontend records -- extern "C" fn definitions
// (exported to foreign callers) and extern "C" { ... } declarations
// (symbols imported from foreign code) -- surfaced as one signal since
// both mean "this file cannot be understood, or safely edited, from its
// Rust source alone."
func ComputeFFIBoundary(fa *lang.FileAnalysis) FFIBoundary {
	var names []string
	count := 0

	for _, imp := range fa.Imports {
		if imp.Kind == "extern_c" {
			count++
			if len(names) < 5 {
				names = append(names, imp.Name)
			}
		}
	}
	for _, def := range fa.Definitions {
		if def.ForeignABI != "" {
			count++
			if len(names) < 5 {
				names = append(names, def.Name)
			}
		}
	}

	return FFIBoundary{
		ExternCCount: count,
		ExternCNames: names,
	}
}
