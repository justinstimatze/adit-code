package score

import (
	"github.com/justinstimatze/adit-code/internal/lang"
)

// CountMacroReferencedDefs counts local definitions that are referenced only
// through a macro invocation's token tree (lang.Definition.MacroReferenceCount
// > 0) rather than through code adit can trace directly. Only Rust frontends
// currently populate the underlying signal; files in other languages always
// score zero. This exists so a definition kept alive by a registration macro
// (e.g. `register!(Handler)`) doesn't read as silently unreferenced to an
// agent grepping for its name.
func CountMacroReferencedDefs(fa *lang.FileAnalysis) int {
	count := 0
	for _, def := range fa.Definitions {
		if def.MacroReferenceCount > 0 {
			count++
		}
	}
	return count
}
