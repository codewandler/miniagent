package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIRealQuerySmoke runs the actual miniagent CLI binary against a real provider.
// This is effectively an end-to-end smoke test, but it lives under integration/
// for now to keep all opt-in live-provider tests together.
func TestCLIRealQuerySmoke(t *testing.T) {
	if os.Getenv("MINIAGENT_E2E") == "" {
		t.Skip("set MINIAGENT_E2E=1 to run real CLI smoke test")
	}
	if os.Getenv("OPENAI_API_KEY") == "" && os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("requires real provider credentials")
	}

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("go", "run", ".", "--model", "codex/gpt-5.4", "--temperature", "1", "Respond with exactly the single word: pong")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cli failed: %v\n%s", err, string(out))
	}
	stripped := stripANSISmoke(string(out))
	if !strings.Contains(strings.ToLower(stripped), "pong") {
		t.Fatalf("expected output to contain pong, got:\n%s", stripped)
	}
}

func stripANSISmoke(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
