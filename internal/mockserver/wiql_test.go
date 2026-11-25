package mockserver

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWIQL(t *testing.T) {
	s := New(Options{})
	ctx := &evalContext{me: s.me, project: s.project}
	run := func(t *testing.T, query string) []int {
		t.Helper()
		q, err := parseWIQL(query, canonicalField)
		if err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		return q.run(ctx, s.sortedItems())
	}

	t.Run("everything", func(t *testing.T) {
		if got := run(t, "SELECT [System.Id] FROM WorkItems"); len(got) != len(seedItems) {
			t.Errorf("got %d items, want %d", len(got), len(seedItems))
		}
	})

	t.Run("me and project macros", func(t *testing.T) {
		got := run(t, "select [system.id] from workitems where [System.TeamProject] = @project and [System.AssignedTo] = @me")
		for _, id := range got {
			if a := s.items[id].fields[fAssignedTo].(identity); a != s.me {
				t.Errorf("item %d assigned to %v", id, a)
			}
		}
		if len(got) == 0 {
			t.Error("no items for @Me")
		}
	})

	t.Run("identity by name or email", func(t *testing.T) {
		byName := run(t, "SELECT [System.Id] FROM WorkItems WHERE [System.AssignedTo] = 'jordan lee'")
		byEmail := run(t, "SELECT [System.Id] FROM WorkItems WHERE [System.AssignedTo] = 'Jordan.Lee@example.com'")
		if len(byName) == 0 || !slices.Equal(byName, byEmail) {
			t.Errorf("by name %v, by email %v", byName, byEmail)
		}
	})

	t.Run("unassigned", func(t *testing.T) {
		got := run(t, "SELECT [System.Id] FROM WorkItems WHERE [System.AssignedTo] = ''")
		if !slices.Equal(got, []int{1020, 1023}) {
			t.Errorf("unassigned = %v", got)
		}
	})

	t.Run("not in, under, order by", func(t *testing.T) {
		got := run(t, `SELECT [System.Id] FROM WorkItems
			WHERE [System.State] NOT IN ('Closed', 'Removed') AND [System.AreaPath] UNDER 'stratus\platform'
			ORDER BY [Microsoft.VSTS.Common.Priority] DESC, [System.Id]`)
		lastPriority := 5.0
		for _, id := range got {
			f := s.items[id].fields
			if st := f[fState]; st == "Closed" || st == "Removed" {
				t.Errorf("item %d is %v", id, st)
			}
			if !strings.HasPrefix(f[fAreaPath].(string), `Stratus\Platform\`) {
				t.Errorf("item %d area %v", id, f[fAreaPath])
			}
			if p := f[fPriority].(float64); p > lastPriority {
				t.Errorf("not sorted by priority desc at %d", id)
			} else {
				lastPriority = p
			}
		}
	})

	t.Run("or and parentheses", func(t *testing.T) {
		got := run(t, "SELECT [System.Id] FROM WorkItems WHERE ([System.Id] = 1001 OR [System.Id] = 1002) AND [System.Title] CONTAINS 'PLATFORM'")
		if !slices.Equal(got, []int{1001}) {
			t.Errorf("got %v, want [1001]", got)
		}
	})

	t.Run("comparison", func(t *testing.T) {
		got := run(t, "SELECT [System.Id] FROM WorkItems WHERE [System.Id] >= 1029")
		if !slices.Equal(got, []int{1029, 1030}) {
			t.Errorf("got %v", got)
		}
	})
}

func TestWIQLErrors(t *testing.T) {
	for _, query := range []string{
		"",
		"SELECT [System.Id]",
		"SELECT [System.Id] FROM WorkItemLinks",
		"SELECT [System.Id] FROM WorkItems WHERE [Nope.Field] = 1",
		"SELECT [System.Id] FROM WorkItems WHERE [System.Title] = 'unterminated",
		"SELECT [System.Id] FROM WorkItems WHERE [System.Id] = @today",
		"SELECT [System.Id] FROM WorkItems WHERE [System.Id] IN (1, 2",
		"SELECT [System.Id] FROM WorkItems WHERE [System.Id] == 1",
	} {
		if _, err := parseWIQL(query, canonicalField); err == nil {
			t.Errorf("parseWIQL(%q) succeeded, want an error", query)
		}
	}
}

func TestSeedDataIsConsistent(t *testing.T) {
	s := New(Options{})
	for id, it := range s.items {
		typ := it.fields[fType].(string)
		if _, ok := s.findState(typ, it.fields[fState].(string)); !ok {
			t.Errorf("item %d has state %v, which %s doesn't allow", id, it.fields[fState], typ)
		}
		if p, ok := it.fields[fParent].(float64); ok {
			if _, exists := s.items[int(p)]; !exists {
				t.Errorf("item %d has missing parent %v", id, p)
			}
		}
		for _, c := range it.comments {
			if c.CreatedDate.After(time.Now()) {
				t.Errorf("item %d has a comment in the future", id)
			}
		}
	}
}

func TestStartWithWildcardAddress(t *testing.T) {
	url, stop, err := New(Options{}).Start(":0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop() //nolint:errcheck
	if !strings.HasPrefix(url, "http://127.0.0.1:") || !strings.HasSuffix(url, "/"+DefaultOrg) {
		t.Errorf("Start(\":0\") = %q, want a loopback URL", url)
	}
}
