package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/codewandler/llm"
	"github.com/codewandler/llm/cmd/llmcli/store"
	"github.com/codewandler/llm/provider/auto"
	"github.com/codewandler/miniagent/agent"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		// SilenceErrors is true, so cobra won't print the error.
		// Print it ourselves so the user sees what went wrong.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		model        string
		workspace    string
		maxSteps     int
		maxTokens    int
		systemPrompt string
		timeout      time.Duration
		thinking     llm.ThinkingMode
		effort       llm.Effort
		temperature  float64
	)

	cmd := &cobra.Command{
		Use:   "miniagent [task]",
		Short: "A minimal agentic CLI with a bash tool",
		Long: `miniagent runs an autonomous agent loop: LLM → bash → LLM → ...

With no arguments it starts an interactive REPL.
With a positional argument it runs the task once and exits.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return execute(args, model, workspace, maxSteps, maxTokens, systemPrompt, timeout, thinking, effort, temperature)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&model, "model", "m", "default", "Model alias or full path")
	f.StringVarP(&workspace, "workspace", "w", "", "Working directory (default: $PWD)")
	f.IntVar(&maxSteps, "max-steps", 30, "Maximum agent loop iterations per turn")
	f.IntVar(&maxTokens, "max-tokens", 16_000, "Maximum output tokens per LLM call")
	f.StringVarP(&systemPrompt, "system", "s", "", "Override the system prompt body")
	f.DurationVar(&timeout, "timeout", 30*time.Second, "Per-command bash timeout")
	f.Float64Var(&temperature, "temperature", 0.1, "Sampling temperature 0.0–2.0")
	f.TextVar(&thinking, "thinking", llm.ThinkingOn, "Thinking mode: auto|on|off")
	f.TextVar(&effort, "effort", llm.EffortMedium, "Effort level: low|medium|high|max")

	cmd.AddCommand(modelsCmd())
	return cmd
}

func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "models",
		Short:         "List all available model IDs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			provider, err := createProvider(cmd.Context())
			if err != nil {
				return err
			}
			for _, m := range provider.Models() {
				fmt.Println(m.ID)
			}
			return nil
		},
	}
}

func execute(
	args []string,
	model, workspace string,
	maxSteps, maxTokens int,
	systemPrompt string,
	timeout time.Duration,
	thinking llm.ThinkingMode,
	effort llm.Effort,
	temperature float64,
) error {
	// Resolve and validate workspace
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		workspace = wd
	}
	workspace, _ = filepath.Abs(workspace)
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace directory does not exist: %s", workspace)
	}

	// Provider setup (mirrors cmd/llmcli)
	ctx := context.Background()
	provider, err := createProvider(ctx)
	if err != nil {
		return err
	}

	// Build agent
	a := agent.New(provider, workspace, timeout, systemPrompt,
		agent.WithModel(model),
		agent.WithMaxSteps(maxSteps),
		agent.WithMaxTokens(maxTokens),
		agent.WithThinking(thinking),
		agent.WithEffort(effort),
		agent.WithTemperature(temperature),
	)

	// One-shot mode
	if len(args) == 1 {
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		err := a.RunTurn(ctx, "1", args[0])
		fmt.Println()
		agent.PrintSessionUsage(os.Stdout, a.Tracker().Aggregate())
		if errors.Is(err, agent.ErrMaxStepsReached) {
			// Partial output was produced; treat as a warning, not a hard failure.
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			return nil
		}
		return err
	}

	// REPL mode
	return agent.RunREPL(ctx, a, os.Stdin)
}

// [REVIEW FIX #6]: return llm.Provider interface, not *router.Provider.
func createProvider(ctx context.Context) (llm.Provider, error) {
	var autoOpts []auto.Option
	autoOpts = append(autoOpts, auto.WithName("miniagent"))

	// Claude OAuth token store — non-fatal if unavailable
	if dir, err := store.DefaultDir(); err == nil {
		if ts, err := store.NewFileTokenStore(dir); err == nil {
			autoOpts = append(autoOpts, auto.WithClaude(ts))
		}
	}

	provider, err := auto.New(ctx, autoOpts...)
	if err != nil {
		return nil, fmt.Errorf(`no LLM providers found.

Set one of:
  ANTHROPIC_API_KEY    — Anthropic direct API
  OPENAI_API_KEY       — OpenAI
  OPENROUTER_API_KEY   — OpenRouter

Or authenticate with Claude:
  llmcli auth login

(%w)`, err)
	}
	return provider, nil
}
