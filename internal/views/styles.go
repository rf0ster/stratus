package views

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/rf0ster/stratus/internal/ado"
)

// Work item type colors (8-bit ANSI), close to the Azure DevOps palette.
var typeColors = map[ado.WorkItemType]lipgloss.Color{
	ado.Epic:        "208", // orange
	ado.Initiative:  "208", // orange
	ado.Feature:     "97",  // purple
	ado.Requirement: "97",  // purple
	ado.UserStory:   "38",  // blue
	ado.Task:        "220", // yellow
	ado.Bug:         "161", // red
}

const (
	colorPrimary = lipgloss.Color("#7D56F4")
	colorMuted   = lipgloss.Color("#626262")
	colorError   = lipgloss.Color("196")
	colorWarn    = lipgloss.Color("214")
	colorSuccess = lipgloss.Color("42")
	colorAccent  = lipgloss.Color("86")
)

var (
	appStyle      = lipgloss.NewStyle().Padding(0, 1)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	subtitleStyle = lipgloss.NewStyle().Foreground(colorMuted)
	mutedStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	errorStyle    = lipgloss.NewStyle().Foreground(colorError)
	warnStyle     = lipgloss.NewStyle().Foreground(colorWarn)
	successStyle  = lipgloss.NewStyle().Foreground(colorSuccess)
	accentStyle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// typeStyle returns the bold, colored style for a work item type.
func typeStyle(t ado.WorkItemType) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if c, ok := typeColors[t]; ok {
		style = style.Foreground(c)
	}
	return style
}
