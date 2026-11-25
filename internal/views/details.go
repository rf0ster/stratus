package views

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
	"github.com/rf0ster/stratus/internal/forms"
)

// Keys of the read-only fields (work item fields use their reference names).
const (
	keyParent      = "parent"
	keyAreaPath    = "area"
	keyIteration   = "iteration"
	keyCreatedBy   = "createdBy"
	keyCreatedDate = "createdDate"
)

// maxContentWidth keeps text readable on very wide terminals.
const maxContentWidth = 120

type detailsKeys struct {
	Save    key.Binding
	Discard key.Binding
	Reload  key.Binding
	Back    key.Binding
}

var defaultDetailsKeys = detailsKeys{
	Save:    key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Discard: key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "discard changes")),
	Reload:  key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload")),
	Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

// detailsView shows and edits a single work item. Field edits are kept
// locally until saved with ctrl+s, which sends them in one update, like the
// Save button in Azure DevOps.
type detailsView struct {
	app     *app
	item    ado.WorkItem
	keys    detailsKeys
	help    help.Model
	spinner spinner.Model

	form       *forms.Form
	assignedTo *forms.RadioField
	state      *forms.RadioField
	tabs       *forms.Tabs
	discussion *discussionField

	saving       bool
	reloading    bool
	confirmLeave bool // asking whether to save before going back
	leaveOnSave  bool // go back once the save succeeds
	status       string
	err          error
}

func newDetailsView(a *app, item ado.WorkItem) *detailsView {
	v := &detailsView{
		app:     a,
		item:    item,
		keys:    defaultDetailsKeys,
		help:    help.New(),
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(accentStyle)),
	}

	values := itemValues(item)
	title := forms.NewTextInputField(ado.FieldTitle, "Title", values[ado.FieldTitle])
	title.SetCharLimit(255)
	v.assignedTo = forms.NewRadioField(ado.FieldAssignedTo, "Assigned To", v.assigneeOptions(), values[ado.FieldAssignedTo], false)
	v.state = forms.NewRadioField(ado.FieldState, "State", v.stateOptions(), values[ado.FieldState], true)
	priority := forms.NewRadioField(ado.FieldPriority, "Priority", forms.Opts("1", "2", "3", "4"), values[ado.FieldPriority], true)
	tags := forms.NewTextInputField(ado.FieldTags, "Tags", values[ado.FieldTags])
	tags.SetPlaceholder("tag one; tag two")

	description := forms.NewTextAreaField(ado.FieldDescription, "Description", values[ado.FieldDescription])
	criteria := forms.NewTextAreaField(ado.FieldAcceptanceCriteria, "Acceptance Criteria", values[ado.FieldAcceptanceCriteria])
	v.discussion = newDiscussionField(func(text string) tea.Cmd { return addComment(a.backend, item.ID, text) })
	v.tabs = forms.NewTabs(description, criteria, v.discussion)

	v.form = forms.New(
		title,
		v.assignedTo,
		v.state,
		priority,
		tags,
		forms.NewReadonly(keyParent, "Parent", values[keyParent]),
		forms.NewReadonly(keyAreaPath, "Area Path", values[keyAreaPath]),
		forms.NewReadonly(keyIteration, "Iteration Path", values[keyIteration]),
		forms.NewReadonly(keyCreatedBy, "Created By", values[keyCreatedBy]),
		forms.NewReadonly(keyCreatedDate, "Created Date", values[keyCreatedDate]),
		v.tabs,
	)
	v.layout()
	return v
}

