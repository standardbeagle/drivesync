package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles holds all the lipgloss styles for the TUI.
type Styles struct {
	// Window
	Window      lipgloss.Style
	WindowTitle lipgloss.Style

	// Selection
	Selected   lipgloss.Style
	Unselected lipgloss.Style
	Cursor     lipgloss.Style

	// Drive display
	DriveName     lipgloss.Style
	DriveSize     lipgloss.Style
	DriveInfo     lipgloss.Style
	Partition     lipgloss.Style
	PartitionTree lipgloss.Style

	// Status
	Warning lipgloss.Style
	Error   lipgloss.Style
	Success lipgloss.Style
	Info    lipgloss.Style

	// Progress
	ProgressBar      lipgloss.Style
	ProgressBarFill  lipgloss.Style
	ProgressBarEmpty lipgloss.Style
	ProgressText     lipgloss.Style

	// Footer
	Footer    lipgloss.Style
	FooterKey lipgloss.Style

	// Input
	InputLabel  lipgloss.Style
	InputField  lipgloss.Style
	InputCursor lipgloss.Style

	// Button
	Button       lipgloss.Style
	ButtonActive lipgloss.Style
}

// DefaultStyles returns the default style configuration.
func DefaultStyles() Styles {
	// Colors
	primary := lipgloss.Color("39")    // Blue
	secondary := lipgloss.Color("245") // Gray
	success := lipgloss.Color("42")    // Green
	warning := lipgloss.Color("214")   // Orange
	danger := lipgloss.Color("196")    // Red
	highlight := lipgloss.Color("226") // Yellow

	return Styles{
		Window: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2),

		WindowTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),

		Selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight),

		Unselected: lipgloss.NewStyle().
			Foreground(secondary),

		Cursor: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),

		DriveName: lipgloss.NewStyle().
			Bold(true),

		DriveSize: lipgloss.NewStyle().
			Foreground(secondary),

		DriveInfo: lipgloss.NewStyle().
			Foreground(secondary).
			Italic(true),

		Partition: lipgloss.NewStyle().
			Foreground(secondary),

		PartitionTree: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		Warning: lipgloss.NewStyle().
			Bold(true).
			Foreground(warning),

		Error: lipgloss.NewStyle().
			Bold(true).
			Foreground(danger),

		Success: lipgloss.NewStyle().
			Bold(true).
			Foreground(success),

		Info: lipgloss.NewStyle().
			Foreground(primary),

		ProgressBar: lipgloss.NewStyle(),

		ProgressBarFill: lipgloss.NewStyle().
			Foreground(success),

		ProgressBarEmpty: lipgloss.NewStyle().
			Foreground(secondary),

		ProgressText: lipgloss.NewStyle().
			Foreground(secondary),

		Footer: lipgloss.NewStyle().
			Foreground(secondary),

		FooterKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),

		InputLabel: lipgloss.NewStyle().
			Bold(true),

		InputField: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(secondary).
			Padding(0, 1),

		InputCursor: lipgloss.NewStyle().
			Background(primary).
			Foreground(lipgloss.Color("0")),

		Button: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Padding(0, 2),

		ButtonActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Bold(true).
			Padding(0, 2),
	}
}
