// Package display provides terminal output formatting for the agent.
package display

// ANSI escape codes for terminal styling.
const (
	Reset        = "\033[0m"
	Bold         = "\033[1m"
	Dim          = "\033[2m"
	BrightRed    = "\033[91m"
	BrightGreen  = "\033[92m"
	BrightYellow = "\033[93m"
	BrightCyan   = "\033[96m"
)

// ThinSpace is a Unicode thin space used for number formatting.
const ThinSpace = '\u2009'

// Truncate truncates a string for terminal display, adding "..." if truncated.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
