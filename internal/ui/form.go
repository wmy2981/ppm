package ui

import (
	"fmt"
	"net"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wmy2981/ppm/internal/netsh"
)

func (m model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.importInput.Value())
		if path == "" {
			m.setStatus(msgError, "path is required")
			return m, nil
		}
		m.view = viewList
		res, warnings, err := m.store.Import(path, m.rules, m.notes)
		if err != nil {
			m.setStatus(msgError, "%s", "import failed: "+err.Error())
			return m, nil
		}
		detail := ""
		if len(warnings) > 0 {
			detail = "; " + strings.Join(warnings[:minInt(len(warnings), 2)], "; ")
		}
		m.setStatus(msgInfo, "import: %d created, %d skipped, %d failed%s", res.Created, res.Skipped, res.Failed, detail)
		return m, loadRules()
	}
	var cmd tea.Cmd
	m.importInput, cmd = m.importInput.Update(msg)
	return m, cmd
}

func (m model) updateConfirmQuit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.quitting = true
		return m, tea.Quit
	default: // n / esc
		m.view = viewList
		return m, nil
	}
}

func (m *model) openForm(editIdx int) {
	m.view = viewForm
	m.formEdit = editIdx
	m.formFocus = 0
	if editIdx >= 0 {
		cur := m.rules[editIdx]
		m.form = []formField{
			{label: "Listen address", value: cur.ListenAddr},
			{label: "Listen port", value: cur.ListenPort},
			{label: "Connect address", value: cur.ConnectAddr},
			{label: "Connect port", value: cur.ConnectPort},
			{label: "Note (local only)", value: m.notes[cur.Key()]},
		}
	} else {
		m.form = []formField{
			{label: "Listen address", value: "0.0.0.0"},
			{label: "Listen port", value: ""},
			{label: "Connect address", value: ""},
			{label: "Connect port", value: ""},
			{label: "Note (local only)", value: ""},
		}
	}
	m.inputs = make([]textinput.Model, len(m.form))
	for i := range m.form {
		in := textinput.New()
		in.Placeholder = m.form[i].label
		in.SetValue(m.form[i].value)
		in.CharLimit = 64
		if i == 0 {
			in.Focus()
		}
		m.inputs[i] = in
	}
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil
	case "tab", "shift+tab", "up", "down":
		dir := 1
		if msg.String() == "shift+tab" || msg.String() == "up" {
			dir = -1
		}
		m.formFocus = (m.formFocus + dir + len(m.form)) % len(m.form)
		var cmds []tea.Cmd
		for i := range m.inputs {
			if i == m.formFocus {
				cmds = append(cmds, m.inputs[i].Focus())
			} else {
				m.inputs[i].Blur()
			}
		}
		return m, tea.Batch(cmds...)
	case "enter":
		return m.submitForm()
	}
	before := m.inputs[m.formFocus].Value()
	var cmd tea.Cmd
	m.inputs[m.formFocus], cmd = m.inputs[m.formFocus].Update(msg)
	if m.inputs[m.formFocus].Value() != before {
		m.form[m.formFocus].value = m.inputs[m.formFocus].Value()
	}
	return m, cmd
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	vals := make([]string, len(m.form))
	for i := range m.inputs {
		vals[i] = strings.TrimSpace(m.inputs[i].Value())
	}
	names := []string{"Listen address", "Listen port", "Connect address", "Connect port"}
	for i, v := range vals[:4] {
		if v == "" {
			m.setStatus(msgError, "%s is required", names[i])
			return m, nil
		}
	}
	r := netsh.Rule{
		ListenAddr:  vals[0],
		ListenPort:  vals[1],
		ConnectAddr: vals[2],
		ConnectPort: vals[3],
	}
	if net.ParseIP(r.ListenAddr) == nil || net.ParseIP(r.ConnectAddr) == nil {
		m.setStatus(msgError, "listen/connect address must be a valid IP")
		return m, nil
	}
	if !netsh.IsPort(r.ListenPort) || !netsh.IsPort(r.ConnectPort) {
		m.setStatus(msgError, "ports must be 1-65535")
		return m, nil
	}
	if vals[4] != "" {
		m.notes[r.Key()] = vals[4]
	} else {
		delete(m.notes, r.Key())
	}
	edit := m.formEdit
	return m, func() tea.Msg {
		if edit >= 0 {
			old := m.rules[edit]
			if old.Key() != r.Key() && ruleExists(m.rulesSnapshot(), r.Key()) {
				return opErrMsg{fmt.Errorf("a rule with listen %s already exists", r.Key())}
			}
			if err := netsh.DeleteRule(old); err != nil {
				return opErrMsg{fmt.Errorf("delete old: %w", err)}
			}
			if err := netsh.AddRule(r); err != nil {
				_ = netsh.AddRule(old) // best-effort restore
				return opErrMsg{fmt.Errorf("create new: %w (old rule restored)", err)}
			}
		} else {
			if ruleExists(m.rulesSnapshot(), r.Key()) {
				return opErrMsg{fmt.Errorf("listen %s already exists", r.Key())}
			}
			if err := netsh.AddRule(r); err != nil {
				return opErrMsg{err}
			}
		}
		return opDoneMsg{}
	}
}

// rulesSnapshot copies the rules slice so the closure does not race with
// reloads.
func (m model) rulesSnapshot() []netsh.Rule {
	out := make([]netsh.Rule, len(m.rules))
	copy(out, m.rules)
	return out
}

func ruleExists(rules []netsh.Rule, key string) bool {
	for _, r := range rules {
		if r.Key() == key {
			return true
		}
	}
	return false
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		idx := m.deleteIdx
		if idx < 0 || idx >= len(m.rules) {
			m.view = viewList
			m.deleteIdx = -1
			return m, nil
		}
		r := m.rules[idx]
		key := r.Key()
		m.view = viewList
		m.deleteIdx = -1
		delete(m.notes, key)
		delete(m.tests, key)
		return m, func() tea.Msg {
			if err := netsh.DeleteRule(r); err != nil {
				return opErrMsg{err}
			}
			return opDoneMsg{}
		}
	default: // n / esc / anything else cancels
		m.view = viewList
		m.deleteIdx = -1
		return m, nil
	}
}
