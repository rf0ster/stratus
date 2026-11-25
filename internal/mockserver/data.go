package mockserver

import (
	"fmt"
	"strings"
	"time"
)

// The mock team. The first member is the signed-in user (@Me).
var seedTeam = []identity{
	{DisplayName: "Alex Chen", UniqueName: "alex.chen@example.com"},
	{DisplayName: "Jordan Lee", UniqueName: "jordan.lee@example.com"},
	{DisplayName: "Priya Patel", UniqueName: "priya.patel@example.com"},
	{DisplayName: "Sam Rivera", UniqueName: "sam.rivera@example.com"},
	{DisplayName: "Morgan Blake", UniqueName: "morgan.blake@example.com"},
	{DisplayName: "Taylor Kim", UniqueName: "taylor.kim@example.com"},
}

var (
	stNew      = state{Name: "New", Color: "b2b2b2", Category: "Proposed"}
	stActive   = state{Name: "Active", Color: "007acc", Category: "InProgress"}
	stResolved = state{Name: "Resolved", Color: "ff9d00", Category: "Resolved"}
	stClosed   = state{Name: "Closed", Color: "339933", Category: "Completed"}
	stRemoved  = state{Name: "Removed", Color: "ffffff", Category: "Removed"}
)

var seedStates = map[string][]state{
	"initiative":  {stNew, stActive, stResolved, stClosed, stRemoved},
	"requirement": {stNew, stActive, stResolved, stClosed, stRemoved},
	"user story":  {stNew, stActive, stResolved, stClosed, stRemoved},
	"task":        {stNew, stActive, stClosed, stRemoved},
	"bug":         {stNew, stActive, stResolved, stClosed},
}

type seedComment struct {
	by    string        // first name of a team member
	after time.Duration // time after the item was created
	text  string        // HTML
}

type seedItem struct {
	id        int
	typ       string
	title     string
	state     string
	assignee  string // first name of a team member, or "" for unassigned
	priority  int
	parent    int
	area      string // relative to the project root
	iteration string // relative to the project root
	tags      string
	createdBy string
	age       int // days since creation
	desc      string
	ac        string
	comments  []seedComment
}

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

