# stratus

A terminal UI for working with Azure DevOps backlogs. It's the successor to
fazure, with the same layout and keyboard-driven forms.

## Quick start

Try it with the built-in mock server. It has sample data, needs no account,
and keeps changes in memory until you quit:

```sh
go run . --mock
```

Connect to Azure DevOps with a personal access token that has the
**Work Items (Read & write)** scope:

```sh
export AZURE_PAT=...        # or AZURE_DEVOPS_EXT_PAT
go run . --org contoso --project Fabrikam
```

`--org` takes an organization name (`contoso`) or URL
(`https://dev.azure.com/contoso`, `https://contoso.visualstudio.com`).

## Flags

| Flag | Default | |
|---|---|---|
| `--org` | `$AZURE_ORG` | Organization name or URL |
| `--project` | `$AZURE_PROJECT` | Project name |
| `--user` | `$AZURE_USER`, else `@Me` | Whose items to show: a name, an email, `@Me`, or `""` for everyone |
| `--team` | `<project> Team` | Team whose members appear in **Assigned To** |
| `--area` | | Only items under this area path |
| `--iteration` | | Only items under this iteration path |
| `--closed` | off | Include closed, done and removed items |
| `--top` | 500 | Maximum number of items to load |
| `--mock` | off | Use the built-in mock server |
| `--mock-latency` | 0 | Delay each mock response (e.g. `400ms`) to see loading states |

## Keys

**Backlog:** `↑/↓` move · `enter` open · `/` filter columns · `x` clear filters ·
`r` refresh · `c` show/hide completed · `a` (or `esc`) filter by assignee · `q` quit

**Column filters:** `/` opens a row of filters above the table, one per column.
Type to filter. A row stays if its text in that column contains what you typed
(case doesn't matter). If you fill in several columns, a row must match all of
them. `tab`/`shift+tab` moves between columns, `enter` keeps the filters, and
`esc` restores the filters you had before pressing `/`. Filtering only applies
to the items already loaded, and the filters stay on when you refresh.

**Work item:**

- `↑/↓` (`j/k`, `tab`) move between fields.
- On a choice field (Assigned To, State, Priority), `←/→` steps through the
  options. `enter` opens the full list, `enter` again picks, and `esc` cancels.
- On a text field, `enter` edits, `enter` finishes, and `esc` cancels.
- On the tabs, `←/→` switches between Description, Acceptance Criteria and
  Discussion. `pgup/pgdn` scrolls. `enter` edits the text or starts a comment.
- `ctrl+s` saves. Changes are held locally until then (changed fields are
  highlighted) and sent in one update, like the Save button in Azure DevOps.
  Inside a description, `ctrl+s` finishes the edit and saves. In the comment
  box, it posts the comment.
- `ctrl+z` discards changes, `ctrl+r` reloads the item, and `esc` goes back.
  If you have unsaved changes, it asks whether to save first.

Saves only succeed if the item hasn't changed since you opened it. If someone
else edited it, stratus says so instead of overwriting their change.

Descriptions and acceptance criteria are edited as plain text. If a field
has rich formatting (bold, links, images, tables), the status line warns that
saving will simplify it. Fields you don't change are never rewritten.

## Mock server

`--mock` runs the mock in-process. It also runs on its own, so you can point
stratus (or curl) at it:

```sh
go run . mock-server --addr 127.0.0.1:8080
go run . --org http://127.0.0.1:8080/mock --project Stratus
```

It implements the endpoints stratus uses, with the same URLs, JSON shapes and
error messages as Azure DevOps:

- WIQL queries. It supports `=`, `<>`, `<`, `>`, `IN`, `UNDER`, `CONTAINS`,
  `AND`/`OR`, `ORDER BY`, `@Me` and `@project`.
- Batch reads, limited to 200 IDs per request.
- JSON-patch updates with `/rev` tests for conflicts. States, assignees and
  priorities are validated.
- Reading and adding comments.
- Work item type states and team members.

The sample project has about 30 linked items: initiatives, requirements,
stories, tasks and bugs, assigned across a six-person team. You're signed in
as Alex Chen. Everything resets when the server stops.

## Layout

```
main.go                  flags, wiring, mock-server subcommand
internal/ado             Azure DevOps REST client, WIQL builder, HTML ⇄ text
internal/mockserver      in-memory fake of the REST API, mini WIQL engine, seed data
internal/forms           form widgets: radio, text input, text area, read-only, tabs
internal/views           the UI: backlog, work item details, assignee filter
```

## Development

```sh
go test ./...        # the client and UI tests run against the mock server
go vet ./...
```
