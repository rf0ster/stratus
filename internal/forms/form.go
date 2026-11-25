// Package forms provides keyboard-driven form widgets for Bubble Tea.
//
// A Form is a vertical list of fields. j/k (or ↑/↓, tab/shift+tab) move
// between fields and enter starts editing the focused field. While editing,
// the field's terminator key (Terminator) accepts the edit and esc cancels
// it, restoring the value from before the edit.
//
// Fields that hold a value (ValueField) remember their original value, so
// the form can report which fields changed (Changes), discard changes
// (Reset), and take new original values after a save (SetValues).
package forms

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Field is a single row of a form.
type Field interface {
	// Label is shown next to the field. Fields inside Tabs use it as the
	// tab title.
	Label() string
	// View renders the field. The form is passed for label layout and styles.
	View(f *Form) string
	// Update handles messages. While the field is editing it receives every
	// message except the cancel and terminator keys; otherwise it receives
	// keys the form doesn't use for navigation.
	Update(msg tea.Msg) tea.Cmd

	Focus() tea.Cmd
	Blur()

	// Editable reports whether Edit does anything.
	Editable() bool
	// Edit starts editing.
	Edit() tea.Cmd
	// Editing reports whether the field is being edited.
	Editing() bool
	// Accept finishes editing and keeps the new value.
	Accept() tea.Cmd
	// Cancel finishes editing and restores the value from before the edit.
	Cancel()
	// Terminator is the key that accepts an edit, like "enter".
	Terminator() string
}

// ValueField is a field that holds a value that can be changed and saved.
type ValueField interface {
	Field
	// Key identifies the value, for example a work item field name.
	Key() string
	// Value is the current (possibly unsaved) value.
	Value() string
	// Dirty reports whether Value differs from the original value.
	Dirty() bool
	// SetValue replaces both the original and the current value.
	SetValue(v string)
	// Reset discards changes, restoring the original value.
	Reset()
}

// Container is a field that holds other fields, like Tabs.
type Container interface {
	Fields() []Field
}

// Sizer is implemented by fields whose size is set by their parent.
type Sizer interface {
	SetSize(width, height int)
}

// Hinter is implemented by fields that describe their keys for help text.
type Hinter interface {
	Hints() []key.Binding
}

// Change is a modified field value.
type Change struct {
	Key   string
	Value string
}

// KeyMap holds the form's keys.
type KeyMap struct {
	Next   key.Binding
	Prev   key.Binding
	Edit   key.Binding
	Cancel key.Binding
}

// DefaultKeyMap is the default form key map.
var DefaultKeyMap = KeyMap{
	Next:   key.NewBinding(key.WithKeys("down", "j", "tab"), key.WithHelp("↓/j", "next field")),
	Prev:   key.NewBinding(key.WithKeys("up", "k", "shift+tab"), key.WithHelp("↑/k", "prev field")),
	Edit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// Form is a vertical list of fields with one focused field.
type Form struct {
	Styles Styles
	Keys   KeyMap

	fields     []Field
	focus      int
	width      int
	labelWidth int
}

// New returns a form with the first field focused.
func New(fields ...Field) *Form {
	f := &Form{Styles: DefaultStyles(), Keys: DefaultKeyMap, fields: fields, width: 80}
	for _, field := range fields {
		f.labelWidth = max(f.labelWidth, lipgloss.Width(field.Label()))
	}
	f.labelWidth += 2 // room for ": "
	if len(fields) > 0 {
		fields[0].Focus()
	}
	return f
}

// Update routes msg to the focused field, handling navigation and the
// edit/accept/cancel keys.
func (f *Form) Update(msg tea.Msg) tea.Cmd {
	field := f.Focused()
	if field == nil {
		return nil
	}

	if field.Editing() {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch {
			case key.Matches(km, f.Keys.Cancel):
				field.Cancel()
				return nil
			case km.String() == field.Terminator():
				return field.Accept()
			}
		}
		return field.Update(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, f.Keys.Next):
			return f.move(1)
		case key.Matches(km, f.Keys.Prev):
			return f.move(-1)
		case key.Matches(km, f.Keys.Edit):
			if field.Editable() {
				return field.Edit()
			}
			return nil
		}
	}
	return field.Update(msg)
}

// View renders every field.
func (f *Form) View() string {
	rows := make([]string, len(f.fields))
	for i, field := range f.fields {
		rows[i] = field.View(f)
	}
	return strings.Join(rows, "\n")
}

