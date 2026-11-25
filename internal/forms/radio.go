package forms

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// defaultMaxRows caps how many rows a radio's option list uses.
const defaultMaxRows = 9

// Option is a choice in a RadioField.
type Option struct {
	Label string
	Value string
}

// Opts makes options whose labels are their values.
func Opts(values ...string) []Option {
	opts := make([]Option, len(values))
	for i, v := range values {
		opts[i] = Option{Label: v, Value: v}
	}
	return opts
}

// RadioField picks one of several options.
//
// When focused, ←/→ (h/l) step through the options directly. Enter opens the
// full option list: horizontal radios lay the options out in a row and use
// ←/→, vertical radios use a list and ↑/↓. Enter accepts the selection and
// esc restores the previous one.
type RadioField struct {
	key        string
	label      string
	options    []Option
	selected   int
	original   string
	editStart  string // value when editing began, restored by Cancel
	horizontal bool
	focused    bool
	editing    bool
	maxRows    int
}

// NewRadioField returns a radio field with value selected. If value isn't
// one of the options it is added, so the current value is never lost.
func NewRadioField(key, label string, options []Option, value string, horizontal bool) *RadioField {
	r := &RadioField{key: key, label: label, horizontal: horizontal, maxRows: defaultMaxRows}
	r.options = withOption(options, value)
	r.SetValue(value)
	return r
}

// SetOptions replaces the options, keeping the current and original values.
func (r *RadioField) SetOptions(options []Option) {
	current := r.Value()
	r.options = withOption(withOption(options, r.original), current)
	r.selected = r.index(current)
}

func withOption(options []Option, value string) []Option {
	for _, o := range options {
		if o.Value == value {
			return options
		}
	}
	label := value
	if label == "" {
		label = "—"
	}
	return append(append([]Option(nil), options...), Option{Label: label, Value: value})
}

// SetMaxRows limits how many rows the option list may use while editing,
// including the "n of m" line shown when the list scrolls.
func (r *RadioField) SetMaxRows(n int) { r.maxRows = max(n, 2) }

func (r *RadioField) index(value string) int {
	for i, o := range r.options {
		if o.Value == value {
			return i
		}
	}
	return 0
}

func (r *RadioField) Key() string   { return r.key }
func (r *RadioField) Label() string { return r.label }
func (r *RadioField) Dirty() bool   { return r.Value() != r.original }

func (r *RadioField) Value() string {
	if len(r.options) == 0 {
		return ""
	}
	return r.options[r.selected].Value
}

func (r *RadioField) SetValue(v string) {
	r.options = withOption(r.options, v)
	r.original = v
	r.selected = r.index(v)
}

func (r *RadioField) Reset() { r.selected = r.index(r.original) }

func (r *RadioField) Update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	prev, next := []string{"h", "left"}, []string{"l", "right"}
	if r.editing && !r.horizontal {
		prev, next = []string{"k", "up", "h", "left"}, []string{"j", "down", "l", "right"}
	}
	switch k := km.String(); {
	case slices.Contains(prev, k) && r.selected > 0:
		r.selected--
	case slices.Contains(next, k) && r.selected < len(r.options)-1:
		r.selected++
	}
	return nil
}

func (r *RadioField) View(f *Form) string {
	label := f.RenderLabel(r.label, r.focused, r.Dirty())
	valueStyle := f.Styles.Value
	if r.focused {
		valueStyle = f.Styles.FocusedValue
	}
	if len(r.options) == 0 {
		return label + f.Styles.Muted.Render("no options")
	}
	current := r.options[r.selected].Label

	switch {
	case r.editing:
		if r.horizontal {
			if row := "  " + r.viewRow(f); lipgloss.Width(row) <= f.ValueWidth() {
				return label + row
			}
		}
		return label + r.viewList(f)
	case r.focused:
		out := current
		if r.selected > 0 {
			out = "◀ " + out
		} else {
			out = "  " + out
		}
		if r.selected < len(r.options)-1 {
			out += " ▶"
		}
		return label + valueStyle.Render(out)
	default:
		return label + valueStyle.Render("  "+current)
	}
}

func (r *RadioField) viewRow(f *Form) string {
	parts := make([]string, len(r.options))
	for i, o := range r.options {
		if i == r.selected {
			parts[i] = f.Styles.Selected.Render("(•) " + o.Label)
		} else {
			parts[i] = f.Styles.Value.Render("( ) " + o.Label)
		}
	}
	return strings.Join(parts, "  ")
}

func (r *RadioField) viewList(f *Form) string {
	visible := len(r.options)
	if visible > r.maxRows {
		visible = r.maxRows - 1 // leave a row for "n of m"
	}
	start := min(max(r.selected-visible/2, 0), len(r.options)-visible)
	end := start + visible

	indent := strings.Repeat(" ", f.LabelWidth()+2)
	var b strings.Builder
	b.WriteString("  ")
	for i := start; i < end; i++ {
		if i > start {
			b.WriteString("\n" + indent)
		}
		o := r.options[i]
		if i == r.selected {
			b.WriteString(f.Styles.Selected.Render("(•) " + o.Label))
		} else {
			b.WriteString(f.Styles.Value.Render("( ) " + o.Label))
		}
	}
	if end-start < len(r.options) {
		b.WriteString("\n" + indent + f.Styles.Muted.Render(fmt.Sprintf("    %d of %d", r.selected+1, len(r.options))))
	}
	return b.String()
}

func (r *RadioField) Focus() tea.Cmd { r.focused = true; return nil }
func (r *RadioField) Blur()          { r.focused = false }
func (r *RadioField) Editable() bool { return len(r.options) > 1 }
func (r *RadioField) Editing() bool  { return r.editing }

func (r *RadioField) Edit() tea.Cmd {
	r.editing = true
	r.editStart = r.Value()
	return nil
}

func (r *RadioField) Accept() tea.Cmd { r.editing = false; return nil }

func (r *RadioField) Cancel() {
	r.editing = false
	r.selected = r.index(r.editStart)
}

func (r *RadioField) Terminator() string { return "enter" }

func (r *RadioField) Hints() []key.Binding {
	switch {
	case r.editing && r.horizontal:
		return []key.Binding{hint("←/→", "select"), hint("enter", "done"), hint("esc", "cancel")}
	case r.editing:
		return []key.Binding{hint("↑/↓", "select"), hint("enter", "done"), hint("esc", "cancel")}
	default:
		return []key.Binding{hint("←/→", "change")}
	}
}

func hint(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}
