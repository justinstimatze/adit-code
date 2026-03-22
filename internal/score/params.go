package score

import "github.com/justinstimatze/adit-code/internal/lang"

// ComputeMaxParams returns the maximum parameter count of any function/method
// in the file. High parameter counts indicate complex interfaces that are
// harder for AI agents to modify safely.
func ComputeMaxParams(fa *lang.FileAnalysis) int {
	max := 0
	for _, def := range fa.Definitions {
		if def.ParamCount > max {
			max = def.ParamCount
		}
	}
	return max
}
