package graph

import (
	"testing"

	"github.com/LFroesch/graphr/internal/parser"
)

func testSymbols() []parser.Symbol {
	return []parser.Symbol{
		{ID: "main.go:main", Name: "main", Kind: "fn", File: "main.go"},
		{ID: "main.go:startServer", Name: "startServer", Kind: "fn", File: "main.go"},
		{ID: "handler.go:handleRequest", Name: "handleRequest", Kind: "fn", File: "handler.go"},
		{ID: "handler.go:parseBody", Name: "parseBody", Kind: "fn", File: "handler.go"},
		{ID: "handler.go:validateInput", Name: "validateInput", Kind: "fn", File: "handler.go"},
	}
}

func testEdges() []parser.Edge {
	return []parser.Edge{
		{From: "main.go:main", To: "main.go:startServer", Resolved: true},
		{From: "main.go:startServer", To: "handler.go:handleRequest", Resolved: true},
		{From: "handler.go:handleRequest", To: "handler.go:parseBody", Resolved: true},
		{From: "handler.go:handleRequest", To: "handler.go:validateInput", Resolved: true},
	}
}

func TestBuild(t *testing.T) {
	g, err := Build(testSymbols(), testEdges())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if n := NodeCount(g); n != 5 {
		t.Errorf("expected 5 nodes, got %d", n)
	}
	if e := EdgeCount(g); e != 4 {
		t.Errorf("expected 4 edges, got %d", e)
	}
}

func TestTracePaths(t *testing.T) {
	g, _ := Build(testSymbols(), testEdges())

	paths := TracePaths(g, "main.go:main", "handler.go:parseBody", 20, 10)
	if len(paths) != 1 {
		t.Errorf("expected 1 path, got %d", len(paths))
	}
	if len(paths) > 0 && len(paths[0]) != 4 {
		t.Errorf("expected path length 4, got %d: %v", len(paths[0]), paths[0])
	}

	// No path
	paths = TracePaths(g, "handler.go:parseBody", "main.go:main", 20, 10)
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestNeighbours(t *testing.T) {
	g, _ := Build(testSymbols(), testEdges())

	callers, callees := Neighbours(g, "handler.go:handleRequest")
	if len(callers) != 1 {
		t.Errorf("expected 1 caller, got %d", len(callers))
	}
	if len(callees) != 2 {
		t.Errorf("expected 2 callees, got %d", len(callees))
	}
}
