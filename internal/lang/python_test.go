package lang

import (
	"os"
	"testing"
)

func TestPythonFrontendConstants(t *testing.T) {
	f := NewPythonFrontend()
	src, err := os.ReadFile("../../testdata/python/constants.py")
	if err != nil {
		t.Fatal(err)
	}

	fa, err := f.Analyze("constants.py", src)
	if err != nil {
		t.Fatal(err)
	}

	if len(fa.Definitions) != 4 {
		t.Errorf("expected 4 definitions, got %d", len(fa.Definitions))
		for _, d := range fa.Definitions {
			t.Logf("  %s (%s) line %d", d.Name, d.Kind, d.Line)
		}
	}

	for _, d := range fa.Definitions {
		if d.Kind != "constant" {
			t.Errorf("expected all constants, got %s for %s", d.Kind, d.Name)
		}
	}
}

func TestPythonFrontendHandlers(t *testing.T) {
	f := NewPythonFrontend()
	src, err := os.ReadFile("../../testdata/python/handlers.py")
	if err != nil {
		t.Fatal(err)
	}

	fa, err := f.Analyze("handlers.py", src)
	if err != nil {
		t.Fatal(err)
	}

	// Should have imports
	if len(fa.Imports) == 0 {
		t.Error("expected imports, got none")
	}

	// Check we found TRUST_HINTS import
	found := false
	for _, imp := range fa.Imports {
		if imp.Name == "TRUST_HINTS" {
			found = true
			if imp.Kind != "constant" {
				t.Errorf("TRUST_HINTS should be classified as constant, got %s", imp.Kind)
			}
		}
	}
	if !found {
		t.Error("TRUST_HINTS import not found")
		for _, imp := range fa.Imports {
			t.Logf("  import %s from %s (%s)", imp.Name, imp.SourceModule, imp.Kind)
		}
	}

	// Should have class and method definitions
	var hasClass, hasMethod bool
	for _, d := range fa.Definitions {
		if d.Kind == "class" {
			hasClass = true
		}
		if d.Kind == "method" {
			hasMethod = true
		}
	}
	if !hasClass {
		t.Error("expected class definitions")
	}
	if !hasMethod {
		t.Error("expected method definitions")
	}

	// Check _validate is found as a method
	validateCount := 0
	for _, d := range fa.Definitions {
		if d.Name == "_validate" {
			validateCount++
		}
	}
	if validateCount != 2 {
		t.Errorf("expected 2 _validate methods (PaymentHandler + RefundHandler), got %d", validateCount)
		for _, d := range fa.Definitions {
			t.Logf("  %s (%s) %s line %d", d.Name, d.Kind, d.QualifiedName, d.Line)
		}
	}
}

func TestPythonFrontendUtils(t *testing.T) {
	f := NewPythonFrontend()
	src, err := os.ReadFile("../../testdata/python/utils.py")
	if err != nil {
		t.Fatal(err)
	}

	fa, err := f.Analyze("utils.py", src)
	if err != nil {
		t.Fatal(err)
	}

	if len(fa.Definitions) != 3 {
		t.Errorf("expected 3 definitions, got %d", len(fa.Definitions))
		for _, d := range fa.Definitions {
			t.Logf("  %s (%s)", d.Name, d.Kind)
		}
	}

	// All should be functions
	for _, d := range fa.Definitions {
		if d.Kind != "function" {
			t.Errorf("expected function, got %s for %s", d.Kind, d.Name)
		}
	}
}
