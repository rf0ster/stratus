package forms

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Tabs shows one of several fields at a time, using each field's label as
// its tab title. It is a Field itself and forwards everything to the active
// field. When focused (and not editing), ←/→ (h/l) switch tabs.
type Tabs struct {
	fields  []Field
	active  int
	focused bool
}

// NewTabs returns tabs over fields. The first field is active.
func NewTabs(fields ...Field) *Tabs {
	if len(fields) == 0 {
		panic("forms: NewTabs needs at least one field")
	}
	return &Tabs{fields: fields}
}

// Fields implements Container.
func (t *Tabs) Fields() []Field { return t.fields }

// Active returns the active field.
func (t *Tabs) Active() Field { return t.fields[t.active] }

// Label is empty: the tab bar replaces the label column.
func (t *Tabs) Label() string { return "" }

// SetSize sets the size of every field that implements Sizer.
func (t *Tabs) SetSize(width, height int) {
	for _, f := range t.fields {
		if s, ok := f.(Sizer); ok {
			s.SetSize(width, height)
		}
	}
}

func (t *Tabs) Update(msg tea.Msg) tea.Cmd {
	if !t.Active().Editing() {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "h", "left":
				return t.activate(t.active - 1)
			case "l", "right":
				return t.activate(t.active + 1)
			}
		}
	}
	return t.Active().Update(msg)
}

func (t *Tabs) View(f *Form) string {
	titles := make([]string, len(t.fields))
	for i, field := range t.fields {
		style := f.Styles.Tab
		switch {
		case i == t.active && t.focused:
			style = f.Styles.FocusedTab
		case i == t.active:
			style = f.Styles.ActiveTab
		}
		title := field.Label()
		if vf, ok := field.(ValueField); ok && vf.Dirty() {
			title += " *"
			if i != t.active {
				style = style.Foreground(f.Styles.Dirty.GetForeground())
			}
		}
		titles[i] = style.Render(title)
	}
	// A blank line separates the tabs from the fields above them.
	return "\n" + strings.Join(titles, " ") + "\n" + t.Active().View(f)
}

// activate switches to tab i, wrapping around.
func (t *Tabs) activate(i int) tea.Cmd {
	t.Active().Blur()
	t.active = (i + len(t.fields)) % len(t.fields)
	if t.focused {
		return t.Active().Focus()
	}
	return nil
}

func (t *Tabs) Focus() tea.Cmd {
	t.focused = true
	return t.Active().Focus()
}

func (t *Tabs) Blur() {
	t.focused = false
	t.Active().Blur()
}

func (t *Tabs) Editable() bool     { return t.Active().Editable() }
func (t *Tabs) Edit() tea.Cmd      { return t.Active().Edit() }
func (t *Tabs) Editing() bool      { return t.Active().Editing() }
func (t *Tabs) Accept() tea.Cmd    { return t.Active().Accept() }
func (t *Tabs) Cancel()            { t.Active().Cancel() }
func (t *Tabs) Terminator() string { return t.Active().Terminator() }

func (t *Tabs) Hints() []key.Binding {
	var hints []key.Binding
	if !t.Editing() {
		hints = append(hints, hint("←/→", "tab"))
	}
	if h, ok := t.Active().(Hinter); ok {
		hints = append(hints, h.Hints()...)
	}
	return hints
}