var seedItems = []seedItem{
	{
		id: 1001, typ: "Initiative", title: "Modernize the authentication platform", state: "Active",
		assignee: "Alex", priority: 1, area: `Platform\Identity`, iteration: `Sprint 24`,
		tags: "auth; security", createdBy: "Priya", age: 60,
		desc: `<div>Replace the legacy username/password service with a standards-based identity platform.</div>` +
			`<div><br></div><div><b>Goals</b></div><ul><li>Support OAuth2 / OpenID Connect providers</li>` +
			`<li>Enforce MFA for privileged accounts</li><li>Retire the legacy session service by end of Q1</li></ul>`,
		ac: `<ul><li>All sign-ins go through the new identity service</li><li>Legacy auth endpoints return 410 Gone</li></ul>`,
		comments: []seedComment{
			{by: "Priya", after: 2 * hour, text: `<div>Created this to track the auth modernization work. Let's aim to wrap up by the end of Q1.</div>`},
			{by: "Alex", after: 3 * day, text: `<div>Started on the OIDC integration. I'll need help with the UI pieces later.</div>`},
			{by: "Morgan", after: 4 * day, text: `<div>Happy to take the front-end work once the API is ready!</div>`},
		},
	},
	{
		id: 1002, typ: "Requirement", title: "Support OAuth2 / OpenID Connect sign-in providers", state: "Active",
		assignee: "Alex", priority: 1, parent: 1001, area: `Platform\Identity`, iteration: `Sprint 24`,
		tags: "auth; oidc", createdBy: "Priya", age: 55,
		desc: `<div>Users must be able to sign in with an external identity provider. Start with Google and GitHub; the design should make adding Microsoft Entra ID straightforward.</div>`,
		ac:   `<ul><li>Provider configuration is data-driven (no code change to add a provider)</li><li>Accounts are linked by verified email</li><li>Failed sign-ins are logged with the provider's error code</li></ul>`,
	},
	{
		id: 1003, typ: "User Story", title: "Sign in with Google", state: "Active",
		assignee: "Alex", priority: 1, parent: 1002, area: `Platform\Identity`, iteration: `Sprint 24`,
		tags: "auth; oidc", createdBy: "Jordan", age: 40,
		desc: `<div>As a user, I want to sign in with my Google account so that I don't have to remember another password.</div>`,
		ac:   `<ul><li>"Continue with Google" button on the sign-in page</li><li>New users get an account created on first sign-in</li><li>Existing users with the same verified email are linked, not duplicated</li></ul>`,
		comments: []seedComment{
			{by: "Jordan", after: 1 * day, text: `<div>Design mocks are in the Figma file under <i>Sign-in v2</i>.</div>`},
			{by: "Alex", after: 6 * day, text: `<div>Google sign-in works end to end in dev. Account linking is next.</div>`},
		},
	},
	{
		id: 1004, typ: "Task", title: "Register OAuth client and store secrets in Key Vault", state: "Closed",
		assignee: "Alex", priority: 2, parent: 1003, area: `Platform\Identity`, iteration: `Sprint 23`,
		tags: "infra", createdBy: "Alex", age: 38,
		desc: `<div>Create OAuth clients for dev, staging and prod and store the client secrets in Key Vault.</div>`,
	},
	{
		id: 1005, typ: "Task", title: "Implement OIDC callback handler", state: "Active",
		assignee: "Alex", priority: 1, parent: 1003, area: `Platform\Identity`, iteration: `Sprint 24`,
		tags: "backend", createdBy: "Alex", age: 30,
		desc: `<div>Handle the authorization code callback:</div><ol><li>Validate the <code>state</code> parameter</li>` +
			`<li>Exchange the code for tokens</li><li>Validate the ID token signature and claims</li><li>Create or link the account and start a session</li></ol>`,
	},
	{
		id: 1006, typ: "User Story", title: "Sign in with GitHub", state: "New",
		assignee: "Jordan", priority: 2, parent: 1002, area: `Platform\Identity`, iteration: `Sprint 25`,
		tags: "auth; oidc", createdBy: "Jordan", age: 40,
		desc: `<div>As a developer, I want to sign in with GitHub so that I can use the account I already have.</div>`,
		ac:   `<ul><li>"Continue with GitHub" button on the sign-in page</li><li>Private emails are handled by calling the GitHub emails API</li></ul>`,
	},
	{
		id: 1007, typ: "Bug", title: "Login redirect drops the originally requested URL", state: "Active",
		assignee: "Alex", priority: 1, parent: 1002, area: `Platform\Identity`, iteration: `Sprint 24`,
		tags: "auth; regression", createdBy: "Taylor", age: 12,
		desc: `<div>After a successful sign-in, users always land on <code>/home</code> instead of the page they asked for.</div>` +
			`<div><br></div><div><b>Repro steps</b></div><ol><li>Sign out</li><li>Open <code>/reports/weekly</code></li>` +
			`<li>Complete sign-in</li><li>Observe you are on <code>/home</code></li></ol>`,
		ac: `<ul><li>Users return to the originally requested URL after sign-in</li><li>Only same-origin redirects are allowed</li></ul>`,
		comments: []seedComment{
			{by: "Taylor", after: 1 * hour, text: `<div>Found this while testing the staging build. Happens in every browser.</div>`},
			{by: "Alex", after: 5 * hour, text: `<div>The return URL isn't carried through the OAuth <code>state</code>. I'll store it there and validate it on the way back.</div>`},
		},
	},
	{
		id: 1008, typ: "User Story", title: "Enforce MFA for admin accounts", state: "New",
		assignee: "Priya", priority: 1, parent: 1001, area: `Platform\Identity`, iteration: `Sprint 25`,
		tags: "security", createdBy: "Priya", age: 35,
		desc: `<div>As a security officer, I want every admin account to require a second factor so that a stolen password isn't enough to take over the tenant.</div>`,
		ac:   `<ul><li>Admins without MFA are prompted to enroll at next sign-in</li><li>TOTP and WebAuthn are supported</li></ul>`,
	},
	{
		id: 1009, typ: "Bug", title: "Session cookie is missing the SameSite attribute", state: "Resolved",
		assignee: "Sam", priority: 2, parent: 1001, area: `Platform\Identity`, iteration: `Sprint 23`,
		tags: "security", createdBy: "Sam", age: 20,
		desc: `<div>The session cookie is set without <code>SameSite</code>, so browsers fall back to their default. Set <code>SameSite=Lax</code> explicitly.</div>`,
		comments: []seedComment{
			{by: "Sam", after: 2 * day, text: `<div>Fixed in the cookie middleware. Waiting for verification in staging.</div>`},
		},
	},
	{
		id: 1010, typ: "Initiative", title: "API rate limiting and quotas", state: "Active",
		assignee: "Priya", priority: 1, area: `Platform\API`, iteration: `Sprint 24`,
		tags: "api", createdBy: "Priya", age: 50,
		desc: `<div>Protect the public API from abuse and make usage fair across customers by adding per-tier rate limits and monthly quotas.</div>`,
	},
	{
		id: 1011, typ: "Requirement", title: "Define per-tier rate limits", state: "Resolved",
		assignee: "Priya", priority: 1, parent: 1010, area: `Platform\API`, iteration: `Sprint 23`,
		tags: "api; requirements", createdBy: "Priya", age: 48,
		desc: `<div>Agree on request limits for each pricing tier with Product and Support.</div>` +
			`<ul><li>Free: 60 requests/minute</li><li>Team: 600 requests/minute</li><li>Enterprise: 6,000 requests/minute (configurable)</li></ul>`,
		comments: []seedComment{
			{by: "Jordan", after: 3 * day, text: `<div>Support asked that enterprise limits be adjustable per customer without a deploy.</div>`},
		},
	},
	{
		id: 1012, typ: "User Story", title: "Rate limiting middleware backed by Redis", state: "Active",
		assignee: "Alex", priority: 1, parent: 1010, area: `Platform\API`, iteration: `Sprint 24`,
		tags: "api; backend", createdBy: "Priya", age: 25,
		desc: `<div>As an API owner, I want requests limited per API key so that one noisy client can't degrade service for everyone.</div>`,
		ac:   `<ul><li>Sliding-window limiter shared across all API instances</li><li>Requests over the limit get HTTP 429 with a <code>Retry-After</code> header</li><li>Limiter adds less than 2ms p99 latency</li></ul>`,
		comments: []seedComment{
			{by: "Alex", after: 2 * day, text: `<div>Going with a Redis sorted-set sliding window. Prototype is on the <code>rate-limit</code> branch.</div>`},
			{by: "Sam", after: 3 * day, text: `<div>Don't forget the <code>X-RateLimit-*</code> headers, clients have been asking for them.</div>`},
		},
	},
	{
		id: 1013, typ: "Task", title: "Add X-RateLimit-* response headers", state: "New",
		assignee: "Alex", priority: 2, parent: 1012, area: `Platform\API`, iteration: `Sprint 24`,
		tags: "api", createdBy: "Alex", age: 22,
		desc: `<div>Return <code>X-RateLimit-Limit</code>, <code>X-RateLimit-Remaining</code> and <code>X-RateLimit-Reset</code> on every response.</div>`,
	},
	{
		id: 1014, typ: "Task", title: "Load test the limiter at 5k requests per second", state: "New",
		assignee: "Sam", priority: 3, parent: 1012, area: `Platform\API`, iteration: `Sprint 25`,
		tags: "performance", createdBy: "Alex", age: 22,
		desc: `<div>Use k6 against staging. Record p50/p99 latency with and without the limiter enabled.</div>`,
	},
	{
		id: 1015, typ: "Bug", title: "429 responses return HTML instead of JSON", state: "New",
		assignee: "Alex", priority: 2, parent: 1010, area: `Platform\API`, iteration: `Sprint 24`,
		tags: "api", createdBy: "Jordan", age: 5,
		desc: `<div>When the gateway throttles a request it returns the default HTML error page. API clients expect a JSON error body.</div>`,
	},
	{
		id: 1016, typ: "Initiative", title: "Mobile app redesign", state: "New",
		assignee: "Morgan", priority: 2, area: `Mobile`, iteration: `Sprint 26`,
		tags: "mobile; ux", createdBy: "Priya", age: 45,
		desc: `<div>Redesign the mobile app around the new design system, with a focus on onboarding and performance on older devices.</div>`,
		comments: []seedComment{
			{by: "Morgan", after: 1 * day, text: `<div>Working on the mockups now, I'll share them at the next design review.</div>`},
		},
	},
	{
		id: 1017, typ: "User Story", title: "New onboarding flow", state: "Active",
		assignee: "Morgan", priority: 2, parent: 1016, area: `Mobile`, iteration: `Sprint 24`,
		tags: "mobile; ux", createdBy: "Morgan", age: 30,
		desc: `<div>As a new user, I want a short guided setup so that I can get to my first project in under a minute.</div>`,
		ac:   `<ul><li>No more than three onboarding screens</li><li>Every step can be skipped</li><li>Completion rate is tracked in analytics</li></ul>`,
	},
	{
		id: 1018, typ: "User Story", title: "Dark mode support", state: "New",
		assignee: "Taylor", priority: 3, parent: 1016, area: `Mobile`, iteration: `Sprint 26`,
		tags: "mobile; ux", createdBy: "Taylor", age: 28,
		desc: `<div>As a user, I want the app to follow my system dark mode setting.</div>`,
	},
	{
		id: 1019, typ: "Bug", title: "Android app crashes when rotating the settings screen", state: "Active",
		assignee: "Taylor", priority: 1, parent: 1016, area: `Mobile`, iteration: `Sprint 24`,
		tags: "mobile; android; crash", createdBy: "Sam", age: 8,
		desc: `<div>Rotating the device while the settings screen is open crashes the app on Android 13 and 14.</div><div><br></div><div><code>IllegalStateException: Fragment not attached to a context</code></div>`,
		comments: []seedComment{
			{by: "Taylor", after: 4 * hour, text: `<div>Reproduced on a Pixel 7. The settings fragment keeps a reference to the old activity.</div>`},
		},
	},
	{
		id: 1020, typ: "Task", title: "Audit color contrast for accessibility (WCAG AA)", state: "New",
		priority: 3, parent: 1016, area: `Mobile`, iteration: `Sprint 26`,
		tags: "accessibility", createdBy: "Morgan", age: 26,
		desc: `<div>Check every screen in the new design against WCAG AA contrast ratios and file bugs for failures.</div>`,
	},
	{
		id: 1021, typ: "Requirement", title: "Data export in CSV, JSON and Excel formats", state: "New",
		assignee: "Jordan", priority: 2, area: `Product\Reporting`, iteration: `Sprint 25`,
		tags: "export; reporting", createdBy: "Priya", age: 33,
		desc: `<div>Customers want to analyze their data in external tools. Provide exports in the most common formats.</div>`,
		comments: []seedComment{
			{by: "Priya", after: 1 * day, text: `<div>This is one of the most requested features. Let's start with CSV.</div>`},
		},
	},
	{
		id: 1022, typ: "User Story", title: "Export the backlog to CSV", state: "New",
		assignee: "Alex", priority: 2, parent: 1021, area: `Product\Reporting`, iteration: `Sprint 25`,
		tags: "export", createdBy: "Jordan", age: 18,
		desc: `<div>As a project manager, I want to export the current backlog view to CSV so that I can build reports in Excel.</div>`,
		ac:   `<ul><li>Export respects the current filters</li><li>Columns match what is shown on screen</li><li>Files with more than 10,000 rows are generated in the background</li></ul>`,
	},
	{
		id: 1023, typ: "User Story", title: "Scheduled exports by email", state: "New",
		priority: 4, parent: 1021, area: `Product\Reporting`, iteration: `Backlog`,
		tags: "export", createdBy: "Jordan", age: 18,
		desc: `<div>As a manager, I want a weekly export emailed to me so that I don't have to remember to pull it.</div>`,
	},
	{
		id: 1024, typ: "Task", title: "Upgrade the Go toolchain to 1.25 across services", state: "Active",
		assignee: "Alex", priority: 3, area: `Platform\Infrastructure`, iteration: `Sprint 24`,
		tags: "maintenance", createdBy: "Sam", age: 14,
		desc: `<div>Bump <code>go.mod</code>, the CI images and the Dockerfiles. Fix any new vet findings.</div>`,
	},
	{
		id: 1025, typ: "Bug", title: "Search results paginate incorrectly past page 10", state: "Active",
		assignee: "Jordan", priority: 2, area: `Product\Search`, iteration: `Sprint 24`,
		tags: "search", createdBy: "Taylor", age: 9,
		desc: `<div>Page 11 onward repeats results from page 10. Looks like the offset is capped at 100 somewhere.</div>`,
	},
	{
		id: 1026, typ: "Task", title: "Write a runbook for the on-call rotation", state: "Closed",
		assignee: "Sam", priority: 3, area: `Platform\Infrastructure`, iteration: `Sprint 23`,
		tags: "docs; ops", createdBy: "Sam", age: 32,
		desc: `<div>Document paging policy, escalation paths and the top ten alerts with their fixes.</div>`,
	},
	{
		id: 1027, typ: "Bug", title: "Typo in the password reset email", state: "Closed",
		assignee: "Alex", priority: 4, area: `Platform\Identity`, iteration: `Sprint 23`,
		tags: "copy", createdBy: "Morgan", age: 27,
		desc: `<div>"Your pasword has been reset" should be "Your password has been reset".</div>`,
	},
	{
		id: 1028, typ: "User Story", title: "Audit log viewer for admins", state: "New",
		assignee: "Priya", priority: 2, area: `Product\Admin`, iteration: `Sprint 26`,
		tags: "security; admin", createdBy: "Priya", age: 16,
		desc: `<div>As an admin, I want to search the audit log by user, action and date so that I can investigate incidents without asking engineering.</div>`,
	},
	{
		id: 1029, typ: "Task", title: "Remove the deprecated v1 endpoints", state: "Removed",
		assignee: "Alex", priority: 3, parent: 1010, area: `Platform\API`, iteration: `Sprint 23`,
		tags: "api; cleanup", createdBy: "Alex", age: 44,
		desc: `<div>Superseded by the v1 sunset plan; tracked separately.</div>`,
	},
	{
		id: 1030, typ: "Bug", title: "Notification emails show times in UTC", state: "New",
		assignee: "Alex", priority: 3, area: `Product\Notifications`, iteration: `Sprint 25`,
		tags: "email", createdBy: "Jordan", age: 3,
		desc: `<div>Times in notification emails are always UTC. They should use the recipient's time zone from their profile.</div>`,
	},
}

