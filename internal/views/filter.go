package views

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
)

// backlogColumn is a column of the backlog table. The table and the column
// filters both use cell, so a filter matches exactly what's displayed.
type backlogColumn struct {
	title string
	width int // 0 takes whatever width the other columns leave
	cell  func(ado.WorkItem) string
}

// titleColumn is the index of the Title column, where filtering starts.
const titleColumn = 2

var backlogColumns = []backlogColumn{
	{"ID", 6, func(i ado.WorkItem) string { return strconv.Itoa(i.ID) }},
	{"Type", 12, func(i ado.WorkItem) string { return string(i.Type) }},
	{"Title", 0, func(i ado.WorkItem) string { return i.Title }},
	{"Assigned To", 18, func(i ado.WorkItem) string { return i.AssignedTo.Name() }},
	{"State", 10, func(i ado.WorkItem) string { return i.State }},
	{"Pri", 3, func(i ado.WorkItem) string {
		if i.Priority > 0 {
			return strconv.Itoa(i.Priority)
		}
		return ""
	}},
}

var filterHelp = []key.Binding{
	key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab/shift+tab", "column")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "done")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// columnFilter is a row of text inputs, one per backlog column, drawn above
// the table header. An item is shown when, for every column with a filter,
// the column's text contains the filter (ignoring case). Filters on
// different columns are always combined with AND.
type columnFilter struct {
	inputs  []textinput.Model
	focus   int
	editing bool
	before  []string // values when editing began, restored by cancel
}

func newColumnFilter() *columnFilter {
	f := &columnFilter{inputs: make([]textinput.Model, len(backlogColumns)), focus: titleColumn}
	for i := range f.inputs {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 100
		f.inputs[i] = ti
	}
	return f
}

// active reports whether any column has a filter.
func (f *columnFilter) active() bool {
	for _, in := range f.inputs {
		if strings.TrimSpace(in.Value()) != "" {
			return true
		}
	}
	return false
}

// matches reports whether item passes every column filter.
func (f *columnFilter) matches(item ado.WorkItem) bool {
	for i, in := range f.inputs {
		want := strings.ToLower(strings.TrimSpace(in.Value()))
		if want != "" && !strings.Contains(strings.ToLower(backlogColumns[i].cell(item)), want) {
			return false
		}
	}
	return true
}

// edit starts editing on the column that was focused last.
func (f *columnFilter) edit() tea.Cmd {
	f.editing = true
	f.before = f.values()
	return f.inputs[f.focus].Focus()
}

// done stops editing and keeps the filters.
func (f *columnFilter) done() {
	f.editing = false
	f.inputs[f.focus].Blur()
}

// cancel stops editing and restores the filters from before editing began.
func (f *columnFilter) cancel() {
	for i, v := range f.before {
		f.inputs[i].SetValue(v)
	}
	f.done()
}

// clear removes every filter.
func (f *columnFilter) clear() {
	for i := range f.inputs {
		f.inputs[i].SetValue("")
	}
}

// move focuses the next (delta 1) or previous (delta -1) column, wrapping
// around at either end.
func (f *columnFilter) move(delta int) tea.Cmd {
	f.inputs[f.focus].Blur()
	f.focus = (f.focus + delta + len(f.inputs)) % len(f.inputs)
	f.inputs[f.focus].CursorEnd()
	return f.inputs[f.focus].Focus()
}

// update handles a message while editing. changed reports whether a filter
// value changed, so the rows need filtering again.
func (f *columnFilter) update(msg tea.Msg) (cmd tea.Cmd, changed bool) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "tab":
			return f.move(1), false
		case "shift+tab":
			return f.move(-1), false
		}
	}
	before := f.inputs[f.focus].Value()
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd, f.inputs[f.focus].Value() != before
}

func (f *columnFilter) values() []string {
	vals := make([]string, len(f.inputs))
	for i, in := range f.inputs {
		vals[i] = in.Value()
	}
	return vals
}

// View renders the filters lined up with the table columns, whose widths are
// given. Cells get the same one-space padding on each side as table cells.
func (f *columnFilter) View(widths []int) string {
	cells := make([]string, len(widths))
	for i, w := range widths {
		var content string
		switch value := f.inputs[i].Value(); {
		case f.editing && i == f.focus:
			// The input draws its cursor one cell past its width.
			f.inputs[i].Width = max(w-1, 1)
			content = f.inputs[i].View()
		case value != "":
			content = accentStyle.Render(truncate(value, w))
		case f.editing:
			content = mutedStyle.Render("·")
		}
		cell := lipgloss.NewStyle().Width(w).MaxWidth(w).Inline(true).Render(content)
		cells[i] = lipgloss.NewStyle().Padding(0, 1).Render(cell)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

// truncate shortens s to fit in width cells, ending it with an ellipsis.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
