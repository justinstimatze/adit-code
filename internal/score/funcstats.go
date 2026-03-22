package score

import "github.com/justinstimatze/adit-code/internal/lang"

// ComputeFunctionStats calculates function/method length distribution for a file.
func ComputeFunctionStats(fa *lang.FileAnalysis) FunctionStats {
	var count, totalLen, maxLen int

	for _, def := range fa.Definitions {
		switch def.Kind {
		case "function", "method", "lambda", "arrow_function", "closure":
		default:
			continue
		}
		if def.EndLine <= def.Line {
			continue // no span info
		}

		length := def.EndLine - def.Line + 1
		count++
		totalLen += length
		if length > maxLen {
			maxLen = length
		}
	}

	avg := 0
	if count > 0 {
		avg = totalLen / count
	}

	return FunctionStats{
		Count:     count,
		MaxLength: maxLen,
		AvgLength: avg,
	}
}
