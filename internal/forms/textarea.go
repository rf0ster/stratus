package forms

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScrollKeys scroll multi-line content that isn't being edited.
var ScrollKeys = struct {
	PageUp, PageDown, HalfUp, HalfDown key.Binding
}{
	PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll up")),
	PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll down")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "½ page up")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "½ page down")),
}

// Scroll applies a scroll key (see ScrollKeys) to vp, reporting whether msg
// was one.
func Scroll(vp *viewport.Model, msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch {
	case key.Matches(km, ScrollKeys.PageUp):
		vp.PageUp()
	case key.Matches(km, ScrollKeys.PageDown):
		vp.PageDown()
	case key.Matches(km, ScrollKeys.HalfUp):
		vp.HalfPageUp()
	case key.Matches(km, ScrollKeys.HalfDown):
		vp.HalfPageDown()
	default:
		return false
	}
	return true
}

// TextAreaField is a multi-line text field. It renders only its content, not
// its label, so it is meant to live inside Tabs, which shows the label as the
// tab title. Ctrl+s accepts an edit and esc restores the previous text. When
// not editing, the content scrolls with pgup/pgdn and ctrl+u/ctrl+d.
type TextAreaField struct {
	key         string
	label       string
	placeholder string
	area        textarea.Model
	view        viewport.Model
	original    string
	editStart   string
	focused     bool
	editing     bool
	width       int
	height      int
}

// NewTextAreaField returns a text area holding value.
func NewTextAreaField(key, label, value string) *TextAreaField {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	// The default MaxHeight (99) stops you typing past 99 lines.
	ta.MaxHeight = 0
	ta.Prompt = "┃ "
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	t := &TextAreaField{
		key: key, label: label, area: ta,
		view:        viewport.New(0, 0),
		placeholder: "Nothing here yet. Press enter to edit.",
	}
	t.SetSize(60, 10)
	t.SetValue(value)
	return t
}

// SetPlaceholder sets the text shown when the field is empty.
func (t *TextAreaField) SetPlaceholder(s string) { t.placeholder = s }

func (t *TextAreaField) Key() string   { return t.key }
func (t *TextAreaField) Label() string { return t.label }
func (t *TextAreaField) Value() string { return t.area.Value() }
func (t *TextAreaField) Dirty() bool   { return t.Value() != t.original }

func (t *TextAreaField) SetValue(v string) {
	t.area.SetValue(v)
	// Read the value back: the textarea may sanitize it (tabs, \r).
	t.original = t.area.Value()
	t.view.GotoTop()
	t.refresh()
}

func (t *TextAreaField) Reset() {
	t.area.SetValue(t.original)
	t.refresh()
}

// SetSize sets the size of the content area.
func (t *TextAreaField) SetSize(width, height int) {
	t.width, t.height = max(width, 10), max(height, 1)
	t.area.SetWidth(t.width)
	t.area.SetHeight(t.height)
	t.view.Width = t.width - DefaultStyles().Box.GetHorizontalFrameSize()
	t.view.Height = t.height
	t.refresh()
}

// refresh re-renders the read-only view of the content.
func (t *TextAreaField) refresh() {
	t.view.SetContent(lipgloss.NewStyle().Width(t.view.Width).Render(t.Value()))
}

func (t *TextAreaField) Update(msg tea.Msg) tea.Cmd {
	if t.editing {
		var cmd tea.Cmd
		t.area, cmd = t.area.Update(msg)
		return cmd
	}
	Scroll(&t.view, msg)
	return nil
}

func (t *TextAreaField) View(f *Form) string {
	if t.editing {
		return t.area.View()
	}
	box := f.Styles.Box
	if t.focused {
		box = f.Styles.FocusedBox
	}
	if t.Value() == "" {
		return box.Height(t.height).Render(f.Styles.Muted.Render(t.placeholder))
	}
	return box.Render(t.view.View())
}

func (t *TextAreaField) Focus() tea.Cmd { t.focused = true; return nil }

func (t *TextAreaField) Blur() {
	t.focused = false
	if t.editing {
		t.Accept()
	}
}

func (t *TextAreaField) Editable() bool { return true }
func (t *TextAreaField) Editing() bool  { return t.editing }

func (t *TextAreaField) Edit() tea.Cmd {
	t.editing = true
	t.editStart = t.Value()
	cmd := t.area.Focus()
	// Start at the top of the text rather than the end.
	t.area, _ = t.area.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	return cmd
}

func (t *TextAreaField) Accept() tea.Cmd {
	t.editing = false
	t.area.Blur()
	t.refresh()
	return nil
}

func (t *TextAreaField) Cancel() {
	t.editing = false
	t.area.Blur()
	t.area.SetValue(t.editStart)
	t.refresh()
}

func (t *TextAreaField) Terminator() string { return "ctrl+s" }

func (t *TextAreaField) Hints() []key.Binding {
	if t.editing {
		return []key.Binding{hint("ctrl+s", "save"), hint("esc", "cancel")}
	}
	return []key.Binding{hint("pgup/pgdn", "scroll")}
}
