package views

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rf0ster/stratus/internal/ado"
	"github.com/rf0ster/stratus/internal/mockserver"
)

// harness runs a Model like a tiny Bubble Tea event loop: commands run in
// goroutines and their messages are fed back in by waitFor.
type harness struct {
	t      *testing.T
	model  Model
	client *ado.Client
	msgs   chan tea.Msg
	quit   bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	srv := httptest.NewServer(mockserver.New(mockserver.Options{}))
	t.Cleanup(srv.Close)
	client, err := ado.NewClient(ado.Config{OrgURL: srv.URL + "/mock", Project: mockserver.DefaultProject})
	if err != nil {
		t.Fatal(err)
	}

	h := &harness{t: t, client: client, msgs: make(chan tea.Msg, 100)}
	h.model = New(Config{
		Backend: client,
		Source:  "test",
		Query:   ado.Query{AssignedTo: ado.Me, HideCompleted: true},
	})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 45})
	h.exec(h.model.Init())
	return h
}

func (h *harness) exec(cmd tea.Cmd) {
	if cmd != nil {
		go func() { h.msgs <- cmd() }()
	}
}

func (h *harness) send(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, cmd := range msg {
			h.exec(cmd)
		}
		return
	case tea.QuitMsg:
		h.quit = true
		return
	case spinner.TickMsg, cursor.BlinkMsg:
		return // animation timers would keep the loop busy forever
	}
	switch name := fmt.Sprintf("%T", msg); {
	case strings.HasSuffix(name, "initialBlinkMsg"):
		return
	case strings.HasSuffix(name, "sequenceMsg"):
		// tea.Sequence: run the commands one after another, in order.
		v := reflect.ValueOf(msg)
		cmds := make([]tea.Cmd, v.Len())
		for i := range cmds {
			cmds[i], _ = v.Index(i).Interface().(tea.Cmd)
		}
		go h.runSequence(cmds)
		return
	}
	m, cmd := h.model.Update(msg)
	h.model = m.(Model)
	h.exec(cmd)
}

// runSequence runs commands in order, finishing any batch before moving on,
// the way Bubble Tea runs tea.Sequence.
func (h *harness) runSequence(cmds []tea.Cmd) {
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		msg := cmd()
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			h.msgs <- msg
			continue
		}
		var wg sync.WaitGroup
		for _, c := range batch {
			if c != nil {
				wg.Add(1)
				go func() { defer wg.Done(); h.msgs <- c() }()
			}
		}
		wg.Wait()
	}
}

func (h *harness) press(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "ctrl+s":
			msg = tea.KeyMsg{Type: tea.KeyCtrlS}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.send(msg)
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// waitFor processes messages until cond holds.
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case msg := <-h.msgs:
			h.send(msg)
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s; screen:\n%s", what, h.model.View())
		}
	}
}

// waitForReload waits for the backlog to be on top and finish a new load.
func (h *harness) waitForReload() {
	h.t.Helper()
	seq := h.backlog().seq
	h.waitFor("reload", func() bool {
		_, onBacklog := h.model.top().(*backlogView)
		return onBacklog && h.backlog().seq > seq && !h.backlog().loading
	})
}

func (h *harness) backlog() *backlogView { return h.model.stack[0].(*backlogView) }

func (h *harness) details() *detailsView {
	h.t.Helper()
	d, ok := h.model.top().(*detailsView)
	if !ok {
		h.t.Fatalf("top view is %T, want details", h.model.top())
	}
	return d
}

// open selects the item with the given ID and opens it.
func (h *harness) open(id int) *detailsView {
	h.t.Helper()
	b := h.backlog()
	for b.table.Cursor() > 0 {
		h.press("k")
	}
	for {
		item, ok := b.selected()
		if !ok {
			h.t.Fatalf("item %d isn't on the backlog", id)
		}
		if item.ID == id {
			break
		}
		h.press("down")
	}
	h.press("enter")
	h.waitFor("details", func() bool { _, ok := h.model.top().(*detailsView); return ok })
	d := h.details()
	h.waitFor("comments and states", func() bool {
		return d.discussion.loaded && len(h.model.app.states[d.item.Type]) > 0 && len(h.model.app.members) > 0
	})
	return d
}

