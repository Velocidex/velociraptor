package survey

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func getTheme() *huh.Theme {
	theme := huh.ThemeBase16()
	theme.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Bold(true)
	theme.Focused.Description = lipgloss.NewStyle().Foreground(lipgloss.Color("#DCDCDC"))
	theme.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	theme.Focused.FocusedButton = theme.Focused.FocusedButton.Foreground(lipgloss.Color("0")).Background(lipgloss.Color("2"))
	theme.Focused.BlurredButton = theme.Focused.BlurredButton.Foreground(lipgloss.Color("8"))
	theme.Blurred.BlurredButton = theme.Focused.BlurredButton.Foreground(lipgloss.Color("8"))
	theme.Blurred.FocusedButton = theme.Blurred.BlurredButton
	theme.Focused.SelectSelector = lipgloss.NewStyle().SetString(" ● ").Foreground(lipgloss.Color("2"))
	theme.Blurred.SelectSelector = lipgloss.NewStyle().SetString("   ")
	theme.Help.Ellipsis = theme.Help.Ellipsis.Foreground(lipgloss.Color("3"))
	theme.Help.ShortKey = theme.Help.ShortKey.Foreground(lipgloss.Color("3"))
	theme.Help.ShortDesc = theme.Help.ShortDesc.Foreground(lipgloss.Color("3"))
	theme.Help.ShortSeparator = theme.Help.ShortSeparator.Foreground(lipgloss.Color("3"))
	theme.Help.FullKey = theme.Help.FullKey.Foreground(lipgloss.Color("3"))
	theme.Help.FullDesc = theme.Help.FullDesc.Foreground(lipgloss.Color("3"))
	theme.Help.FullSeparator = theme.Help.FullSeparator.Foreground(lipgloss.Color("3"))

	return theme
}
