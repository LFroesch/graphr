package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Symbol represents a function, method, type, or class in the codebase.
type Symbol struct {
	ID       string // "file:name"
	Name     string
	Kind     string // "fn", "method", "type", "interface", "class"
	File     string // relative path
	StartRow uint32
	EndRow   uint32
	Src      []byte
}

// Edge represents a call from one symbol to another.
type Edge struct {
	From     string // Symbol ID
	To       string // Symbol ID if Resolved, bare callee name otherwise
	Resolved bool   // false = external/stdlib call, not in project
}

// ParseResult holds the output of parsing a directory.
type ParseResult struct {
	Symbols []Symbol
	Edges   []Edge
	Files   []string // all supported files found
}

// ParseDir walks dir, parses all supported files, and builds symbols + edges.
func ParseDir(dir string) (*ParseResult, error) {
	result := &ParseResult{}
	symbolsByName := map[string][]string{} // name -> []IDs for resolution

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if !IsSupported(ext) {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		result.Files = append(result.Files, rel)

		if MarkdownExts[ext] {
			return nil // markdown: file tree only
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		symbols, edges, err := parseFile(rel, src, ext)
		if err != nil {
			return nil
		}

		for _, s := range symbols {
			symbolsByName[s.Name] = append(symbolsByName[s.Name], s.ID)
		}
		result.Symbols = append(result.Symbols, symbols...)
		result.Edges = append(result.Edges, edges...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Resolve edge targets: match callee name to symbol IDs
	result.Edges = resolveEdges(result.Edges, symbolsByName)

	return result, nil
}

func parseFile(relPath string, src []byte, ext string) ([]Symbol, []Edge, error) {
	lang := LangForExt(ext)
	if lang == nil {
		return nil, nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang.Language)

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, nil, err
	}
	root := tree.RootNode()

	var symbols []Symbol
	var edges []Edge

	// Parse function/method declarations
	if lang.FuncQuery != "" {
		q, qErr := sitter.NewQuery([]byte(lang.FuncQuery), lang.Language)
		if qErr == nil {
			qc := sitter.NewQueryCursor()
			qc.Exec(q, root)
			for {
				m, ok := qc.NextMatch()
				if !ok {
					break
				}
				sym := extractSymbol(m, q, relPath, src)
				if sym != nil {
					symbols = append(symbols, *sym)
				}
			}
		}
	}

	// Parse type declarations
	if lang.TypeQuery != "" {
		q, qErr := sitter.NewQuery([]byte(lang.TypeQuery), lang.Language)
		if qErr == nil {
			qc := sitter.NewQueryCursor()
			qc.Exec(q, root)
			for {
				m, ok := qc.NextMatch()
				if !ok {
					break
				}
				sym := extractTypeSymbol(m, q, relPath, src)
				if sym != nil {
					symbols = append(symbols, *sym)
				}
			}
		}
	}

	// Build a map of row ranges -> symbol ID for resolving call edges
	type rowRange struct {
		start, end uint32
		id         string
	}
	var fnRanges []rowRange
	for _, s := range symbols {
		if s.Kind == "fn" || s.Kind == "method" {
			fnRanges = append(fnRanges, rowRange{s.StartRow, s.EndRow, s.ID})
		}
	}

	// Parse call expressions
	if lang.CallQuery != "" {
		q, qErr := sitter.NewQuery([]byte(lang.CallQuery), lang.Language)
		if qErr == nil {
			qc := sitter.NewQueryCursor()
			qc.Exec(q, root)
			for {
				m, ok := qc.NextMatch()
				if !ok {
					break
				}
				callee, callRow := extractCall(m, q, src)
				if callee == "" {
					continue
				}
				// Find enclosing function
				from := ""
				for _, r := range fnRanges {
					if callRow >= r.start && callRow <= r.end {
						from = r.id
						break
					}
				}
				if from != "" {
					edges = append(edges, Edge{From: from, To: callee})
				}
			}
		}
	}

	return symbols, edges, nil
}

func extractSymbol(m *sitter.QueryMatch, q *sitter.Query, file string, src []byte) *Symbol {
	var name string
	var declNode *sitter.Node
	kind := "fn"

	for _, c := range m.Captures {
		capName := q.CaptureNameForId(c.Index)
		switch capName {
		case "fn.name":
			name = c.Node.Content(src)
			kind = "fn"
		case "method.name":
			name = c.Node.Content(src)
			kind = "method"
		case "fn.decl", "method.decl":
			declNode = c.Node
		}
	}

	if name == "" || declNode == nil {
		return nil
	}

	startRow := declNode.StartPoint().Row
	endRow := declNode.EndPoint().Row
	return &Symbol{
		ID:       file + ":" + name,
		Name:     name,
		Kind:     kind,
		File:     file,
		StartRow: startRow,
		EndRow:   endRow,
		Src:      src[declNode.StartByte():declNode.EndByte()],
	}
}

func extractTypeSymbol(m *sitter.QueryMatch, q *sitter.Query, file string, src []byte) *Symbol {
	var name string
	var declNode *sitter.Node

	for _, c := range m.Captures {
		capName := q.CaptureNameForId(c.Index)
		switch capName {
		case "type.name":
			name = c.Node.Content(src)
		case "type.decl":
			declNode = c.Node
		}
	}

	if name == "" || declNode == nil {
		return nil
	}

	return &Symbol{
		ID:       file + ":" + name,
		Name:     name,
		Kind:     "type",
		File:     file,
		StartRow: declNode.StartPoint().Row,
		EndRow:   declNode.EndPoint().Row,
		Src:      src[declNode.StartByte():declNode.EndByte()],
	}
}

func extractCall(m *sitter.QueryMatch, q *sitter.Query, src []byte) (string, uint32) {
	var callee string
	var row uint32

	for _, c := range m.Captures {
		capName := q.CaptureNameForId(c.Index)
		switch capName {
		case "call.fn":
			callee = c.Node.Content(src)
			row = c.Node.StartPoint().Row
		case "call.method":
			callee = c.Node.Content(src)
			row = c.Node.StartPoint().Row
		}
	}
	return callee, row
}

// resolveEdges replaces bare callee names with symbol IDs where possible.
// Edges that can't be resolved (external/stdlib) are kept with Resolved=false.
func resolveEdges(edges []Edge, symbolsByName map[string][]string) []Edge {
	var out []Edge
	for _, e := range edges {
		if e.Resolved {
			out = append(out, e)
			continue
		}
		ids := symbolsByName[e.To]
		if len(ids) == 1 {
			out = append(out, Edge{From: e.From, To: ids[0], Resolved: true})
		} else if len(ids) > 1 {
			// Ambiguous: prefer same file
			fromFile := strings.Split(e.From, ":")[0]
			matched := false
			for _, id := range ids {
				if strings.HasPrefix(id, fromFile+":") {
					out = append(out, Edge{From: e.From, To: id, Resolved: true})
					matched = true
					break
				}
			}
			if !matched {
				out = append(out, Edge{From: e.From, To: ids[0], Resolved: true})
			}
		} else {
			// No match — external/stdlib call, keep as unresolved
			out = append(out, Edge{From: e.From, To: e.To, Resolved: false})
		}
	}
	return out
}
