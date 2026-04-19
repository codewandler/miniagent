package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	acoremd "github.com/codewandler/agentcore/markdown"
	"github.com/codewandler/llm/usage"
	"golang.org/x/term"
)

// ANSI escape codes
const (
	ansiReset        = "\033[0m"
	ansiBold         = "\033[1m"
	ansiDim          = "\033[2m"
	ansiBrightRed    = "\033[91m"
	ansiBrightGreen  = "\033[92m"
	ansiBrightYellow = "\033[93m"
	ansiBrightCyan   = "\033[96m"
)

const thinSpace = '\u2009'

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// formatTokenCount formats an integer with thin-space thousands separators.
func formatTokenCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	remainder := len(s) % 3
	for i, c := range s {
		if i > 0 && i%3 == remainder {
			b.WriteRune(thinSpace)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// formatCost formats a dollar cost with adaptive precision.
// Returns "" for zero cost.
func formatCost(cost float64) string {
	if cost == 0 {
		return ""
	}
	switch {
	case cost < 0.0001:
		return fmt.Sprintf("$%.6f", cost)
	case cost < 1.0:
		return fmt.Sprintf("$%.4f", cost)
	default:
		return fmt.Sprintf("$%.2f", cost)
	}
}

// truncateDisplay truncates a string for terminal display.
func truncateDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// formatUsageParts builds a compact usage summary string.
//
// When caching is active the total input token count is shown first, followed
// by a parenthesised cache breakdown so it's immediately clear how many tokens
// were sent in total:
//
//	in: 12.5k (cache_r: 12.2k 98%  cache_w: 0  new: 300)  out: 1.2k  cost: $0.0032
//
// When there is no caching the display is simply:
//
//	in: 12.5k  out: 1.2k  cost: $0.0032
//
// Shared by step, turn, and session usage display.
func formatUsageParts(rec usage.Record) string {
	var parts []string

	// ── Input section ──
	totalIn := rec.Tokens.TotalInput()
	cacheRead := rec.Tokens.Count(usage.KindCacheRead)
	cacheWrite := rec.Tokens.Count(usage.KindCacheWrite)
	nonCache := rec.Tokens.Count(usage.KindInput)
	hasCaching := cacheRead > 0 || cacheWrite > 0

	if totalIn > 0 {
		if hasCaching {
			// Show total with cache breakdown in parentheses.
			var cacheParts []string
			if cacheRead > 0 {
				hitRate := float64(cacheRead) * 100.0 / float64(totalIn)
				cacheParts = append(cacheParts, fmt.Sprintf("cache_r: %s %.1f%%", compactCount(cacheRead), hitRate))
			}
			if cacheWrite > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("cache_w: %s", compactCount(cacheWrite)))
			}
			if nonCache > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("new: %s", compactCount(nonCache)))
			}
			parts = append(parts, fmt.Sprintf("in: %s (%s)", compactCount(totalIn), strings.Join(cacheParts, "  ")))
		} else {
			parts = append(parts, fmt.Sprintf("in: %s", compactCount(totalIn)))
		}
	}

	// ── Output section ──
	output := rec.Tokens.Count(usage.KindOutput)
	reasoning := rec.Tokens.Count(usage.KindReasoning)
	if output > 0 {
		parts = append(parts, fmt.Sprintf("out: %s", compactCount(output)))
	}
	if reasoning > 0 {
		parts = append(parts, fmt.Sprintf("reason: %s", compactCount(reasoning)))
	}

	// ── Cost ──
	if cs := formatCost(rec.Cost.Total); cs != "" {
		parts = append(parts, fmt.Sprintf("cost: %s", cs))
	}
	return strings.Join(parts, "  ")
}

// extractBashOutput extracts the human-readable output from a tool result.
// The handler returns JSON like {"output":"hello"} — this parses it to "hello".
// Falls back to fmt.Sprint for anything unexpected.
func extractBashOutput(raw any) string {
	s, ok := raw.(string)
	if !ok {
		return fmt.Sprint(raw)
	}
	var result BashResult
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return s // not JSON — return as-is
	}
	return result.Output
}

// ---------------------------------------------------------------------------
// Step display state machine
// ---------------------------------------------------------------------------

type displayState int

const (
	stateIdle displayState = iota
	stateReasoning
	stateText
)

type stepDisplay struct {
	w                   io.Writer
	state               displayState
	mdBuffer            *acoremd.Buffer
	markdownRender      func(string) string
	hasRenderedMarkdown bool
}

func newStepDisplay(w io.Writer) *stepDisplay {
	return newStepDisplayWithRenderer(w, newMarkdownRendererForWriter(w))
}

func newStepDisplayWithRenderer(w io.Writer, renderer func(string) string) *stepDisplay {
	if renderer == nil {
		renderer = func(s string) string { return s }
	}
	d := &stepDisplay{w: w, state: stateIdle, markdownRender: renderer}
	d.mdBuffer = acoremd.NewBuffer(func(blocks []acoremd.Block) {
		for _, block := range blocks {
			d.writeRenderedMarkdown(block.Markdown)
		}
	})
	return d
}

// WriteReasoning outputs a reasoning token chunk in dim.
func (d *stepDisplay) WriteReasoning(chunk string) {
	if d.state == stateIdle {
		fmt.Fprint(d.w, ansiDim)
		d.state = stateReasoning
	}
	fmt.Fprint(d.w, chunk)
}

// WriteText buffers markdown chunks and renders stable blocks as they become available.
func (d *stepDisplay) WriteText(chunk string) {
	if d.state == stateReasoning {
		fmt.Fprintf(d.w, "%s\n\n", ansiReset)
	}
	if d.state != stateText {
		d.state = stateText
	}
	_, _ = d.mdBuffer.WriteString(chunk)
}

