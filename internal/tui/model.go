package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gr "github.com/dominikbraun/graph"
	"github.com/fsnotify/fsnotify"

	graphpkg "github.com/LFroesch/graphr/internal/graph"
	"github.com/LFroesch/graphr/internal/parser"
)

// Terminal constraints
const (
	minWidth  = 60
	minHeight = 15
	// header(1) + status(1) + panel top-border(1) + panel bottom-border(1)
	uiOverhead = 4
)

type panel int

const (
	panelFiles panel = iota
	panelSymbols
	panelInfo
)

type graphReloadedMsg struct{ result *parser.ParseResult }
type fileChangedMsg struct{}
type errMsg struct{ err error }

// infoLine is a single rendered line in the info panel.
// symID non-empty means pressing enter navigates to that symbol.
type infoLine struct {
	text  string
	symID string // non-empty = navigable
	kind  string // "title", "dim", "highlight", ""
}

type Model struct {
	dir     string
	graph   gr.Graph[string, parser.Symbol]
	symbols []parser.Symbol
	edges   []parser.Edge
	files   []string

	focus      panel
	fileIdx    int
	symIdx     int
	fileScroll int
	symScroll  int
	infoScroll int

	filteredSyms []parser.Symbol

	search    textinput.Model
	searching bool

	traceFrom string
	traceTo   string
	tracePath [][]string

	showHelp bool

	infoLines  []infoLine
	infoCursor int // line index into infoLines

	srcLines  []string
	srcScroll int

	panelH int // inner panel height, kept in sync with WindowSizeMsg

	width  int
	height int

	watcher  *fsnotify.Watcher
	lastLoad time.Time
	err      error
}

func New(dir string) (*Model, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	result, err := parser.ParseDir(absDir)
	if err != nil {
		return nil, err
	}

	ti := textinput.New()
	ti.Placeholder = "search..."

	m := &Model{
		dir:      absDir,
		search:   ti,
		lastLoad: time.Now(),
	}
	m.loadResult(result)

	w, err := fsnotify.NewWatcher()
	if err == nil {
		m.watcher = w
		_ = w.Add(absDir)
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" || name == "__pycache__" || name == ".venv" {
					return filepath.SkipDir
				}
				_ = w.Add(path)
			}
			return nil
		})
	}

	return m, nil
}

func (m *Model) loadResult(result *parser.ParseResult) {
	g, _ := graphpkg.Build(result.Symbols, result.Edges)
	m.graph = g
	m.symbols = result.Symbols
	m.edges = result.Edges
	m.files = result.Files
	sort.Strings(m.files)

	if m.fileIdx >= len(m.files) {
		m.fileIdx = 0
	}
	m.updateFilteredSyms()
}

func (m *Model) updateFilteredSyms() {
	m.filteredSyms = nil
	if m.fileIdx >= len(m.files) {
		return
	}
	file := m.files[m.fileIdx]
	for _, s := range m.symbols {
		if s.File == file {
			m.filteredSyms = append(m.filteredSyms, s)
		}
	}
	if m.symIdx >= len(m.filteredSyms) {
		m.symIdx = 0
		m.symScroll = 0
	}
	m.buildInfoLines()
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.SetWindowTitle("graphr"),
	}
	if m.watcher != nil {
		cmds = append(cmds, watchFiles(m.watcher))
	}
	return tea.Batch(cmds...)
}

func watchFiles(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return nil
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
					if parser.IsSupported(filepath.Ext(ev.Name)) {
						return fileChangedMsg{}
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return nil
				}
				return errMsg{err}
			}
		}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		if msg.Width == m.width && msg.Height == m.height {
			return m, nil
		}
		m.width = msg.Width
		m.height = msg.Height
		if m.width < minWidth {
			m.width = minWidth
		}
		if m.height < minHeight {
			m.height = minHeight
		}
		m.panelH = m.height - uiOverhead
		if m.panelH < 5 {
			m.panelH = 5
		}
		return m, nil
	case fileChangedMsg:
		return m, m.reload()
	case graphReloadedMsg:
		m.loadResult(msg.result)
		m.lastLoad = time.Now()
		m.err = nil
		return m, watchFiles(m.watcher)
	case errMsg:
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