// itemValues maps a work item to form values. Rich text is converted to
// plain text for editing.
func itemValues(item ado.WorkItem) map[string]string {
	priority, parent, created := "", "", ""
	if item.Priority > 0 {
		priority = strconv.Itoa(item.Priority)
	}
	if item.ParentID > 0 {
		parent = "#" + strconv.Itoa(item.ParentID)
	}
	if !item.CreatedDate.IsZero() {
		created = formatDate(item.CreatedDate)
	}
	return map[string]string{
		ado.FieldTitle:              item.Title,
		ado.FieldAssignedTo:         item.AssignedTo.Ref(),
		ado.FieldState:              item.State,
		ado.FieldPriority:           priority,
		ado.FieldTags:               ado.JoinTags(item.Tags),
		ado.FieldDescription:        ado.HTMLToText(item.Description),
		ado.FieldAcceptanceCriteria: ado.HTMLToText(item.AcceptanceCriteria),
		keyParent:                   parent,
		keyAreaPath:                 item.AreaPath,
		keyIteration:                item.IterationPath,
		keyCreatedBy:                item.CreatedBy.Name(),
		keyCreatedDate:              created,
	}
}

func (v *detailsView) Init() tea.Cmd {
	cmds := []tea.Cmd{loadComments(v.app.backend, v.item.ID)}
	if _, ok := v.app.states[v.item.Type]; !ok {
		cmds = append(cmds, loadStates(v.app.backend, v.item.Type))
	}
	return tea.Batch(cmds...)
}

func (v *detailsView) Update(msg tea.Msg) tea.Cmd {
	cmd := v.update(msg)
	// Fields can change height (a radio opening its option list, a wrapped
	// title), so recompute how much room the tab content gets.
	v.layout()
	return cmd
}

func (v *detailsView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case statesLoadedMsg:
		if msg.typ == v.item.Type {
			if msg.err != nil {
				v.err = fmt.Errorf("couldn't load states: %w", msg.err)
			} else {
				v.state.SetOptions(v.stateOptions())
			}
		}
		return nil

	case teamLoadedMsg:
		v.assignedTo.SetOptions(v.assigneeOptions())
		return nil

	case commentsLoadedMsg:
		if msg.id == v.item.ID {
			v.discussion.setComments(msg.comments, msg.err)
		}
		return nil

	case commentAddedMsg:
		if msg.id != v.item.ID {
			return nil
		}
		v.discussion.commentAdded(msg.comment, msg.text, msg.err)
		if msg.err != nil {
			v.err = msg.err
			return nil
		}
		v.status = "Comment added."
		// The comment bumped the revision. Only take the new revision if
		// nothing else changed the item in between, so a concurrent edit by
		// someone else still makes the next save fail instead of
		// overwriting it.
		if msg.rev == v.item.Rev+1 {
			v.item.Rev = msg.rev
		}
		return nil

	case itemSavedMsg:
		if msg.id != v.item.ID {
			return nil
		}
		v.saving = false
		if msg.err != nil {
			v.leaveOnSave = false
			v.err = saveError(msg.err)
			return nil
		}
		v.setItem(msg.item)
		v.status = "Saved."
		if v.leaveOnSave {
			return pop()
		}
		return nil

	case itemReloadedMsg:
		if msg.id != v.item.ID {
			return nil
		}
		v.reloading = false
		if msg.err != nil {
			v.err = msg.err
			return nil
		}
		v.setItem(msg.item)
		v.status = "Reloaded."
		return loadComments(v.app.backend, v.item.ID)

	case spinner.TickMsg:
		if v.saving || v.reloading {
			var cmd tea.Cmd
			v.spinner, cmd = v.spinner.Update(msg)
			return cmd
		}
		return nil
	}

	// Anything else (cursor blinks and so on) goes to the form.
	return v.form.Update(msg)
}

