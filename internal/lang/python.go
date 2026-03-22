package lang

import (
	"bytes"
	"strings"
	"unicode"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// PythonFrontend extracts definitions and imports from Python files.
type PythonFrontend struct {
	parser *tree_sitter.Parser
}

func NewPythonFrontend() *PythonFrontend {
	parser := tree_sitter.NewParser()
	lang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	_ = parser.SetLanguage(lang)
	return &PythonFrontend{parser: parser}
}

func (f *PythonFrontend) Extensions() []string {
	return []string{".py"}
}

func (f *PythonFrontend) Analyze(path string, src []byte) (*FileAnalysis, error) {
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

	// Walk top-level children
	if cursor.GotoFirstChild() {
		for {
			node := cursor.Node()
			kind := node.Kind()

			switch kind {
			case "function_definition":
				fa.Definitions = append(fa.Definitions, extractPythonFuncDef(node, src, ""))
			case "class_definition":
				fa.Definitions = append(fa.Definitions, extractPythonClassDef(node, src))
				// Also extract methods from the class body
				fa.Definitions = append(fa.Definitions, extractPythonMethods(node, src)...)
			case "import_statement":
				fa.Imports = append(fa.Imports, extractPythonImport(node, src)...)
			case "import_from_statement":
				fa.Imports = append(fa.Imports, extractPythonFromImport(node, src)...)
			case "expression_statement":
				// Check for top-level assignments (constants)
				child := node.NamedChild(0)
				if child != nil && child.Kind() == "assignment" {
					if def, ok := extractPythonAssignment(child, src); ok {
						fa.Definitions = append(fa.Definitions, def)
					}
				}
			}

			if !cursor.GotoNextSibling() {
				break
			}
		}
	}

	return fa, nil
}

func extractPythonFuncDef(node *tree_sitter.Node, src []byte, className string) Definition {
	nameNode := node.ChildByFieldName("name")
	name := nodeText(nameNode, src)
	qualified := name
	kind := "function"
	if className != "" {
		qualified = className + "." + name
		kind = "method"
	}
	return Definition{
		Name:          name,
		QualifiedName: qualified,
		Kind:          kind,
		Line:          int(node.StartPosition().Row) + 1,
	}
}

func extractPythonClassDef(node *tree_sitter.Node, src []byte) Definition {
	nameNode := node.ChildByFieldName("name")
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "class",
		Line:          int(node.StartPosition().Row) + 1,
	}
}

func extractPythonMethods(classNode *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	classNameNode := classNode.ChildByFieldName("name")
	className := nodeText(classNameNode, src)

	body := classNode.ChildByFieldName("body")
	if body == nil {
		return defs
	}

	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		if child.Kind() == "function_definition" {
			defs = append(defs, extractPythonFuncDef(child, src, className))
		}
	}
	return defs
}

func extractPythonImport(node *tree_sitter.Node, src []byte) []Import {
	// import foo, bar, baz
	var imports []Import
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "dotted_name" || child.Kind() == "aliased_import" {
			var name, module string
			if child.Kind() == "aliased_import" {
				nameNode := child.ChildByFieldName("name")
				module = nodeText(nameNode, src)
				aliasNode := child.ChildByFieldName("alias")
				if aliasNode != nil {
					name = nodeText(aliasNode, src)
				} else {
					name = module
				}
			} else {
				module = nodeText(child, src)
				// For "import foo.bar", the name used in code is "foo"
				parts := strings.SplitN(module, ".", 2)
				name = parts[0]
			}
			imports = append(imports, Import{
				Name:         name,
				SourceModule: module,
				Kind:         classifyPythonImportKind(name),
				Line:         int(child.StartPosition().Row) + 1,
			})
		}
	}
	return imports
}

func extractPythonFromImport(node *tree_sitter.Node, src []byte) []Import {
	// from module import name1, name2
	var imports []Import

	moduleNode := node.ChildByFieldName("module_name")
	module := ""
	if moduleNode != nil {
		module = nodeText(moduleNode, src)
	}

	// Check for relative imports (dots before module name)
	text := nodeText(node, src)
	if strings.HasPrefix(text, "from .") || strings.HasPrefix(text, "from ..") {
		dotCount := 0
		for _, ch := range text[5:] { // skip "from "
			if ch == '.' {
				dotCount++
			} else {
				break
			}
		}
		prefix := strings.Repeat(".", dotCount)
		if module != "" {
			module = prefix + module
		} else {
			module = prefix
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "dotted_name":
			name := nodeText(child, src)
			// Skip the module name itself (first dotted_name is the module)
			if child == moduleNode {
				continue
			}
			imports = append(imports, Import{
				Name:         name,
				SourceModule: module,
				Kind:         classifyPythonImportKind(name),
				Line:         int(child.StartPosition().Row) + 1,
			})
		case "aliased_import":
			nameNode := child.ChildByFieldName("name")
			name := nodeText(nameNode, src)
			imports = append(imports, Import{
				Name:         name,
				SourceModule: module,
				Kind:         classifyPythonImportKind(name),
				Line:         int(child.StartPosition().Row) + 1,
			})
		}
	}

	return imports
}

func extractPythonAssignment(node *tree_sitter.Node, src []byte) (Definition, bool) {
	// Look for UPPER_CASE = ... at module level
	left := node.ChildByFieldName("left")
	if left == nil || left.Kind() != "identifier" {
		return Definition{}, false
	}
	name := nodeText(left, src)
	if !isUpperCase(name) {
		return Definition{}, false
	}
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "constant",
		Line:          int(node.StartPosition().Row) + 1,
	}, true
}

func classifyPythonImportKind(name string) string {
	if isUpperCase(name) {
		return "constant"
	}
	if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
		return "type"
	}
	return "function"
}

// isUpperCase returns true if the name is ALL_UPPER_CASE (with underscores).
func isUpperCase(name string) bool {
	if len(name) == 0 {
		return false
	}
	hasLetter := false
	for _, r := range name {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}

func nodeText(node *tree_sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if start >= uint(len(src)) || end > uint(len(src)) {
		return ""
	}
	return string(src[start:end])
}
