package forms

import tea "github.com/charmbracelet/bubbletea"

// Readonly displays a value that can't be edited. It is a ValueField so
// SetValues can refresh it, but it is never dirty.
type Readonly struct {
	key     string
	label   string
	value   string
	focused bool
}

// NewReadonly returns a read-only field.
func NewReadonly(key, label, value string) *Readonly {
	return &Readonly{key: key, label: label, value: value}
}

func (r *Readonly) Key() string       { return r.key }
func (r *Readonly) Label() string     { return r.label }
func (r *Readonly) Value() string     { return r.value }
func (r *Readonly) Dirty() bool       { return false }
func (r *Readonly) SetValue(v string) { r.value = v }
func (r *Readonly) Reset()            {}

func (r *Readonly) View(f *Form) string {
	style := f.Styles.Value
	if r.focused {
		style = f.Styles.FocusedValue
	}
	value := "  " + truncate(singleLine(r.value), f.ValueWidth()-2)
	if r.value == "" {
		return f.RenderLabel(r.label, r.focused, false) + f.Styles.Muted.Render("  —")
	}
	return f.RenderLabel(r.label, r.focused, false) + style.Render(value)
}

func (r *Readonly) Update(tea.Msg) tea.Cmd { return nil }
func (r *Readonly) Focus() tea.Cmd         { r.focused = true; return nil }
func (r *Readonly) Blur()                  { r.focused = false }
func (r *Readonly) Editable() bool         { return false }
func (r *Readonly) Edit() tea.Cmd          { return nil }
func (r *Readonly) Editing() bool          { return false }
func (r *Readonly) Accept() tea.Cmd        { return nil }
func (r *Readonly) Cancel()                {}
func (r *Readonly) Terminator() string     { return "" }