func TestEditAndSaveWorkItem(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })

	screen := h.model.View()
	for _, want := range []string{"Assigned to me", "Sign in with Google", "Implement OIDC callback handler"} {
		if !strings.Contains(screen, want) {
			t.Errorf("backlog doesn't show %q:\n%s", want, screen)
		}
	}

	d := h.open(1013) // a Task in the New state, priority 2
	if screen := h.model.View(); !strings.Contains(screen, "Task #1013") || !strings.Contains(screen, "Add X-RateLimit-* response headers") {
		t.Fatalf("details screen:\n%s", screen)
	}
	if got := d.state.Value(); got != "New" {
		t.Errorf("state field shows %q, want the item's state New", got)
	}
	if got := d.assignedTo.Value(); got != "alex.chen@example.com" {
		t.Errorf("assigned-to field = %q, want the item's assignee", got)
	}

	// Title → Assigned To → State, then step the state to Active.
	h.press("j", "j", "l")
	if got := d.state.Value(); got != "Active" {
		t.Fatalf("state = %q after l, want Active", got)
	}
	if !strings.Contains(h.model.View(), "1 unsaved change") {
		t.Errorf("no unsaved-change notice:\n%s", h.model.View())
	}

	h.press("ctrl+s")
	h.waitFor("save", func() bool { return !d.saving && d.status == "Saved." })
	saved, err := h.client.GetWorkItem(context.Background(), 1013)
	if err != nil {
		t.Fatal(err)
	}
	if saved.State != "Active" {
		t.Errorf("server state = %q, want Active", saved.State)
	}
	if d.form.Dirty() {
		t.Error("form still dirty after saving")
	}

	// Post a comment from the Discussion tab.
	h.press("j", "j", "j", "j", "j", "j", "j", "j") // to the tabs
	h.press("l", "l")                               // Description → Acceptance Criteria → Discussion
	if d.tabs.Active() != d.discussion {
		t.Fatalf("active tab is %s", d.tabs.Active().Label())
	}
	h.press("enter")
	h.typeText("Picking this up today")
	h.press("ctrl+s")
	h.waitFor("comment", func() bool { return !d.discussion.posting && d.status == "Comment added." })
	comments, err := h.client.GetComments(context.Background(), 1013)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || ado.HTMLToText(comments[0].Text) != "Picking this up today" {
		t.Errorf("server comments = %+v", comments)
	}

	// The comment bumped the revision; saving again must not conflict.
	h.press("k", "k", "k", "k", "k", "k", "k") // back up to Priority
	h.press("h")
	h.press("ctrl+s")
	h.waitFor("second save", func() bool { return !d.saving })
	if d.err != nil {
		t.Fatalf("second save failed: %v", d.err)
	}

	// The backlog shows the saved values.
	h.press("esc")
	h.waitFor("backlog", func() bool { _, ok := h.model.top().(*backlogView); return ok })
	for _, item := range h.backlog().items {
		if item.ID == 1013 && (item.State != "Active" || item.Priority != 1) {
			t.Errorf("backlog row = %s/P%d, want Active/P1", item.State, item.Priority)
		}
	}
}

