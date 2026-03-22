package lang

import (
	"os"
	"testing"
)

func TestTypeScriptFrontendConstants(t *testing.T) {
	f := NewTypeScriptFrontend()
	src, err := os.ReadFile("../../testdata/typescript/constants.ts")
	if err != nil {
		t.Fatal(err)
	}

	fa, err := f.Analyze("constants.ts", src)
	if err != nil {
		t.Fatal(err)
	}

	// Should have constants + type + interface
	if len(fa.Definitions) < 5 {
		t.Errorf("expected at least 5 definitions, got %d", len(fa.Definitions))
		for _, d := range fa.Definitions {
			t.Logf("  %s (%s) line %d", d.Name, d.Kind, d.Line)
		}
	}

	constCount := 0
	typeCount := 0
	for _, d := range fa.Definitions {
		switch d.Kind {
		case "constant":
			constCount++
		case "type":
			typeCount++
		}
	}
	if constCount < 3 {
		t.Errorf("expected at least 3 constants, got %d", constCount)
	}
	if typeCount < 2 {
		t.Errorf("expected at least 2 types (Config + Logger), got %d", typeCount)
	}
}

func TestTypeScriptFrontendHandlers(t *testing.T) {
	f := NewTypeScriptFrontend()
	src, err := os.ReadFile("../../testdata/typescript/handlers.ts")
	if err != nil {
		t.Fatal(err)
	}

	fa, err := f.Analyze("handlers.ts", src)
	if err != nil {
		t.Fatal(err)
	}

	// Should have imports
	if len(fa.Imports) == 0 {
		t.Error("expected imports, got none")
	}

	// Check MAX_RETRIES import
	found := false
	for _, imp := range fa.Imports {
		if imp.Name == "MAX_RETRIES" {
			found = true
			if imp.Kind != "constant" {
				t.Errorf("MAX_RETRIES should be constant, got %s", imp.Kind)
			}
		}
		t.Logf("  import %s from %s (%s)", imp.Name, imp.SourceModule, imp.Kind)
	}
	if !found {
		t.Error("MAX_RETRIES import not found")
	}

	// Check type imports
	typeImportCount := 0
	for _, imp := range fa.Imports {
		if imp.Kind == "type" {
			typeImportCount++
		}
	}
	if typeImportCount < 2 {
		t.Errorf("expected at least 2 type imports (Config, Logger), got %d", typeImportCount)
	}

	// Should have class + function definitions
	var hasClass, hasFunc bool
	for _, d := range fa.Definitions {
		t.Logf("  def %s (%s) %s line %d", d.Name, d.Kind, d.QualifiedName, d.Line)
		if d.Kind == "class" {
			hasClass = true
		}
		if d.Kind == "function" {
			hasFunc = true
		}
	}
	if !hasClass {
		t.Error("expected class definition")
	}
	if !hasFunc {
		t.Error("expected function definition (handleRefund)")
	}
}
