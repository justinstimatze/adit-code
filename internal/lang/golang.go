package lang

import (
	"bytes"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// GoFrontend extracts definitions and imports from Go files.
type GoFrontend struct {
	parser *tree_sitter.Parser
}

func NewGoFrontend() *GoFrontend {
	parser := tree_sitter.NewParser()
	lang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	_ = parser.SetLanguage(lang)
	return &GoFrontend{parser: parser}
}

func (f *GoFrontend) Extensions() []string {
	return []string{".go"}
}

func (f *GoFrontend) Analyze(path string, src []byte) (*FileAnalysis, error) {
	// Skip test files by default
	if strings.HasSuffix(path, "_test.go") {
		return &FileAnalysis{Path: path, Lines: bytes.Count(src, []byte("\n")) + 1}, nil
	}

	tree := f.parser.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	lines := bytes.Count(src, []byte("\n")) + 1

	fa := &FileAnalysis{
		Path:  path,
		Lines: lines,
	}

	cursor := root.Walk()
	defer cursor.Close()

	if cursor.GotoFirstChild() {
		for {
			node := cursor.Node()
			kind := node.Kind()

			switch kind {
			case "function_declaration":
				if def, ok := extractGoFuncDecl(node, src); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "method_declaration":
				if def, ok := extractGoMethodDecl(node, src); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "type_declaration":
				fa.Definitions = append(fa.Definitions, extractGoTypeDecl(node, src)...)
			case "const_declaration":
				fa.Definitions = append(fa.Definitions, extractGoConstDecl(node, src)...)
			case "var_declaration":
				fa.Definitions = append(fa.Definitions, extractGoVarDecl(node, src)...)
			case "import_declaration":
				fa.Imports = append(fa.Imports, extractGoImports(node, src)...)
			}

			if !cursor.GotoNextSibling() {
				break
			}
		}
	}

	// Collect anonymous functions (func literals) spanning 3+ lines
	fa.Definitions = append(fa.Definitions, collectAnonymousFunctions(root, src, 3)...)

	fa.MaxNestingDepth = ComputeMaxNestingDepth(root)
	fa.NodeDiversity = ComputeNodeDiversity(root)
	fa.MaxBranching = ComputeMaxBranching(root)

	return fa, nil
}

func countGoParams(node *tree_sitter.Node) int {
	params := node.ChildByFieldName("parameters")
	if params == nil {
		return 0
	}
	count := 0
	for i := uint(0); i < params.NamedChildCount(); i++ {
		param := params.NamedChild(i)
		if param.Kind() == "parameter_declaration" {
			// Go allows grouped params: `a, b int` is one declaration with multiple names.
			// Count the identifier children to get the actual parameter count.
			names := 0
			for j := uint(0); j < param.NamedChildCount(); j++ {
				if param.NamedChild(j).Kind() == "identifier" {
					names++
				}
			}
			if names == 0 {
				// Unnamed param (e.g., `func foo(int)` or interface methods
				// like `Handle(context.Context, *Request) error`).
				// Each parameter_declaration without identifiers is one param.
				count++
			} else {
				count += names
			}
		} else if param.Kind() == "variadic_parameter_declaration" {
			count++
		}
	}
	return count
}

func extractGoFuncDecl(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "function",
		Line:          int(node.StartPosition().Row) + 1,
		EndLine:       int(node.EndPosition().Row) + 1,
		ParamCount:    countGoParams(node),
	}, true
}

func extractGoMethodDecl(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)

	// Extract receiver type for qualified name
	receiver := ""
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode != nil {
		// Walk the parameter list to find the type
		for i := uint(0); i < receiverNode.NamedChildCount(); i++ {
			param := receiverNode.NamedChild(i)
			if param.Kind() == "parameter_declaration" {
				typeNode := param.ChildByFieldName("type")
				if typeNode != nil {
					receiver = nodeText(typeNode, src)
					// Strip pointer: *Foo -> Foo
					receiver = strings.TrimPrefix(receiver, "*")
				}
			}
		}
	}

	qualified := name
	if receiver != "" {
		qualified = receiver + "." + name
	}

	return Definition{
		Name:          name,
		QualifiedName: qualified,
		Kind:          "method",
		Line:          int(node.StartPosition().Row) + 1,
		EndLine:       int(node.EndPosition().Row) + 1,
		ParamCount:    countGoParams(node),
	}, true
}

func extractGoTypeDecl(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	// type_declaration can contain type_spec children (single or grouped)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, src)
				defs = append(defs, Definition{
					Name:          name,
					QualifiedName: name,
					Kind:          "type",
					Line:          int(child.StartPosition().Row) + 1,
					EndLine:       int(child.EndPosition().Row) + 1,
				})
			}
		}
	}
	return defs
}

func extractGoConstDecl(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "const_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, src)
				defs = append(defs, Definition{
					Name:          name,
					QualifiedName: name,
					Kind:          "constant",
					Line:          int(child.StartPosition().Row) + 1,
					EndLine:       int(child.EndPosition().Row) + 1,
				})
			}
		}
	}
	return defs
}

func extractGoVarDecl(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "var_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, src)
				// Only track exported or UPPER_CASE vars
				if len(name) > 0 && (name[0] >= 'A' && name[0] <= 'Z') {
					defs = append(defs, Definition{
						Name:          name,
						QualifiedName: name,
						Kind:          "constant", // exported vars are effectively constants for import purposes
						Line:          int(child.StartPosition().Row) + 1,
						EndLine:       int(child.EndPosition().Row) + 1,
					})
				}
			}
		}
	}
	return defs
}

func extractGoImports(node *tree_sitter.Node, src []byte) []Import {
	var imports []Import
	// import_declaration contains import_spec_list which contains import_spec nodes
	collectImportSpecs(node, src, &imports)
	return imports
}

func collectImportSpecs(node *tree_sitter.Node, src []byte, imports *[]Import) {
	if node.Kind() == "import_spec" {
		// Find the path (interpreted_string_literal)
		var importPath string
		var alias string

		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			switch child.Kind() {
			case "interpreted_string_literal":
				importPath = unquote(nodeText(child, src))
			case "package_identifier":
				alias = nodeText(child, src)
			}
		}

		if importPath == "" {
			return
		}

		// Determine the name used in code
		name := alias
		if name == "" {
			parts := strings.Split(importPath, "/")
			name = parts[len(parts)-1]
		}

		kind := "function" // Go packages expose functions by default
		if isUpperCase(name) {
			kind = "constant"
		}

		*imports = append(*imports, Import{
			Name:         name,
			SourceModule: importPath,
			Kind:         kind,
			Line:         int(node.StartPosition().Row) + 1,
		})
		return
	}

	// Recurse into children (handles import_spec_list, import_declaration)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		collectImportSpecs(node.NamedChild(i), src, imports)
	}
}
