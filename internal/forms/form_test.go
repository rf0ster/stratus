package forms

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func press(f *Form, keys ...string) {
	for _, k := range keys {
		f.Update(keyMsg(k))
	}
}

func typeText(f *Form, s string) {
	for _, r := range s {
		f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestRadioQuickChangeAndEdit(t *testing.T) {
	state := NewRadioField("state", "State", Opts("New", "Active", "Closed"), "Active", true)
	f := New(state)

	press(f, "l")
	if state.Value() != "Closed" || !state.Dirty() {
		t.Fatalf("after l: value %q dirty %v", state.Value(), state.Dirty())
	}
	press(f, "l") // already at the last option
	if state.Value() != "Closed" {
		t.Errorf("moved past the last option: %q", state.Value())
	}

	// Editing then cancelling restores the value from before the edit.
	press(f, "enter", "h", "h", "esc")
	if state.Editing() || state.Value() != "Closed" {
		t.Errorf("after cancel: editing %v value %q", state.Editing(), state.Value())
	}

	// Editing then accepting keeps the new value.
	press(f, "enter", "h", "enter")
	if state.Editing() || state.Value() != "Active" || state.Dirty() {
		t.Errorf("after accept: editing %v value %q dirty %v", state.Editing(), state.Value(), state.Dirty())
	}
}

func TestRadioKeepsUnknownValue(t *testing.T) {
	r := NewRadioField("state", "State", Opts("New", "Active"), "Blocked", true)
	if r.Value() != "Blocked" {
		t.Errorf("value = %q, want Blocked", r.Value())
	}
	r.SetOptions(Opts("New", "Active", "Resolved"))
	if r.Value() != "Blocked" || r.Dirty() {
		t.Errorf("after SetOptions: value %q dirty %v", r.Value(), r.Dirty())
	}
}

func TestRadioCancelAfterOptionsChange(t *testing.T) {
	who := NewRadioField("who", "Who", []Option{{"Unassigned", ""}, {"Taylor", "taylor"}}, "taylor", false)
	f := New(who)

	press(f, "enter", "up")
	// The team finishes loading while the list is open.
	who.SetOptions([]Option{{"Unassigned", ""}, {"Alex", "alex"}, {"Jordan", "jordan"}, {"Taylor", "taylor"}})
	press(f, "esc")
	if who.Value() != "taylor" || who.Dirty() {
		t.Errorf("after cancel: value %q dirty %v, want taylor and clean", who.Value(), who.Dirty())
	}
}

func TestVerticalRadioUsesUpDownWhileEditing(t *testing.T) {
	who := NewRadioField("who", "Who", []Option{{"Unassigned", ""}, {"Ann", "ann"}, {"Bob", "bob"}}, "", false)
	other := NewReadonly("x", "Other", "value")
	f := New(who, other)

	press(f, "enter", "down", "down", "enter")
	if who.Value() != "bob" {
		t.Errorf("value = %q, want bob", who.Value())
	}
	if f.Focused() != who {
		t.Error("up/down while editing moved focus to another field")
	}
	if got := f.Changes(); len(got) != 1 || got[0] != (Change{Key: "who", Value: "bob"}) {
		t.Errorf("changes = %+v", got)
	}
}

func TestReadonlyCannotBeEdited(t *testing.T) {
	ro := NewReadonly("area", "Area", "Proj\\Team")
	next := NewTextInputField("title", "Title", "x")
	f := New(ro, next)

	press(f, "enter")
	if f.Editing() {
		t.Fatal("enter put a read-only field into edit mode")
	}
	// Navigation still works afterwards.
	press(f, "j")
	if f.Focused() != next {
		t.Error("j didn't move to the next field")
	}
}

func TestTextInputAcceptAndCancel(t *testing.T) {
	title := NewTextInputField("title", "Title", "Old")
	f := New(title)

	press(f, "enter")
	typeText(f, " title  ")
	press(f, "enter")
	if title.Value() != "Old title" || !title.Dirty() {
		t.Errorf("after accept: %q dirty %v", title.Value(), title.Dirty())
	}

	press(f, "enter", "backspace", "backspace", "esc")
	if title.Value() != "Old title" {
		t.Errorf("cancel didn't restore the text: %q", title.Value())
	}

	f.Reset()
	if title.Value() != "Old" || f.Dirty() {
		t.Errorf("after Reset: %q dirty %v", title.Value(), f.Dirty())
	}
}

func TestTabsAndTextArea(t *testing.T) {
	desc := NewTextAreaField("desc", "Description", "line one")
	ac := NewTextAreaField("ac", "Acceptance Criteria", "")
	tabs := NewTabs(desc, ac)
	title := NewTextInputField("title", "Title", "T")
	f := New(title, tabs)

	press(f, "j") // focus the tabs
	press(f, "l") // switch to the second tab
	if tabs.Active() != ac {
		t.Fatal("l didn't switch tabs")
	}
	press(f, "enter")
	typeText(f, "must work")
	press(f, "ctrl+s")
	if ac.Editing() || ac.Value() != "must work" {
		t.Errorf("after ctrl+s: editing %v value %q", ac.Editing(), ac.Value())
	}

	// Changes inside tabs are reported, and Reset reaches them.
	if got := f.Changes(); len(got) != 1 || got[0].Key != "ac" {
		t.Errorf("changes = %+v", got)
	}
	f.SetValues(map[string]string{"ac": "saved", "desc": "new desc"})
	if f.Dirty() || ac.Value() != "saved" || desc.Value() != "new desc" {
		t.Errorf("after SetValues: dirty %v ac %q desc %q", f.Dirty(), ac.Value(), desc.Value())
	}
}

func TestTextAreaKeepsLongText(t *testing.T) {
	long := strings.Repeat("line\n", 300) + "end"
	ta := NewTextAreaField("desc", "Description", long)
	if ta.Value() != long || ta.Dirty() {
		t.Errorf("long text was changed: %d lines kept, dirty %v", strings.Count(ta.Value(), "\n"), ta.Dirty())
	}
}

func TestHintsAndView(t *testing.T) {
	state := NewRadioField("state", "State", Opts("New", "Active"), "New", true)
	f := New(state, NewReadonly("area", "Area", "A"))
	view := f.View()
	if !strings.Contains(view, "State:") || !strings.Contains(view, "Area:") {
		t.Errorf("view missing labels:\n%s", view)
	}
	help := func() string {
		var parts []string
		for _, h := range f.Hints() {
			parts = append(parts, h.Help().Key+" "+h.Help().Desc)
		}
		return strings.Join(parts, ", ")
	}
	if h := help(); !strings.Contains(h, "enter edit") {
		t.Errorf("radio hints = %q, want enter edit", h)
	}
	press(f, "j")
	if h := help(); strings.Contains(h, "enter") {
		t.Errorf("read-only hints = %q, shouldn't offer enter", h)
	}
}
