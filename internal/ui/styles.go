package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

// Colors use the terminal's 16-color palette so that they follow the user's
// theme.
var (
	colorAccent = lipgloss.Color("6") // cyan
	colorOK     = lipgloss.Color("2") // green
	colorChange = lipgloss.Color("3") // yellow
	colorFail   = lipgloss.Color("1") // red
	colorMuted  = lipgloss.Color("8") // gray
	colorTitle  = lipgloss.Color("5") // magenta
)

var (
	styleHeader      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleHeaderLabel = lipgloss.NewStyle().Foreground(colorMuted)
	styleBorder      = lipgloss.NewStyle().Foreground(colorAccent)
	styleTitle       = lipgloss.NewStyle().Foreground(colorTitle).Bold(true)
	styleTitleCount  = lipgloss.NewStyle().Foreground(colorAccent)
	styleColHeader   = lipgloss.NewStyle().Bold(true)
	styleSelected    = lipgloss.NewStyle().Reverse(true)
	styleMuted       = lipgloss.NewStyle().Foreground(colorMuted)
	styleSection     = lipgloss.NewStyle().Foreground(colorTitle).Bold(true)
	styleKey         = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleInfo        = lipgloss.NewStyle().Foreground(colorOK)
	styleError       = lipgloss.NewStyle().Foreground(colorFail)
	stylePrompt      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleConfirm     = lipgloss.NewStyle().Foreground(colorChange).Bold(true)

	styleJSONKey    = lipgloss.NewStyle().Foreground(colorAccent)
	styleJSONString = lipgloss.NewStyle().Foreground(colorOK)
	styleJSONNumber = lipgloss.NewStyle().Foreground(colorChange)
	styleJSONLit    = lipgloss.NewStyle().Foreground(colorTitle)
)

var statusStyles = map[ansible.Status]lipgloss.Style{
	ansible.StatusRunning:     lipgloss.NewStyle().Foreground(colorAccent),
	ansible.StatusOK:          lipgloss.NewStyle().Foreground(colorOK),
	ansible.StatusChanged:     lipgloss.NewStyle().Foreground(colorChange),
	ansible.StatusSkipped:     lipgloss.NewStyle().Foreground(colorMuted),
	ansible.StatusFailed:      lipgloss.NewStyle().Foreground(colorFail).Bold(true),
	ansible.StatusIgnored:     lipgloss.NewStyle().Foreground(colorFail),
	ansible.StatusUnreachable: lipgloss.NewStyle().Foreground(colorFail).Bold(true),
}

// StatusStyle returns the style of a task status.
func StatusStyle(s ansible.Status) lipgloss.Style {
	return statusStyles[s]
}

// Section renders a section title.
func Section(s string) string { return styleSection.Render(s) }

// Muted renders s in the muted (gray) color.
func Muted(s string) string { return styleMuted.Render(s) }
