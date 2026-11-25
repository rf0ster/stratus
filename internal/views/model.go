// Package views is the stratus user interface: a stack of views (backlog,
// work item details, assignee filter) on top of a root Bubble Tea model.
package views

import (
	"context"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rf0ster/stratus/internal/ado"
)

// Backend is the part of the Azure DevOps client the UI uses.
type Backend interface {
	QueryWorkItems(ctx context.Context, q ado.Query) ([]ado.WorkItem, error)
	GetWorkItem(ctx context.Context, id int) (ado.WorkItem, error)
	UpdateWorkItem(ctx context.Context, id, rev int, changes []ado.Change) (ado.WorkItem, error)
	GetComments(ctx context.Context, id int) ([]ado.Comment, error)
	AddComment(ctx context.Context, id int, text string) (ado.Comment, error)
	GetStates(ctx context.Context, t ado.WorkItemType) ([]ado.State, error)
	GetTeamMembers(ctx context.Context, team string) ([]ado.Identity, error)
}

// Config configures the UI.
type Config struct {
	Backend Backend
	// Query is the initial backlog query.
	Query ado.Query
	// Team lists the people work items can be assigned to. Empty means the
	// project's default team.
	Team string
	// Source describes where the data comes from, for the title bar.
	Source string
}

// View is one screen. Views are pointers and update themselves in place.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) tea.Cmd
	View() string
}

// app is state shared by all views. It is only modified from Update, never
// from commands, so it needs no locking.
type app struct {
	backend Backend
	team    string
	source  string
	query   ado.Query
	width   int
	height  int

	states  map[ado.WorkItemType][]ado.State
	members []ado.Identity // the team, once loaded
	seen    []ado.Identity // assignees seen on the backlog, if the team can't be loaded
}

// rememberAssignees records the people items are assigned to, as a fallback
// list of assignees when the team can't be loaded.
func (a *app) rememberAssignees(items []ado.WorkItem) {
	for _, item := range items {
		p := item.AssignedTo
		if p.IsZero() || slices.ContainsFunc(a.seen, func(s ado.Identity) bool { return s.Ref() == p.Ref() }) {
			continue
		}
		a.seen = append(a.seen, p)
	}
	sort.Slice(a.seen, func(i, j int) bool {
		return strings.ToLower(a.seen[i].Name()) < strings.ToLower(a.seen[j].Name())
	})
}

// innerWidth and innerHeight are the space inside the app padding.
func (a *app) innerWidth() int  { return max(a.width-appStyle.GetHorizontalFrameSize(), 20) }
func (a *app) innerHeight() int { return max(a.height, 10) }

// Model is the root Bubble Tea model. It keeps a stack of views: key presses
// go to the top view, everything else goes to every view so that results of
// background work reach the view that started it even if it's covered.
type Model struct {
	app   *app
	stack []View
}

// New returns the root model, starting on the backlog.
func New(cfg Config) Model {
	a := &app{
		backend: cfg.Backend,
		team:    cfg.Team,
		source:  cfg.Source,
		query:   cfg.Query,
		states:  map[ado.WorkItemType][]ado.State{},
	}
	return Model{app: a, stack: []View{newBacklogView(a)}}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.top().Init(), loadTeam(m.app.backend, m.app.team))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, m.top().Update(msg)

	case pushMsg:
		m.stack = append(m.stack, msg.view)
		return m, msg.view.Init()

	case popMsg:
		if len(m.stack) == 1 {
			return m, tea.Quit
		}
		m.stack = m.stack[:len(m.stack)-1]
		return m, nil

	case tea.WindowSizeMsg:
		m.app.width, m.app.height = msg.Width, msg.Height

	case teamLoadedMsg:
		if msg.err == nil {
			m.app.members = msg.members
		}

	case statesLoadedMsg:
		if msg.err == nil {
			m.app.states[msg.typ] = msg.states
		}
	}

	cmds := make([]tea.Cmd, len(m.stack))
	for i, v := range m.stack {
		cmds[i] = v.Update(msg)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.app.width == 0 {
		return "" // wait for the first WindowSizeMsg
	}
	return appStyle.Render(m.top().View())
}

func (m Model) top() View { return m.stack[len(m.stack)-1] }