func (m *Model) reload() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		result, err := parser.ParseDir(dir)
		if err != nil {
			return errMsg{err}
		}
		return graphReloadedMsg{result}
	}
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		if m.watcher != nil {
			m.watcher.Close()
		}
		return m, tea.Quit
	}

	if m.showHelp {
		if key == "?" || key == "esc" || key == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.search.Blur()
		case "enter":
			m.searching = false
			m.search.Blur()
			m.jumpToSearch()
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Trace mode: intercept T/esc/t; all other keys fall through to normal nav
	if m.traceFrom != "" {
		switch key {
		case "esc":
			m.traceFrom = ""
			m.traceTo = ""
			m.tracePath = nil
			m.buildInfoLines()
			return m, nil
		case "T":
			m.traceTo = m.selectedSymID()
			m.tracePath = graphpkg.TracePaths(m.graph, m.traceFrom, m.traceTo, 20, 10)
			m.buildInfoLines()
			return m, nil
		case "t":
			m.traceFrom = m.selectedSymID()
			m.traceTo = ""
			m.tracePath = nil
			m.buildInfoLines()
			return m, nil
		}
		// fall through to normal navigation so you can browse files/symbols freely
	}

	switch key {
	case "q":
		if m.watcher != nil {
			m.watcher.Close()
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		m.searching = true
		m.search.Focus()
		return m, textinput.Blink
	case "t":
		m.traceFrom = m.selectedSymID()
		m.traceTo = ""
		m.tracePath = nil
		m.buildInfoLines()
		m.focus = panelSymbols
	case "r":
		return m, m.reload()
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
	case "h":
		if m.focus > 0 {
			m.focus--
		}
	case "l":
		if m.focus < panelInfo {
			m.focus++
		}
	case "j", "down":
		m.moveDown()
	case "k", "up":
		m.moveUp()
	case "enter":
		switch m.focus {
		case panelFiles:
			m.focus = panelSymbols
		case panelSymbols:
			m.focus = panelInfo
			m.buildInfoLines()
		case panelInfo:
			if m.infoCursor < len(m.infoLines) {
				line := m.infoLines[m.infoCursor]
				if line.symID != "" {
					m.navigateToSym(line.symID)
				}
			}
		}
	case "[":
		if m.focus == panelInfo && m.srcScroll > 0 {
			m.srcScroll--
		}
	case "]":
		if m.focus == panelInfo {
			m.srcScroll++
		}
	case "o":
		var sym *parser.Symbol
		if m.focus == panelInfo && m.infoCursor < len(m.infoLines) {
			line := m.infoLines[m.infoCursor]
			if line.symID != "" {
				sym = m.findSymbol(line.symID)
			}
		}
		if sym == nil {
			sym = m.findSymbol(m.selectedSymID())
		}
		if sym != nil {
			return m, openInEditor(filepath.Join(m.dir, sym.File), int(sym.StartRow))
		}
	case "G":
		switch m.focus {
		case panelFiles:
			if len(m.files) > 0 {
				m.fileIdx = len(m.files) - 1
				m.fileScroll = clamp(m.fileIdx-m.panelH+3, 0, len(m.files))
				m.updateFilteredSyms()
			}
		case panelSymbols:
			if len(m.filteredSyms) > 0 {
				m.symIdx = len(m.filteredSyms) - 1
				m.symScroll = clamp(m.symIdx-m.panelH+3, 0, len(m.filteredSyms))
				m.buildInfoLines()
			}
		case panelInfo:
			if len(m.infoLines) > 0 {
				m.infoCursor = len(m.infoLines) - 1
				for m.infoCursor > 0 && m.infoLines[m.infoCursor].kind == "title" {
					m.infoCursor--
				}
				m.infoScroll = clamp(m.infoCursor-m.panelH+1, 0, len(m.infoLines))
			}
		}
	case "g":
		switch m.focus {
		case panelFiles:
			m.fileIdx = 0
			m.fileScroll = 0
			m.updateFilteredSyms()
		case panelSymbols:
			m.symIdx = 0
			m.symScroll = 0
			m.buildInfoLines()
		case panelInfo:
			m.infoCursor = 0
			for m.infoCursor < len(m.infoLines)-1 && m.infoLines[m.infoCursor].kind == "title" {
				m.infoCursor++
			}
			m.infoScroll = 0
		}
	}
	return m, nil
}

func (m *Model) moveDown() {
	switch m.focus {
	case panelFiles:
		if m.fileIdx < len(m.files)-1 {
			m.fileIdx++
			if m.panelH > 0 && m.fileIdx >= m.fileScroll+m.panelH-2 {
				m.fileScroll = m.fileIdx - m.panelH + 3
			}
			m.updateFilteredSyms()
		}
	case panelSymbols:
		if m.symIdx < len(m.filteredSyms)-1 {
			m.symIdx++
			if m.panelH > 0 && m.symIdx >= m.symScroll+m.panelH-2 {
				m.symScroll = m.symIdx - m.panelH + 3
			}
			m.buildInfoLines()
		}
	case panelInfo:
		next := m.infoCursor + 1
		for next < len(m.infoLines) && m.infoLines[next].kind == "title" {
			next++
		}
		if next < len(m.infoLines) {
			m.infoCursor = next
			if m.panelH > 0 && m.infoCursor >= m.infoScroll+m.panelH {
				m.infoScroll = m.infoCursor - m.panelH + 1
			}
		}
	}
}

func (m *Model) moveUp() {
	switch m.focus {
	case panelFiles:
		if m.fileIdx > 0 {
			m.fileIdx--
			if m.fileIdx < m.fileScroll {
				m.fileScroll = m.fileIdx
			}
			m.updateFilteredSyms()
		}
	case panelSymbols:
		if m.symIdx > 0 {
			m.symIdx--
			if m.symIdx < m.symScroll {
				m.symScroll = m.symIdx
			}
			m.buildInfoLines()
		}
	case panelInfo:
		prev := m.infoCursor - 1
		for prev >= 0 && m.infoLines[prev].kind == "title" {
			prev--
		}
		if prev >= 0 {
			m.infoCursor = prev
			if m.infoCursor < m.infoScroll {
				m.infoScroll = m.infoCursor
			}
		}
	}
}

func (m *Model) selectedSymID() string {
	if m.symIdx < len(m.filteredSyms) {
		return m.filteredSyms[m.symIdx].ID
	}
	return ""
}

func (m *Model) jumpToSearch() {
	query := strings.ToLower(m.search.Value())
	if query == "" {
		return
	}
	for i, f := range m.files {
		if strings.Contains(strings.ToLower(f), query) {
			m.fileIdx = i
			m.fileScroll = 0
			m.updateFilteredSyms()
			m.focus = panelFiles
			return
		}
	}
	for _, s := range m.symbols {
		if strings.Contains(strings.ToLower(s.Name), query) {
			for i, f := range m.files {
				if f == s.File {
					m.fileIdx = i
					m.fileScroll = 0
					m.updateFilteredSyms()
					for k, fs := range m.filteredSyms {
						if fs.ID == s.ID {
							m.symIdx = k
							m.symScroll = 0
							break
						}
					}
					m.focus = panelSymbols
					m.buildInfoLines()
					return
				}
			}
		}
	}
}

// --- buildInfoLines ---

func (m *Model) buildInfoLines() {
	m.infoLines = nil
	m.infoCursor = 0
	m.infoScroll = 0

	id := m.selectedSymID()
	if id == "" {
		return
	}
	sym := m.findSymbol(id)
	if sym == nil {
		return
	}

	push := func(l infoLine) { m.infoLines = append(m.infoLines, l) }
	title := func(t string) { push(infoLine{text: t, kind: "title"}) }
	dim := func(t string) { push(infoLine{text: t, kind: "dim"}) }
	plain := func(t string) { push(infoLine{text: t}) }
	nav := func(prefix, name, symID string) { push(infoLine{text: prefix + name, symID: symID}) }

	title(sym.Name)
	dim(fmt.Sprintf("%s:%d  [%s]", sym.File, sym.StartRow+1, sym.Kind))
	plain("")

	callers, callees := graphpkg.Neighbours(m.graph, id)

	var externalCalls []string
	seen := map[string]bool{}
	for _, e := range m.edges {
		if e.From == id && !e.Resolved && !seen[e.To] {
			seen[e.To] = true
			externalCalls = append(externalCalls, e.To)
		}
	}

	title("CALLS")
	if len(callees) == 0 && len(externalCalls) == 0 {
		dim("  (none)")
	}
	for _, c := range callees {
		nav("→ ", shortName(c), c)
	}
	for _, c := range externalCalls {
		push(infoLine{text: "~ " + c, kind: "dim"})
	}

	plain("")
	title("CALLED BY")
	if len(callers) == 0 {
		dim("  (none)")
	}
	for _, c := range callers {
		nav("← ", shortName(c), c)
	}

	if m.traceTo != "" {
		plain("")
		if len(m.tracePath) > 0 {
			push(infoLine{text: fmt.Sprintf("TRACE → %s", shortName(m.traceTo)), kind: "highlight"})
			for i, path := range m.tracePath {
				if i >= 6 {
					dim(fmt.Sprintf("  +%d more", len(m.tracePath)-6))
					break
				}
				var parts []string
				for _, n := range path {
					parts = append(parts, shortName(n))
				}
				plain("  " + strings.Join(parts, " → "))
			}
		} else {
			push(infoLine{text: fmt.Sprintf("TRACE → %s  (no path found)", shortName(m.traceTo)), kind: "dim"})
		}
	}

	// Populate source lines for the bottom section
	m.srcLines = nil
	m.srcScroll = 0
	if len(sym.Src) > 0 {
		for i, l := range strings.Split(strings.TrimRight(string(sym.Src), "\n"), "\n") {
			lineNum := int(sym.StartRow) + i + 1
			m.srcLines = append(m.srcLines, fmt.Sprintf("%4d  %s", lineNum, expandTabs(l)))
		}
	}

	// Start cursor on first non-title line
	for m.infoCursor < len(m.infoLines)-1 && m.infoLines[m.infoCursor].kind == "title" {
		m.infoCursor++
	}
}

// --- View ---

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("33")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("33"))

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	kindStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).Bold(true)

	statusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			Padding(1)

	navItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	if m.showHelp {
		return m.viewHelp()
	}

	if m.width < minWidth || m.height < minHeight {
		return warnStyle.Render(fmt.Sprintf(
			"terminal too small: %dx%d\nminimum: %dx%d\nplease resize",
			m.width, m.height, minWidth, minHeight,
		))
	}

	header := m.viewHeader()
	status := m.viewStatus()

	panelH := m.height - uiOverhead
	if panelH < 5 {
		panelH = 5
	}

	// Panel widths — proportional to terminal
	fileW := clamp(m.width/5, 18, 36)
	infoW := clamp(m.width*2/5, 30, 70)
	symW := m.width - fileW - infoW - 6 // 3 panels × 2 border chars
	if symW < 20 {
		symW = 20
	}

	files := m.viewFiles(fileW, panelH)
	syms := m.viewSymbols(symW, panelH)
	info := m.viewInfo(infoW, panelH)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, files, syms, info)

	return lipgloss.JoinVertical(lipgloss.Left, header, panels, status)
}

var helpStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("33")).
	Padding(1, 3)

func (m Model) viewHelp() string {
	lines := []string{
		titleStyle.Render("graphr — call graph browser"),
		"",
		dimStyle.Render("What it does:"),
		"  Parses your codebase and builds a call graph — shows which functions",
		"  call which other functions. Browse files, symbols, and connections.",
		"",
		titleStyle.Render("LAYOUT"),
		"  FILES │ SYMBOLS │ INFO",
		"  Files in your project │ Functions in selected file │ Calls + source",
		"",
		titleStyle.Render("NAVIGATION"),
		dimStyle.Render("  tab / shift+tab") + "  cycle panels",
		dimStyle.Render("  h / l         ") + "  move left/right panel",
		dimStyle.Render("  j / k         ") + "  move up/down in panel",
		dimStyle.Render("  enter         ") + "  select / jump to symbol",
		dimStyle.Render("  g / G         ") + "  jump to top / bottom",
		dimStyle.Render("  /             ") + "  search files and symbols",
		dimStyle.Render("  o             ") + "  open symbol in editor",
		dimStyle.Render("  r             ") + "  reload (also auto-reloads on save)",
		"",
		titleStyle.Render("TRACE MODE  — find call paths between two functions"),
		"  Example: does handleRequest eventually call writeDB?",
		"",
		dimStyle.Render("  1. Navigate to the FROM function (any file), press  ") + highlightStyle.Render("t"),
		dimStyle.Render("     (a ★ marks it in the symbols panel)"),
		dimStyle.Render("  2. Navigate to the TO function (any file), press    ") + highlightStyle.Render("T"),
		dimStyle.Render("     You can switch files freely — full navigation works in trace mode"),
		dimStyle.Render("     graphr BFS-searches all paths from FROM → TO"),
		dimStyle.Render("  3. Paths appear in the info panel under TRACE PATHS"),
		dimStyle.Render("     Symbols on any path are highlighted in the list"),
		dimStyle.Render("  esc  clear trace"),
		"",
		dimStyle.Render("  Press ") + highlightStyle.Render("?") + dimStyle.Render(" or ") + highlightStyle.Render("esc") + dimStyle.Render(" to close this help"),
	}

	content := strings.Join(lines, "\n")
	box := helpStyle.Render(content)

	// Center in terminal
	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	padLeft := (m.width - bw) / 2
	padTop := (m.height - bh) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}
	topPad := strings.Repeat("\n", padTop)
	leftPad := strings.Repeat(" ", padLeft)

	var out strings.Builder
	out.WriteString(topPad)
	for i, line := range strings.Split(box, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(leftPad + line)
	}
	return out.String()
}

