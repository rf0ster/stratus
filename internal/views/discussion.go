package views

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
	"github.com/rf0ster/stratus/internal/forms"
)

const composeHeight = 5

var (
	commentAuthorStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	commentDateStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// discussionField is the Discussion tab: the work item's comments, newest
// first. Enter opens a box to write a comment and ctrl+s posts it. Comments
// are posted straight away; they aren't part of the item's unsaved changes.
type discussionField struct {
	comments []ado.Comment
	loaded   bool
	err      error

	compose textarea.Model
	view    viewport.Model
	post    func(text string) tea.Cmd

	focused bool
	editing bool
	posting bool
	width   int
	height  int
}

func newDiscussionField(post func(text string) tea.Cmd) *discussionField {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.Prompt = "┃ "
	ta.Placeholder = "Write a comment…"
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorAccent)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	d := &discussionField{compose: ta, view: viewport.New(0, 0), post: post}
	d.SetSize(60, 10)
	return d
}

func (d *discussionField) setComments(comments []ado.Comment, err error) {
	d.loaded = err == nil
	d.err = err
	if err == nil {
		d.comments = comments
	}
	d.refresh()
}

// commentAdded handles the result of posting. On failure the draft is kept
// so it can be retried.
func (d *discussionField) commentAdded(c ado.Comment, text string, err error) {
	d.posting = false
	if err != nil {
		d.compose.SetValue(text)
		return
	}
	d.comments = append([]ado.Comment{c}, d.comments...)
	d.refresh()
}

func (d *discussionField) Label() string {
	switch {
	case d.posting:
		return "Discussion (posting…)"
	case d.loaded:
		return "Discussion (" + strconv.Itoa(len(d.comments)) + ")"
	}
	return "Discussion"
}

func (d *discussionField) SetSize(width, height int) {
	d.width, d.height = max(width, 10), max(height, 1)
	d.compose.SetWidth(d.width)
	d.compose.SetHeight(composeHeight)
	d.view.Width = d.width - forms.DefaultStyles().Box.GetHorizontalFrameSize()
	d.view.Height = d.height
	if d.editing {
		d.view.Height = max(d.height-composeHeight-1, 1)
	}
	d.refresh()
}

func (d *discussionField) refresh() {
	var b strings.Builder
	for i, c := range d.comments {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(commentAuthorStyle.Render(c.CreatedBy.Name()))
		b.WriteString(commentDateStyle.Render(" · " + formatSince(c.CreatedDate)))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Width(d.view.Width).Render(ado.HTMLToText(c.Text)))
	}
	d.view.SetContent(b.String())
}

func (d *discussionField) Update(msg tea.Msg) tea.Cmd {
	if d.editing {
		var cmd tea.Cmd
		d.compose, cmd = d.compose.Update(msg)
		return cmd
	}
	forms.Scroll(&d.view, msg)
	return nil
}

func (d *discussionField) View(f *forms.Form) string {
	box := f.Styles.Box
	if d.focused {
		box = f.Styles.FocusedBox
	}

	var list string
	switch {
	case d.err != nil:
		list = box.Height(d.view.Height).Render(errorStyle.Render("Couldn't load comments: " + d.err.Error()))
	case !d.loaded:
		list = box.Height(d.view.Height).Render(f.Styles.Muted.Render("Loading comments…"))
	case len(d.comments) == 0:
		list = box.Height(d.view.Height).Render(f.Styles.Muted.Render("No comments yet. Press enter to add one."))
	default:
		list = box.Render(d.view.View())
	}

	if d.editing {
		return d.compose.View() + "\n" + list
	}
	return list
}

func (d *discussionField) Focus() tea.Cmd { d.focused = true; return nil }
func (d *discussionField) Blur()          { d.focused = false }
func (d *discussionField) Editable() bool { return !d.posting }
func (d *discussionField) Editing() bool  { return d.editing }

func (d *discussionField) Edit() tea.Cmd {
	if d.posting {
		return nil
	}
	d.editing = true
	d.SetSize(d.width, d.height)
	return d.compose.Focus()
}

// Accept posts the comment. An empty comment just closes the box.
func (d *discussionField) Accept() tea.Cmd {
	d.close()
	text := strings.TrimSpace(d.compose.Value())
	if text == "" {
		return nil
	}
	d.posting = true
	d.compose.Reset()
	return d.post(text)
}

// Cancel closes the box but keeps the draft for next time.
func (d *discussionField) Cancel() { d.close() }

func (d *discussionField) close() {
	d.editing = false
	d.compose.Blur()
	d.SetSize(d.width, d.height)
}

func (d *discussionField) Terminator() string { return "ctrl+s" }

func (d *discussionField) Hints() []key.Binding {
	if d.editing {
		return []key.Binding{
			key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "post")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close (keeps draft)")),
		}
	}
	hints := []key.Binding{key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "scroll"))}
	if !d.posting {
		hints = append(hints, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "comment")))
	}
	return hints
}

var _ forms.Field = (*discussionField)(nil)