func (v *detailsView) handleKey(msg tea.KeyMsg) tea.Cmd {
	// Ignore keys until a save or reload finishes: either one replaces the
	// form's values, which would clobber an edit made in the meantime.
	if v.saving || v.reloading {
		return nil
	}

	if v.confirmLeave {
		v.confirmLeave = false
		switch msg.String() {
		case "s", "y":
			v.leaveOnSave = true
			return v.save()
		case "d", "n":
			v.form.Reset()
			return pop()
		}
		return nil
	}

	v.status, v.err = "", nil
	editing := v.form.Editing()
	switch {
	case key.Matches(msg, v.keys.Save):
		if v.composing() {
			// In the comment box, ctrl+s only posts the comment. Field
			// changes stay pending until the next ctrl+s.
			return v.form.AcceptEdit()
		}
		// Accept the edit in progress so it's included in the save.
		v.form.AcceptEdit()
		save := v.save()
		if save == nil && v.err == nil {
			v.status = "No changes to save."
		}
		return save
	case key.Matches(msg, v.keys.Discard) && !editing:
		if v.form.Dirty() {
			v.form.Reset()
			v.status = "Changes discarded."
		}
		return nil
	case key.Matches(msg, v.keys.Reload) && !editing:
		v.form.Reset()
		v.reloading = true
		return tea.Batch(reloadItem(v.app.backend, v.item.ID), v.spinner.Tick)
	case key.Matches(msg, v.keys.Back) && !editing:
		if v.form.Dirty() {
			v.confirmLeave = true
			return nil
		}
		return pop()
	}
	return v.form.Update(msg)
}

// composing reports whether a comment is being written.
func (v *detailsView) composing() bool {
	return v.form.Focused() == v.tabs && v.tabs.Active() == v.discussion && v.discussion.Editing()
}

// save sends the changed fields in a single update.
func (v *detailsView) save() tea.Cmd {
	if !v.form.Dirty() {
		if v.leaveOnSave {
			return pop()
		}
		return nil
	}
	if v.discussion.posting {
		v.err = errors.New("a comment is still posting; save again in a moment")
		v.leaveOnSave = false
		return nil
	}
	changes, err := v.changes()
	if err != nil {
		v.err = err
		v.leaveOnSave = false
		return nil
	}
	if len(changes) == 0 {
		if v.leaveOnSave {
			return pop()
		}
		return nil
	}
	v.saving = true
	return tea.Batch(saveItem(v.app.backend, v.item.ID, v.item.Rev, changes), v.spinner.Tick)
}

// changes converts the form's changed values to work item field changes.
func (v *detailsView) changes() ([]ado.Change, error) {
	var changes []ado.Change
	for _, c := range v.form.Changes() {
		var value any = c.Value
		switch c.Key {
		case ado.FieldTitle:
			if strings.TrimSpace(c.Value) == "" {
				return nil, errors.New("the title can't be empty")
			}
		case ado.FieldPriority:
			n, err := strconv.Atoi(c.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid priority %q", c.Value)
			}
			value = n
		case ado.FieldTags:
			value = ado.JoinTags(ado.SplitTags(c.Value))
		case ado.FieldDescription, ado.FieldAcceptanceCriteria:
			value = ado.TextToHTML(c.Value)
		}
		changes = append(changes, ado.Change{Field: c.Key, Value: value})
	}
	return changes, nil
}

// setItem replaces the item after a save or reload. Form values become the
// new originals, discarding local changes.
func (v *detailsView) setItem(item ado.WorkItem) {
	v.item = item
	v.form.CancelEdit()
	v.form.SetValues(itemValues(item))
	v.state.SetOptions(v.stateOptions())
	v.assignedTo.SetOptions(v.assigneeOptions())
}

func saveError(err error) error {
	if ado.IsConflict(err) {
		return errors.New("someone else changed this item since it was loaded; ctrl+r reloads it (your changes will be lost)")
	}
	return err
}

// assigneeOptions lists the team (or, if the team couldn't be loaded, the
// people seen on the backlog).
func (v *detailsView) assigneeOptions() []forms.Option {
	people := v.app.members
	if len(people) == 0 {
		people = v.app.seen
	}
	opts := []forms.Option{{Label: "Unassigned", Value: ""}}
	for _, p := range people {
		opts = append(opts, forms.Option{Label: p.Name(), Value: p.Ref()})
	}
	if a := v.item.AssignedTo; !a.IsZero() {
		found := false
		for _, o := range opts {
			found = found || o.Value == a.Ref()
		}
		if !found {
			opts = append(opts, forms.Option{Label: a.Name(), Value: a.Ref()})
		}
	}
	return opts
}