func (m Model) viewHeader() string {
	nodes := graphpkg.NodeCount(m.graph)
	edges := graphpkg.EdgeCount(m.graph)

	right := fmt.Sprintf("%d syms · %d edges · %d files", nodes, edges, len(m.files))
	rightRender := headerStyle.Render(right)
	rightW := lipgloss.Width(rightRender)

	maxLeft := m.width - rightW
	if maxLeft < 10 {
		maxLeft = 10
	}
	leftText := truncate("graphr  "+m.dir, maxLeft-2)
	leftRender := headerStyle.Render(leftText)
	leftW := lipgloss.Width(leftRender)

	gap := m.width - leftW - rightW
	if gap < 0 {
		gap = 0
	}

	return leftRender + headerStyle.Render(strings.Repeat(" ", gap)) + rightRender
}

// treeEntry is used to build the file tree display
type treeEntry struct {
	isDir   bool
	label   string
	fileIdx int // index into m.files; -1 for dir headers
	depth   int
}

func buildTreeEntries(files []string) []treeEntry {
	var entries []treeEntry
	seenDirs := map[string]bool{}

	for i, f := range files {
		dir := filepath.Dir(f)
		if dir != "." && !seenDirs[dir] {
			parts := strings.Split(dir, string(os.PathSeparator))
			cumulative := ""
			for d, part := range parts {
				if cumulative == "" {
					cumulative = part
				} else {
					cumulative = cumulative + string(os.PathSeparator) + part
				}
				if !seenDirs[cumulative] {
					seenDirs[cumulative] = true
					entries = append(entries, treeEntry{
						isDir:   true,
						label:   part + "/",
						fileIdx: -1,
						depth:   d,
					})
				}
			}
		}
		depth := 0
		if dir != "." {
			depth = len(strings.Split(dir, string(os.PathSeparator)))
		}
		entries = append(entries, treeEntry{
			isDir:   false,
			label:   filepath.Base(f),
			fileIdx: i,
			depth:   depth,
		})
	}
	return entries
}

