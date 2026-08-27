// Package ui implements the Bubble Tea TUI: a k9s-style single table with
// form overlay and inline delete confirmation.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wmy2981/ppm/internal/netsh"
	"github.com/wmy2981/ppm/internal/store"
)

type view int

const (
	viewList view = iota
	viewForm
	viewConfirmDelete
	viewImport
	viewConfirmQuit
)

const (
	msgInfo = iota
	msgError
	msgSuccess
)

// Messages ------------------------------------------------------------------

type rulesLoadedMsg struct {
	rules     []netsh.Rule
	listening map[string]bool
	notes     map[string]string
	err       error
}

type opDoneMsg struct{}

type opErrMsg struct{ err error }

type testResultMsg struct {
	key string
	ms  float64
	err error
}

type saveNotesMsg struct{ err error }

// Model ---------------------------------------------------------------------

type formField struct {
	label string
	value string
}

type model struct {
	version     string
	store       Store
	view        view
	tbl         table.Model
	inputs      []textinput.Model
	rules       []netsh.Rule
	ruleCount   int
	listenCount int
	listening   map[string]bool
	notes      map[string]string
	tests      map[string]string // key -> rendered status fragment
	status     string
	statusKind int
	form       []formField
	formFocus  int
	formEdit   int // -1 = create, else index into rules
	deleteIdx  int
	importInput textinput.Model
	quitting   bool
	width      int
	height     int
}

// Store is what the UI needs from the storage layer.
type Store interface {
	LoadNotes() (map[string]string, error)
	SaveNotes(map[string]string) error
	Export(rules []netsh.Rule, notes map[string]string) (string, error)
	Import(path string, live []netsh.Rule, notes map[string]string) (*store.ImportResult, []string, error)
	NewestBackup() string
	Dir() string
}

// Styles --------------------------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	adminStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	statusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	okStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	confirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("203")).
			Padding(0, 2)

	formBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3)

	focusedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle   = statusBarStyle
	focusLabelW    = 18
	helpStyle      = statusBarStyle
)

// column widths; NOTE is flexible, the rest are fixed. Cell values must stay
// plain text: bubbles tables mis-measure ANSI-styled cells and shift columns.
var (
	colListen  = 22
	colConnect = 22
	colTest    = 28
	colState   = 6
)

func noteColWidth(termW int) int {
	// borders+padding consume ~12 cells across 5 columns
	fixed := colListen + colConnect + colTest + colState + 12 + 4 // +4 title margin
	w := termW - fixed
	if w < 10 {
		return 10
	}
	if w > 40 {
		return 40
	}
	return w
}

func stCell(listening map[string]bool, r netsh.Rule) string {
	if listening[strings.ToLower(r.Key())] {
		return "up"
	}
	return "DOWN"
}

func newTable(noteW int) table.Model {
	cols := []table.Column{
		{Title: "LISTEN", Width: colListen},
		{Title: "CONNECT", Width: colConnect},
		{Title: "NOTE", Width: noteW},
		{Title: "TEST", Width: colTest},
		{Title: "STATE", Width: colState},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("236")).
		Bold(false)
	t.SetStyles(s)
	return t
}

// New creates the initial TUI model.
func New(version string, store Store, notes map[string]string) tea.Model {
	return model{
		version:   version,
		store:     store,
		view:      viewList,
		tbl:       newTable(noteColWidth(80)),
		listening: map[string]bool{},
		notes:     notes,
		tests:     map[string]string{},
		formFocus: 0,
		formEdit:  -1,
		deleteIdx: -1,
	}
}

func (m *model) setStatus(kind int, format string, args ...any) {
	m.statusKind = kind
	m.status = fmt.Sprintf(format, args...)
}

// Commands ------------------------------------------------------------------

func loadRules(st Store) tea.Cmd {
	return func() tea.Msg {
		rules, err := netsh.ListRules()
		if err != nil {
			return rulesLoadedMsg{err: err}
		}
		lst, err := netsh.ListeningAddrs()
		if err != nil {
			return rulesLoadedMsg{rules: rules, err: fmt.Errorf("netstat: %w", err)}
		}
		notes, _ := st.LoadNotes()
		if notes == nil {
			notes = map[string]string{}
		}
		return rulesLoadedMsg{rules: rules, listening: lst, notes: notes}
	}
}

func testCmd(key string, r netsh.Rule) tea.Cmd {
	return func() tea.Msg {
		ms, err := netsh.TestConnectivity(r)
		return testResultMsg{key: key, ms: ms.Seconds() * 1000, err: err}
	}
}

// Init / Update -------------------------------------------------------------

