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

## Editor

`o` opens the selected symbol in your editor. Resolution order: `$VISUAL` → `$EDITOR` → cursor → code → nvim → vim → nano → vi.

```bash
export EDITOR=nvim   # terminal editor
export VISUAL=cursor # GUI editor (checked first)
```

## License

[AGPL-3.0](LICENSE)