func (m Model) viewFiles(w, h int) string {
	style := borderStyle
	if m.focus == panelFiles {
		style = activeBorder
	}
	style = style.Width(w).Height(h)

	var b strings.Builder
	b.WriteString(titleStyle.Render("FILES") + "\n")

	entries := buildTreeEntries(m.files)

	// Find display row for selected file
	selectedRow := -1
	for i, e := range entries {
		if !e.isDir && e.fileIdx == m.fileIdx {
			selectedRow = i
			break
		}
	}

	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	scroll := m.fileScroll
	if selectedRow >= 0 {
		scroll = scrollOffset(selectedRow, scroll, visibleH, len(entries))
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + visibleH
	if end > len(entries) {
		end = len(entries)
	}

	maxLabel := w - 4
	for i := scroll; i < end; i++ {
		e := entries[i]
		indent := strings.Repeat("  ", e.depth)
		label := truncate(e.label, maxLabel-len(indent))

		if e.isDir {
			b.WriteString(dimStyle.Render(indent+label) + "\n")
		} else if e.fileIdx == m.fileIdx {
			b.WriteString(cursorStyle.Render(indent+"▸ "+label) + "\n")
		} else {
			b.WriteString(indent + dimStyle.Render(label) + "\n")
		}
	}

	if len(m.files) > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d/%d", m.fileIdx+1, len(m.files))))
	}

	return style.Render(b.String())
}

