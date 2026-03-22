package lang

import (
	"bytes"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// TypeScriptFrontend extracts definitions and imports from TypeScript/TSX files.
type TypeScriptFrontend struct {
	tsParser  *tree_sitter.Parser
	tsxParser *tree_sitter.Parser
}

func NewTypeScriptFrontend() *TypeScriptFrontend {
	tsParser := tree_sitter.NewParser()
	tsLang := tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	_ = tsParser.SetLanguage(tsLang)

	tsxParser := tree_sitter.NewParser()
	tsxLang := tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	_ = tsxParser.SetLanguage(tsxLang)

	return &TypeScriptFrontend{
		tsParser:  tsParser,
		tsxParser: tsxParser,
	}
}

func (f *TypeScriptFrontend) Extensions() []string {
	return []string{".ts", ".tsx"}
}

func (f *TypeScriptFrontend) Analyze(path string, src []byte) (*FileAnalysis, error) {
	parser := f.tsParser
	if strings.HasSuffix(path, ".tsx") {
		parser = f.tsxParser
	}

	tree := parser.Parse(src, nil)
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
				if def, ok := extractTSFuncDecl(node, src, ""); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "class_declaration":
				fa.Definitions = append(fa.Definitions, extractTSClassDecl(node, src))
				fa.Definitions = append(fa.Definitions, extractTSMethods(node, src)...)
			case "lexical_declaration":
				fa.Definitions = append(fa.Definitions, extractTSLexicalDecl(node, src)...)
			case "type_alias_declaration":
				if def, ok := extractTSTypeAlias(node, src); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "interface_declaration":
				if def, ok := extractTSInterface(node, src); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "enum_declaration":
				if def, ok := extractTSEnum(node, src); ok {
					fa.Definitions = append(fa.Definitions, def)
				}
			case "import_statement":
				fa.Imports = append(fa.Imports, extractTSImport(node, src)...)
			case "export_statement":
				// Handle "export function ...", "export class ...", etc.
				fa.Definitions = append(fa.Definitions, extractTSExportedDefs(node, src)...)
				// Handle "export { ... } from '...'"
				fa.Imports = append(fa.Imports, extractTSReExport(node, src)...)
			}

			if !cursor.GotoNextSibling() {
				break
			}
		}
	}

	return fa, nil
}

func extractTSFuncDecl(node *tree_sitter.Node, src []byte, className string) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
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
	}, true
}

func extractTSClassDecl(node *tree_sitter.Node, src []byte) Definition {
	nameNode := node.ChildByFieldName("name")
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "class",
		Line:          int(node.StartPosition().Row) + 1,
	}
}

func extractTSMethods(classNode *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	classNameNode := classNode.ChildByFieldName("name")
	className := nodeText(classNameNode, src)

	body := classNode.ChildByFieldName("body")
	if body == nil {
		return defs
	}

	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		switch child.Kind() {
		case "method_definition", "public_field_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, src)
				defs = append(defs, Definition{
					Name:          name,
					QualifiedName: className + "." + name,
					Kind:          "method",
					Line:          int(child.StartPosition().Row) + 1,
				})
			}
		}
	}
	return defs
}

func extractTSLexicalDecl(node *tree_sitter.Node, src []byte) []Definition {
	// const X = ..., let Y = ...
	var defs []Definition
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, src)
				kind := "function" // default for const arrow functions
				if isUpperCase(name) {
					kind = "constant"
				}
				// Check if the value is an arrow function or function expression
				valueNode := child.ChildByFieldName("value")
				if valueNode != nil {
					vk := valueNode.Kind()
					if vk == "arrow_function" || vk == "function" || vk == "function_expression" {
						kind = "function"
					}
				}
				defs = append(defs, Definition{
					Name:          name,
					QualifiedName: name,
					Kind:          kind,
					Line:          int(child.StartPosition().Row) + 1,
				})
			}
		}
	}
	return defs
}

func extractTSTypeAlias(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "type",
		Line:          int(node.StartPosition().Row) + 1,
	}, true
}