// stateOptions lists the states of the item's type, once they're loaded.
func (v *detailsView) stateOptions() []forms.Option {
	states := v.app.states[v.item.Type]
	opts := make([]forms.Option, len(states))
	for i, s := range states {
		opts[i] = forms.Option{Label: s.Name, Value: s.Name}
	}
	return opts
}

func (v *detailsView) contentWidth() int { return min(v.app.innerWidth(), maxContentWidth) }

// layout fits the view to the screen. The tab content gets whatever height
// the rest leaves, and an open option list may use every row the other
// fields don't, so the header stays visible on small terminals.
func (v *detailsView) layout() {
	const (
		tabBar   = 2 // blank line and tab titles
		minBody  = 1 // tab content
		footer   = 3 // blank line, status and help
		rowsEach = 1 // fields other than the open radio take one row each
	)
	w := v.contentWidth()
	v.form.SetWidth(w)
	header := lipgloss.Height(v.header()) + 1 // and a blank line
	fields := v.form.Len() - 1                // fields above the tabs
	listRows := v.app.innerHeight() - header - (fields-1)*rowsEach - tabBar - minBody - footer
	v.assignedTo.SetMaxRows(listRows)
	v.state.SetMaxRows(listRows)

	used := header + lipgloss.Height(v.form.View()) - lipgloss.Height(v.tabs.View(v.form)) + tabBar + footer
	v.tabs.SetSize(w, max(v.app.innerHeight()-used, minBody))
}

func (v *detailsView) header() string {
	style := typeStyle(v.item.Type)
	line := style.Render(fmt.Sprintf("%s #%d", v.item.Type, v.item.ID))
	if v.item.State != "" {
		line += "  " + v.stateBadge()
	}
	title := lipgloss.NewStyle().Width(v.contentWidth()).Render(v.item.Title)
	return line + "\n" + style.Render(title)
}

// stateBadge renders the state with its Azure DevOps color, when known.
func (v *detailsView) stateBadge() string {
	dot := lipgloss.NewStyle()
	for _, s := range v.app.states[v.item.Type] {
		if strings.EqualFold(s.Name, v.item.State) && s.Color != "" {
			dot = dot.Foreground(lipgloss.Color("#" + s.Color))
		}
	}
	return dot.Render("●") + " " + v.item.State
}

func (v *detailsView) View() string {
	body := v.header() + "\n\n" + v.form.View()
	gap := v.app.innerHeight() - lipgloss.Height(body) - 2
	return body + strings.Repeat("\n", max(gap, 1)) + v.statusLine() + "\n" + v.helpLine()
}

func (v *detailsView) statusLine() string {
	switch {
	case v.confirmLeave:
		return warnStyle.Render("You have unsaved changes. s save · d discard · any other key to keep editing")
	case v.saving:
		return v.spinner.View() + " Saving…"
	case v.reloading:
		return v.spinner.View() + " Reloading…"
	case v.err != nil:
		return errorStyle.Render(v.err.Error())
	}
	if changes := v.form.Changes(); len(changes) > 0 {
		line := fmt.Sprintf("● %s · ctrl+s save · ctrl+z discard", plural(len(changes), "unsaved change"))
		if v.losesFormatting(changes) {
			line += " · saving simplifies the text's formatting"
		}
		return warnStyle.Render(line)
	}
	if v.status != "" {
		return successStyle.Render(v.status)
	}
	return ""
}

// losesFormatting reports whether saving would replace rich text (bold,
// links, images, tables) in a changed field with plain text.
func (v *detailsView) losesFormatting(changes []forms.Change) bool {
	for _, c := range changes {
		switch c.Key {
		case ado.FieldDescription:
			if ado.HasRichFormatting(v.item.Description) {
				return true
			}
		case ado.FieldAcceptanceCriteria:
			if ado.HasRichFormatting(v.item.AcceptanceCriteria) {
				return true
			}
		}
	}
	return false
}

func (v *detailsView) helpLine() string {
	hints := v.form.Hints()
	if !v.form.Editing() {
		hints = append(hints, v.keys.Save, v.keys.Back)
	}
	return v.help.ShortHelpView(hints)
}
