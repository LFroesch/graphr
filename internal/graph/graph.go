package graph

import (
	"github.com/LFroesch/graphr/internal/parser"
	gr "github.com/dominikbraun/graph"
)

// Build creates a directed graph from parsed symbols and edges.
func Build(symbols []parser.Symbol, edges []parser.Edge) (gr.Graph[string, parser.Symbol], error) {
	g := gr.New(func(s parser.Symbol) string { return s.ID }, gr.Directed())

	for _, s := range symbols {
		_ = g.AddVertex(s)
	}

	for _, e := range edges {
		if !e.Resolved {
			continue // external calls don't go in the graph
		}
		_, errFrom := g.Vertex(e.From)
		_, errTo := g.Vertex(e.To)
		if errFrom == nil && errTo == nil {
			_ = g.AddEdge(e.From, e.To)
		}
	}

	return g, nil
}

// TracePaths finds paths from `from` to `to` using BFS, capped at maxPaths/maxDepth.
func TracePaths(g gr.Graph[string, parser.Symbol], from, to string, maxPaths, maxDepth int) [][]string {
	if maxPaths <= 0 {
		maxPaths = 20
	}
	if maxDepth <= 0 {
		maxDepth = 10
	}

	var results [][]string
	type state struct {
		path []string
	}

	adj, err := g.AdjacencyMap()
	if err != nil {
		return nil
	}

	queue := []state{{path: []string{from}}}
	for len(queue) > 0 && len(results) < maxPaths {
		cur := queue[0]
		queue = queue[1:]

		node := cur.path[len(cur.path)-1]
		if node == to {
			pathCopy := make([]string, len(cur.path))
			copy(pathCopy, cur.path)
			results = append(results, pathCopy)
			continue
		}

		if len(cur.path) >= maxDepth {
			continue
		}

		for target := range adj[node] {
			// Avoid cycles
			visited := false
			for _, p := range cur.path {
				if p == target {
					visited = true
					break
				}
			}
			if !visited {
				newPath := make([]string, len(cur.path)+1)
				copy(newPath, cur.path)
				newPath[len(cur.path)] = target
				queue = append(queue, state{path: newPath})
			}
		}
	}

	return results
}

// Neighbours returns direct callers and callees of a node.
func Neighbours(g gr.Graph[string, parser.Symbol], id string) (callers, callees []string) {
	// Callees: nodes this one calls (outgoing edges)
	adj, err := g.AdjacencyMap()
	if err == nil {
		for target := range adj[id] {
			callees = append(callees, target)
		}
	}

	// Callers: nodes that call this one (incoming edges)
	pred, err := g.PredecessorMap()
	if err == nil {
		for source := range pred[id] {
			callers = append(callers, source)
		}
	}

	return
}

// NodeCount returns the number of vertices.
func NodeCount(g gr.Graph[string, parser.Symbol]) int {
	adj, err := g.AdjacencyMap()
	if err != nil {
		return 0
	}
	return len(adj)
}

// EdgeCount returns the number of edges.
func EdgeCount(g gr.Graph[string, parser.Symbol]) int {
	count := 0
	adj, err := g.AdjacencyMap()
	if err != nil {
		return 0
	}
	for _, targets := range adj {
		count += len(targets)
	}
	return count
}
