package lang

import (
	"bytes"
	"strings"
	"unicode"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// RustFrontend parses Rust source files.
type RustFrontend struct {
	parser *tree_sitter.Parser
}

func NewRustFrontend() *RustFrontend {
	parser := tree_sitter.NewParser()
	lang := tree_sitter.NewLanguage(tree_sitter_rust.Language())
	_ = parser.SetLanguage(lang)
	return &RustFrontend{parser: parser}
}

func (f *RustFrontend) Extensions() []string {
	return []string{".rs"}
}

// classifyRustImportKind applies Rust naming convention: UpperCamelCase is a
// type or trait, SCREAMING_CASE is a const/static, everything else is a
// function or module.
func classifyRustImportKind(name string) string {
	if name == "*" {
		return "unknown"
	}
	if isUpperCase(name) {
		return "constant"
	}
	if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
		return "type"
	}
	return "function"
}

// countRustParams counts function parameters, excluding self/&self/&mut self.
func countRustParams(node *tree_sitter.Node) int {
	params := node.ChildByFieldName("parameters")
	if params == nil {
		return 0
	}
	count := 0
	for i := uint(0); i < params.NamedChildCount(); i++ {
		switch params.NamedChild(i).Kind() {
		case "parameter", "variadic_parameter":
			count++
		}
	}
	return count
}

// extractRustFuncItem handles both free functions and methods (receiver != "").
// Also handles function_signature_item (trait method declarations with no body),
// since both node kinds share the name/parameters/return_type fields.
func extractRustFuncItem(node *tree_sitter.Node, src []byte, receiver string) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	kind := "function"
	qualified := name
	if receiver != "" {
		kind = "method"
		qualified = receiver + "." + name
	}
	def := Definition{
		Name:          name,
		QualifiedName: qualified,
		Kind:          kind,
		Line:          int(node.StartPosition().Row) + 1,
		EndLine:       int(node.EndPosition().Row) + 1,
		ParamCount:    countRustParams(node),
	}
	if abi, ok := rustExternABI(node, src); ok {
		def.ForeignABI = abi
	}
	return def, true
}

// extractRustImplMethods pulls fn items out of an impl block's body,
// qualifying each by the type being implemented (Foo, not Trait for Foo).
func extractRustImplMethods(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return defs
	}
	receiver := rustBaseTypeName(nodeText(typeNode, src))

	body := node.ChildByFieldName("body")
	if body == nil {
		return defs
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		if child.Kind() == "function_item" {
			if def, ok := extractRustFuncItem(child, src, receiver); ok {
				defs = append(defs, def)
			}
		}
	}
	return defs
}

// extractRustNamedItem handles struct/enum/trait/type-alias/mod declarations,
// which all expose their name via a "name" field and carry no parameters.
func extractRustNamedItem(node *tree_sitter.Node, src []byte, kind string) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          kind,
		Line:          int(node.StartPosition().Row) + 1,
		EndLine:       int(node.EndPosition().Row) + 1,
	}, true
}

// extractRustTraitMethods pulls method signatures and default bodies out of
// a trait's body, qualified by the trait name.
func extractRustTraitMethods(node *tree_sitter.Node, src []byte) []Definition {
	var defs []Definition
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return defs
	}
	receiver := nodeText(nameNode, src)

	body := node.ChildByFieldName("body")
	if body == nil {
		return defs
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		switch child.Kind() {
		case "function_item", "function_signature_item":
			if def, ok := extractRustFuncItem(child, src, receiver); ok {
				defs = append(defs, def)
			}
		}
	}
	return defs
}

// extractRustUse expands a `use` declaration into one Import per leaf item,
// walking nested `use a::{b, c::d, e as f}` trees and joining the path prefix.
func extractRustUse(node *tree_sitter.Node, src []byte) []Import {
	var imports []Import
	arg := node.ChildByFieldName("argument")
	if arg == nil {
		return imports
	}
	line := int(node.StartPosition().Row) + 1
	collectRustUseTree(arg, src, "", line, &imports)
	return imports
}

// extractRustValueItem handles const and static items.
func extractRustValueItem(node *tree_sitter.Node, src []byte) (Definition, bool) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return Definition{}, false
	}
	name := nodeText(nameNode, src)
	return Definition{
		Name:          name,
		QualifiedName: name,
		Kind:          "constant",
		Line:          int(node.StartPosition().Row) + 1,
		EndLine:       int(node.EndPosition().Row) + 1,
	}, true
}

