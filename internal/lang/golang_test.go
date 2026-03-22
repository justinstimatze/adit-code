package lang

import (
	"testing"
)

func TestGoFrontend_SelfParse(t *testing.T) {
	f := NewGoFrontend()
	src := []byte(`package main

import (
	"fmt"
	"os"

	"github.com/example/pkg/config"
)

const Version = "1.0.0"

type Config struct {
	Name string
	Port int
}

type Server interface {
	Start() error
	Stop() error
}

func NewServer(cfg Config) *server {
	return &server{cfg: cfg}
}

type server struct {
	cfg Config
}

func (s *server) Start() error {
	fmt.Println("starting on", s.cfg.Port)
	return nil
}

func (s *server) Stop() error {
	return nil
}

var DefaultConfig = Config{Name: "default", Port: 8080}
`)

	fa, err := f.Analyze("main.go", src)
	if err != nil {
		t.Fatal(err)
	}

	// Check imports
	if len(fa.Imports) != 3 {
		t.Errorf("expected 3 imports (fmt, os, config), got %d", len(fa.Imports))
		for _, imp := range fa.Imports {
			t.Logf("  import %s from %s (%s)", imp.Name, imp.SourceModule, imp.Kind)
		}
	}

	// Check definitions
	var funcs, methods, types, consts int
	for _, d := range fa.Definitions {
		switch d.Kind {
		case "function":
			funcs++
		case "method":
			methods++
		case "type":
			types++
		case "constant":
			consts++
		}
	}

	if funcs != 1 {
		t.Errorf("expected 1 function (NewServer), got %d", funcs)
	}
	if methods != 2 {
		t.Errorf("expected 2 methods (Start, Stop), got %d", methods)
	}
	if types < 3 {
		t.Errorf("expected at least 3 types (Config, Server, server), got %d", types)
	}
	if consts < 2 {
		t.Errorf("expected at least 2 constants (Version, DefaultConfig), got %d", consts)
	}

	// Check method qualified names
	for _, d := range fa.Definitions {
		if d.Name == "Start" && d.Kind == "method" {
			if d.QualifiedName != "server.Start" {
				t.Errorf("expected qualified name server.Start, got %s", d.QualifiedName)
			}
		}
	}

	// Log all for inspection
	for _, d := range fa.Definitions {
		t.Logf("  %s (%s) %s line %d", d.Name, d.Kind, d.QualifiedName, d.Line)
	}
}

func TestGoFrontend_ParamCounting(t *testing.T) {
	f := NewGoFrontend()
	src := []byte(`package main

// Grouped params: a, b int = 2 params
func grouped(a, b int) {}

// Unnamed params (common in interfaces)
func unnamed(int, string, error) {}

// Mixed: named + type
func mixed(ctx context.Context, name string, count int) {}

// Variadic
func variadic(args ...string) {}

// No params
func noParams() {}
`)

	fa, err := f.Analyze("params.go", src)
	if err != nil {
		t.Fatal(err)
	}

	paramsByName := make(map[string]int)
	for _, d := range fa.Definitions {
		if d.Kind == "function" {
			paramsByName[d.Name] = d.ParamCount
		}
	}

	tests := []struct {
		name string
		want int
	}{
		{"grouped", 2},
		{"unnamed", 3},
		{"mixed", 3},
		{"variadic", 1},
		{"noParams", 0},
	}
	for _, tt := range tests {
		got, ok := paramsByName[tt.name]
		if !ok {
			t.Errorf("function %s not found", tt.name)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: expected %d params, got %d", tt.name, tt.want, got)
		}
	}
}

func TestGoFrontend_SkipsTestFiles(t *testing.T) {
	f := NewGoFrontend()
	src := []byte(`package main

func TestSomething(t *testing.T) {
	// test code
}
`)

	fa, err := f.Analyze("main_test.go", src)
	if err != nil {
		t.Fatal(err)
	}

	// Test files should be parsed but return minimal data
	if len(fa.Definitions) != 0 {
		t.Errorf("expected 0 definitions from test file, got %d", len(fa.Definitions))
	}
}
