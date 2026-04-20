package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/llm"
	"github.com/codewandler/llm/auto"
	"github.com/codewandler/llm/cmd/llmcli/store"
	"github.com/codewandler/llm/provider/anthropic"
	"github.com/codewandler/llm/provider/anthropic/claude"
	"github.com/codewandler/llm/provider/bedrock"
	"github.com/codewandler/llm/provider/codex"
	"github.com/codewandler/llm/provider/ollama"
	"github.com/codewandler/llm/provider/openai"
	"github.com/codewandler/llm/provider/openrouter"
	"github.com/codewandler/miniagent/agent"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type InferenceConfig = agent.InferenceOptions

type providerRuntime struct {
	service *llm.Service
	models  llm.Models
}

func (p *providerRuntime) Name() string       { return "miniagent" }
func (p *providerRuntime) Models() llm.Models { return p.models }
func (p *providerRuntime) CreateStream(ctx context.Context, src llm.Buildable) (llm.Stream, error) {
	return p.service.CreateStream(ctx, src)
}

func rootCmd() *cobra.Command {
	var (
		inference    InferenceConfig = agent.DefaultInferenceOptions()
		maxSteps                     = 30
		workspace    string
		systemPrompt string
		totalTimeout time.Duration
		toolTimeout  time.Duration
		debug        bool
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
			return execute(args, inference, maxSteps, workspace, systemPrompt, totalTimeout, toolTimeout, debug)
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
	f.BoolVar(&debug, "debug", false, "Enable debug logging for the auto provider")
	f.Float64Var(&inference.Temperature, "temperature", inference.Temperature, "Sampling temperature 0.0–2.0")
	f.TextVar(&inference.Thinking, "thinking", inference.Thinking, "Thinking mode: auto|on|off")
	f.TextVar(&inference.Effort, "effort", inference.Effort, "Effort level: low|medium|high|max")
	_ = cmd.RegisterFlagCompletionFunc("model", completeModelFlag)
	cmd.AddCommand(modelsCmd())
	cmd.AddCommand(completionCmd(cmd))
	return cmd
}

func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "models",
		Short:         "List all available model IDs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			provider, err := createProvider(cmd.Context(), false)
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

func completionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "completion",
		Short:         "Generate or install shell completion",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		RunE: func(_ *cobra.Command, _ []string) error {
			return root.GenBashCompletionV2(os.Stdout, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		RunE: func(_ *cobra.Command, _ []string) error {
			return root.GenZshCompletion(os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		RunE: func(_ *cobra.Command, _ []string) error {
			return root.GenFishCompletion(os.Stdout, true)
		},
	})
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
	provider, err := createProvider(cmd.Context(), false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	matches := make([]string, 0, len(provider.Models()))
	for _, m := range provider.Models() {
		if !containsFold(m.ID, toComplete) && !containsFold(m.Name, toComplete) {
			continue
		}
		desc := m.Name
		if desc == "" {
			desc = m.Provider
		}
		if desc != "" {
			matches = append(matches, m.ID+"\t"+desc)
		} else {
			matches = append(matches, m.ID)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func execute(args []string, inference InferenceConfig, maxSteps int, workspace, systemPrompt string, totalTimeout, toolTimeout time.Duration, debug bool) error {
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		workspace = wd
	}

	ctx := context.Background()
	provider, err := createProvider(ctx, debug)
	if err != nil {
		return err
	}
	a := agent.New(provider,
		agent.WithWorkspace(workspace),
		agent.WithToolTimeout(toolTimeout),
		agent.WithSystemOverride(systemPrompt),
		agent.WithInferenceOptions(inference),
		agent.WithMaxSteps(maxSteps),
	)
	if len(args) == 1 {
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		if totalTimeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, totalTimeout)
		}
		defer cancel()
		err := a.RunTurn(ctx, 1, args[0])
		fmt.Println()
		agent.PrintSessionUsage(os.Stdout, a.SessionID(), a.Tracker().Aggregate())
		if errors.Is(err, agent.ErrMaxStepsReached) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			return nil
		}
		return err
	}
	return agent.RunREPL(ctx, a, os.Stdin)
}

func createProvider(ctx context.Context, debug bool) (*providerRuntime, error) {
	var autoOpts []auto.Option
	autoOpts = append(autoOpts, auto.WithName("miniagent"))
	if debug {
		autoOpts = append(autoOpts, auto.WithLLMOptions(llm.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))))
	}
	if dir, err := store.DefaultDir(); err == nil {
		if ts, err := store.NewFileTokenStore(dir); err == nil {
			autoOpts = append(autoOpts, auto.WithClaude(ts))
		}
	}
	service, err := auto.New(ctx, autoOpts...)
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
	models := aggregateKnownModels()
	if len(models) == 0 {
		return nil, fmt.Errorf("no models available")
	}
	return &providerRuntime{service: service, models: models}, nil
}

func aggregateKnownModels() llm.Models {
	seen := map[string]bool{}
	var out llm.Models
	appendModels := func(providerName string, models llm.Models, prefixIDs bool) {
		for _, m := range models {
			if m.ID == "" {
				continue
			}
			if prefixIDs {
				m.ID = providerName + "/" + m.ID
			}
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	appendModels("anthropic", anthropic.New().Models(), false)
	appendModels("openai", openai.New().Models(), false)
	appendModels("openrouter", openrouter.New().Models(), false)
	appendModels("bedrock", bedrock.New().Models(), false)
	appendModels("ollama", ollama.New().Models(), false)
	appendModels("claude", claude.New().Models(), false)
	if auth, err := codex.LoadAuth(); err == nil {
		appendModels("codex", codex.New(auth).Models(), true)
	}
	return out
}
