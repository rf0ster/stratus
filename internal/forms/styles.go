package forms

import "github.com/charmbracelet/lipgloss"

// Styles controls how form fields render.
type Styles struct {
	Label        lipgloss.Style // label of an unfocused field
	FocusedLabel lipgloss.Style // label of the focused field
	Value        lipgloss.Style // value of an unfocused field
	FocusedValue lipgloss.Style // value of the focused field
	Dirty        lipgloss.Style // its foreground marks changed fields
	Muted        lipgloss.Style // placeholders and secondary text
	Selected     lipgloss.Style // the selected option while editing a radio
	Box          lipgloss.Style // frame around multi-line content
	FocusedBox   lipgloss.Style // frame around focused multi-line content
	Tab          lipgloss.Style // inactive tab
	ActiveTab    lipgloss.Style // active tab when the tabs aren't focused
	FocusedTab   lipgloss.Style // active tab when the tabs are focused
}

// DefaultStyles returns the default styles.
func DefaultStyles() Styles {
	const (
		accent = lipgloss.Color("86")  // cyan-green
		purple = lipgloss.Color("57")  // tab background
		white  = lipgloss.Color("15")  //
		gray   = lipgloss.Color("245") //
		dim    = lipgloss.Color("240") //
		orange = lipgloss.Color("214") // unsaved changes
	)
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(dim).
		PaddingLeft(1)

	return Styles{
		Label:        lipgloss.NewStyle().Foreground(gray),
		FocusedLabel: lipgloss.NewStyle().Foreground(accent).Bold(true),
		Value:        lipgloss.NewStyle(),
		FocusedValue: lipgloss.NewStyle().Foreground(white).Bold(true),
		Dirty:        lipgloss.NewStyle().Foreground(orange),
		Muted:        lipgloss.NewStyle().Foreground(dim).Italic(true),
		Selected:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		Box:          box,
		FocusedBox:   box.BorderForeground(accent),
		Tab:          lipgloss.NewStyle().Foreground(gray).Padding(0, 1),
		ActiveTab:    lipgloss.NewStyle().Foreground(white).Bold(true).Underline(true).Padding(0, 1),
		FocusedTab:   lipgloss.NewStyle().Foreground(white).Background(purple).Bold(true).Padding(0, 1),
	}
}
