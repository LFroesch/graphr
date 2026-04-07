package graph

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LFroesch/graphr/internal/parser"
)

// repoRoot returns the graphr repo root regardless of where the test runs from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../graphr/internal/graph/integration_test.go
	return filepath.Join(filepath.Dir(file), "../..")
}

func TestParseGraphrRepo(t *testing.T) {
	root := repoRoot(t)
	result, err := parser.ParseDir(root)
	if err != nil {
		t.Fatalf("ParseDir failed: %v", err)
	}

	if len(result.Symbols) == 0 {
		t.Fatal("no symbols found — parser is broken")
	}
	if len(result.Files) == 0 {
		t.Fatal("no files found")
	}

	// Count resolved vs unresolved edges
	resolved, unresolved := 0, 0
	for _, e := range result.Edges {
		if e.Resolved {
			resolved++
		} else {
			unresolved++
		}
	}
	t.Logf("files=%d  symbols=%d  edges resolved=%d unresolved=%d",
		len(result.Files), len(result.Symbols), resolved, unresolved)

	if resolved == 0 {
		t.Error("zero resolved edges — cross-file resolution is broken")
	}

	// Verify key symbols exist
	symMap := map[string]bool{}
	for _, s := range result.Symbols {
		symMap[s.ID] = true
	}
	mustExist := []string{
		"cmd/graphr/main.go:main",
		"internal/tui/model.go:New",
		"internal/parser/parser.go:ParseDir",
		"internal/graph/graph.go:Build",
		"internal/graph/graph.go:TracePaths",
	}
	for _, id := range mustExist {
		if !symMap[id] {
			t.Errorf("expected symbol %q not found", id)
		}
	}

	// Verify cross-file edges exist
	edgeMap := map[string]map[string]bool{}
	for _, e := range result.Edges {
		if !e.Resolved {
			continue
		}
		if edgeMap[e.From] == nil {
			edgeMap[e.From] = map[string]bool{}
		}
		edgeMap[e.From][e.To] = true
	}

	// main → tui.New (call.method resolves "New" → internal/tui/model.go:New)
	if !edgeMap["cmd/graphr/main.go:main"]["internal/tui/model.go:New"] {
		t.Error("missing edge: main → tui.New")
	}
	// tui.New → parser.ParseDir
	if !edgeMap["internal/tui/model.go:New"]["internal/parser/parser.go:ParseDir"] {
		t.Error("missing edge: tui.New → parser.ParseDir")
	}
}

func TestCrossFileTracePaths(t *testing.T) {
	root := repoRoot(t)
	result, err := parser.ParseDir(root)
	if err != nil {
		t.Fatalf("ParseDir failed: %v", err)
	}

	g, err := Build(result.Symbols, result.Edges)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Trace from main → ParseDir (crosses 3 files)
	paths := TracePaths(g, "cmd/graphr/main.go:main", "internal/parser/parser.go:ParseDir", 20, 10)
	if len(paths) == 0 {
		t.Error("TracePaths found no path from main → ParseDir — cross-file trace is broken")
	} else {
		t.Logf("found %d path(s) from main → ParseDir:", len(paths))
		for _, p := range paths {
			t.Logf("  %v", p)
		}
	}
}
