package display

import (
	"encoding/json"
	"fmt"
	"io"

	acoremd "github.com/codewandler/agentcore/markdown"
)

// State represents the current display state during step output.
type State int

const (
	StateIdle State = iota
	StateReasoning
	StateText
)

// StepDisplay manages streaming output display for a single agent step.
type StepDisplay struct {
	w                   io.Writer
	state               State
	mdBuffer            *acoremd.Buffer
	markdownRender      func(string) string
	hasRenderedMarkdown bool
}

// NewStepDisplay creates a new StepDisplay for the given writer.
func NewStepDisplay(w io.Writer) *StepDisplay {
	return NewStepDisplayWithRenderer(w, NewMarkdownRendererForWriter(w))
}

// NewStepDisplayWithRenderer creates a StepDisplay with a custom markdown renderer.
func NewStepDisplayWithRenderer(w io.Writer, renderer func(string) string) *StepDisplay {
	if renderer == nil {
		renderer = func(s string) string { return s }
	}
	d := &StepDisplay{w: w, state: StateIdle, markdownRender: renderer}
	d.mdBuffer = acoremd.NewBuffer(func(blocks []acoremd.Block) {
		for _, block := range blocks {
			d.writeRenderedMarkdown(block.Markdown)
		}
	})
	return d
}

// WriteReasoning outputs a reasoning token chunk in dim.
func (d *StepDisplay) WriteReasoning(chunk string) {
	if d.state == StateIdle {
		fmt.Fprint(d.w, Dim)
		d.state = StateReasoning
	}
	fmt.Fprint(d.w, chunk)
}

// WriteText buffers markdown chunks and renders stable blocks as they become available.
func (d *StepDisplay) WriteText(chunk string) {
	if d.state == StateReasoning {
		fmt.Fprintf(d.w, "%s\n\n", Reset)
	}
	if d.state != StateText {
		d.state = StateText
	}
	_, _ = d.mdBuffer.WriteString(chunk)
}

// PrintToolCall displays a tool call header and resets any open ANSI state.
func (d *StepDisplay) PrintToolCall(name string, args map[string]any) {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.mdBuffer.Flush()
		fmt.Fprint(d.w, "\n")
	}
	d.state = StateIdle
	d.hasRenderedMarkdown = false
	fmt.Fprintf(d.w, "\n%s🔧 %s%s\n", BrightYellow, name, Reset)
	if len(args) > 0 {
		jsonArgs, _ := json.MarshalIndent(args, "   ", "  ")
		fmt.Fprintf(d.w, "   %s$ %s%s\n", Dim, string(jsonArgs), Reset)
	} else {
		fmt.Fprintf(d.w, "   %s(no args)%s\n", Dim, Reset)
	}
}

func (d *StepDisplay) writeRenderedMarkdown(md string) {
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

// End closes any open ANSI state. Call after streaming completes.
func (d *StepDisplay) End() {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.mdBuffer.Flush()
		fmt.Fprint(d.w, "\n")
		d.hasRenderedMarkdown = false
	}
	d.state = StateIdle
}
