package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/justinstimatze/adit-code/internal/config"
	"github.com/justinstimatze/adit-code/internal/lang"
	"github.com/justinstimatze/adit-code/internal/score"
	aditversion "github.com/justinstimatze/adit-code/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Session state: shared across all tool calls within one MCP session.
// Lazily initialized on first use to avoid allocating tree-sitter
// parsers at package import time.
var (
	sessionCache    *repoCache
	sessionPipeline *score.Pipeline
	sessionConfig   config.Config
	sessionOnce     sync.Once
)

func ensureSession() {
	sessionOnce.Do(func() {
		sessionCache = newRepoCache(30 * time.Second)
		cfg, _ := config.Load(".")
		sessionConfig = cfg
		frontends := []lang.Frontend{
			lang.NewPythonFrontend(),
			lang.NewTypeScriptFrontend(),
			lang.NewGoFrontend(),
		}
		sessionPipeline = score.NewPipeline(frontends, cfg)
	})
}

// cacheKey normalizes paths to absolute, sorted form for consistent cache hits.
func cacheKey(paths []string) string {
	normalized := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			normalized[i] = p
		} else {
			normalized[i] = abs
		}
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\x00")
}

// cachedScoreRepo scores a directory, returning a cached result if available.
func cachedScoreRepo(paths []string) (*score.RepoScore, error) {
	ensureSession()
	key := cacheKey(paths)
	if result, ok := sessionCache.Get(key); ok {
		return result, nil
	}
	result, err := sessionPipeline.ScoreRepo(paths)
	if err != nil {
		return nil, err
	}
	sessionCache.Set(key, result)
	return result, nil
}

// Run starts the MCP server on stdio.
func Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "adit",
		Version: aditversion.Version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_score_repo",
		Description: "Score all files in a directory for AI-navigability metrics: context reads, grep ambiguity, file size, blast radius, import cycles",
	}, handleScoreRepo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_score_file",
		Description: "Score a single file for AI-navigability metrics (analyzes sibling files for cross-file metrics)",
	}, handleScoreFile)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_relocatable",
		Description: "List single-consumer imports that should be co-located with their only consumer to reduce AI read cost",
	}, handleRelocatable)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_ambiguous",
		Description: "List function/method/class names defined in multiple files that will produce noisy grep results",
	}, handleAmbiguous)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_blast_radius",
		Description: "Show how many files import from a given file and which names are most widely consumed",
	}, handleBlastRadius)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_cycles",
		Description: "Detect import cycles that cause circular comprehension for AI agents",
	}, handleCycles)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_diff",
		Description: "Compare code structure metrics between a git ref and HEAD, reporting regressions (metrics that got worse)",
	}, handleDiff)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "adit_briefing",
		Description: "Pre-edit briefing: returns structural warnings about a file BEFORE you edit it. Call this before editing any file to learn about cross-file risks (ambiguous names, blast radius, single-consumer imports) that you'd otherwise discover one grep at a time.",
	}, handleBriefing)

	return server.Run(ctx, &mcp.StdioTransport{})
}

type pathParams struct {
	Path string `json:"path"`
}

type diffParams struct {
	Path string `json:"path"`
	Ref  string `json:"ref"`
}

func handleScoreRepo(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	result, err := cachedScoreRepo([]string{path})
	if err != nil {
		return nil, nil, fmt.Errorf("score failed: %w", err)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func handleScoreFile(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	if args.Path == "" {
		return nil, nil, fmt.Errorf("path is required")
	}

	dir := filepath.Dir(args.Path)
	repo, err := cachedScoreRepo([]string{dir})
	if err != nil {
		return nil, nil, fmt.Errorf("score failed: %w", err)
	}

	absPath, _ := filepath.Abs(args.Path)
	for _, fs := range repo.Files {
		fAbs, _ := filepath.Abs(fs.Path)
		if fAbs == absPath {
			data, _ := json.MarshalIndent(fs, "", "  ")
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(data)},
				},
			}, nil, nil
		}
	}

	return nil, nil, fmt.Errorf("file not found: %s", args.Path)
}

