package views

import (
	"fmt"
	"time"
)

// now is replaceable in tests.
var now = time.Now

// formatDate formats a timestamp in local time.
func formatDate(t time.Time) string {
	return t.Local().Format("Jan 2, 2006 15:04")
}

// formatSince describes how long ago t was, falling back to the date for
// anything older than a week.
func formatSince(t time.Time) string {
	d := now().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("Jan 2, 2006")
}