func extractTSInterface(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "type",
		Line:          int(node.StartPosition().Row) + 1,
	}, true
}

func extractTSEnum(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "type",
		Line:          int(node.StartPosition().Row) + 1,
	}, true
}

func extractTSImport(node *tree_sitter.Node, src []byte) []Import {
	var imports []Import

	// Find the source string (the module path)
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return imports
	}
	module := unquote(nodeText(sourceNode, src))

	// Check if this is `import type`
	text := nodeText(node, src)
	isTypeImport := strings.Contains(text, "import type")

	// Find import clause (named imports, default import, namespace import)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "import_clause":
			imports = append(imports, extractTSImportClause(child, src, module, isTypeImport)...)
		}
	}

	return imports
}

func extractTSImportClause(node *tree_sitter.Node, src []byte, module string, isTypeImport bool) []Import {
	var imports []Import

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "identifier":
			// Default import: import Foo from '...'
			name := nodeText(child, src)
			imports = append(imports, Import{
				Name:         name,
				SourceModule: module,
				Kind:         classifyTSImportKind(name, isTypeImport),
				Line:         int(child.StartPosition().Row) + 1,
			})
		case "named_imports":
			// import { X, Y } from '...'
			for j := uint(0); j < child.NamedChildCount(); j++ {
				specifier := child.NamedChild(j)
				if specifier.Kind() == "import_specifier" {
					nameNode := specifier.ChildByFieldName("name")
					if nameNode != nil {
						name := nodeText(nameNode, src)
						imports = append(imports, Import{
							Name:         name,
							SourceModule: module,
							Kind:         classifyTSImportKind(name, isTypeImport),
							Line:         int(specifier.StartPosition().Row) + 1,
						})
					}
				}
			}
		case "namespace_import":
			// import * as X from '...'
			nameNode := child.NamedChild(0)
			if nameNode != nil {
				name := nodeText(nameNode, src)
				imports = append(imports, Import{
					Name:         name,
					SourceModule: module,
					Kind:         "unknown",
					Line:         int(child.StartPosition().Row) + 1,
				})
			}
		}
	}

	return imports
}

func extractTSExportedDefs(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "function_declaration":
			if def, ok := extractTSFuncDecl(child, src, ""); ok {
				defs = append(defs, def)
			}
		case "class_declaration":
			defs = append(defs, extractTSClassDecl(child, src))
			defs = append(defs, extractTSMethods(child, src)...)
		case "lexical_declaration":
			defs = append(defs, extractTSLexicalDecl(child, src)...)
		case "type_alias_declaration":
			if def, ok := extractTSTypeAlias(child, src); ok {
				defs = append(defs, def)
			}
		case "interface_declaration":
			if def, ok := extractTSInterface(child, src); ok {
				defs = append(defs, def)
			}
		case "enum_declaration":
			if def, ok := extractTSEnum(child, src); ok {
				defs = append(defs, def)
			}
		}
	}
	return defs
}

func extractTSReExport(node *tree_sitter.Node, src []byte) []Import {
	// export { X, Y } from './module'
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return nil
	}
	module := unquote(nodeText(sourceNode, src))

	var imports []Import
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "export_clause" {
			for j := uint(0); j < child.NamedChildCount(); j++ {
				specifier := child.NamedChild(j)
				if specifier.Kind() == "export_specifier" {
					nameNode := specifier.ChildByFieldName("name")
					if nameNode != nil {
						name := nodeText(nameNode, src)
						imports = append(imports, Import{
							Name:         name,
							SourceModule: module,
							Kind:         classifyTSImportKind(name, false),
							Line:         int(specifier.StartPosition().Row) + 1,
						})
					}
				}
			}
		}
	}
	return imports
}

func classifyTSImportKind(name string, isTypeImport bool) string {
	if isTypeImport {
		return "type"
	}
	if isUpperCase(name) {
		return "constant"
	}
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return "type"
	}
	return "function"
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}