func handleRelocatable(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	result, err := cachedScoreRepo([]string{path})
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	data, _ := json.MarshalIndent(result.Summary.Relocatable, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func handleAmbiguous(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	result, err := cachedScoreRepo([]string{path})
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	data, _ := json.MarshalIndent(result.Summary.AmbiguousNames, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func handleBlastRadius(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	if args.Path == "" {
		return nil, nil, fmt.Errorf("path is required")
	}

	dir := filepath.Dir(args.Path)
	repo, err := cachedScoreRepo([]string{dir})
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	absPath, _ := filepath.Abs(args.Path)
	for _, fs := range repo.Files {
		fAbs, _ := filepath.Abs(fs.Path)
		if fAbs == absPath {
			data, _ := json.MarshalIndent(fs.BlastRadius, "", "  ")
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(data)},
				},
			}, nil, nil
		}
	}

	return nil, nil, fmt.Errorf("file not found: %s", args.Path)
}

func handleCycles(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	result, err := cachedScoreRepo([]string{path})
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	cycles := result.Summary.Cycles
	if cycles == nil {
		cycles = []score.ImportCycle{}
	}

	data, _ := json.MarshalIndent(cycles, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func handleBriefing(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	if args.Path == "" {
		return nil, nil, fmt.Errorf("path is required")
	}

	dir := filepath.Dir(args.Path)
	result, err := cachedScoreRepo([]string{dir})
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	absPath, _ := filepath.Abs(args.Path)
	var target *score.FileScore
	for _, f := range result.Files {
		fAbs, _ := filepath.Abs(f.Path)
		if fAbs == absPath {
			target = &f
			break
		}
	}
	if target == nil {
		return nil, nil, fmt.Errorf("file not found: %s", args.Path)
	}

	t := sessionConfig.ThresholdsForPath(target.Path)

	var briefing strings.Builder
	fmt.Fprintf(&briefing, "%s: %d lines (Grade %s), nesting %d, %d AST types\n",
		filepath.Base(target.Path), target.Lines, target.SizeGrade,
		target.MaxNestingDepth, target.NodeDiversity)

	if target.MaxParams > 10 {
		fmt.Fprintf(&briefing, "⚠ Function with %d parameters\n", target.MaxParams)
	}
	if t.MaxBlastRadius > 0 && target.BlastRadius.ImportedByCount > t.MaxBlastRadius/4 {
		fmt.Fprintf(&briefing, "⚠ %d files import from here — verify callers after editing\n",
			target.BlastRadius.ImportedByCount)
	}
	if target.Ambiguity.GrepNoise > 0 {
		fmt.Fprintf(&briefing, "⚠ Grep noise %d — some names here also defined in other files\n",
			target.Ambiguity.GrepNoise)
	}
	for _, r := range target.ContextReads.Relocatable {
		fmt.Fprintf(&briefing, "⚠ %s only used here — co-locate from %s\n", r.Name, r.From)
	}
	if target.Comments.CommentLines == 0 && target.Lines > 200 {
		briefing.WriteString("⚠ No comments in large file\n")
	}
	for _, a := range result.Summary.AmbiguousNames {
		for _, site := range a.Sites {
			siteAbs, _ := filepath.Abs(site.File)
			if siteAbs == absPath {
				fmt.Fprintf(&briefing, "⚠ %s defined in %d other files — use qualified search\n",
					a.Name, a.Count-1)
				break
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: briefing.String()},
		},
	}, nil, nil
}

func handleDiff(_ context.Context, req *mcp.CallToolRequest, args diffParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}
	ref := args.Ref
	if ref == "" {
		ref = "HEAD~1"
	}

	ensureSession()
	// Diff runs fresh and invalidates cache (repo state may have changed)
	sessionCache.Invalidate()
	result, err := sessionPipeline.ScoreRepoDiff([]string{path}, ref)
	if err != nil {
		return nil, nil, fmt.Errorf("diff failed: %w", err)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}
