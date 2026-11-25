package views

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rf0ster/stratus/internal/ado"
)

// Navigation.
type (
	pushMsg struct{ view View }
	popMsg  struct{}
)

func push(v View) tea.Cmd { return func() tea.Msg { return pushMsg{v} } }
func pop() tea.Cmd        { return func() tea.Msg { return popMsg{} } }

// Results of background work. They are delivered to every view, so each
// carries enough information for a view to tell whether it's the recipient.
type (
	itemsLoadedMsg struct {
		seq   int
		items []ado.WorkItem
		err   error
	}
	itemSavedMsg struct {
		id   int
		item ado.WorkItem
		err  error
	}
	itemReloadedMsg struct {
		id   int
		item ado.WorkItem
		err  error
	}
	commentsLoadedMsg struct {
		id       int
		comments []ado.Comment
		err      error
	}
	commentAddedMsg struct {
		id      int
		text    string
		comment ado.Comment
		rev     int // the item's revision after the comment, or 0 if unknown
		err     error
	}
	statesLoadedMsg struct {
		typ    ado.WorkItemType
		states []ado.State
		err    error
	}
	teamLoadedMsg struct {
		members []ado.Identity
		err     error
	}
	// queryChangedMsg asks the backlog to reload after app.query changed.
	queryChangedMsg struct{}
)

func loadItems(b Backend, seq int, q ado.Query) tea.Cmd {
	return func() tea.Msg {
		items, err := b.QueryWorkItems(context.Background(), q)
		return itemsLoadedMsg{seq: seq, items: items, err: err}
	}
}

func saveItem(b Backend, id, rev int, changes []ado.Change) tea.Cmd {
	return func() tea.Msg {
		item, err := b.UpdateWorkItem(context.Background(), id, rev, changes)
		return itemSavedMsg{id: id, item: item, err: err}
	}
}

func reloadItem(b Backend, id int) tea.Cmd {
	return func() tea.Msg {
		item, err := b.GetWorkItem(context.Background(), id)
		return itemReloadedMsg{id: id, item: item, err: err}
	}
}

func loadComments(b Backend, id int) tea.Cmd {
	return func() tea.Msg {
		comments, err := b.GetComments(context.Background(), id)
		return commentsLoadedMsg{id: id, comments: comments, err: err}
	}
}

// addComment posts a comment, then reads the item's new revision: comments
// bump the revision, and the details view needs it for its next save.
func addComment(b Backend, id int, text string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		comment, err := b.AddComment(ctx, id, text)
		if err != nil {
			return commentAddedMsg{id: id, text: text, err: err}
		}
		msg := commentAddedMsg{id: id, text: text, comment: comment}
		if item, err := b.GetWorkItem(ctx, id); err == nil {
			msg.rev = item.Rev
		}
		return msg
	}
}

func loadStates(b Backend, t ado.WorkItemType) tea.Cmd {
	return func() tea.Msg {
		states, err := b.GetStates(context.Background(), t)
		return statesLoadedMsg{typ: t, states: states, err: err}
	}
}

func loadTeam(b Backend, team string) tea.Cmd {
	return func() tea.Msg {
		members, err := b.GetTeamMembers(context.Background(), team)
		return teamLoadedMsg{members: members, err: err}
	}
}
