package forms

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TextInputField is a single-line text field. Enter accepts an edit and esc
// restores the previous text. Accepted text is trimmed.
type TextInputField struct {
	key       string
	label     string
	input     textinput.Model
	original  string
	editStart string
	focused   bool
	editing   bool
}

// NewTextInputField returns a text field holding value.
func NewTextInputField(key, label, value string) *TextInputField {
	ti := textinput.New()
	ti.Prompt = ""
	t := &TextInputField{key: key, label: label, input: ti}
	t.SetValue(value)
	return t
}

// SetCharLimit limits the length of the text (0 means no limit).
func (t *TextInputField) SetCharLimit(n int) { t.input.CharLimit = n }

// SetPlaceholder sets the text shown while editing an empty field.
func (t *TextInputField) SetPlaceholder(s string) { t.input.Placeholder = s }

func (t *TextInputField) Key() string   { return t.key }
func (t *TextInputField) Label() string { return t.label }
func (t *TextInputField) Value() string { return t.input.Value() }
func (t *TextInputField) Dirty() bool   { return t.Value() != t.original }

func (t *TextInputField) SetValue(v string) {
	t.input.SetValue(v)
	// Read the value back: the input may sanitize it (newlines, tabs).
	t.original = t.input.Value()
}

func (t *TextInputField) Reset() { t.input.SetValue(t.original) }

func (t *TextInputField) Update(msg tea.Msg) tea.Cmd {
	if !t.editing {
		return nil
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return cmd
}

func (t *TextInputField) View(f *Form) string {
	label := f.RenderLabel(t.label, t.focused, t.Dirty())
	if t.editing {
		t.input.Width = f.ValueWidth() - 3
		return label + "  " + t.input.View()
	}
	if t.Value() == "" {
		return label + f.Styles.Muted.Render("  —")
	}
	style := f.Styles.Value
	if t.focused {
		style = f.Styles.FocusedValue
	}
	return label + style.Render("  "+truncate(t.Value(), f.ValueWidth()-2))
}

func (t *TextInputField) Focus() tea.Cmd { t.focused = true; return nil }

func (t *TextInputField) Blur() {
	t.focused = false
	if t.editing {
		t.Accept()
	}
}

func (t *TextInputField) Editable() bool { return true }
func (t *TextInputField) Editing() bool  { return t.editing }

func (t *TextInputField) Edit() tea.Cmd {
	t.editing = true
	t.editStart = t.Value()
	t.input.CursorEnd()
	return t.input.Focus()
}

func (t *TextInputField) Accept() tea.Cmd {
	t.editing = false
	t.input.Blur()
	t.input.SetValue(strings.TrimSpace(t.input.Value()))
	return nil
}

func (t *TextInputField) Cancel() {
	t.editing = false
	t.input.Blur()
	t.input.SetValue(t.editStart)
}

func (t *TextInputField) Terminator() string { return "enter" }

func (t *TextInputField) Hints() []key.Binding {
	if t.editing {
		return []key.Binding{hint("enter", "done"), hint("esc", "cancel")}
	}
	return nil
}
