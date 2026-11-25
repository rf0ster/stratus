package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
)

type backlogKeys struct {
	Move        key.Binding
	Open        key.Binding
	Filter      key.Binding
	ClearFilter key.Binding
	Refresh     key.Binding
	Completed   key.Binding
	Assignee    key.Binding
	Quit        key.Binding
}

var defaultBacklogKeys = backlogKeys{
	Move:        key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move")),
	Open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	ClearFilter: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "clear filters")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Completed:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "show/hide completed")),
	Assignee:    key.NewBinding(key.WithKeys("a", "esc"), key.WithHelp("a", "assignee")),
	Quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
}

// backlogView lists work items in a table, optionally narrowed down by
// column filters.
type backlogView struct {
	app     *app
	keys    backlogKeys
	help    help.Model
	table   table.Model
	styles  table.Styles
	spinner spinner.Model
	filter  *columnFilter
	items   []ado.WorkItem // everything the query loaded
	visible []int          // indexes into items of the rows shown, which pass the column filters
	loading bool
	loaded  bool
	err     error
	seq     int // identifies the latest load, so stale results are dropped
}

func newBacklogView(a *app) *backlogView {
	t := table.New(table.WithFocused(true))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(styles)

	v := &backlogView{
		app:     a,
		keys:    defaultBacklogKeys,
		help:    help.New(),
		table:   t,
		styles:  styles,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(accentStyle)),
		filter:  newColumnFilter(),
	}
	v.layout()
	return v
}

func (v *backlogView) Init() tea.Cmd { return v.load() }

func (v *backlogView) load() tea.Cmd {
	v.loading = true
	v.seq++
	return tea.Batch(loadItems(v.app.backend, v.seq, v.app.query), v.spinner.Tick)
}

func (v *backlogView) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.layout()

	case itemsLoadedMsg:
		if msg.seq != v.seq {
			return nil
		}
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			v.loaded = true
			v.setItems(msg.items)
		}

	case itemSavedMsg:
		if msg.err == nil {
			v.replace(msg.item)
		}

	case itemReloadedMsg:
		if msg.err == nil {
			v.replace(msg.item)
		}

	case commentAddedMsg:
		// A comment adds a revision. Track it, unless something else changed
		// the item too (then the next save should fail as a conflict).
		for i := range v.items {
			if v.items[i].ID == msg.id && msg.err == nil && msg.rev == v.items[i].Rev+1 {
				v.items[i].Rev = msg.rev
			}
		}

	case queryChangedMsg:
		return v.load()

	case spinner.TickMsg:
		if v.loading {
			var cmd tea.Cmd
			v.spinner, cmd = v.spinner.Update(msg)
			return cmd
		}

	case tea.KeyMsg:
		if v.filter.editing {
			return v.updateFilter(msg)
		}
		switch {
		case key.Matches(msg, v.keys.Quit):
			return tea.Quit
		case key.Matches(msg, v.keys.Open):
			if item, ok := v.selected(); ok {
				return push(newDetailsView(v.app, item))
			}
			return nil
		case key.Matches(msg, v.keys.Refresh):
			return v.load()
		case key.Matches(msg, v.keys.Completed):
			v.app.query.HideCompleted = !v.app.query.HideCompleted
			return v.load()
		case key.Matches(msg, v.keys.Assignee):
			return push(newAssigneeView(v.app))
		case key.Matches(msg, v.keys.Filter):
			if !v.loaded {
				return nil
			}
			cmd := v.filter.edit()
			v.layout() // the filter row takes a line from the table
			return cmd
		case key.Matches(msg, v.keys.ClearFilter):
			if v.filter.active() {
				v.filter.clear()
				v.layout()
				v.refreshRows(v.selectedID())
			}
			return nil
		}
		var cmd tea.Cmd
		v.table, cmd = v.table.Update(msg)
		return cmd

	default:
		// Cursor blinks for the filter input.
		if v.filter.editing {
			cmd, _ := v.filter.update(msg)
			return cmd
		}
	}
	return nil
}

// updateFilter handles a key while a column filter is being edited. The
// rows are filtered again as you type.
func (v *backlogView) updateFilter(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		v.filter.done()
		v.layout()
		return nil
	case "esc":
		v.filter.cancel()
		v.layout()
		v.refreshRows(v.selectedID())
		return nil
	}
	cmd, changed := v.filter.update(msg)
	if changed {
		v.refreshRows(v.selectedID())
	}
	return cmd
}

// filterShown reports whether the filter row is on screen.
func (v *backlogView) filterShown() bool { return v.filter.editing || v.filter.active() }

func (v *backlogView) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Stratus") + subtitleStyle.Render(" · "+v.app.source) + "\n")
	b.WriteString(subtitleStyle.Render(v.describeQuery()) + "\n\n")

	switch {
	case !v.loaded && v.err == nil:
		b.WriteString(v.spinner.View() + " Loading work items…")
	case !v.loaded:
		b.WriteString(errorStyle.Render("Couldn't load work items: " + v.err.Error()))
	case len(v.items) == 0:
		b.WriteString(mutedStyle.Render("No work items match this filter."))
	default:
		if v.filterShown() {
			b.WriteString(v.filter.View(v.columnWidths()) + "\n")
		}
		if len(v.visible) == 0 {
			b.WriteString(v.tableHeader() + "\n" + mutedStyle.Render("No work items match the column filters."))
		} else {
			b.WriteString(v.table.View())
		}
	}

	// Pin the status and help lines to the bottom of the screen.
	body := b.String()
	gap := v.app.innerHeight() - lipgloss.Height(body) - 2
	return body + strings.Repeat("\n", max(gap, 1)) + v.statusLine() + "\n" +
		v.help.ShortHelpView(v.helpKeys())
}

