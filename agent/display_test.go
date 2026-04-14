package agent

import (
	"strings"
	"testing"

	"github.com/codewandler/llm/usage"
	"github.com/stretchr/testify/assert"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1\u2009000"},
		{8432, "8\u2009432"},
		{12345, "12\u2009345"},
		{100000, "100\u2009000"},
		{1234567, "1\u2009234\u2009567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, formatTokenCount(tt.input))
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0, ""},
		{"tiny", 0.00001, "$0.000010"},
		{"small", 0.0023, "$0.0023"},
		{"medium", 0.0412, "$0.0412"},
		{"dollar", 1.24, "$1.24"},
		{"large", 12.50, "$12.50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatCost(tt.cost))
		})
	}
}

func TestTruncateDisplay(t *testing.T) {
	assert.Equal(t, "hello", truncateDisplay("hello", 300))

	long := strings.Repeat("x", 400)
	result := truncateDisplay(long, 300)
	assert.Equal(t, 303, len(result))
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestFormatUsageParts(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 1204},
				{Kind: usage.KindCacheRead, Count: 8432},
				{Kind: usage.KindOutput, Count: 87},
			},
			Cost: usage.Cost{Total: 0.0023},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "input: 1\u2009204")
		// cache_r is annotated with hit rate; no cache_w → 100 % hit
		assert.Contains(t, parts, "cache_r: 8\u2009432 (100%)")
		assert.Contains(t, parts, "output: 87")
		assert.Contains(t, parts, "cost: $0.0023")
	})

	t.Run("zero tokens omitted", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 100},
				{Kind: usage.KindOutput, Count: 50},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "input: 100")
		assert.Contains(t, parts, "output: 50")
		assert.NotContains(t, parts, "cache")
		assert.NotContains(t, parts, "cost")
	})

	t.Run("cache hit rate 75 percent", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 200},
				{Kind: usage.KindCacheRead, Count: 300},  // 300/(300+100) = 75 %
				{Kind: usage.KindCacheWrite, Count: 100},
				{Kind: usage.KindOutput, Count: 50},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "cache_r: 300 (75%)")
		assert.Contains(t, parts, "cache_w: 100")
		// hit rate annotation must NOT appear on cache_w
		assert.NotContains(t, parts, "cache_w: 100 (")
	})

	t.Run("cache write only cold start 0 pct hit", func(t *testing.T) {
		// No cache reads at all: cache_r line is omitted entirely (zero guard),
		// so the hit-rate annotation is also absent.
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 500},
				{Kind: usage.KindCacheWrite, Count: 400},
				{Kind: usage.KindOutput, Count: 60},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "cache_w: 400")
		// No cache reads → no hit-rate annotation at all
		assert.NotContains(t, parts, "cache_r")
		assert.NotContains(t, parts, "%)")
	})

	t.Run("empty record", func(t *testing.T) {
		assert.Equal(t, "", formatUsageParts(usage.Record{}))
	})
}

func TestExtractBashOutput(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"json result", `{"output":"hello"}`, "hello"},
		{"plain string", "just text", "just text"},
		{"non-string", 42, "42"},
		{"empty json", `{"output":""}`, ""},
		{"malformed json", `{bad`, `{bad`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractBashOutput(tt.input))
		})
	}
}

func TestStepDisplay_StateTransitions(t *testing.T) {
	t.Run("reasoning then text", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteReasoning("thinking...")
		sd.WriteText("answer")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "thinking...")
		assert.Contains(t, out, "answer")
		assert.Contains(t, out, ansiDim)
		assert.Contains(t, out, ansiReset)
	})

	t.Run("text only", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteText("hello ")
		sd.WriteText("world")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "hello world")
		assert.NotContains(t, out, ansiDim)
	})

	t.Run("tool call resets state", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteText("let me check")
		sd.PrintToolCall("bash", "ls -la")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "let me check")
		assert.Contains(t, out, "🔧 bash")
		assert.Contains(t, out, "$ ls -la")
	})
}
