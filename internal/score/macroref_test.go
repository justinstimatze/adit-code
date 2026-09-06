package score

import (
	"testing"

	"github.com/justinstimatze/adit-code/internal/lang"
)

func TestCountMacroReferencedDefs(t *testing.T) {
	fa := &lang.FileAnalysis{
		Definitions: []lang.Definition{
			{Name: "Handler", MacroReferenceCount: 1},
			{Name: "Orphan", MacroReferenceCount: 0},
			{Name: "Registered", MacroReferenceCount: 3},
		},
	}
	if got := CountMacroReferencedDefs(fa); got != 2 {
		t.Errorf("CountMacroReferencedDefs() = %d, want 2", got)
	}
}