func (m model) Init() tea.Cmd { return loadRules(m.store) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.tbl.SetColumns([]table.Column{
			{Title: "LISTEN", Width: colListen},
			{Title: "CONNECT", Width: colConnect},
			{Title: "NOTE", Width: noteColWidth(msg.Width)},
			{Title: "TEST", Width: colTest},
			{Title: "STATE", Width: colState},
		})
		m.resizeTable()
		// clear resize residue (stale lines left by the previous frame)
		return m, tea.ClearScreen

	case rulesLoadedMsg:
		if msg.err != nil {
			m.setStatus(msgError, "%s", msg.err.Error())
			if msg.rules == nil {
				return m, nil
			}
		}
		// counts live in the title line; the status line stays reserved for
		// op messages so they are never clobbered by a refresh
		m.ruleCount = len(msg.rules)
		m.listenCount = countListening(msg.rules, msg.listening)
		m.rules = msg.rules
		m.listening = msg.listening
		m.notes = msg.notes
		m.syncRows()
		return m, nil

	case opDoneMsg:
		m.view = viewList
		cmds := []tea.Cmd{loadRules(m.store)}
		if err := m.store.SaveNotes(m.notes); err != nil {
			m.setStatus(msgError, "save notes: %v", err)
		}
		return m, tea.Batch(cmds...)

	case opErrMsg:
		// stay on the current view (form/import) so the user can fix the
		// input; only refresh the list in case a partial change landed
		m.setStatus(msgError, "%s", msg.err.Error())
		return m, loadRules(m.store)

	case saveNotesMsg:
		if msg.err != nil {
			m.setStatus(msgError, "save notes: %v", msg.err)
		}
		return m, nil

	case testResultMsg:
		if msg.err != nil {
			m.tests[msg.key] = truncatePlain("fail: "+shortErr(msg.err), colTest)
		} else {
			m.tests[msg.key] = fmt.Sprintf("%.0fms", msg.ms)
		}
		m.syncRows()
		return m, nil

	case tea.KeyMsg:
		switch m.view {
		case viewForm:
			return m.updateForm(msg)
		case viewConfirmDelete:
			return m.updateConfirm(msg)
		case viewImport:
			return m.updateImport(msg)
		case viewConfirmQuit:
			return m.updateConfirmQuit(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func countListening(rules []netsh.Rule, lst map[string]bool) int {
	n := 0
	for _, r := range rules {
		if lst[strings.ToLower(r.Key())] {
			n++
		}
	}
	return n
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 40 {
		s = s[:37] + "..."
	}
	return strings.ReplaceAll(s, "\n", " ")
}

// truncatePlain cuts s to at most w display cells (runes are 1 cell here;
// content is ASCII-only), appending an ellipsis when truncated.
func truncatePlain(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// truncateTail keeps the last max cells of s — for paths, the filename part
// is what matters.
func truncateTail(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-(max-1):])
}

// syncRows rebuilds table rows from m.rules, preserving selection by key.
func (m *model) syncRows() {
	var selKey string
	if cur := m.currentRule(); cur != nil {
		selKey = cur.Key()
	}
	noteW := noteColWidth(m.width)
	rows := make([]table.Row, 0, len(m.rules))
	for _, r := range m.rules {
		rows = append(rows, table.Row{
			r.Key(),
			r.Target(),
			truncatePlain(m.notes[r.Key()], noteW),
			m.tests[r.Key()],
			stCell(m.listening, r),
		})
	}
	m.tbl.SetRows(rows)
	found := false
	for i, r := range m.rules {
		if r.Key() == selKey {
			m.tbl.SetCursor(i)
			found = true
			break
		}
	}
	if !found {
		m.clampCursor()
	}
	m.resizeTable()
}

// noteW returns the current NOTE column width from the table's columns.
func (m model) noteW() int {
	cols := m.tbl.Columns()
	if len(cols) >= 4 {
		return cols[2].Width
	}
	return 28
}

// clampCursor pulls the bubbles-table cursor back into [0, len(rules)); the
// table lets it drift to -1 when keys are pressed while the list is empty.
func (m *model) clampCursor() {
	if len(m.rules) == 0 {
		return
	}
	switch c := m.tbl.Cursor(); {
	case c < 0:
		m.tbl.SetCursor(0)
	case c >= len(m.rules):
		m.tbl.SetCursor(len(m.rules) - 1)
	}
}

func (m *model) resizeTable() {
	h := m.height - 5 // title + header + help margins
	if h < 3 {
		h = 3
	}
	m.tbl.SetHeight(h)
}

func (m model) currentRule() *netsh.Rule {
	cur := m.tbl.Cursor()
	if cur < 0 || cur >= len(m.rules) {
		return nil
	}
	r := m.rules[cur]
	return &r
}

// List view key handling -----------------------------------------------------

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.view = viewConfirmQuit
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		m.tbl.MoveUp(1)
	case "down", "j":
		m.tbl.MoveDown(1)
	case "pgup":
		m.tbl.MoveUp(m.tbl.Height())
	case "pgdown":
		m.tbl.MoveDown(m.tbl.Height())
	case "home", "g":
		m.tbl.GotoTop()
	case "end", "G":
		m.tbl.GotoBottom()
	case "r":
		return m, loadRules(m.store)
	case "a":
		m.openForm(-1)
	case "e", "enter":
		cur := m.currentRule()
		if cur == nil {
			m.setStatus(msgError, "no rule selected")
			return m, nil
		}
		m.openForm(m.tbl.Cursor())
	case "d":
		cur := m.currentRule()
		if cur == nil {
			m.setStatus(msgError, "no rule selected")
			return m, nil
		}
		m.view = viewConfirmDelete
		m.deleteIdx = m.tbl.Cursor()
	case "t":
		cur := m.currentRule()
		if cur == nil {
			m.setStatus(msgError, "no rule selected")
			return m, nil
		}
		key := cur.Key()
		m.tests[key] = warnStyle.Render("testing...")
		m.syncRows()
		return m, testCmd(key, *cur)
	case "T":
		if len(m.rules) == 0 {
			return m, nil
		}
		var cmds []tea.Cmd
		for _, r := range m.rules {
			key := r.Key()
			m.tests[key] = warnStyle.Render("testing...")
			cmds = append(cmds, testCmd(key, r))
		}
		m.syncRows()
		return m, tea.Batch(cmds...)
	case "E":
		path, err := m.store.Export(m.rules, m.notes)
		if err != nil {
			m.setStatus(msgError, "%s", "export failed: "+err.Error())
			return m, nil
		}
		m.setStatus(msgSuccess, "%s", "exported to "+path)
	case "I":
		m.view = viewImport
		in := textinput.New()
		in.Placeholder = "path to backup-*.json"
		in.CharLimit = 260
		in.Focus()
		in.PromptStyle = focusedStyle
		in.TextStyle = focusedStyle
		m.importInput = in
		return m, textinput.Blink
	}
	return m, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// View -----------------------------------------------------------------------

func (m model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(" PPM · PORTPROXY MANAGER v" + m.version + " "))
	b.WriteString("  ")
	b.WriteString(adminStyle.Render("[Admin OK]"))
	b.WriteString("  ")
	b.WriteString(statusBarStyle.Render(fmt.Sprintf("%d rules, %d listening", m.ruleCount, m.listenCount)))
	b.WriteString("\n")
	body := m.tbl.View()
	switch m.view {
	case viewForm:
		body = m.renderForm()
	case viewImport:
		body = formBoxStyle.Render(fmt.Sprintf(
			"Import backup\n\n  Path: %s\n\n  enter: import    esc: cancel",
			m.importInput.View(),
		))
	case viewConfirmDelete:
		cur := m.rules[m.deleteIdx]
		body = confirmBoxStyle.Render(fmt.Sprintf(
			"Delete rule %s ?  This cannot be undone.\n\n  y: delete    n/esc: cancel",
			okStyle.Render(cur.Key()),
		))
	case viewConfirmQuit:
		body = confirmBoxStyle.Render(
			"Quit ppm?\n\n  y: quit    n/esc: stay",
		)
	}
	b.WriteString(body)
	b.WriteString("\n")

	// help line: truncated to terminal width so it never wraps
	help := "a:add  e:edit  d:del  t:test  T:test-all  E:export  I:import  r:refresh  q:quit"
	switch m.view {
	case viewForm:
		help = "tab/shift+tab: next field  enter: save  esc: cancel"
	case viewImport:
		help = "enter: import  esc: cancel"
	case viewConfirmDelete:
		help = "y: confirm delete  n/esc: cancel"
	case viewConfirmQuit:
		help = "enter/y: quit  n/esc: stay"
	}
	if m.width > 0 {
		help = truncatePlain(help, m.width)
	}
	b.WriteString(helpStyle.Render(help))

	// status line: own row, truncated to terminal width
	if m.status != "" {
		style := statusBarStyle
		switch m.statusKind {
		case msgError:
			style = errStyle
		case msgSuccess:
			style = okStyle
		}
		max := m.width
		if max <= 0 {
			max = 120
		}
		b.WriteString("\n")
		b.WriteString(style.Render(truncateTail("  "+m.status, max)))
	}
	return b.String()
}

func (m model) renderForm() string {
	var b strings.Builder
	title := "New rule"
	if m.formEdit >= 0 {
		title = "Edit rule"
	}
	b.WriteString(title + "\n\n")
	for i, in := range m.inputs {
		label := blurredStyle.Render(fmt.Sprintf("%-*s", focusLabelW, m.form[i].label))
		if i == m.formFocus {
			label = focusedStyle.Render(m.form[i].label + strings.Repeat(" ", focusLabelW-len(m.form[i].label)))
		}
		b.WriteString(label + in.View() + "\n")
	}
	return formBoxStyle.Render(b.String())
}