// rustBaseTypeName strips generic parameters and reference sigils so
// `impl<T> Foo<T>` and `impl Foo` qualify methods under the same "Foo".
func rustBaseTypeName(s string) string {
	s = strings.TrimPrefix(s, "&")
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (f *RustFrontend) Analyze(path string, src []byte) (*FileAnalysis, error) {
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
			extractRustTopLevelItem(cursor.Node(), src, fa)
			if !cursor.GotoNextSibling() {
				break
			}
		}
	}

	// Collect closures spanning 3+ lines
	fa.Definitions = append(fa.Definitions, collectAnonymousFunctions(root, src, 3)...)

	// Local-file-only heuristics: upgrade casing-guessed import kinds using
	// definitions collected in this same pass, and mark definitions that
	// are only reachable via a macro invocation's raw tokens.
	hardenRustImportKinds(fa.Imports, fa.Definitions)
	markRustMacroReferences(root, src, fa.Definitions)

	fa.MaxNestingDepth = ComputeMaxNestingDepth(root)
	fa.NodeDiversity = ComputeNodeDiversity(root)
	fa.MaxBranching = ComputeMaxBranching(root)

	return fa, nil
}

func collectRustUseTree(node *tree_sitter.Node, src []byte, prefix string, line int, imports *[]Import) {
	switch node.Kind() {
	case "identifier", "super", "crate":
		name := nodeText(node, src)
		*imports = append(*imports, Import{
			Name:         name,
			SourceModule: joinRustPath(prefix, name),
			Kind:         classifyRustImportKind(name),
			Line:         line,
		})
	case "self":
		// `use a::b::{self, ...}` imports `b` itself, not a literal "self".
		name := lastRustSegment(prefix)
		if name == "" {
			name = "self"
		}
		*imports = append(*imports, Import{
			Name:         name,
			SourceModule: prefix,
			Kind:         classifyRustImportKind(name),
			Line:         line,
		})
	case "scoped_identifier":
		pathText := ""
		if p := node.ChildByFieldName("path"); p != nil {
			pathText = nodeText(p, src)
		}
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil {
			return
		}
		name := nodeText(nameNode, src)
		*imports = append(*imports, Import{
			Name:         name,
			SourceModule: joinRustPath(joinRustPath(prefix, pathText), name),
			Kind:         classifyRustImportKind(name),
			Line:         line,
		})
	case "use_as_clause":
		pathNode := node.ChildByFieldName("path")
		aliasNode := node.ChildByFieldName("alias")
		if pathNode == nil || aliasNode == nil {
			return
		}
		alias := nodeText(aliasNode, src)
		underlying := nodeText(pathNode, src)
		*imports = append(*imports, Import{
			Name:         alias,
			SourceModule: joinRustPath(prefix, underlying),
			Kind:         classifyRustImportKind(alias),
			Line:         line,
		})
	case "use_wildcard":
		module := prefix
		if node.NamedChildCount() > 0 {
			module = joinRustPath(prefix, nodeText(node.NamedChild(0), src))
		}
		*imports = append(*imports, Import{
			Name:         "*",
			SourceModule: module,
			Kind:         "unknown",
			Line:         line,
		})
	case "use_list":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			collectRustUseTree(node.NamedChild(i), src, prefix, line, imports)
		}
	case "scoped_use_list":
		pathText := ""
		if p := node.ChildByFieldName("path"); p != nil {
			pathText = nodeText(p, src)
		}
		listNode := node.ChildByFieldName("list")
		if listNode == nil {
			return
		}
		collectRustUseTree(listNode, src, joinRustPath(prefix, pathText), line, imports)
	}
}

func joinRustPath(prefix, part string) string {
	if prefix == "" {
		return part
	}
	if part == "" {
		return prefix
	}
	return prefix + "::" + part
}

// lastRustSegment returns the final `::`-separated segment of a path.
func lastRustSegment(path string) string {
	if i := strings.LastIndex(path, "::"); i >= 0 {
		return path[i+2:]
	}
	return path
}

// extractRustExternBlock records declarations inside `extern "C" { ... }` as
// imports of kind "extern_c". These are genuine cross-file (often cross-
// language) coupling points that a use/mod-only import graph would otherwise
// miss entirely -- surfacing them as a distinct kind makes that blind spot
// visible instead of silently invisible.
func extractRustExternBlock(node *tree_sitter.Node, src []byte) []Import {
	var imports []Import
	body := node.ChildByFieldName("body")
	if body == nil {
		return imports
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		var nameNode *tree_sitter.Node
		switch child.Kind() {
		case "function_signature_item", "function_item", "static_item":
			nameNode = child.ChildByFieldName("name")
		}
		if nameNode == nil {
			continue
		}
		imports = append(imports, Import{
			Name:         nodeText(nameNode, src),
			SourceModule: `extern "C"`,
			Kind:         "extern_c",
			Line:         int(child.StartPosition().Row) + 1,
		})
	}
	return imports
}