func TestCommentThenSave(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })
	d := h.open(1013)

	// Change the priority, then write a comment. ctrl+s in the comment box
	// only posts the comment; the priority change stays pending.
	h.press("j", "j", "j", "l")
	h.press("j", "j", "j", "j", "j", "j", "j", "l", "l", "enter")
	h.typeText("Bumped the priority")
	h.press("ctrl+s")
	h.waitFor("comment", func() bool { return !d.discussion.posting && d.status == "Comment added." })
	if !d.form.Dirty() {
		t.Fatal("posting a comment also saved the field change")
	}

	// Saving now must not trip over the revision the comment added.
	h.press("ctrl+s")
	h.waitFor("save", func() bool { return !d.saving && (d.status == "Saved." || d.err != nil) })
	if d.err != nil {
		t.Fatalf("save after commenting failed: %v", d.err)
	}

	// Comment again, go back, reopen from the backlog and save: the backlog's
	// copy must have picked up the comment's revision too.
	h.press("enter")
	h.typeText("One more thing")
	h.press("ctrl+s")
	h.waitFor("second comment", func() bool { return !d.discussion.posting && d.status == "Comment added." })
	h.press("esc")
	h.waitFor("backlog", func() bool { _, ok := h.model.top().(*backlogView); return ok })
	d = h.open(1013)
	h.press("j", "j", "j", "h")
	h.press("ctrl+s")
	h.waitFor("save after reopening", func() bool { return !d.saving && (d.status == "Saved." || d.err != nil) })
	if d.err != nil {
		t.Fatalf("save after reopening failed: %v", d.err)
	}

	item, err := h.client.GetWorkItem(context.Background(), 1013)
	if err != nil {
		t.Fatal(err)
	}
	comments, err := h.client.GetComments(context.Background(), 1013)
	if err != nil {
		t.Fatal(err)
	}
	if item.Priority != 2 || len(comments) != 2 {
		t.Errorf("server has priority %d and %d comments, want 2 and 2", item.Priority, len(comments))
	}
}

func TestStaleRefreshKeepsNewerRevision(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })
	b := h.backlog()

	// A refresh that started before a save finishes after it.
	stale := append([]ado.WorkItem(nil), b.items...)
	saved := stale[0]
	saved.Rev++
	saved.Title = "Saved title"
	h.send(itemSavedMsg{id: saved.ID, item: saved})
	h.send(itemsLoadedMsg{seq: b.seq, items: stale})

	if got := b.items[0]; got.Rev != saved.Rev || got.Title != "Saved title" {
		t.Errorf("stale load replaced the saved item: rev %d title %q", got.Rev, got.Title)
	}
}

func TestLeavingWithUnsavedChangesAsks(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })
	d := h.open(1013)

	h.press("j", "j", "l", "esc")
	if !d.confirmLeave {
		t.Fatal("esc with unsaved changes didn't ask first")
	}
	h.press("x") // any other key keeps editing
	if d.confirmLeave || h.model.top() != View(d) || !d.form.Dirty() {
		t.Fatal("keep editing didn't keep the changes")
	}

	h.press("esc", "d")
	h.waitFor("backlog", func() bool { _, ok := h.model.top().(*backlogView); return ok })
	if item, _ := h.client.GetWorkItem(context.Background(), 1013); item.State != "New" {
		t.Errorf("discarded change was saved: state %q", item.State)
	}
}

func TestAssigneeFilterAndEmptyBacklog(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })

	h.press("a")
	h.waitFor("assignee view", func() bool { _, ok := h.model.top().(*assigneeView); return ok })
	h.model.top().(*assigneeView).input.SetValue("Nobody Here")
	h.press("enter")
	h.waitForReload()
	if len(h.backlog().items) != 0 || !strings.Contains(h.model.View(), "No work items match") {
		t.Fatalf("expected an empty backlog:\n%s", h.model.View())
	}

	// Enter on an empty backlog used to crash fazure.
	h.press("enter")
	time.Sleep(50 * time.Millisecond)
	for len(h.msgs) > 0 {
		h.send(<-h.msgs)
	}
	if _, ok := h.model.top().(*backlogView); !ok {
		t.Fatalf("enter on an empty backlog opened %T", h.model.top())
	}

	h.press("a")
	h.waitFor("assignee view", func() bool { _, ok := h.model.top().(*assigneeView); return ok })
	h.model.top().(*assigneeView).input.SetValue("Jordan Lee")
	h.press("enter")
	h.waitForReload()
	items := h.backlog().items
	if len(items) == 0 {
		t.Fatal("no items for Jordan Lee")
	}
	for _, item := range items {
		if item.AssignedTo.DisplayName != "Jordan Lee" {
			t.Errorf("item %d is assigned to %q", item.ID, item.AssignedTo.DisplayName)
		}
	}

	h.press("q")
	h.waitFor("quit", func() bool { return h.quit })
}

