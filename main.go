// Command stratus is a terminal UI for working with Azure DevOps backlogs.
//
//	stratus --org contoso --project Fabrikam   # real Azure DevOps (AZURE_PAT)
//	stratus --mock                             # built-in in-memory mock server
//	stratus mock-server --addr 127.0.0.1:8080  # standalone mock server
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rf0ster/stratus/internal/ado"
	"github.com/rf0ster/stratus/internal/mockserver"
	"github.com/rf0ster/stratus/internal/views"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "mock-server" {
		err = runMockServer(os.Args[2:])
	} else {
		err = run(os.Args[1:])
	}
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "stratus:", err)
		os.Exit(1)
	}
}

const usage = `Usage:
  stratus [flags]                 open the backlog
  stratus mock-server [flags]     run a standalone mock Azure DevOps server

Connection settings default to these environment variables:
  AZURE_ORG       organization name or URL (--org)
  AZURE_PROJECT   project name (--project)
  AZURE_PAT       personal access token with Work Items read & write scope
                  (AZURE_DEVOPS_EXT_PAT also works)
  AZURE_USER      whose items to show (--user)

Examples:
  stratus --org contoso --project Fabrikam
  stratus --mock
  stratus --mock --mock-latency 400ms --user "Jordan Lee"

Flags:
`

func run(args []string) error {
	fs := flag.NewFlagSet("stratus", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}

	org := fs.String("org", os.Getenv("AZURE_ORG"), "organization name or URL")
	project := fs.String("project", os.Getenv("AZURE_PROJECT"), "project name")
	user := fs.String("user", envOr("AZURE_USER", ado.Me), `show items assigned to this person (name or email), "@Me" for yourself, or "" for everyone`)
	team := fs.String("team", "", `team whose members can be assigned items (default "<project> Team")`)
	area := fs.String("area", "", "only show items under this area path")
	iteration := fs.String("iteration", "", "only show items under this iteration path")
	closed := fs.Bool("closed", false, "include closed, done and removed items (toggle with c)")
	top := fs.Int("top", 500, "maximum number of work items to load")
	mock := fs.Bool("mock", false, "use a built-in mock Azure DevOps server with sample data; changes are kept in memory only")
	mockLatency := fs.Duration("mock-latency", 0, "delay every mock server response, e.g. 300ms")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (see --help)", fs.Arg(0))
	}

	pat := envOr("AZURE_PAT", os.Getenv("AZURE_DEVOPS_EXT_PAT"))
	source := ""
	if *mock {
		srv := mockserver.New(mockserver.Options{Latency: *mockLatency})
		orgURL, stop, err := srv.Start("127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("start mock server: %w", err)
		}
		defer stop() //nolint:errcheck // best effort on exit

		*org, *project, pat = orgURL, srv.Project(), ""
		// AZURE_USER names someone in your real organization, not the mock
		// team, so only honor --user when it's given explicitly.
		if !flagSet(fs, "user") {
			*user = ado.Me
		}
		source = fmt.Sprintf("mock server as %s · in memory, resets on exit", srv.Me())
	} else {
		if *org == "" || *project == "" {
			return errors.New("set --org and --project (or AZURE_ORG and AZURE_PROJECT), or try --mock")
		}
		if pat == "" && !isLocal(*org) {
			return errors.New("set AZURE_PAT to a personal access token with Work Items read & write scope")
		}
	}

	client, err := ado.NewClient(ado.Config{OrgURL: *org, Project: *project, PAT: pat})
	if err != nil {
		return err
	}
	if source == "" {
		source = strings.TrimPrefix(client.OrgURL(), "https://") + "/" + client.Project()
	}

	model := views.New(views.Config{
		Backend: client,
		Team:    *team,
		Source:  source,
		Query: ado.Query{
			AssignedTo:    *user,
			HideCompleted: !*closed,
			AreaPath:      *area,
			IterationPath: *iteration,
			Top:           *top,
		},
	})
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func runMockServer(args []string) error {
	fs := flag.NewFlagSet("stratus mock-server", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "Usage: stratus mock-server [flags]\n\nRuns an in-memory fake of the Azure DevOps work item API with sample data.\nChanges are lost when it stops.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	addr := fs.String("addr", "127.0.0.1:8080", "address to listen on")
	latency := fs.Duration("latency", 0, "delay every response, e.g. 300ms")
	quiet := fs.Bool("quiet", false, "don't log requests")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var logger *log.Logger
	if !*quiet {
		logger = log.New(os.Stderr, "", log.Ltime)
	}
	srv := mockserver.New(mockserver.Options{Latency: *latency, Logger: logger})
	orgURL, stop, err := srv.Start(*addr)
	if err != nil {
		return err
	}

	fmt.Printf(`Mock Azure DevOps server running (in memory; changes reset when it stops)
  organization: %s
  project:      %s
  signed in as: %s

Connect with:
  stratus --org %s --project %s

Press Ctrl+C to stop.
`, orgURL, srv.Project(), srv.Me(), orgURL, srv.Project())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	fmt.Println("\nStopping.")
	return stop()
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// isLocal reports whether org points at this machine (such as a standalone
// mock server), where no token is needed.
func isLocal(org string) bool {
	u, err := url.Parse(org)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
