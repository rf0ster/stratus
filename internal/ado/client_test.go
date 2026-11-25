package ado_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rf0ster/stratus/internal/ado"
	"github.com/rf0ster/stratus/internal/mockserver"
)

// newMockClient returns a client talking to a fresh mock server.
func newMockClient(t *testing.T) *ado.Client {
	t.Helper()
	srv := httptest.NewServer(mockserver.New(mockserver.Options{}))
	t.Cleanup(srv.Close)
	c, err := ado.NewClient(ado.Config{OrgURL: srv.URL + "/mock", Project: mockserver.DefaultProject})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewClientNormalizesOrg(t *testing.T) {
	tests := map[string]string{
		"contoso":                           "https://dev.azure.com/contoso",
		"https://dev.azure.com/contoso/":    "https://dev.azure.com/contoso",
		"http://127.0.0.1:8080/mock":        "http://127.0.0.1:8080/mock",
		"https://contoso.visualstudio.com/": "https://contoso.visualstudio.com",
	}
	for in, want := range tests {
		c, err := ado.NewClient(ado.Config{OrgURL: in, Project: "P"})
		if err != nil {
			t.Fatalf("NewClient(%q): %v", in, err)
		}
		if c.OrgURL() != want {
			t.Errorf("NewClient(%q).OrgURL() = %q, want %q", in, c.OrgURL(), want)
		}
	}
	if _, err := ado.NewClient(ado.Config{OrgURL: "contoso"}); err == nil {
		t.Error("NewClient without a project succeeded")
	}
}

func TestQueryWorkItems(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()

	mine, err := c.QueryWorkItems(ctx, ado.Query{AssignedTo: ado.Me, HideCompleted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) == 0 {
		t.Fatal("no items assigned to @Me")
	}
	lastPriority := 0
	for _, wi := range mine {
		if wi.AssignedTo.DisplayName != "Alex Chen" {
			t.Errorf("item %d is assigned to %q, want Alex Chen", wi.ID, wi.AssignedTo.DisplayName)
		}
		if wi.State == "Closed" || wi.State == "Removed" {
			t.Errorf("item %d is %s but completed items are hidden", wi.ID, wi.State)
		}
		if wi.Priority < lastPriority {
			t.Errorf("items aren't sorted by priority: %d after %d", wi.Priority, lastPriority)
		}
		lastPriority = wi.Priority
	}

	all, err := c.QueryWorkItems(ctx, ado.Query{AssignedTo: ado.Me})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) <= len(mine) {
		t.Errorf("including completed items returned %d items, want more than %d", len(all), len(mine))
	}

	byEmail, err := c.QueryWorkItems(ctx, ado.Query{AssignedTo: "jordan.lee@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byEmail) == 0 || byEmail[0].AssignedTo.UniqueName != "jordan.lee@example.com" {
		t.Errorf("query by email returned %+v", byEmail)
	}

	under, err := c.QueryWorkItems(ctx, ado.Query{AreaPath: `Stratus\Mobile`, Top: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(under) != 2 {
		t.Errorf("Top: 2 returned %d items", len(under))
	}
	for _, wi := range under {
		if !strings.HasPrefix(wi.AreaPath, `Stratus\Mobile`) {
			t.Errorf("item %d has area %q, want under Stratus\\Mobile", wi.ID, wi.AreaPath)
		}
	}
}

func TestQueryEscapesQuotes(t *testing.T) {
	c := newMockClient(t)
	// Without escaping, the quote would end the string literal and the
	// server would reject the query.
	items, err := c.QueryWorkItems(context.Background(), ado.Query{AssignedTo: "O'Brien"})
	if err != nil {
		t.Fatalf("query with a quote failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %d items for an unknown user", len(items))
	}
}

func TestWorkItemFields(t *testing.T) {
	c := newMockClient(t)
	wi, err := c.GetWorkItem(context.Background(), 1003)
	if err != nil {
		t.Fatal(err)
	}
	if wi.Type != ado.UserStory || wi.Title != "Sign in with Google" || wi.ParentID != 1002 {
		t.Errorf("unexpected item: %+v", wi)
	}
	if wi.AssignedTo.UniqueName != "alex.chen@example.com" || wi.CreatedBy.DisplayName != "Jordan Lee" {
		t.Errorf("identities not parsed: assigned %+v, created by %+v", wi.AssignedTo, wi.CreatedBy)
	}
	if wi.CreatedDate.IsZero() || wi.Rev < 1 || len(wi.Tags) != 2 {
		t.Errorf("created %v, rev %d, tags %v", wi.CreatedDate, wi.Rev, wi.Tags)
	}
	if !strings.Contains(ado.HTMLToText(wi.Description), "sign in with my Google account") {
		t.Errorf("description = %q", wi.Description)
	}
}

func TestUpdateWorkItem(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()
	before, err := c.GetWorkItem(ctx, 1013)
	if err != nil {
		t.Fatal(err)
	}

	after, err := c.UpdateWorkItem(ctx, before.ID, before.Rev, []ado.Change{
		{Field: ado.FieldTitle, Value: "Add rate limit headers"},
		{Field: ado.FieldState, Value: "Active"},
		{Field: ado.FieldPriority, Value: 1},
		{Field: ado.FieldAssignedTo, Value: "priya.patel@example.com"},
		{Field: ado.FieldTags, Value: "api; headers"},
		{Field: ado.FieldDescription, Value: ado.TextToHTML("line one\n\nline <two>")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Rev != before.Rev+1 {
		t.Errorf("rev = %d, want %d", after.Rev, before.Rev+1)
	}
	if after.Title != "Add rate limit headers" || after.State != "Active" || after.Priority != 1 ||
		after.AssignedTo.DisplayName != "Priya Patel" || ado.JoinTags(after.Tags) != "api; headers" {
		t.Errorf("update not applied: %+v", after)
	}
	if got := ado.HTMLToText(after.Description); got != "line one\n\nline <two>" {
		t.Errorf("description round trip = %q", got)
	}

	// The change is visible to later reads.
	reread, err := c.GetWorkItem(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Title != after.Title || reread.Rev != after.Rev {
		t.Errorf("reread = %+v, want title %q rev %d", reread, after.Title, after.Rev)
	}

	// Unassigning clears the field.
	cleared, err := c.UpdateWorkItem(ctx, before.ID, after.Rev, []ado.Change{{Field: ado.FieldAssignedTo, Value: ""}})
	if err != nil {
		t.Fatal(err)
	}
	if !cleared.AssignedTo.IsZero() {
		t.Errorf("assignee = %+v, want none", cleared.AssignedTo)
	}
}

func TestUpdateConflict(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()
	wi, err := c.GetWorkItem(ctx, 1013)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateWorkItem(ctx, wi.ID, wi.Rev, []ado.Change{{Field: ado.FieldPriority, Value: 3}}); err != nil {
		t.Fatal(err)
	}
	// Saving again with the old revision must fail rather than overwrite.
	_, err = c.UpdateWorkItem(ctx, wi.ID, wi.Rev, []ado.Change{{Field: ado.FieldPriority, Value: 4}})
	if !ado.IsConflict(err) {
		t.Fatalf("stale update error = %v, want a conflict", err)
	}
}

func TestUpdateValidation(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		change ado.Change
		want   string
	}{
		{"invalid state", ado.Change{Field: ado.FieldState, Value: "Resolved"}, "not a valid state"},
		{"unknown person", ado.Change{Field: ado.FieldAssignedTo, Value: "nobody@example.com"}, "unknown identity"},
		{"priority out of range", ado.Change{Field: ado.FieldPriority, Value: 9}, "Priority"},
		{"empty title", ado.Change{Field: ado.FieldTitle, Value: ""}, "Required"},
		{"read-only field", ado.Change{Field: ado.FieldCreatedBy, Value: "x"}, "ReadOnly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1013 is a Task, which has no Resolved state.
			_, err := c.UpdateWorkItem(ctx, 1013, 0, []ado.Change{tt.change})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestComments(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()
	before, err := c.GetComments(ctx, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 3 {
		t.Fatalf("got %d comments, want 3", len(before))
	}
	for i := 1; i < len(before); i++ {
		if before[i].CreatedDate.After(before[i-1].CreatedDate) {
			t.Errorf("comments aren't newest first")
		}
	}

	added, err := c.AddComment(ctx, 1001, "Looks good <3")
	if err != nil {
		t.Fatal(err)
	}
	if added.CreatedBy.DisplayName != "Alex Chen" || ado.HTMLToText(added.Text) != "Looks good <3" {
		t.Errorf("added comment = %+v", added)
	}
	after, err := c.GetComments(ctx, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 4 || after[0].ID != added.ID {
		t.Errorf("after adding: %d comments, newest %d, want 4 and %d", len(after), after[0].ID, added.ID)
	}
}

func TestStatesAndTeam(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()
	states, err := c.GetStates(ctx, ado.UserStory)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 || states[0].Name != "New" || states[0].Category != "Proposed" {
		t.Errorf("states = %+v", states)
	}
	members, err := c.GetTeamMembers(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 6 || members[0].DisplayName != "Alex Chen" {
		t.Errorf("members = %+v", members)
	}
	if _, err := c.GetTeamMembers(ctx, "No Such Team"); err == nil {
		t.Error("unknown team succeeded")
	}
}

func TestGetWorkItemsBatches(t *testing.T) {
	var requests, maxIDs int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		if n := int32(len(ids)); n > atomic.LoadInt32(&maxIDs) {
			atomic.StoreInt32(&maxIDs, n)
		}
		var value []map[string]any
		for _, id := range ids {
			n, _ := strconv.Atoi(id)
			value = append(value, map[string]any{"id": n, "rev": 1, "fields": map[string]any{"System.Title": id}})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": value}) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := ado.NewClient(ado.Config{OrgURL: srv.URL + "/org", Project: "P"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int, 450)
	for i := range ids {
		ids[i] = i + 1
	}
	items, err := c.GetWorkItems(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 450 || items[449].ID != 450 {
		t.Errorf("got %d items", len(items))
	}
	if requests != 3 || maxIDs > 200 {
		t.Errorf("made %d requests with up to %d ids, want 3 requests of at most 200", requests, maxIDs)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        string
	}{
		{"api message", 400, "application/json", `{"message":"TF51005: bad query"}`, "TF51005: bad query"},
		{"unauthorized", 401, "text/html", "<html>sign in</html>", "personal access token"},
		{"sign-in page", 203, "text/html", "<html>sign in</html>", "non-JSON response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body)) //nolint:errcheck
			}))
			defer srv.Close()
			c, err := ado.NewClient(ado.Config{OrgURL: srv.URL + "/org", Project: "P", PAT: "x"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.GetWorkItem(context.Background(), 1)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
