package diff

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoRoot finds the git repository root from a given path.
func RepoRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--show-toplevel") //nolint:gosec // git path is from user input, expected
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ValidateRef checks that a git ref doesn't look like a flag.
func ValidateRef(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid git ref %q: must not start with -", ref)
	}
	return nil
}

// ChangedFiles returns file paths changed between ref and HEAD.
// Paths are relative to the repo root.
// Excludes deleted files (only Added, Copied, Modified, Renamed).
func ChangedFiles(repoRoot, ref string) ([]string, error) {
	if err := ValidateRef(ref); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=ACMR", ref) //nolint:gosec // git args from user input
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// FileAtRef reads a file's content at a specific git ref.
// path should be relative to the repo root.
func FileAtRef(repoRoot, ref, path string) ([]byte, error) {
	if err := ValidateRef(ref); err != nil {
		return nil, err
	}
	spec := fmt.Sprintf("%s:%s", ref, path)
	cmd := exec.Command("git", "-C", repoRoot, "show", spec) //nolint:gosec // git args from user input
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s failed: %w", spec, err)
	}
	return out, nil
}
