package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/llm/provider/fake"
	"github.com/stretchr/testify/assert"
)

func newREPLTestAgent(t *testing.T) (*Agent, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	a := New(
		fake.NewProvider(),
		t.TempDir(),
		5*time.Second,
		"",
		WithOutput(&buf),
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
	input := strings.NewReader("") // immediate EOF

	err := RunREPL(context.Background(), a, input)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "session")
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
