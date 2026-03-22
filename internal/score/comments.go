package score

import (
	"strings"

	"github.com/justinstimatze/adit-code/internal/lang"
)

// CommentStats describes comment density in a file.
type CommentStats struct {
	CommentLines int     `json:"comment_lines"`
	CodeLines    int     `json:"code_lines"`
	Density      float64 `json:"density"` // comment_lines / total_lines (0.0-1.0)
}

// ComputeCommentStats counts comment vs code lines.
// Uses simple heuristics per language — not tree-sitter, just line scanning.
func ComputeCommentStats(fa *lang.FileAnalysis, src []byte) CommentStats {
	lines := strings.Split(string(src), "\n")
	total := len(lines)
	if total == 0 {
		return CommentStats{}
	}

	commentLines := 0
	blankLines := 0
	inDocstring := false
	inBlockComment := false

	for _, line := range lines {
		stripped := strings.TrimSpace(line)

		// Check multi-line state before blank line check —
		// blank lines inside block comments/docstrings are comment lines.
		if inBlockComment {
			commentLines++
			if strings.Contains(stripped, "*/") {
				inBlockComment = false
			}
			continue
		}
		if inDocstring {
			commentLines++
			if stripped == "" {
				continue
			}
			if strings.Contains(stripped, `"""`) || strings.Contains(stripped, `'''`) {
				inDocstring = false
			}
			continue
		}

		if stripped == "" {
			blankLines++
			continue
		}

		// Python docstrings
		if strings.HasPrefix(stripped, `"""`) || strings.HasPrefix(stripped, `'''`) {
			tripleQuote := stripped[:3]
			rest := stripped[3:]
			if strings.Contains(rest, tripleQuote) {
				commentLines++
				continue
			}
			inDocstring = true
			commentLines++
			continue
		}

		// Single-line comments: Python #, Go //, TypeScript //
		if strings.HasPrefix(stripped, "#") ||
			strings.HasPrefix(stripped, "//") {
			commentLines++
			continue
		}

		// Block comment start: /* ... */
		if strings.HasPrefix(stripped, "/*") {
			commentLines++
			if !strings.Contains(stripped, "*/") {
				inBlockComment = true
			}
			continue
		}
	}

	codeLines := total - commentLines - blankLines

	density := 0.0
	if total > 0 {
		density = float64(commentLines) / float64(total)
	}

	return CommentStats{
		CommentLines: commentLines,
		CodeLines:    codeLines,
		Density:      density,
	}
}
