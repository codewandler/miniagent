package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/codewandler/llm/usage"
	"github.com/stretchr/testify/assert"
)

func TestCompactCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1.0k"},
		{1340, "1.3k"},
		{1500, "1.5k"},
		{9999, "10.0k"},
		{10000, "10.0k"},
		{10500, "10.5k"},
		{99999, "100.0k"},
		{100000, "100k"},
		{123456, "123k"},
		{999999, "1000k"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, compactCount(tt.input))
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
	t.Run("all fields with cache", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 1204},
				{Kind: usage.KindCacheRead, Count: 8432},
				{Kind: usage.KindOutput, Count: 87},
			},
			Cost: usage.Cost{Total: 0.0023},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "in: 9.6k")
		assert.Contains(t, parts, "cache_r: 8.4k 87.5%")
		assert.Contains(t, parts, "new: 1.2k")
		assert.Contains(t, parts, "out: 87")
		assert.Contains(t, parts, "cost: $0.0023")
	})

	t.Run("no cache plain input output", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 100},
				{Kind: usage.KindOutput, Count: 50},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "in: 100")
		assert.Contains(t, parts, "out: 50")
		assert.NotContains(t, parts, "cache")
		assert.NotContains(t, parts, "cost")
	})

	t.Run("cache read and write with non-cache input", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 200},
				{Kind: usage.KindCacheRead, Count: 300},
				{Kind: usage.KindCacheWrite, Count: 100},
				{Kind: usage.KindOutput, Count: 50},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "in: 600")
		assert.Contains(t, parts, "cache_r: 300 50.0%")
		assert.Contains(t, parts, "cache_w: 100")
		assert.Contains(t, parts, "new: 200")
	})

	t.Run("cache write only cold start", func(t *testing.T) {
		rec := usage.Record{
			Tokens: usage.TokenItems{
				{Kind: usage.KindInput, Count: 500},
				{Kind: usage.KindCacheWrite, Count: 400},
				{Kind: usage.KindOutput, Count: 60},
			},
		}
		parts := formatUsageParts(rec)
		assert.Contains(t, parts, "in: 900")
		assert.Contains(t, parts, "cache_w: 400")
		assert.Contains(t, parts, "new: 500")
		assert.NotContains(t, parts, "cache_r")
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

func TestRenderMarkdown_UsesExplicitStyle(t *testing.T) {
	out := renderMarkdown("# Title\n\n- item\n")
	assert.Contains(t, stripANSI(out), "Title")
	assert.Contains(t, stripANSI(out), "item")
	assert.NotEqual(t, "# Title\n\n- item\n", out)
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
		assert.Contains(t, stripANSI(out), "answer")
		assert.Contains(t, out, ansiDim)
		assert.Contains(t, out, ansiReset)
	})

	t.Run("text only paragraph waits until stable boundary", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteText("hello ")
		assert.NotContains(t, stripANSI(buf.String()), "hello")
		sd.WriteText("world\n\n")
		sd.End()

		out := buf.String()
		assert.Contains(t, stripANSI(out), "hello world")
		assert.NotContains(t, out, ansiDim)
	})

	t.Run("fenced code is withheld until closed", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteText("Before\n\n```go\nfmt.Println(1)\n")
		stripped := stripANSI(buf.String())
		assert.Contains(t, stripped, "Before")
		assert.NotContains(t, stripped, "fmt.Println(1)")

		sd.WriteText("```\n")
		sd.End()
		stripped = stripANSI(buf.String())
		assert.Contains(t, stripped, "fmt.Println(1)")
	})

	t.Run("tool call flushes pending markdown", func(t *testing.T) {
		var buf strings.Builder
		sd := newStepDisplay(&buf)

		sd.WriteText("let me check")
		sd.PrintToolCall("bash", map[string]any{"command": "ls -la"})
		sd.End()

		out := buf.String()
		assert.Contains(t, stripANSI(out), "let me check")
		assert.Contains(t, out, "🔧 bash")
		assert.Contains(t, out, `"command"`)
		assert.Contains(t, out, `"ls -la"`)
	})
}

// stripANSI removes ANSI escape codes from a string for test comparison.
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiRegex.ReplaceAllString(s, "")
}