func (m Model) viewSymbols(w, h int) string {
	style := borderStyle
	if m.focus == panelSymbols {
		style = activeBorder
	}
	style = style.Width(w).Height(h)

	var b strings.Builder

	fileName := ""
	if m.fileIdx < len(m.files) {
		fileName = filepath.Base(m.files[m.fileIdx])
	}
	titleText := titleStyle.Render("SYMBOLS")
	fileLabel := dimStyle.Render(" " + truncate(fileName, w-10))
	// Show FROM indicator when tracing and FROM is in a different file
	fromLabel := ""
	if m.traceFrom != "" {
		fromFile := strings.SplitN(m.traceFrom, ":", 2)[0]
		currentFile := ""
		if m.fileIdx < len(m.files) {
			currentFile = m.files[m.fileIdx]
		}
		if fromFile != currentFile {
			fromLabel = highlightStyle.Render(" ★" + shortName(m.traceFrom))
		}
	}
	b.WriteString(titleText + fileLabel + fromLabel + "\n")

	if len(m.filteredSyms) == 0 {
		b.WriteString(dimStyle.Render("\n  (no symbols)"))
		return style.Render(b.String())
	}

	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	scroll := scrollOffset(m.symIdx, m.symScroll, visibleH, len(m.filteredSyms))

	end := scroll + visibleH
	if end > len(m.filteredSyms) {
		end = len(m.filteredSyms)
	}

	traceNodes := map[string]bool{}
	for _, path := range m.tracePath {
		for _, n := range path {
			traceNodes[n] = true
		}
	}

	maxName := w - 8
	for i := scroll; i < end; i++ {
		s := m.filteredSyms[i]
		kind := kindTag(s.Kind)
		line := fmt.Sprintf(":%d", s.StartRow+1)
		name := truncate(s.Name, maxName)

		fromMarker := " "
		if s.ID == m.traceFrom {
			fromMarker = highlightStyle.Render("★")
		}

		if i == m.symIdx && m.focus == panelSymbols {
			b.WriteString(cursorStyle.Render("▸") + fromMarker + cursorStyle.Render(kind+" "+name) + dimStyle.Render(line) + "\n")
		} else if traceNodes[s.ID] {
			b.WriteString(" " + fromMarker + highlightStyle.Render(kind+" "+name) + dimStyle.Render(line) + "\n")
		} else {
			b.WriteString(" " + fromMarker + kindStyle.Render(kind) + " " + name + dimStyle.Render(line) + "\n")
		}
	}

	if len(m.filteredSyms) > visibleH {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d/%d", m.symIdx+1, len(m.filteredSyms))))
	}

	return style.Render(b.String())
}

