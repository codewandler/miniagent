package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/codewandler/llm/usage"
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

// formatUsageParts builds "input: N  cache_r: N (HIT%)  cache_w: N  output: N  cost: $X".
// The hit-rate annotation on cache_r shows what fraction of cache-touched tokens
// were reads (hits) vs writes (cold misses): cache_read / (cache_read + cache_write).
// Shared by step, turn, and session usage display.
func formatUsageParts(rec usage.Record) string {
	kindLabels := []struct {
		kind  usage.TokenKind
		label string
	}{
		{usage.KindInput, "input"},
		{usage.KindCacheRead, "cache_r"},
		{usage.KindCacheWrite, "cache_w"},
		{usage.KindOutput, "output"},
		{usage.KindReasoning, "reason"},
	}
	var parts []string
	for _, kl := range kindLabels {
		count := rec.Tokens.Count(kl.kind)
		if count == 0 {
			continue
		}
		s := fmt.Sprintf("%s: %s", kl.label, formatTokenCount(count))
		// Annotate cache_r with the hit rate: reads / (reads + writes).
		// 100 % means the cache was fully warm; 0 % would mean no cache_r at all
		// (which cannot happen here since count > 0 for KindCacheRead).
		if kl.kind == usage.KindCacheRead {
			cacheWrite := rec.Tokens.Count(usage.KindCacheWrite)
			hitRate := float64(count) * 100.0 / float64(count+cacheWrite)
			s += fmt.Sprintf(" (%.0f%%)", hitRate)
		}
		parts = append(parts, s)
	}
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
	w     io.Writer
	state displayState
}

func newStepDisplay(w io.Writer) *stepDisplay {
	return &stepDisplay{w: w, state: stateIdle}
}

// WriteReasoning outputs a reasoning token chunk in dim.
func (d *stepDisplay) WriteReasoning(chunk string) {
	if d.state == stateIdle {
		fmt.Fprint(d.w, ansiDim)
		d.state = stateReasoning
	}
	fmt.Fprint(d.w, chunk)
}

// WriteText outputs a text token chunk in normal weight.
func (d *stepDisplay) WriteText(chunk string) {
	switch d.state {
	case stateIdle:
		fmt.Fprint(d.w, "\n")
	case stateReasoning:
		fmt.Fprintf(d.w, "%s\n\n", ansiReset)
	}
	if d.state != stateText {
		d.state = stateText
	}
	fmt.Fprint(d.w, chunk)
}

// PrintToolCall displays a tool call header. Resets any open ANSI state.
func (d *stepDisplay) PrintToolCall(name, command string) {
	switch d.state {
	case stateReasoning:
		fmt.Fprintf(d.w, "%s\n", ansiReset)
	case stateText:
		fmt.Fprintln(d.w)
	}
	d.state = stateIdle
	fmt.Fprintf(d.w, "\n%s🔧 %s%s\n", ansiBrightYellow, name, ansiReset)
	fmt.Fprintf(d.w, "   %s$ %s%s\n", ansiDim, command, ansiReset)
}

// End closes any open ANSI state. Call after Result() returns.
func (d *stepDisplay) End() {
	switch d.state {
	case stateReasoning:
		fmt.Fprintf(d.w, "%s\n", ansiReset)
	case stateText:
		fmt.Fprintln(d.w)
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

func printStepUsage(w io.Writer, step int, rec usage.Record) {
	parts := formatUsageParts(rec)
	if parts == "" {
		return
	}
	fmt.Fprintf(w, "%s   ── step %d ── %s%s\n", ansiDim, step, parts, ansiReset)
}

func printTurnUsage(w io.Writer, turnID string, rec usage.Record) {
	parts := formatUsageParts(rec)
	if parts == "" {
		return
	}
	fmt.Fprintf(w, "%s   ── turn %s ── %s%s\n", ansiDim, turnID, parts, ansiReset)
}

// PrintSessionUsage prints the session-total usage line.
// Always emits the separator so REPL exit is visible even with no usage.
// Exported — called from main.go for one-shot mode.
func PrintSessionUsage(w io.Writer, rec usage.Record) {
	parts := formatUsageParts(rec)
	if parts == "" {
		fmt.Fprintf(w, "── session ──\n")
		return
	}
	fmt.Fprintf(w, "── session ── %s\n", parts)
}

// ---------------------------------------------------------------------------
// Error display
// ---------------------------------------------------------------------------

func printError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n%sError: %s%s\n", ansiBrightRed, err, ansiReset)
}