// PrintToolCall displays a tool call header and resets any open ANSI state.
func (d *stepDisplay) PrintToolCall(name string, args map[string]any) {
	switch d.state {
	case stateReasoning:
		fmt.Fprintf(d.w, "%s\n", ansiReset)
	case stateText:
		_ = d.mdBuffer.Flush()
		fmt.Fprint(d.w, "\n")
	}
	d.state = stateIdle
	d.hasRenderedMarkdown = false
	fmt.Fprintf(d.w, "\n%s🔧 %s%s\n", ansiBrightYellow, name, ansiReset)
	if len(args) > 0 {
		jsonArgs, _ := json.MarshalIndent(args, "   ", "  ")
		fmt.Fprintf(d.w, "   %s$ %s%s\n", ansiDim, string(jsonArgs), ansiReset)
	} else {
		fmt.Fprintf(d.w, "   %s(no args)%s\n", ansiDim, ansiReset)
	}
}

func (d *stepDisplay) writeRenderedMarkdown(md string) {
	rendered := d.markdownRender(md)
	if rendered == "" {
		return
	}
	if d.hasRenderedMarkdown {
		fmt.Fprint(d.w, "\n\n")
	}
	fmt.Fprint(d.w, rendered)
	d.hasRenderedMarkdown = true
}

// renderMarkdown renders markdown text for terminal display using glamour.
// Uses a fixed "dark" style for consistent colored output regardless of TTY status.
func renderMarkdown(text string) string {
	return newMarkdownRendererForWriter(os.Stdout)(text)
}

func newMarkdownRendererForWriter(w io.Writer) func(string) string {
	return newMarkdownRenderer(markdownRenderWidth(w))
}

func newMarkdownRenderer(width int) func(string) string {
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
		return trimOuterRenderedBlankLines(out)
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

func trimOuterRenderedBlankLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && isVisuallyBlankRenderedLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && isVisuallyBlankRenderedLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func isVisuallyBlankRenderedLine(s string) bool {
	s = ansiOnlyLineRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s) == ""
}

// End closes any open ANSI state. Call after Result() returns.
func (d *stepDisplay) End() {
	switch d.state {
	case stateReasoning:
		fmt.Fprintf(d.w, "%s\n", ansiReset)
	case stateText:
		_ = d.mdBuffer.Flush()
		fmt.Fprint(d.w, "\n")
		d.hasRenderedMarkdown = false
	}
	d.state = stateIdle
}

// ---------------------------------------------------------------------------
// Step header
// ---------------------------------------------------------------------------

func printStepHeader(w io.Writer, step, maxSteps int) {
	fmt.Fprintf(w, "\n%s── %s💭 Step %d/%d%s %s────────────────────────────────%s\n",
		ansiDim, ansiBold+ansiBrightCyan, step, maxSteps, ansiReset, ansiDim, ansiReset,
	)
}

// ---------------------------------------------------------------------------
// Tool result display
// ---------------------------------------------------------------------------

func printToolResult(w io.Writer, output string, isError bool) {
	prefix := ansiBrightGreen + "✓" + ansiReset
	if isError {
		prefix = ansiBrightRed + "✗" + ansiReset
	}
	display := truncateDisplay(strings.TrimSpace(output), 300)
	if display == "" {
		display = "(no output)"
	}
	fmt.Fprintf(w, "%s %s\n", prefix, display)
}

// ---------------------------------------------------------------------------
// Usage lines
// ---------------------------------------------------------------------------

func printStepUsage(w io.Writer, step int, rec usage.Record, model string) {
	parts := formatUsageParts(rec)
	modelPart := ""
	if model != "" {
		modelPart = fmt.Sprintf("  model: %s", model)
	}
	if parts == "" && modelPart == "" {
		return
	}
	if parts == "" {
		fmt.Fprintf(w, "%s   ── step %d ──%s%s\n", ansiDim, step, modelPart, ansiReset)
		return
	}
	fmt.Fprintf(w, "%s   ── step %d ── %s%s%s\n", ansiDim, step, parts, modelPart, ansiReset)
}

func printTurnUsage(w io.Writer, turnID int, rec usage.Record) {
	parts := formatUsageParts(rec)
	if parts == "" {
		return
	}
	fmt.Fprintf(w, "%s   ── turn %d ── %s%s\n", ansiDim, turnID, parts, ansiReset)
}

// PrintSessionUsage prints the session-total usage line.
// Always emits the separator so REPL exit is visible even with no usage.
// Exported — called from main.go for one-shot mode.
func PrintSessionUsage(w io.Writer, sessionID string, rec usage.Record) {
	parts := formatUsageParts(rec)
	if parts == "" {
		fmt.Fprintf(w, "── session %s ──\n", sessionID)
		return
	}
	fmt.Fprintf(w, "── session %s ── %s\n", sessionID, parts)
}

// ---------------------------------------------------------------------------
// Error display
// ---------------------------------------------------------------------------

func printError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n%sError: %s%s\n", ansiBrightRed, err, ansiReset)
}

// compactCount returns a human-readable compact representation of a count:
// - < 1000: returns the number as-is (e.g., "842")
// - >= 1000 and < 100,000: returns X.Xk with 1 decimal (e.g., "1.3k", "12.5k", "99.9k")
// - >= 100,000: returns XXXk with no decimals (e.g., "100k", "123k", "999k")
func compactCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 100_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.0fk", float64(n)/1000)
}
