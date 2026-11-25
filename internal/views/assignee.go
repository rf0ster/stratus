package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
)

var assigneeHelp = []key.Binding{
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
	key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// assigneeView changes whose work items the backlog shows.
type assigneeView struct {
	app   *app
	input textinput.Model
	help  help.Model
}

func newAssigneeView(a *app) *assigneeView {
	ti := textinput.New()
	ti.Placeholder = "name or email"
	ti.CharLimit = 100
	ti.Width = 40
	ti.ShowSuggestions = true
	ti.SetValue(a.query.AssignedTo)
	ti.CursorEnd()

	v := &assigneeView{app: a, input: ti, help: help.New()}
	v.refreshSuggestions()
	return v
}

func (v *assigneeView) Init() tea.Cmd { return v.input.Focus() }

func (v *assigneeView) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case teamLoadedMsg:
		v.refreshSuggestions()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return pop()
		case "enter":
			v.app.query.AssignedTo = strings.TrimSpace(v.input.Value())
			// Pop first so the backlog is on top when it starts reloading.
			return tea.Sequence(pop(), func() tea.Msg { return queryChangedMsg{} })
		}
	}
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return cmd
}

func (v *assigneeView) refreshSuggestions() {
	suggestions := []string{ado.Me}
	for _, p := range v.people() {
		suggestions = append(suggestions, p.Name())
	}
	v.input.SetSuggestions(suggestions)
}

func (v *assigneeView) people() []ado.Identity {
	if len(v.app.members) > 0 {
		return v.app.members
	}
	return v.app.seen
}

func (v *assigneeView) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Filter by assignee") + "\n\n")
	b.WriteString("Show work items assigned to:\n\n")
	b.WriteString(v.input.View() + "\n\n")
	b.WriteString(mutedStyle.Render("Use @Me for yourself, or leave blank for everyone.") + "\n")

	if people := v.people(); len(people) > 0 {
		names := make([]string, len(people))
		for i, p := range people {
			names[i] = p.Name()
		}
		label := "Team: "
		if len(v.app.members) == 0 {
			label = "Seen on the backlog: "
		}
		wrapped := lipgloss.NewStyle().Width(min(v.app.innerWidth(), maxContentWidth)).
			Render(label + strings.Join(names, ", "))
		b.WriteString(mutedStyle.Render(wrapped) + "\n")
	}

	body := b.String()
	gap := v.app.innerHeight() - lipgloss.Height(body) - 1
	return body + strings.Repeat("\n", max(gap, 1)) + v.help.ShortHelpView(assigneeHelp)
}
