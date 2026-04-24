package display

import (
	"fmt"
	"strings"

	coreusage "github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

// FormatTokenCount formats an integer with thin-space thousands separators.
func FormatTokenCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	remainder := len(s) % 3
	for i, c := range s {
		if i > 0 && i%3 == remainder {
			b.WriteRune(ThinSpace)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// FormatCost formats a dollar cost with adaptive precision.
// Returns "" for zero cost.
func FormatCost(cost float64) string {
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

// CompactCount returns a human-readable compact representation of a count:
//   - < 1000: returns the number as-is (e.g., "842")
//   - >= 1000 and < 100,000: returns X.Xk with 1 decimal (e.g., "1.3k", "12.5k", "99.9k")
//   - >= 100,000: returns XXXk with no decimals (e.g., "100k", "123k", "999k")
func CompactCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 100_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.0fk", float64(n)/1000)
}

// FormatUsageParts builds a compact usage summary string.
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
func FormatUsageParts(rec coreusage.Record) string {
	var parts []string

	// ── Input section ──
	totalIn := rec.Usage.Tokens.InputTotal()
	cacheRead := rec.Usage.Tokens.Count(unified.TokenKindInputCacheRead)
	cacheWrite := rec.Usage.Tokens.Count(unified.TokenKindInputCacheWrite)
	nonCache := rec.Usage.Tokens.Count(unified.TokenKindInputNew)
	hasCaching := cacheRead > 0 || cacheWrite > 0

	if totalIn > 0 {
		if hasCaching {
			// Show total with cache breakdown in parentheses.
			var cacheParts []string
			if cacheRead > 0 {
				hitRate := float64(cacheRead) * 100.0 / float64(totalIn)
				cacheParts = append(cacheParts, fmt.Sprintf("cache_r: %s %.1f%%", CompactCount(cacheRead), hitRate))
			}
			if cacheWrite > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("cache_w: %s", CompactCount(cacheWrite)))
			}
			if nonCache > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("new: %s", CompactCount(nonCache)))
			}
			parts = append(parts, fmt.Sprintf("in: %s (%s)", CompactCount(totalIn), strings.Join(cacheParts, "  ")))
		} else {
			parts = append(parts, fmt.Sprintf("in: %s", CompactCount(totalIn)))
		}
	}

	// ── Output section ──
	output := rec.Usage.Tokens.Count(unified.TokenKindOutput)
	reasoning := rec.Usage.Tokens.Count(unified.TokenKindOutputReasoning)
	if output > 0 {
		parts = append(parts, fmt.Sprintf("out: %s", CompactCount(output)))
	}
	if reasoning > 0 {
		parts = append(parts, fmt.Sprintf("reason: %s", CompactCount(reasoning)))
	}

	// ── Cost ──
	if cs := FormatCost(rec.Usage.Costs.Total()); cs != "" {
		parts = append(parts, fmt.Sprintf("cost: %s", cs))
	}
	return strings.Join(parts, "  ")
}
