package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/justindotpub/adit-code/internal/config"
	"github.com/justindotpub/adit-code/internal/lang"
	"github.com/justindotpub/adit-code/internal/score"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run starts the MCP server on stdio.
func Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "adit",
		Version: "0.1.0",
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

	return server.Run(ctx, &mcp.StdioTransport{})
}

func newPipeline() *score.Pipeline {
	cfg, _ := config.Load(".")
	frontends := []lang.Frontend{
		lang.NewPythonFrontend(),
		lang.NewTypeScriptFrontend(),
	}
	return score.NewPipeline(frontends, cfg)
}

type pathParams struct {
	Path string `json:"path"`
}

func handleScoreRepo(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	pipeline := newPipeline()
	result, err := pipeline.ScoreRepo([]string{path})
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

	pipeline := newPipeline()
	result, err := pipeline.ScoreFile(args.Path)
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

func handleRelocatable(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	pipeline := newPipeline()
	result, err := pipeline.ScoreRepo([]string{path})
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

	pipeline := newPipeline()
	result, err := pipeline.ScoreRepo([]string{path})
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

	pipeline := newPipeline()
	result, err := pipeline.ScoreFile(args.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("analysis failed: %w", err)
	}

	data, _ := json.MarshalIndent(result.BlastRadius, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func handleCycles(_ context.Context, req *mcp.CallToolRequest, args pathParams) (*mcp.CallToolResult, any, error) {
	path := args.Path
	if path == "" {
		path = "."
	}

	pipeline := newPipeline()
	result, err := pipeline.ScoreRepo([]string{path})
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
