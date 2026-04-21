package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newREPLTestAgent(t *testing.T) (*Agent, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	testModel := TestServiceID + "/" + TestModelID
	a := New(newFakeService(),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithOutput(&buf),
		WithInferenceOptions(InferenceOptions{
			Model:     testModel,
			MaxTokens: 1000,
			Thinking:  "on",
			Effort:    "medium",
		}),
	)
	return a, &buf
}

func TestRunREPL_ExitCommand(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("exit\n")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "session")
}

func TestRunREPL_QuitCommand(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("quit\n")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "session")
}

func TestRunREPL_EOF(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "session")
}

func TestRunREPL_ShowsParamsBeforePrompt(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("exit\n")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	out := buf.String()
	expectedModel := TestServiceID + "/" + TestModelID
	assert.Contains(t, out, "model: "+expectedModel)
	assert.Contains(t, out, "resolved_instance: test")
	assert.Contains(t, out, "resolved_model: "+TestModelID)
	assert.Contains(t, out, "thinking: on")
	assert.Contains(t, out, "effort: medium")
	assert.Less(t, strings.Index(out, "model: "+expectedModel), strings.Index(out, "miniagent> "))
}

func TestRunREPL_ExecutesThenExits(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("say hello\nexit\n")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Step 1")
	assert.Contains(t, out, "session")
}

func TestRunREPL_SkipsEmptyLines(t *testing.T) {
	a, buf := newREPLTestAgent(t)
	input := strings.NewReader("\n\n  \nexit\n")
	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	assert.NotContains(t, buf.String(), "Step 1")
}