func (m Model) viewInfo(w, h int) string {
	style := borderStyle
	if m.focus == panelInfo {
		style = activeBorder
	}
	style = style.Width(w).Height(h)

	innerH := h - 2 // subtract top+bottom border
	innerW := w - 2

	if len(m.infoLines) == 0 {
		return style.Render(dimStyle.Render("select a symbol"))
	}

	// Split: top = calls/calledby/trace (~60%), bottom = source (~40%, capped at content)
	hasSrc := len(m.srcLines) > 0
	topH := innerH
	botH := 0
	if hasSrc {
		botH = min(len(m.srcLines), innerH*2/5)
		if botH < 3 {
			botH = 3
		}
		topH = innerH - botH - 1 // 1 for separator line
		if topH < 4 {
			topH = 4
			botH = innerH - topH - 1
			if botH < 1 {
				botH = 1
			}
		}
	}

	// --- Top section: calls/calledby/trace ---
	maxTopScroll := len(m.infoLines) - topH
	if maxTopScroll < 0 {
		maxTopScroll = 0
	}
	topScroll := clamp(m.infoScroll, 0, maxTopScroll)
	topEnd := topScroll + topH
	if topEnd > len(m.infoLines) {
		topEnd = len(m.infoLines)
	}

	infoFocused := m.focus == panelInfo
	var topLines []string
	for i := topScroll; i < topEnd; i++ {
		line := m.infoLines[i]
		isCursor := infoFocused && i == m.infoCursor
		text := truncate(line.text, innerW)
		var s string
		if isCursor {
			prefix := "  "
			if line.symID != "" {
				prefix = "▸ "
			}
			s = cursorStyle.Render(prefix + strings.TrimLeft(text, " "))
		} else {
			switch line.kind {
			case "title":
				s = titleStyle.Render(text)
			case "dim":
				s = dimStyle.Render(text)
			case "highlight":
				s = highlightStyle.Render(text)
			default:
				if line.symID != "" {
					s = navItemStyle.Render("  " + text)
				} else {
					s = text
				}
			}
		}
		topLines = append(topLines, s)
	}
	if len(m.infoLines) > topH && len(topLines) > 0 {
		pct := 0
		if maxTopScroll > 0 {
			pct = topScroll * 100 / maxTopScroll
		}
		topLines[len(topLines)-1] = dimStyle.Render(fmt.Sprintf(" %d%%", pct))
	}

	var sections []string
	sections = append(sections, strings.Join(topLines, "\n"))

	// --- Bottom section: source ---
	if hasSrc {
		sections = append(sections, dimStyle.Render("─── SOURCE "+strings.Repeat("─", max(0, innerW-10))))

		maxSrcScroll := len(m.srcLines) - botH
		if maxSrcScroll < 0 {
			maxSrcScroll = 0
		}
		srcScroll := clamp(m.srcScroll, 0, maxSrcScroll)
		srcEnd := srcScroll + botH
		if srcEnd > len(m.srcLines) {
			srcEnd = len(m.srcLines)
		}

		var srcRendered []string
		for i := srcScroll; i < srcEnd; i++ {
			srcRendered = append(srcRendered, dimStyle.Render(truncate(m.srcLines[i], innerW)))
		}
		if len(m.srcLines) > botH && len(srcRendered) > 0 {
			pct := 0
			if maxSrcScroll > 0 {
				pct = srcScroll * 100 / maxSrcScroll
			}
			srcRendered[len(srcRendered)-1] = dimStyle.Render(fmt.Sprintf("     … %d%%", pct))
		}
		sections = append(sections, strings.Join(srcRendered, "\n"))
	}

	return style.Render(strings.Join(sections, "\n"))
}

func (m Model) viewStatus() string {
	mode := "NORMAL"
	if m.searching {
		mode = "SEARCH: " + m.search.View()
	} else if m.traceFrom != "" {
		from := shortName(m.traceFrom)
		if len(m.tracePath) > 0 {
			mode = fmt.Sprintf("TRACE ★ %s → %s (%d paths)  T:new TO  t:re-FROM  esc:clear",
				from, shortName(m.traceTo), len(m.tracePath))
		} else {
			mode = fmt.Sprintf("TRACE ★ FROM=%s  navigate to target, press T  esc:clear", from)
		}
	}

	ago := time.Since(m.lastLoad).Truncate(time.Second)
	right := fmt.Sprintf("watching · %s ago", ago)
	if m.err != nil {
		right = fmt.Sprintf("err: %v", m.err)
	}

	left := " " + mode
	rightPad := " " + right + " "

	var keys string
	if !m.searching {
		if m.focus == panelInfo {
			keys = " jk:calls  []:src  enter:jump  o:open  tab:panel  ?:help  q "
		} else if m.traceFrom != "" {
			keys = " tab/hl:nav  jk:move  t:re-FROM  T:set-TO  esc:clear "
		} else {
			keys = " tab/hl:nav  jk:move  enter  o:open  /:search  t:trace  ?:help  q "
		}
	}

	total := lipgloss.Width(left) + lipgloss.Width(keys) + lipgloss.Width(rightPad)
	gap := m.width - total - 2
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap/2) + keys + strings.Repeat(" ", gap-gap/2) + rightPad
	return statusStyle.Width(m.width).Render(bar)
}

