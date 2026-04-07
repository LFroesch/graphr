# graphr

TUI call-graph visualizer. Point it at a codebase, see functions and how they connect.

## Languages
Go, Python, TypeScript, JavaScript (Markdown files shown in tree, not graphed)

## Requirements
- Go 1.23+
- C compiler (tree-sitter uses CGO)

## Build & Run

```bash
cd graphr
go mod tidy
go build -o graphr ./cmd/graphr
./graphr /path/to/project
```

## Keybindings

| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `h/l` or `tab` | Switch panels |
| `enter` | Jump to symbol in graph |
| `/` | Search symbols |
| `t` | Trace path between two nodes |
| `r` | Reload/re-parse |
| `q` | Quit |

## Layout

```
┌─ files ──────┬─── call graph ────────────────┬─ info ──────────┐
│ main.go      │  [nodes + directed edges]     │ handleRequest   │
│ handler.go   │  ASCII rendering              │ handler.go:14   │
├─ symbols ────│  arrow keys navigate          │ CALLS / CALLED  │
│ fn  main     │                               │ BY + source     │
└──────────────┴───────────────────────────────┴─────────────────┘
```
