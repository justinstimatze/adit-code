package score_test

import (
	"testing"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	"github.com/justinstimatze/adit-code/internal/score"
)

func newAllLangPipeline() *score.Pipeline {
	cfg := config.Default()
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
		lang.NewGoFrontend(),
	}
	return score.NewPipeline(frontends, cfg)
}

// BenchmarkScoreRepo_Fixtures benchmarks scoring the testdata fixtures (~42 files).
func BenchmarkScoreRepo_Fixtures(b *testing.B) {
	pipeline := newTestPipeline()
	// Warm up tree-sitter grammar compilation
	_, _ = pipeline.ScoreRepo([]string{"../../testdata/python"})

	b.ResetTimer()
	for b.Loop() {
		_, _ = pipeline.ScoreRepo([]string{"../../testdata"})
	}
}

// BenchmarkScoreRepo_Self benchmarks scoring adit's own source (~35 files, mixed Go/Python).
func BenchmarkScoreRepo_Self(b *testing.B) {
	pipeline := newAllLangPipeline()
	// Warm up
	_, _ = pipeline.ScoreRepo([]string{"../../cmd", "../../internal"})

	b.ResetTimer()
	for b.Loop() {
		_, _ = pipeline.ScoreRepo([]string{"../../cmd", "../../internal"})
	}
}

// BenchmarkScoreFile benchmarks scoring a single file (includes directory context).
func BenchmarkScoreFile(b *testing.B) {
	pipeline := newTestPipeline()
	// Warm up
	_, _ = pipeline.ScoreFile("../../testdata/python/handlers.py")

	b.ResetTimer()
	for b.Loop() {
		_, _ = pipeline.ScoreFile("../../testdata/python/handlers.py")
	}
}

// BenchmarkParse_Only benchmarks just the parsing phase (no cross-file scoring).
func BenchmarkParse_Only(b *testing.B) {
	fe := lang.NewPythonFrontend()
	src := make([]byte, 0, 10000)
	for i := range 200 {
		src = append(src, []byte("def func_"+string(rune('a'+i%26))+"(x, y):\n    return x + y\n\n")...)
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = fe.Analyze("bench.py", src)
	}
}