// rustExternABI reports the ABI string of an `extern "ABI" fn` modifier
// (e.g. "C"), and whether the function carries one at all. A bare `extern
// fn` with no string literal defaults to the "C" ABI.
func rustExternABI(node *tree_sitter.Node, src []byte) (string, bool) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "function_modifiers" {
			continue
		}
		for j := uint(0); j < child.NamedChildCount(); j++ {
			m := child.NamedChild(j)
			if m.Kind() == "extern_modifier" {
				if m.NamedChildCount() > 0 {
					return unquote(nodeText(m.NamedChild(0), src)), true
				}
				return "C", true
			}
		}
	}
	return "", false
}

// hardenRustImportKinds upgrades an import's casing-guessed Kind to the
// actual kind of a local definition sharing its tail name, when one was
// collected in this same file-analysis pass. Falls back to the casing
// heuristic already set on the import for anything the pass can't resolve
// locally (external-crate imports have no local definition to match).
func hardenRustImportKinds(imports []Import, defs []Definition) {
	if len(defs) == 0 {
		return
	}
	localKind := make(map[string]string, len(defs))
	for _, d := range defs {
		if _, exists := localKind[d.Name]; !exists {
			localKind[d.Name] = d.Kind
		}
	}
	for i := range imports {
		if kind, ok := localKind[imports[i].Name]; ok {
			imports[i].Kind = rustDefKindToImportKind(kind)
		}
	}
}

// markRustMacroReferences walks the whole tree for macro_invocation nodes
// and increments MacroReferenceCount on any local definition whose name
// appears as a bare identifier token inside the invocation's token tree.
// Declarative-macro-only: tree-sitter never sees what a macro_rules! macro
// expands to, only that a name was passed as a raw token where one was
// invoked, so this cannot see through proc-macro expansion at all.
func markRustMacroReferences(root *tree_sitter.Node, src []byte, defs []Definition) {
	if len(defs) == 0 {
		return
	}
	indexByName := make(map[string][]int, len(defs))
	for i, d := range defs {
		indexByName[d.Name] = append(indexByName[d.Name], i)
	}

	var walk func(node *tree_sitter.Node)
	walk = func(node *tree_sitter.Node) {
		if node.Kind() == "macro_invocation" {
			for i := uint(0); i < node.NamedChildCount(); i++ {
				child := node.NamedChild(i)
				if child.Kind() == "token_tree" {
					countRustTokenTreeRefs(child, src, indexByName, defs)
				}
			}
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			walk(node.NamedChild(i))
		}
	}
	walk(root)
}

// countRustTokenTreeRefs increments MacroReferenceCount for definitions whose
// name appears as a bare identifier inside a macro invocation's token tree.
// Only unambiguous matches count: if 2+ local definitions share the matched
// name, there is no way to tell which one the macro actually referenced, so
// the match is skipped rather than crediting all of them. A missed reference
// (false negative) is far cheaper here than crediting an unrelated definition
// (false positive), since a false positive would hide a real unused-import
// finding elsewhere.
func countRustTokenTreeRefs(node *tree_sitter.Node, src []byte, indexByName map[string][]int, defs []Definition) {
	if node.Kind() == "identifier" {
		name := nodeText(node, src)
		if idxs := indexByName[name]; len(idxs) == 1 {
			defs[idxs[0]].MacroReferenceCount++
		}
		return
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		countRustTokenTreeRefs(node.NamedChild(i), src, indexByName, defs)
	}
}

func rustDefKindToImportKind(defKind string) string {
	switch defKind {
	case "function", "method":
		return "function"
	case "type":
		return "type"
	case "module":
		return "module"
	case "constant":
		return "constant"
	default:
		return "unknown"
	}
}

// extractRustTopLevelItem dispatches a single top-level node to the
// extractor for its kind, appending any resulting definition or imports
// to fa. Split out of Analyze to keep the walk loop's cognitive
// complexity low despite the wide set of top-level item kinds.
func extractRustTopLevelItem(node *tree_sitter.Node, src []byte, fa *FileAnalysis) {
	switch node.Kind() {
	case "function_item":
		if def, ok := extractRustFuncItem(node, src, ""); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
	case "impl_item":
		fa.Definitions = append(fa.Definitions, extractRustImplMethods(node, src)...)
	case "struct_item", "enum_item", "trait_item":
		if def, ok := extractRustNamedItem(node, src, "type"); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
		if node.Kind() == "trait_item" {
			fa.Definitions = append(fa.Definitions, extractRustTraitMethods(node, src)...)
		}
	case "type_item":
		if def, ok := extractRustNamedItem(node, src, "type"); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
	case "mod_item":
		if def, ok := extractRustNamedItem(node, src, "module"); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
	case "const_item", "static_item":
		if def, ok := extractRustValueItem(node, src); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
	case "macro_definition":
		if def, ok := extractRustNamedItem(node, src, "function"); ok {
			fa.Definitions = append(fa.Definitions, def)
		}
	case "use_declaration":
		fa.Imports = append(fa.Imports, extractRustUse(node, src)...)
	case "foreign_mod_item":
		fa.Imports = append(fa.Imports, extractRustExternBlock(node, src)...)
	}
}
