# graphr

TUI call-graph visualizer. Point it at a codebase, see functions and how they connect.

## Quick Install

Supported platforms: Linux and macOS. On Windows, use WSL.

Recommended (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/graphr/main/install.sh | bash
```

Or download a binary from [GitHub Releases](https://github.com/LFroesch/graphr/releases).

Or install with Go:

```bash
go install github.com/LFroesch/graphr@latest
```

Or build from source:

```bash
make install
```

Command:

```bash
graphr
```

Note: automated releases currently publish Linux amd64 only (tree-sitter/CGO).
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