// Len returns the number of top-level fields.
func (f *Form) Len() int { return len(f.fields) }

// Focused returns the focused field, or nil if the form is empty.
func (f *Form) Focused() Field {
	if len(f.fields) == 0 {
		return nil
	}
	return f.fields[f.focus]
}

// Editing reports whether the focused field is being edited.
func (f *Form) Editing() bool {
	field := f.Focused()
	return field != nil && field.Editing()
}

// AcceptEdit accepts the focused field's edit, if any.
func (f *Form) AcceptEdit() tea.Cmd {
	if f.Editing() {
		return f.Focused().Accept()
	}
	return nil
}

// CancelEdit cancels the focused field's edit, if any.
func (f *Form) CancelEdit() {
	if f.Editing() {
		f.Focused().Cancel()
	}
}

// Changes returns the value fields that differ from their original values,
// in form order.
func (f *Form) Changes() []Change {
	var changes []Change
	f.walk(func(field Field) {
		if vf, ok := field.(ValueField); ok && vf.Dirty() {
			changes = append(changes, Change{Key: vf.Key(), Value: vf.Value()})
		}
	})
	return changes
}

// Dirty reports whether any value field has changed.
func (f *Form) Dirty() bool { return len(f.Changes()) > 0 }

// Reset cancels any edit in progress and discards all changes.
func (f *Form) Reset() {
	f.CancelEdit()
	f.walk(func(field Field) {
		if vf, ok := field.(ValueField); ok {
			vf.Reset()
		}
	})
}

// SetValues sets the original and current value of each value field whose
// key is in values.
func (f *Form) SetValues(values map[string]string) {
	f.walk(func(field Field) {
		if vf, ok := field.(ValueField); ok {
			if v, ok := values[vf.Key()]; ok {
				vf.SetValue(v)
			}
		}
	})
}

// SetWidth sets the width fields may use.
func (f *Form) SetWidth(w int) { f.width = max(w, 20) }

// Width returns the width fields may use.
func (f *Form) Width() int { return f.width }

// LabelWidth returns the width of the label column.
func (f *Form) LabelWidth() int { return f.labelWidth }

// ValueWidth returns the width available to the right of the label column.
func (f *Form) ValueWidth() int { return max(f.width-f.labelWidth, 10) }

// RenderLabel renders a field label padded to the label column.
func (f *Form) RenderLabel(label string, focused, dirty bool) string {
	style := f.Styles.Label
	if focused {
		style = f.Styles.FocusedLabel
	}
	if dirty {
		style = style.Foreground(f.Styles.Dirty.GetForeground())
	}
	return style.Width(f.labelWidth).Render(label + ":")
}

// Hints returns key bindings for the help line, depending on the focused
// field and whether it is being edited.
func (f *Form) Hints() []key.Binding {
	field := f.Focused()
	if field == nil {
		return nil
	}
	var hints []key.Binding
	if !field.Editing() {
		hints = append(hints, key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "field")))
	}
	if h, ok := field.(Hinter); ok {
		hints = append(hints, h.Hints()...)
	} else if field.Editing() {
		hints = append(hints,
			key.NewBinding(key.WithKeys(field.Terminator()), key.WithHelp(field.Terminator(), "done")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")))
	}
	if !field.Editing() && field.Editable() && !hasKey(hints, f.Keys.Edit.Keys()...) {
		hints = append(hints, f.Keys.Edit)
	}
	return hints
}

// hasKey reports whether any binding uses one of keys.
func hasKey(bindings []key.Binding, keys ...string) bool {
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if slices.Contains(keys, k) {
				return true
			}
		}
	}
	return false
}

func (f *Form) move(delta int) tea.Cmd {
	next := f.focus + delta
	if next < 0 || next >= len(f.fields) {
		return nil
	}
	f.fields[f.focus].Blur()
	f.focus = next
	return f.fields[f.focus].Focus()
}

func (f *Form) walk(fn func(Field)) {
	var visit func(fields []Field)
	visit = func(fields []Field) {
		for _, field := range fields {
			fn(field)
			if c, ok := field.(Container); ok {
				visit(c.Fields())
			}
		}
	}
	visit(f.fields)
}

// truncate shortens s to fit in width cells, adding an ellipsis.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// singleLine collapses newlines so a value renders on one row.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