func TestColumnFilters(t *testing.T) {
	h := newHarness(t)
	h.waitFor("backlog", func() bool { return h.backlog().loaded })
	b := h.backlog()
	all := len(b.items)

	shown := func() []ado.WorkItem {
		var items []ado.WorkItem
		for _, i := range b.visible {
			items = append(items, b.items[i])
		}
		return items
	}
	// want returns the loaded items whose title and state contain the given
	// text, ignoring case.
	want := func(title, state string) []ado.WorkItem {
		var items []ado.WorkItem
		for _, item := range b.items {
			if strings.Contains(strings.ToLower(item.Title), title) && strings.Contains(strings.ToLower(item.State), state) {
				items = append(items, item)
			}
		}
		return items
	}
	ids := func(items []ado.WorkItem) []int {
		out := make([]int, len(items))
		for i, item := range items {
			out[i] = item.ID
		}
		return out
	}

	// Filtering starts on the Title column and narrows the rows as you type.
	h.press("/")
	h.typeText("RATE")
	titleOnly := want("rate", "")
	if got := ids(shown()); len(got) == 0 || len(got) == all || !reflect.DeepEqual(got, ids(titleOnly)) {
		t.Fatalf("title filter shows %v, want %v (of %d)", got, ids(titleOnly), all)
	}

	// Title → Assigned To → State. Filters on different columns are ANDed.
	h.press("tab", "tab")
	h.typeText("new")
	both := want("rate", "new")
	if got := ids(shown()); len(got) == 0 || len(got) >= len(titleOnly) || !reflect.DeepEqual(got, ids(both)) {
		t.Fatalf("title and state filters show %v, want %v", got, ids(both))
	}

	h.press("enter")
	if b.filter.editing {
		t.Fatal("enter didn't finish editing the filters")
	}
	screen := h.model.View()
	if status := fmt.Sprintf("%d of %d work items", len(both), all); !strings.Contains(screen, status) {
		t.Errorf("screen doesn't say %q:\n%s", status, screen)
	}
	if !strings.Contains(screen, "RATE") || !strings.Contains(screen, "new") {
		t.Errorf("filter row isn't showing:\n%s", screen)
	}

	// Enter opens the row under the cursor, not the unfiltered item there.
	h.press("enter")
	h.waitFor("details", func() bool { _, ok := h.model.top().(*detailsView); return ok })
	if got := h.details().item.ID; got != both[0].ID {
		t.Errorf("opened #%d, want the first filtered row #%d", got, both[0].ID)
	}
	h.press("esc")
	h.waitFor("backlog", func() bool { _, ok := h.model.top().(*backlogView); return ok })

	// esc while editing puts the filters back the way they were.
	h.press("/")
	h.typeText("zzz")
	if len(shown()) != 0 || !strings.Contains(h.model.View(), "No work items match the column filters") {
		t.Fatalf("expected no matching rows:\n%s", h.model.View())
	}
	h.press("esc")
	if got := ids(shown()); b.filter.editing || !reflect.DeepEqual(got, ids(both)) {
		t.Fatalf("after esc the backlog shows %v, want %v", got, ids(both))
	}

	// Filters stay applied across a refresh.
	h.press("r")
	h.waitFor("refresh", func() bool { return !b.loading })
	if got := ids(shown()); !reflect.DeepEqual(got, ids(both)) {
		t.Errorf("after refresh the backlog shows %v, want %v", got, ids(both))
	}

	// x clears every filter.
	h.press("x")
	if len(shown()) != all || b.filter.active() {
		t.Errorf("after x the backlog shows %d of %d items", len(shown()), all)
	}
	if strings.Contains(h.model.View(), "RATE") {
		t.Errorf("filter row still showing after x:\n%s", h.model.View())
	}
}

func TestViewsFitSmallTerminals(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 60, Height: 20})
	h.waitFor("backlog", func() bool { return h.backlog().loaded })
	h.open(1001)
	for name, screen := range map[string]string{"details": h.model.View()} {
		for i, line := range strings.Split(screen, "\n") {
			if w := len([]rune(stripANSI(line))); w > 60 {
				t.Errorf("%s line %d is %d wide: %q", name, i, w, stripANSI(line))
			}
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return b.String()
}
