package ado

import "strings"

// Me is the WIQL macro for the authenticated user.
const Me = "@Me"

// completedStates are the "done" states of the built-in processes
// (Agile, Scrum, CMMI and Basic).
var completedStates = []string{"Closed", "Done", "Removed"}

// Query describes which backlog items to load.
type Query struct {
	// AssignedTo filters by assignee: a display name, an email, Me ("@Me"),
	// or empty for everyone.
	AssignedTo string
	// HideCompleted excludes items in a closed, done or removed state.
	HideCompleted bool
	// AreaPath and IterationPath restrict results to items under these paths.
	AreaPath      string
	IterationPath string
	// Top caps the number of results (0 means the server default).
	Top int
}

// AssignedToMe reports whether the query is for the authenticated user.
func (q Query) AssignedToMe() bool {
	return strings.EqualFold(strings.TrimSpace(q.AssignedTo), Me)
}

// WIQL renders the query as a WIQL statement.
func (q Query) WIQL() string {
	conds := []string{"[System.TeamProject] = @project"}

	switch assignee := strings.TrimSpace(q.AssignedTo); {
	case assignee == "":
	case q.AssignedToMe():
		conds = append(conds, "[System.AssignedTo] = @Me")
	default:
		conds = append(conds, "[System.AssignedTo] = "+wiqlString(assignee))
	}

	if q.HideCompleted {
		quoted := make([]string, len(completedStates))
		for i, s := range completedStates {
			quoted[i] = wiqlString(s)
		}
		conds = append(conds, "[System.State] NOT IN ("+strings.Join(quoted, ", ")+")")
	}
	if q.AreaPath != "" {
		conds = append(conds, "[System.AreaPath] UNDER "+wiqlString(q.AreaPath))
	}
	if q.IterationPath != "" {
		conds = append(conds, "[System.IterationPath] UNDER "+wiqlString(q.IterationPath))
	}

	return "SELECT [System.Id] FROM WorkItems WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY [Microsoft.VSTS.Common.Priority] ASC, [System.Id] ASC"
}

// wiqlString quotes s as a WIQL string literal.
func wiqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