// --- Helpers ---

func (m *Model) navigateToSym(id string) {
	sym := m.findSymbol(id)
	if sym == nil {
		return
	}
	for i, f := range m.files {
		if f == sym.File {
			m.fileIdx = i
			m.fileScroll = 0
			m.updateFilteredSyms()
			for k, fs := range m.filteredSyms {
				if fs.ID == id {
					m.symIdx = k
					m.symScroll = 0
					break
				}
			}
			m.focus = panelSymbols
			m.buildInfoLines()
			return
		}
	}
}

func isTerminalEditor(editor string) bool {
	switch filepath.Base(editor) {
	case "vim", "vi", "nvim", "nano", "micro", "helix", "hx", "emacs":
		return true
	}
	return false
}

func findEditor() (string, bool) {
	// Respect $VISUAL / $EDITOR first
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(env); e != "" {
			if _, err := exec.LookPath(e); err == nil {
				return e, true
			}
		}
	}
	for _, editor := range []string{"cursor", "code", "codium", "zed", "nvim", "vim", "nano", "vi"} {
		if _, err := exec.LookPath(editor); err == nil {
			return editor, true
		}
	}
	return "", false
}

func editorCmd(editor, absPath string, line int) *exec.Cmd {
	target := fmt.Sprintf("%s:%d", absPath, line+1)
	switch filepath.Base(editor) {
	case "cursor", "code", "codium":
		// --reuse-window reuses the existing window instead of opening a new one
		return exec.Command(editor, "--reuse-window", "-g", target)
	case "zed":
		return exec.Command(editor, "-g", target)
	case "vim", "vi", "nvim", "nano", "micro", "helix", "hx", "emacs":
		return exec.Command(editor, fmt.Sprintf("+%d", line+1), absPath)
	default:
		return exec.Command(editor, absPath)
	}
}

func openInEditor(absPath string, line int) tea.Cmd {
	editor, found := findEditor()
	if !found {
		return nil
	}
	cmd := editorCmd(editor, absPath, line)
	if isTerminalEditor(editor) {
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return errMsg{err} })
	}
	_ = cmd.Start()
	return nil
}

func (m Model) findSymbol(id string) *parser.Symbol {
	for i := range m.symbols {
		if m.symbols[i].ID == id {
			return &m.symbols[i]
		}
	}
	return nil
}

func shortName(id string) string {
	if parts := strings.SplitN(id, ":", 2); len(parts) == 2 {
		return parts[1]
	}
	return id
}

func kindTag(kind string) string {
	switch kind {
	case "fn":
		return "fn"
	case "method":
		return "m "
	case "type":
		return "T "
	case "class":
		return "C "
	case "interface":
		return "I "
	default:
		return "  "
	}
}

func truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	var result []rune
	for _, r := range []rune(s) {
		candidate := string(result) + string(r)
		if lipgloss.Width(candidate) > maxW-1 {
			return string(result) + "…"
		}
		result = append(result, r)
	}
	return string(result)
}

func expandTabs(s string) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			spaces := 4 - (col % 4)
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
		} else {
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

func scrollOffset(cursor, currentScroll, visible, total int) int {
	if total <= visible {
		return 0
	}
	if cursor < currentScroll {
		return cursor
	}
	if cursor >= currentScroll+visible {
		return cursor - visible + 1
	}
	return currentScroll
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