// seed fills the server with the sample backlog. Dates are relative to now so
// the data always looks recent.
func (s *Server) seed(now time.Time) {
	s.team = make([]identity, len(seedTeam))
	for i, m := range seedTeam {
		m.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		s.team[i] = m
	}
	s.me = s.team[0]
	s.states = seedStates

	member := func(firstName string) identity {
		for _, m := range s.team {
			if strings.HasPrefix(m.DisplayName, firstName+" ") {
				return m
			}
		}
		panic("mockserver: unknown seed member " + firstName)
	}

	s.items = make(map[int]*item, len(seedItems))
	for _, si := range seedItems {
		created := now.Add(-time.Duration(si.age) * day)
		f := map[string]any{
			fID:            float64(si.id),
			fType:          si.typ,
			fTitle:         si.title,
			fState:         si.state,
			fPriority:      float64(si.priority),
			fAreaPath:      s.project + `\` + si.area,
			fIterationPath: s.project + `\` + si.iteration,
			fCreatedBy:     member(si.createdBy),
			fCreatedDate:   formatTime(created),
			fChangedBy:     member(si.createdBy),
			fChangedDate:   formatTime(created),
			fTeamProject:   s.project,
		}
		if si.assignee != "" {
			f[fAssignedTo] = member(si.assignee)
		}
		if si.parent != 0 {
			f[fParent] = float64(si.parent)
		}
		if si.tags != "" {
			f[fTags] = normalizeTags(si.tags)
		}
		if si.desc != "" {
			f[fDescription] = si.desc
		}
		if si.ac != "" {
			f[fAcceptanceCriteria] = si.ac
		}

		it := &item{id: si.id, rev: 1, fields: f}
		for _, sc := range si.comments {
			at := created.Add(sc.after)
			if at.After(now) {
				at = now.Add(-time.Minute)
			}
			s.nextCommentID++
			by := member(sc.by)
			it.comments = append(it.comments, &comment{
				WorkItemID: si.id, ID: s.nextCommentID, Version: 1, Text: sc.text,
				CreatedBy: by, CreatedDate: at.UTC(), ModifiedBy: by, ModifiedDate: at.UTC(),
			})
			// Each comment bumps the revision, like the real service.
			it.rev++
		}
		s.items[si.id] = it
	}
}
