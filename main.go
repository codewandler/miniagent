package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/llmadapter/unified"
	"github.com/codewandler/miniagent/agent"
	"github.com/codewandler/miniagent/agent/display"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type InferenceConfig = agent.InferenceOptions

func rootCmd() *cobra.Command {
	var (
		inference     InferenceConfig = agent.DefaultInferenceOptions()
		maxSteps                      = 30
		workspace     string
		systemPrompt  string
		totalTimeout  time.Duration
		toolTimeout   time.Duration
		thinkingFlag  string
		effortFlag    string
		session       string
		continueLast  bool
		sessionsDir   string
		contextBudget int
		verbose       bool
	)
	cmd := &cobra.Command{
		Use:   "miniagent [task]",
		Short: "A minimal agentic CLI with tools",
		Long: `miniagent runs an autonomous agent loop: model → tools → model → ...
With no arguments it starts an interactive REPL.
With a positional argument it runs the task once and exits.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if thinkingFlag != "" {
				inference.Thinking = agent.ThinkingMode(thinkingFlag)
			}
			if effortFlag != "" {
				inference.Effort = unified.ReasoningEffort(effortFlag)
			}
			budget, err := resolveContextBudget(contextBudget)
			if err != nil {
				return err
			}
			return execute(args, inference, maxSteps, workspace, systemPrompt, totalTimeout, toolTimeout, session, continueLast, sessionsDir, budget, verbose)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&inference.Model, "model", "m", inference.Model, "Model alias or full path")
	f.StringVarP(&workspace, "workspace", "w", "", "Working directory (default: $PWD)")
	f.IntVar(&maxSteps, "max-steps", maxSteps, "Maximum agent loop iterations per turn")
	f.IntVar(&inference.MaxTokens, "max-tokens", inference.MaxTokens, "Maximum output tokens per LLM call")
	f.StringVarP(&systemPrompt, "system", "s", "", "Override the system prompt body")
	f.DurationVar(&totalTimeout, "timeout", 0, "Total runtime timeout (0 = no limit)")
	f.DurationVar(&toolTimeout, "tool-timeout", 30*time.Second, "Per-tool call timeout")
	f.Float64Var(&inference.Temperature, "temperature", inference.Temperature, "Sampling temperature 0.0–2.0")
	f.StringVar(&thinkingFlag, "thinking", string(inference.Thinking), "Thinking mode: auto|on|off")
	f.StringVar(&effortFlag, "effort", string(inference.Effort), "Effort level: low|medium|high")
	f.StringVar(&session, "session", "", "Resume a session by id or JSONL path")
	f.BoolVar(&continueLast, "continue", false, "Resume the most recently active session")
	f.StringVar(&sessionsDir, "sessions-dir", "", "Session storage directory (default: ~/.miniagent/sessions)")
	f.IntVar(&contextBudget, "context-budget", 0, "Approximate input token budget for projected conversation history (0 = disabled, env: MINIAGENT_CONTEXT_BUDGET)")
	f.BoolVarP(&verbose, "verbose", "v", false, "Show resolved provider/model diagnostics")
	_ = cmd.RegisterFlagCompletionFunc("model", completeModelFlag)

	cmd.AddCommand(completionCmd(cmd))
	return cmd
}

func completionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: "completion", Short: "Generate or install shell completion", SilenceUsage: true, SilenceErrors: true}
	cmd.AddCommand(&cobra.Command{Use: "bash", Short: "Generate bash completion script", RunE: func(_ *cobra.Command, _ []string) error { return root.GenBashCompletionV2(os.Stdout, true) }})
	cmd.AddCommand(&cobra.Command{Use: "zsh", Short: "Generate zsh completion script", RunE: func(_ *cobra.Command, _ []string) error { return root.GenZshCompletion(os.Stdout) }})
	cmd.AddCommand(&cobra.Command{Use: "fish", Short: "Generate fish completion script", RunE: func(_ *cobra.Command, _ []string) error { return root.GenFishCompletion(os.Stdout, true) }})
	cmd.AddCommand(completionInstallCmd(root))
	return cmd
}

func completionInstallCmd(root *cobra.Command) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:           "install [bash|zsh|fish]",
		Short:         "Install shell completion to a standard user location",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = strings.ToLower(args[0])
			} else {
				shell = detectShell()
			}
			if shell == "" {
				return fmt.Errorf("unable to detect shell; pass bash, zsh, or fish")
			}
			target, err := completionInstallPath(shell, file)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create completion directory: %w", err)
			}
			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create completion file: %w", err)
			}
			defer f.Close()
			switch shell {
			case "bash":
				err = root.GenBashCompletionV2(f, true)
			case "zsh":
				err = root.GenZshCompletion(f)
			case "fish":
				err = root.GenFishCompletion(f, true)
			default:
				return fmt.Errorf("unsupported shell %q", shell)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Installed %s completion to %s\n", shell, target)
			fmt.Fprintln(os.Stdout, "Restart your shell or source the file to enable completions.")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Override installation target file")
	return cmd
}

func detectShell() string {
	shell := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return ""
	}
}

func completionInstallPath(shell, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local/share/bash-completion/completions/miniagent"), nil
	case "zsh":
		return filepath.Join(home, ".zsh/completions/_miniagent"), nil
	case "fish":
		return filepath.Join(home, ".config/fish/completions/miniagent.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func completeModelFlag(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	models := []string{"default", "fast", "powerful", "codex/gpt-5.4"}
	var matches []string
	for _, m := range models {
		if containsFold(m, toComplete) {
			matches = append(matches, m)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func execute(args []string, inference InferenceConfig, maxSteps int, workspace, systemPrompt string, totalTimeout, toolTimeout time.Duration, session string, continueLast bool, sessionsDir string, contextBudget int, verbose bool) error {
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		workspace = wd
	}
	resolvedSessionsDir, err := defaultSessionDir(sessionsDir)
	if err != nil {
		return err
	}
	resumePath, err := resolveSessionPath(resolvedSessionsDir, session, continueLast)
	if err != nil {
		return err
	}
	ctx := context.Background()
	opts := []agent.Option{
		agent.WithWorkspace(workspace),
		agent.WithToolTimeout(toolTimeout),
		agent.WithSystemOverride(systemPrompt),
		agent.WithInferenceOptions(inference),
		agent.WithMaxSteps(maxSteps),
		agent.WithSessionStoreDir(resolvedSessionsDir),
		agent.WithVerbose(verbose),
	}
	if contextBudget > 0 {
		opts = append(opts, agent.WithContextBudget(contextBudget))
	}
	if resumePath != "" {
		opts = append(opts, agent.WithResumeSession(resumePath))
	}
	a, err := agent.NewE(opts...)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		if totalTimeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, totalTimeout)
		}
		defer cancel()
		err := a.RunTurn(ctx, 1, args[0])
		fmt.Println()
		display.PrintSessionUsage(os.Stdout, a.SessionID(), a.Tracker().Aggregate())
		if errors.Is(err, agent.ErrMaxStepsReached) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			return nil
		}
		return err
	}
	return agent.RunREPL(ctx, a, os.Stdin)
}

func resolveContextBudget(flagValue int) (int, error) {
	if flagValue > 0 {
		return flagValue, nil
	}
	raw := strings.TrimSpace(os.Getenv("MINIAGENT_CONTEXT_BUDGET"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid MINIAGENT_CONTEXT_BUDGET %q", raw)
	}
	return value, nil
}

func defaultSessionDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".miniagent", "sessions"), nil
}

func resolveSessionPath(dir, session string, continueLast bool) (string, error) {
	if session != "" && continueLast {
		return "", fmt.Errorf("--session and --continue cannot be used together")
	}
	if continueLast {
		return latestSessionPath(dir)
	}
	if session == "" {
		return "", nil
	}
	if strings.ContainsAny(session, `/\`) || strings.HasSuffix(session, ".jsonl") {
		path, err := filepath.Abs(session)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("session %s: %w", path, err)
		}
		return path, nil
	}
	candidates := []string{
		filepath.Join(dir, session+".jsonl"),
		filepath.Join(dir, "*-"+session+".jsonl"),
	}
	var matches []string
	for _, pattern := range candidates {
		found, err := filepath.Glob(pattern)
		if err != nil {
			return "", err
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("session %q not found in %s", session, dir)
	}
	return newestPath(matches)
}

func latestSessionPath(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no sessions found in %s", dir)
	}
	return newestPath(matches)
}

func newestPath(paths []string) (string, error) {
	var newest string
	var newestMod time.Time
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = path
			newestMod = info.ModTime()
		}
	}
	return newest, nil
}
