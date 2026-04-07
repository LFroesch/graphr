package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LanguageConfig defines how to parse a given language.
type LanguageConfig struct {
	Language    *sitter.Language
	Extensions []string
	FuncQuery  string // query to find function/method declarations
	CallQuery  string // query to find call expressions
	TypeQuery  string // query to find type declarations
}

var Languages = map[string]LanguageConfig{
	"go": {
		Language:   golang.GetLanguage(),
		Extensions: []string{".go"},
		FuncQuery: `
(function_declaration name: (identifier) @fn.name) @fn.decl
(method_declaration name: (field_identifier) @method.name) @method.decl
`,
		CallQuery: `
(call_expression
  function: [
    (identifier) @call.fn
    (selector_expression field: (field_identifier) @call.method)
  ]) @call.expr
`,
		TypeQuery: `
(type_declaration (type_spec name: (type_identifier) @type.name)) @type.decl
`,
	},
	"python": {
		Language:   python.GetLanguage(),
		Extensions: []string{".py"},
		FuncQuery: `
(function_definition name: (identifier) @fn.name) @fn.decl
(class_definition name: (identifier) @type.name) @type.decl
`,
		CallQuery: `
(call function: [
  (identifier) @call.fn
  (attribute attribute: (identifier) @call.method)
]) @call.expr
`,
		TypeQuery: `
(class_definition name: (identifier) @type.name) @type.decl
`,
	},
	"typescript": {
		Language:   typescript.GetLanguage(),
		Extensions: []string{".ts", ".tsx"},
		FuncQuery: `
(function_declaration name: (identifier) @fn.name) @fn.decl
(method_definition name: (property_identifier) @method.name) @method.decl
(lexical_declaration (variable_declarator name: (identifier) @fn.name value: (arrow_function) @fn.decl))
(lexical_declaration (variable_declarator name: (identifier) @fn.name value: (function_expression) @fn.decl))
(variable_declaration (variable_declarator name: (identifier) @fn.name value: (arrow_function) @fn.decl))
(variable_declaration (variable_declarator name: (identifier) @fn.name value: (function_expression) @fn.decl))
(class_declaration name: (type_identifier) @type.name) @type.decl
`,
		CallQuery: `
(call_expression function: [
  (identifier) @call.fn
  (member_expression property: (property_identifier) @call.method)
]) @call.expr
`,
		TypeQuery: `
(class_declaration name: (type_identifier) @type.name) @type.decl
(interface_declaration name: (type_identifier) @type.name) @type.decl
`,
	},
	"javascript": {
		Language:   javascript.GetLanguage(),
		Extensions: []string{".js", ".jsx"},
		FuncQuery: `
(function_declaration name: (identifier) @fn.name) @fn.decl
(method_definition name: (property_identifier) @method.name) @method.decl
(lexical_declaration (variable_declarator name: (identifier) @fn.name value: (arrow_function) @fn.decl))
(lexical_declaration (variable_declarator name: (identifier) @fn.name value: (function_expression) @fn.decl))
(variable_declaration (variable_declarator name: (identifier) @fn.name value: (arrow_function) @fn.decl))
(variable_declaration (variable_declarator name: (identifier) @fn.name value: (function_expression) @fn.decl))
(class_declaration name: (identifier) @type.name) @type.decl
`,
		CallQuery: `
(call_expression function: [
  (identifier) @call.fn
  (member_expression property: (property_identifier) @call.method)
]) @call.expr
`,
		TypeQuery: `
(class_declaration name: (identifier) @type.name) @type.decl
`,
	},
}

// LangForExt returns the language config for a file extension, or nil.
func LangForExt(ext string) *LanguageConfig {
	for _, cfg := range Languages {
		for _, e := range cfg.Extensions {
			if e == ext {
				return &cfg
			}
		}
	}
	return nil
}

// MarkdownExts are shown in file tree + preview but not parsed for graph.
var MarkdownExts = map[string]bool{".md": true, ".mdx": true}

// IsSupported returns true if the extension is parseable or viewable.
func IsSupported(ext string) bool {
	return LangForExt(ext) != nil || MarkdownExts[ext]
}