func (v *backlogView) helpKeys() []key.Binding {
	if v.filter.editing {
		return filterHelp
	}
	keys := []key.Binding{v.keys.Move, v.keys.Open, v.keys.Filter}
	if v.filter.active() {
		keys = append(keys, v.keys.ClearFilter)
	}
	return append(keys, v.keys.Refresh, v.keys.Completed, v.keys.Assignee, v.keys.Quit)
}

// tableHeader renders just the table's header, for when no rows pass the
// filters.
func (v *backlogView) tableHeader() string {
	n := lipgloss.Height(v.styles.Header.Render("x"))
	lines := strings.SplitN(v.table.View(), "\n", n+1)
	return strings.Join(lines[:min(n, len(lines))], "\n")
}

func (v *backlogView) statusLine() string {
	switch {
	case v.loading && v.loaded:
		return v.spinner.View() + " Refreshing…"
	case v.err != nil && v.loaded:
		return errorStyle.Render("Refresh failed: " + v.err.Error())
	case v.loaded && v.filterShown():
		return mutedStyle.Render(fmt.Sprintf("%d of %s", len(v.visible), plural(len(v.items), "work item")))
	case v.loaded:
		return mutedStyle.Render(plural(len(v.items), "work item"))
	}
	return ""
}

func (v *backlogView) describeQuery() string {
	q := v.app.query
	var parts []string
	switch {
	case strings.TrimSpace(q.AssignedTo) == "":
		parts = append(parts, "Assigned to anyone")
	case q.AssignedToMe():
		parts = append(parts, "Assigned to me")
	default:
		parts = append(parts, "Assigned to "+q.AssignedTo)
	}
	if q.HideCompleted {
		parts = append(parts, "open items")
	} else {
		parts = append(parts, "including completed")
	}
	if q.AreaPath != "" {
		parts = append(parts, "area "+q.AreaPath)
	}
	if q.IterationPath != "" {
		parts = append(parts, "iteration "+q.IterationPath)
	}
	return strings.Join(parts, " · ")
}

// layout sizes the table columns to the window, and the table to the space
// left over.
func (v *backlogView) layout() {
	widths := v.columnWidths()
	cols := make([]table.Column, len(backlogColumns))
	for i, c := range backlogColumns {
		cols[i] = table.Column{Title: c.title, Width: widths[i]}
	}
	v.table.SetColumns(cols)
	v.table.SetWidth(v.app.innerWidth())
	// Leave room for the title (3 lines), the filter row if it's showing,
	// and the status and help lines.
	reserved := 6
	if v.filterShown() {
		reserved++
	}
	v.table.SetHeight(max(v.app.innerHeight()-reserved, 3))
	v.help.Width = v.app.innerWidth()
}

// columnWidths returns the width of each column. The flexible column (the
// title) takes whatever the fixed-width columns leave.
func (v *backlogView) columnWidths() []int {
	const cellPadding = 2 // table cells have 1 space either side
	widths := make([]int, len(backlogColumns))
	rest := v.app.innerWidth() - len(backlogColumns)*cellPadding
	for i, c := range backlogColumns {
		widths[i] = c.width
		rest -= c.width
	}
	for i, c := range backlogColumns {
		if c.width == 0 {
			widths[i] = max(rest, 10)
		}
	}
	return widths
}

// setItems replaces the items after a load.
func (v *backlogView) setItems(items []ado.WorkItem) {
	selectedID := v.selectedID()
	// A load that started before a save can finish after it. Keep whichever
	// copy of an item is newer so a stale revision doesn't come back.
	newer := make(map[int]ado.WorkItem, len(v.items))
	for _, item := range v.items {
		newer[item.ID] = item
	}
	for i, item := range items {
		if old, ok := newer[item.ID]; ok && old.Rev > item.Rev {
			items[i] = old
		}
	}
	v.items = items
	v.app.rememberAssignees(items)
	v.refreshRows(selectedID)
}

// replace updates a single item in place after it was saved or reloaded.
func (v *backlogView) replace(updated ado.WorkItem) {
	for i := range v.items {
		if v.items[i].ID == updated.ID && updated.Rev >= v.items[i].Rev {
			v.items[i] = updated
			v.refreshRows(v.selectedID())
			return
		}
	}
}

// refreshRows applies the column filters to the items and redraws the rows,
// keeping the item with ID keepID selected if it's still shown.
func (v *backlogView) refreshRows(keepID int) {
	v.visible = v.visible[:0]
	rows := []table.Row{}
	for i, item := range v.items {
		if !v.filter.matches(item) {
			continue
		}
		row := make(table.Row, len(backlogColumns))
		for c, col := range backlogColumns {
			row[c] = col.cell(item)
		}
		v.visible = append(v.visible, i)
		rows = append(rows, row)
	}
	v.table.SetRows(rows)

	cursor := min(v.table.Cursor(), len(v.visible)-1)
	for row, i := range v.visible {
		if v.items[i].ID == keepID {
			cursor = row
			break
		}
	}
	v.table.SetCursor(cursor)
}

func (v *backlogView) selected() (ado.WorkItem, bool) {
	i := v.table.Cursor()
	if i < 0 || i >= len(v.visible) {
		return ado.WorkItem{}, false
	}
	return v.items[v.visible[i]], true
}

// selectedID returns the ID of the selected item, or 0 if there isn't one.
func (v *backlogView) selectedID() int {
	item, _ := v.selected()
	return item.ID
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
