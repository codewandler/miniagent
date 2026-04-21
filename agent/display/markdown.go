package display

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// RenderMarkdown renders markdown text for terminal display using glamour.
// Uses a fixed "dark" style for consistent colored output regardless of TTY status.
func RenderMarkdown(text string) string {
	return NewMarkdownRendererForWriter(os.Stdout)(text)
}

// NewMarkdownRendererForWriter creates a markdown renderer sized for the given writer.
func NewMarkdownRendererForWriter(w io.Writer) func(string) string {
	return NewMarkdownRenderer(markdownRenderWidth(w))
}

// NewMarkdownRenderer creates a markdown renderer with the specified width.
func NewMarkdownRenderer(width int) func(string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return func(s string) string { return s }
	}
	return func(s string) string {
		out, err := r.Render(s)
		if err != nil {
			return s
		}
		return TrimOuterRenderedBlankLines(out)
	}
}

func markdownRenderWidth(w io.Writer) int {
	const fallback = 80
	f, ok := w.(*os.File)
	if !ok {
		return fallback
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 20 {
		return fallback
	}
	return width
}

// TrimOuterRenderedBlankLines removes leading and trailing blank lines from rendered output.
func TrimOuterRenderedBlankLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && IsVisuallyBlankRenderedLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && IsVisuallyBlankRenderedLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// IsVisuallyBlankRenderedLine returns true if a line is visually blank
// (contains only ANSI codes and/or whitespace).
func IsVisuallyBlankRenderedLine(s string) bool {
	s = ansiOnlyLineRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s) == ""
}
