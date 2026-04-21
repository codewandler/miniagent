package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/codewandler/miniagent/agent/usage"
)

// PrintStepHeader prints the step header with step number.
func PrintStepHeader(w io.Writer, step, maxSteps int) {
	fmt.Fprintf(w, "\n%s── %s💭 Step %d/%d%s %s────────────────────────────────%s\n",
		Dim, Bold+BrightCyan, step, maxSteps, Reset, Dim, Reset,
	)
}

// PrintResolvedModel prints the resolved model name if non-empty.
func PrintResolvedModel(w io.Writer, model string) {
	if model == "" {
		return
	}
	fmt.Fprintf(w, "%s   model: %s%s\n", Dim, model, Reset)
}

// PrintToolResult displays a tool result with success/error indicator.
func PrintToolResult(w io.Writer, output string, isError bool) {
	prefix := BrightGreen + "✓" + Reset
	if isError {
		prefix = BrightRed + "✗" + Reset
	}
	display := Truncate(strings.TrimSpace(output), 300)
	if display == "" {
		display = "(no output)"
	}
	fmt.Fprintf(w, "%s %s\n", prefix, display)
}

// PrintStepUsage prints usage statistics for a single step.
func PrintStepUsage(w io.Writer, step int, rec usage.Record, model string) {
	parts := FormatUsageParts(rec)
	modelPart := ""
	if model != "" {
		modelPart = fmt.Sprintf("  model: %s", model)
	}
	if parts == "" && modelPart == "" {
		return
	}
	if parts == "" {
		fmt.Fprintf(w, "%s   ── step %d ──%s%s\n", Dim, step, modelPart, Reset)
	} else {
		fmt.Fprintf(w, "%s   ── step %d ── %s%s%s\n", Dim, step, parts, modelPart, Reset)
	}
	printStepUsageDetails(w, rec)
}

func printStepUsageDetails(w io.Writer, rec usage.Record) {
	if parts := stepUsageDimsParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   dims: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
	if parts := stepUsageUsageParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   usage: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
	if parts := stepUsageCostParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   costs: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
}

func stepUsageDimsParts(rec usage.Record) []string {
	var parts []string
	if rec.Dims.Provider != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", rec.Dims.Provider))
	}
	if rec.Dims.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", rec.Dims.Model))
	}
	if rec.Dims.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", rec.Dims.RequestID))
	}
	if rec.Dims.TurnID != "" {
		parts = append(parts, fmt.Sprintf("turn_id=%s", rec.Dims.TurnID))
	}
	if rec.Dims.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id=%s", rec.Dims.SessionID))
	}
	if len(rec.Dims.Labels) > 0 {
		parts = append(parts, fmt.Sprintf("labels=%v", rec.Dims.Labels))
	}
	return parts
}

func stepUsageUsageParts(rec usage.Record) []string {
	var parts []string
	if v := rec.Tokens.TotalInput(); v != 0 {
		parts = append(parts, fmt.Sprintf("total_input=%d", v))
	}
	if v := rec.Tokens.Count(usage.KindInput); v != 0 {
		parts = append(parts, fmt.Sprintf("input=%d", v))
	}
	if v := rec.Tokens.Count(usage.KindCacheRead); v != 0 {
		parts = append(parts, fmt.Sprintf("cache_read=%d", v))
	}
	if v := rec.Tokens.Count(usage.KindCacheWrite); v != 0 {
		parts = append(parts, fmt.Sprintf("cache_write=%d", v))
	}
	if v := rec.Tokens.TotalOutput(); v != 0 {
		parts = append(parts, fmt.Sprintf("total_output=%d", v))
	}
	if v := rec.Tokens.Count(usage.KindOutput); v != 0 {
		parts = append(parts, fmt.Sprintf("output=%d", v))
	}
	if v := rec.Tokens.Count(usage.KindReasoning); v != 0 {
		parts = append(parts, fmt.Sprintf("reasoning=%d", v))
	}
	return parts
}

func stepUsageCostParts(rec usage.Record) []string {
	var parts []string
	if v := rec.Cost.Total; v != 0 {
		parts = append(parts, fmt.Sprintf("total=%.6f", v))
	}
	if v := rec.Cost.Input; v != 0 {
		parts = append(parts, fmt.Sprintf("input=%.6f", v))
	}
	if v := rec.Cost.CacheRead; v != 0 {
		parts = append(parts, fmt.Sprintf("cache_read=%.6f", v))
	}
	if v := rec.Cost.CacheWrite; v != 0 {
		parts = append(parts, fmt.Sprintf("cache_write=%.6f", v))
	}
	if v := rec.Cost.Output; v != 0 {
		parts = append(parts, fmt.Sprintf("output=%.6f", v))
	}
	if v := rec.Cost.Reasoning; v != 0 {
		parts = append(parts, fmt.Sprintf("reasoning=%.6f", v))
	}
	return parts
}

// PrintTurnUsage prints usage statistics for a turn.
func PrintTurnUsage(w io.Writer, turnID int, rec usage.Record) {
	parts := FormatUsageParts(rec)
	if parts == "" {
		return
	}
	fmt.Fprintf(w, "%s   ── turn %d ── %s%s\n", Dim, turnID, parts, Reset)
}

// PrintSessionUsage prints the session-total usage line.
// Always emits the separator so REPL exit is visible even with no usage.
func PrintSessionUsage(w io.Writer, sessionID string, rec usage.Record) {
	parts := FormatUsageParts(rec)
	if parts == "" {
		fmt.Fprintf(w, "── session %s ──\n", sessionID)
		return
	}
	fmt.Fprintf(w, "── session %s ── %s\n", sessionID, parts)
}

// PrintError prints an error message in red.
func PrintError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n%sError: %s%s\n", BrightRed, err, Reset)
}
